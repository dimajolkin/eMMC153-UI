// Command ui — разметка фото плат: сетка JEDEC, поиск ISP-пинов.
package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
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
	board   *boardView
	status  *widget.Label
	stepLbl *widget.Label

	a1x, a1y *widget.Entry
	pitchX   *widget.Entry
	pitchY   *widget.Entry
	parseBtn *widget.Button
	a1Btn    *widget.Button
	gridBtn  *widget.Button

	stage  stage
	fit    emmcisp.Lattice
	a1Set  bool
	gridOK bool
}

func main() {
	if err := clipboard.Init(); err != nil {
		panic(err)
	}

	a := app.NewWithID("cytatv.emmc-isp")
	w := a.NewWindow("eMMC153 — разметка платы")
	w.Resize(fyne.NewSize(1180, 800))

	st := &uiState{
		win:     w,
		status:  widget.NewLabel("⌘V — фото footprint платы"),
		stepLbl: widget.NewLabel("Этап 0 · картинка"),
		stage:   stageNeedImage,
		board:   newBoardView(),
	}
	st.status.Wrapping = fyne.TextWrapWord
	st.stepLbl.TextStyle = fyne.TextStyle{Bold: true}

	st.board.pad.onGrid = st.onGridPlaced
	st.board.pad.onA1 = st.onA1Clicked
	st.board.pad.onDragPreview = st.onGridDrag

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
	st.parseBtn = widget.NewButtonWithIcon("3. Найти пины", theme.ConfirmIcon(), st.parsePins)
	st.parseBtn.Importance = widget.HighImportance
	st.parseBtn.Disable()
	st.a1Btn.Disable()
	st.gridBtn.Disable()

	saveBtn := widget.NewButtonWithIcon("Сохранить…", theme.DocumentSaveIcon(), st.save)

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("A1 X"), st.a1x,
		widget.NewLabel("A1 Y"), st.a1y,
		widget.NewLabel("Pitch X"), st.pitchX,
		widget.NewLabel("Pitch Y"), st.pitchY,
	)

	side := container.NewVBox(
		widget.NewLabelWithStyle("Разметка платы", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("eMMC153 · JEDEC 14×14"),
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
		saveBtn,
		widget.NewSeparator(),
		st.status,
	)

	root := container.NewBorder(nil, nil, container.NewPadded(side), nil,
		container.NewPadded(st.board))
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
func (st *uiState) setStep(s string)   { st.stepLbl.SetText(s) }

func (st *uiState) refreshOverlay() {
	showGrid := st.gridOK || st.fit.PX > 0
	showISP := st.stage == stageDone && st.result != nil
	st.board.setLattice(st.fit, showGrid, st.a1Set, showISP)
}

func (st *uiState) setSource(img image.Image, label string) {
	st.src = emmcisp.ToRGBA(img)
	st.path = label
	st.result = nil
	st.fit = emmcisp.Lattice{}
	st.a1Set = false
	st.gridOK = false
	st.a1x.SetText("")
	st.a1y.SetText("")
	st.pitchX.SetText("")
	st.pitchY.SetText("")
	st.parseBtn.Disable()
	st.a1Btn.Disable()
	st.gridBtn.Enable()
	st.board.setPhoto(st.src)
	st.board.setLattice(emmcisp.Lattice{}, false, false, false)
	st.stage = stagePlaceGrid
	st.setStep("Этап 1 · сетка")
	st.board.pad.mode = "grid"
	st.setStatus(fmt.Sprintf("%s (%dx%d). Растяни сетку: A1 → противоположный угол",
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
	st.board.pad.mode = "grid"
	st.result = nil
	st.setStep("Этап 1 · сетка")
	st.setStatus("Тяни мышью от центра A1 к противоположному углу (P14)")
	st.refreshOverlay()
}

func (st *uiState) beginA1() {
	if st.src == nil || !st.gridOK {
		st.setStatus("Сначала поставь сетку")
		return
	}
	st.stage = stageMarkA1
	st.board.pad.mode = "a1"
	st.result = nil
	st.setStep("Этап 2 · ключ A1")
	st.setStatus("Кликни ключ A1 (вырез шёлка / первый пад)")
	st.refreshOverlay()
}

func (st *uiState) onGridDrag(ox, oy, px, py float64) {
	st.fit = emmcisp.Lattice{OX: ox, OY: oy, PX: px, PY: py}
	st.gridOK = true
	st.a1Set = true
	st.board.setLatticeLive(st.fit)
}

func (st *uiState) onGridPlaced(ox, oy, px, py float64) {
	st.fit = emmcisp.Lattice{OX: ox, OY: oy, PX: px, PY: py}
	st.gridOK = true
	st.a1Set = true
	st.syncFields()
	st.a1Btn.Enable()
	st.parseBtn.Enable()
	st.stage = stageMarkA1
	st.board.pad.mode = "a1"
	st.setStep("Этап 2 · ключ A1")
	st.setStatus(fmt.Sprintf("Сетка pitch=%.2f×%.2f — кликни ключ A1", px, py))
	st.refreshOverlay()
}

func (st *uiState) onA1Clicked(x, y float64) {
	st.fit.OX, st.fit.OY = x, y
	st.a1Set = true
	st.syncFields()
	st.stage = stageTune
	st.board.pad.mode = ""
	st.setStep("Этап 3 · подгонка")
	st.setStatus(fmt.Sprintf("A1=(%.1f,%.1f) — подгони pitch, затем «Найти пины»", x, y))
	st.refreshOverlay()
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
	st.refreshOverlay()
}

func (st *uiState) parsePins() {
	if st.src == nil || !st.gridOK || st.fit.PX < 2 {
		st.setStatus("Сначала сетка и A1")
		return
	}
	st.applyTuneFromFields()
	st.setStatus("Ищу пины по сетке…")
	res, err := emmcisp.AnnotateLattice(st.src, st.fit)
	if err != nil {
		dialog.ShowError(err, st.win)
		return
	}
	res.Input = st.path
	st.result = res
	st.fit = res.Lattice
	st.stage = stageDone
	st.board.pad.mode = ""
	st.setStep("Готово")
	st.refreshOverlay()
	st.setStatus(fmt.Sprintf("Найдено: cells=%d pads=%d  A1=(%.1f,%.1f) pitch=%.2f×%.2f",
		res.Lattice.Cells, len(res.Pads), res.Lattice.OX, res.Lattice.OY, res.Lattice.PX, res.Lattice.PY))
}

func (st *uiState) save() {
	if st.result == nil {
		st.setStatus("Сначала найди пины")
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
