// Command ui — desktop GUI for eMMC153 ISP pad annotation.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"golang.design/x/clipboard"

	emmcisp "github.com/dimajolkin/eMMC153-UI"
)

type stage int

const (
	stageNeedImage stage = iota
	stagePlaceGrid
	stageMarkA1
	stageTune
	stageDone
)

type uiState struct {
	win     fyne.Window
	path    string
	src     *image.RGBA
	result  *emmcisp.Result
	preview *canvas.Image
	pad     *mousePad
	status  *widget.Label
	stepLbl *widget.Label

	a1x, a1y   *widget.Entry
	pitchX     *widget.Entry
	pitchY     *widget.Entry
	parseBtn   *widget.Button
	a1Btn      *widget.Button
	gridBtn    *widget.Button

	stage  stage
	fit    emmcisp.Lattice
	a1Set  bool
	gridOK bool
	showMask bool
}

func main() {
	if err := clipboard.Init(); err != nil {
		panic(err)
	}

	a := app.NewWithID("cytatv.emmc-isp")
	w := a.NewWindow("eMMC153 ISP Annotate")
	w.Resize(fyne.NewSize(1180, 800))

	st := &uiState{
		win:     w,
		status:  widget.NewLabel("⌘V — вставь скрин footprint"),
		stepLbl: widget.NewLabel("Этап 0 · картинка"),
		stage:   stageNeedImage,
	}
	st.status.Wrapping = fyne.TextWrapWord
	st.stepLbl.TextStyle = fyne.TextStyle{Bold: true}

	st.preview = canvas.NewImageFromImage(nil)
	st.pad = newMousePad()
	st.pad.onGrid = st.onGridPlaced
	st.pad.onA1 = st.onA1Clicked
	st.pad.onDragPreview = st.onGridDrag

	st.a1x = numEntry("")
	st.a1y = numEntry("")
	st.pitchX = numEntry("")
	st.pitchY = numEntry("")
	for _, e := range []*widget.Entry{st.a1x, st.a1y, st.pitchX, st.pitchY} {
		e.OnChanged = func(string) { st.applyTuneFromFields() }
	}

	pasteBtn := widget.NewButtonWithIcon("Из буфера", theme.ContentPasteIcon(), st.pasteClipboard)
	pasteBtn.Importance = widget.HighImportance
	openBtn := widget.NewButtonWithIcon("Файл…", theme.FolderOpenIcon(), st.openFile)

	st.gridBtn = widget.NewButton("1. Сетка мышкой", st.beginGrid)
	st.a1Btn = widget.NewButton("2. Ключ A1", st.beginA1)
	st.parseBtn = widget.NewButtonWithIcon("3. Разобрать пины", theme.ConfirmIcon(), st.parsePins)
	st.parseBtn.Importance = widget.HighImportance
	st.parseBtn.Disable()
	st.a1Btn.Disable()
	st.gridBtn.Disable()

	saveBtn := widget.NewButtonWithIcon("Сохранить…", theme.DocumentSaveIcon(), st.save)
	maskBtn := widget.NewButton("Маска B&W", st.toggleMask)

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("A1 X"), st.a1x,
		widget.NewLabel("A1 Y"), st.a1y,
		widget.NewLabel("Pitch X"), st.pitchX,
		widget.NewLabel("Pitch Y"), st.pitchY,
	)

	side := container.NewVBox(
		widget.NewLabelWithStyle("eMMC153 ISP", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("JEDEC 14×14"),
		st.stepLbl,
		widget.NewSeparator(),
		pasteBtn,
		openBtn,
		widget.NewSeparator(),
		st.gridBtn,
		st.a1Btn,
		widget.NewLabel("Подгонка:"),
		form,
		st.parseBtn,
		maskBtn,
		saveBtn,
		widget.NewSeparator(),
		st.status,
	)

	root := container.NewBorder(nil, nil, container.NewPadded(side), nil,
		container.NewPadded(wrapPreview(st.preview, st.pad)))
	w.SetContent(root)
	w.Canvas().AddShortcut(&fyne.ShortcutPaste{}, func(fyne.Shortcut) {
		st.pasteClipboard()
	})
	w.ShowAndRun()
}

func numEntry(v string) *widget.Entry {
	e := widget.NewEntry()
	e.SetText(v)
	return e
}

func (st *uiState) setStatus(s string) { st.status.SetText(s) }

func (st *uiState) setStep(s string) { st.stepLbl.SetText(s) }

func (st *uiState) setSource(img image.Image, label string) {
	st.src = emmcisp.ToRGBA(img)
	st.path = label
	st.result = nil
	st.showMask = false
	st.fit = emmcisp.Lattice{}
	st.a1Set = false
	st.gridOK = false
	st.pad.setImageSize(st.src.Bounds().Dx(), st.src.Bounds().Dy())
	st.a1x.SetText("")
	st.a1y.SetText("")
	st.pitchX.SetText("")
	st.pitchY.SetText("")
	st.parseBtn.Disable()
	st.a1Btn.Disable()
	st.gridBtn.Enable()
	st.showImage(st.src)
	st.stage = stagePlaceGrid
	st.setStep("Этап 1 · сетка")
	st.pad.mode = "grid"
	st.setStatus(fmt.Sprintf("%s (%dx%d). Растяни сетку от центра A1 до противоположного угла (P14)",
		label, st.src.Bounds().Dx(), st.src.Bounds().Dy()))
}

func (st *uiState) pasteClipboard() {
	data := clipboard.Read(clipboard.FmtImage)
	if len(data) == 0 {
		st.setStatus("В буфере нет картинки")
		return
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		dialog.ShowError(err, st.win)
		return
	}
	st.setSource(img, "clipboard.png")
}

func (st *uiState) openFile() {
	dialog.ShowFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, st.win)
			return
		}
		if r == nil {
			return
		}
		defer r.Close()
		path := r.URI().Path()
		img, err := emmcisp.LoadImage(path)
		if err != nil {
			dialog.ShowError(err, st.win)
			return
		}
		st.setSource(img, path)
	}, st.win)
}

func (st *uiState) beginGrid() {
	if st.src == nil {
		return
	}
	st.stage = stagePlaceGrid
	st.pad.mode = "grid"
	st.result = nil
	st.setStep("Этап 1 · сетка")
	st.setStatus("Тяни мышью: от центра шара A1 к центру противоположного угла (ряд P / кол. 14)")
	st.refreshPreview()
}

func (st *uiState) beginA1() {
	if st.src == nil || !st.gridOK {
		st.setStatus("Сначала поставь сетку")
		return
	}
	st.stage = stageMarkA1
	st.pad.mode = "a1"
	st.result = nil
	st.setStep("Этап 2 · ключ A1")
	st.setStatus("Кликни ключ A1: вырез на шёлке / первый пад (верхний левый)")
	st.refreshPreview()
}

func (st *uiState) onGridDrag(ox, oy, px, py float64) {
	st.fit = emmcisp.Lattice{OX: ox, OY: oy, PX: px, PY: py}
	st.refreshPreview()
}

func (st *uiState) onGridPlaced(ox, oy, px, py float64) {
	st.fit = emmcisp.Lattice{OX: ox, OY: oy, PX: px, PY: py}
	st.gridOK = true
	st.a1Set = true // drag start is provisional A1
	st.syncFields()
	st.a1Btn.Enable()
	st.parseBtn.Enable()
	st.stage = stageMarkA1
	st.pad.mode = "a1"
	st.setStep("Этап 2 · ключ A1")
	st.setStatus(fmt.Sprintf("Сетка: pitch=%.2f×%.2f. Кликни ключ A1 (вырез шёлка), потом подгони числа справа", px, py))
	st.refreshPreview()
}

func (st *uiState) onA1Clicked(x, y float64) {
	st.fit.OX, st.fit.OY = x, y
	st.a1Set = true
	st.syncFields()
	st.stage = stageTune
	st.pad.mode = ""
	st.setStep("Этап 3 · подгонка")
	st.setStatus(fmt.Sprintf("A1=(%.1f,%.1f). Подгони Pitch X/Y в полях, затем «Разобрать пины»", x, y))
	st.refreshPreview()
}

func (st *uiState) syncFields() {
	st.a1x.SetText(fmt.Sprintf("%.1f", st.fit.OX))
	st.a1y.SetText(fmt.Sprintf("%.1f", st.fit.OY))
	st.pitchX.SetText(fmt.Sprintf("%.2f", st.fit.PX))
	st.pitchY.SetText(fmt.Sprintf("%.2f", st.fit.PY))
}

func (st *uiState) applyTuneFromFields() {
	if st.src == nil || !st.gridOK {
		return
	}
	ax, err1 := strconv.ParseFloat(strings.TrimSpace(st.a1x.Text), 64)
	ay, err2 := strconv.ParseFloat(strings.TrimSpace(st.a1y.Text), 64)
	px, err3 := strconv.ParseFloat(strings.TrimSpace(st.pitchX.Text), 64)
	py, err4 := strconv.ParseFloat(strings.TrimSpace(st.pitchY.Text), 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || px < 2 || py < 2 {
		return
	}
	st.fit.OX, st.fit.OY, st.fit.PX, st.fit.PY = ax, ay, px, py
	st.a1Set = true
	if st.stage == stageDone {
		st.stage = stageTune
		st.result = nil
	}
	st.refreshPreview()
}

func (st *uiState) refreshPreview() {
	if st.src == nil {
		return
	}
	if st.result != nil && st.stage == stageDone {
		st.showImage(st.result.Annotated)
		return
	}
	if st.gridOK || st.fit.PX > 0 {
		st.showImage(emmcisp.DrawGridPreview(st.src, st.fit, st.a1Set))
		return
	}
	st.showImage(st.src)
}

func (st *uiState) parsePins() {
	if st.src == nil || !st.gridOK || st.fit.PX < 2 {
		st.setStatus("Сначала сетка и A1")
		return
	}
	st.applyTuneFromFields()
	st.setStatus("Разбираю пины по сетке…")
	res, err := emmcisp.AnnotateLattice(st.src, st.fit)
	if err != nil {
		dialog.ShowError(err, st.win)
		return
	}
	res.Input = st.path
	st.result = res
	st.fit = res.Lattice
	st.stage = stageDone
	st.pad.mode = ""
	st.setStep("Готово")
	st.showImage(res.Annotated)
	st.setStatus(fmt.Sprintf("ISP: A1=(%.1f,%.1f) pitch=%.2f×%.2f  cells=%d pads=%d",
		res.Lattice.OX, res.Lattice.OY, res.Lattice.PX, res.Lattice.PY, res.Lattice.Cells, len(res.Pads)))
}

func (st *uiState) toggleMask() {
	if st.result == nil {
		st.setStatus("Сначала разбери пины")
		return
	}
	st.showMask = !st.showMask
	if st.showMask {
		st.showImage(grayToRGBA(st.result.Mask))
		st.setStatus("Маска B&W")
	} else {
		st.showImage(st.result.Annotated)
	}
}

func (st *uiState) save() {
	if st.result == nil {
		st.setStatus("Сначала разбери пины")
		return
	}
	dialog.ShowFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, st.win)
			return
		}
		if w == nil {
			return
		}
		path := w.URI().Path()
		_ = w.Close()
		st.result.Output = path
		st.result.JSONPath = strings.TrimSuffix(path, filepath.Ext(path)) + ".json"
		st.result.BWPath = strings.TrimSuffix(path, filepath.Ext(path)) + "-bw.png"
		st.result.Payload = emmcisp.BuildJSON(st.path, path, st.result.ROI, st.result.Pads, st.result.Lattice)
		if err := st.result.WriteFiles(); err != nil {
			dialog.ShowError(err, st.win)
			return
		}
		st.setStatus(fmt.Sprintf("Сохранено: %s", filepath.Base(path)))
	}, st.win)
}

func (st *uiState) showImage(img image.Image) {
	if img == nil {
		return
	}
	st.preview.Image = img
	st.preview.Refresh()
}

func grayToRGBA(g *image.Gray) *image.RGBA {
	if g == nil {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	b := g.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			v := g.GrayAt(x, y).Y
			out.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return out
}
