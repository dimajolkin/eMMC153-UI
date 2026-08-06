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

type uiState struct {
	win      fyne.Window
	path     string
	src      *image.RGBA
	result   *emmcisp.Result
	preview  *canvas.Image
	status   *widget.Label
	roiX     *widget.Entry
	roiY     *widget.Entry
	roiW     *widget.Entry
	roiH     *widget.Entry
	a1       *widget.Entry
	pitch    *widget.Entry
	showMask bool
}

func main() {
	if err := clipboard.Init(); err != nil {
		panic(err)
	}

	a := app.NewWithID("cytatv.emmc-isp")
	w := a.NewWindow("eMMC153 ISP Annotate")
	w.Resize(fyne.NewSize(1100, 780))

	st := &uiState{
		win:    w,
		status: widget.NewLabel("⌘V / «Из буфера» — вставь скрин footprint"),
	}
	st.status.Wrapping = fyne.TextWrapWord

	st.preview = canvas.NewImageFromImage(nil)
	st.preview.FillMode = canvas.ImageFillContain
	st.preview.SetMinSize(fyne.NewSize(640, 520))

	st.roiX = numEntry("240")
	st.roiY = numEntry("240")
	st.roiW = numEntry("400")
	st.roiH = numEntry("400")
	st.a1 = widget.NewEntry()
	st.a1.SetPlaceHolder("опц. x,y")
	st.pitch = widget.NewEntry()
	st.pitch.SetPlaceHolder("опц. px")

	pasteBtn := widget.NewButtonWithIcon("Из буфера", theme.ContentPasteIcon(), st.pasteClipboard)
	pasteBtn.Importance = widget.HighImportance
	openBtn := widget.NewButtonWithIcon("Файл…", theme.FolderOpenIcon(), st.openFile)
	runBtn := widget.NewButtonWithIcon("Разметить", theme.MediaPlayIcon(), st.run)
	runBtn.Importance = widget.HighImportance
	saveBtn := widget.NewButtonWithIcon("Сохранить…", theme.DocumentSaveIcon(), st.save)
	maskBtn := widget.NewButton("Маска B&W", st.toggleMask)
	autoROIBtn := widget.NewButton("Auto ROI", st.fillAutoROI)

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("ROI X"), st.roiX,
		widget.NewLabel("ROI Y"), st.roiY,
		widget.NewLabel("ROI W"), st.roiW,
		widget.NewLabel("ROI H"), st.roiH,
		widget.NewLabel("A1"), st.a1,
		widget.NewLabel("Pitch"), st.pitch,
	)

	side := container.NewVBox(
		widget.NewLabelWithStyle("eMMC153 ISP", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("JEDEC 14×14 · A1 = notch TL"),
		widget.NewSeparator(),
		pasteBtn,
		openBtn,
		autoROIBtn,
		form,
		runBtn,
		maskBtn,
		saveBtn,
		widget.NewSeparator(),
		st.status,
	)

	root := container.NewBorder(nil, nil, container.NewPadded(side), nil,
		container.NewPadded(container.NewMax(st.preview)))
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

func (st *uiState) setStatus(s string) {
	st.status.SetText(s)
}

func (st *uiState) setSource(img image.Image, label string) {
	st.src = emmcisp.ToRGBA(img)
	st.path = label
	st.result = nil
	st.showMask = false
	st.showImage(st.src)
	st.setStatus(fmt.Sprintf("%s (%dx%d)", label, st.src.Bounds().Dx(), st.src.Bounds().Dy()))
}

func (st *uiState) pasteClipboard() {
	data := clipboard.Read(clipboard.FmtImage)
	if len(data) == 0 {
		st.setStatus("В буфере нет картинки — скопируй скрин (⌘C) и снова ⌘V")
		return
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		dialog.ShowError(fmt.Errorf("буфер: %w", err), st.win)
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

func (st *uiState) fillAutoROI() {
	if st.src == nil {
		st.setStatus("Сначала вставь картинку из буфера")
		return
	}
	roi := emmcisp.AutoROI(st.src)
	st.roiX.SetText(strconv.Itoa(roi.Min.X))
	st.roiY.SetText(strconv.Itoa(roi.Min.Y))
	st.roiW.SetText(strconv.Itoa(roi.Dx()))
	st.roiH.SetText(strconv.Itoa(roi.Dy()))
	st.setStatus(fmt.Sprintf("Auto ROI: %v", roi))
}

func (st *uiState) parseROI() (image.Rectangle, error) {
	x, err := strconv.Atoi(strings.TrimSpace(st.roiX.Text))
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("ROI X: %w", err)
	}
	y, err := strconv.Atoi(strings.TrimSpace(st.roiY.Text))
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("ROI Y: %w", err)
	}
	w, err := strconv.Atoi(strings.TrimSpace(st.roiW.Text))
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("ROI W: %w", err)
	}
	h, err := strconv.Atoi(strings.TrimSpace(st.roiH.Text))
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("ROI H: %w", err)
	}
	roi := image.Rect(x, y, x+w, y+h)
	if st.src != nil {
		roi = roi.Intersect(st.src.Bounds())
	}
	if roi.Empty() {
		return image.Rectangle{}, fmt.Errorf("пустой ROI")
	}
	return roi, nil
}

func (st *uiState) run() {
	if st.src == nil {
		st.setStatus("Сначала вставь картинку из буфера")
		return
	}
	roi, err := st.parseROI()
	if err != nil {
		dialog.ShowError(err, st.win)
		return
	}
	a1 := strings.TrimSpace(st.a1.Text)
	var pitch float64
	if p := strings.TrimSpace(st.pitch.Text); p != "" {
		pitch, err = strconv.ParseFloat(p, 64)
		if err != nil {
			dialog.ShowError(fmt.Errorf("pitch: %w", err), st.win)
			return
		}
	}
	st.setStatus("Считаю сетку сокета…")
	res, err := emmcisp.Annotate(st.src, roi, a1, pitch)
	if err != nil {
		dialog.ShowError(err, st.win)
		return
	}
	res.Input = st.path
	st.result = res
	st.showMask = false
	st.showImage(res.Annotated)
	st.setStatus(fmt.Sprintf("A1=(%.1f,%.1f) pitch=%.2f  cells=%d  pads=%d",
		res.Lattice.OX, res.Lattice.OY, res.Lattice.PX, res.Lattice.Cells, len(res.Pads)))
}

func (st *uiState) toggleMask() {
	if st.result == nil {
		st.setStatus("Сначала разметь")
		return
	}
	st.showMask = !st.showMask
	if st.showMask {
		st.showImage(grayToRGBA(st.result.Mask))
		st.setStatus("Маска B&W (сетка + кольца)")
	} else {
		st.showImage(st.result.Annotated)
	}
}

func (st *uiState) save() {
	if st.result == nil {
		st.setStatus("Сначала разметь")
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
		st.setStatus(fmt.Sprintf("Сохранено: %s (+ json, bw)", filepath.Base(path)))
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
