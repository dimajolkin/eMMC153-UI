package main

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	emmcisp "github.com/dimajolkin/eMMC153-UI"
)

const jedecN = 14

// boardView: static photo + vector overlay (grid / A1 / ISP).
type boardView struct {
	widget.BaseWidget
	photo   *canvas.Image
	pad     *mousePad
	overlay *fyne.Container

	hLines [jedecN + 1]*canvas.Line
	vLines [jedecN + 1]*canvas.Line
	rowLbl [jedecN]*canvas.Text
	colLbl [jedecN]*canvas.Text
	a1Ring *canvas.Circle
	a1Fill *canvas.Circle
	a1Text *canvas.Text

	ispRing  []*canvas.Circle
	ispFill  []*canvas.Circle
	ispLabel []*canvas.Text
	ispLead  []*canvas.Line

	imgW, imgH float32
	fit        emmcisp.Lattice
	showGrid   bool
	showA1     bool
	showISP    bool
}

func newBoardView() *boardView {
	b := &boardView{
		photo: canvas.NewImageFromImage(nil),
		pad:   newMousePad(),
	}
	b.photo.FillMode = canvas.ImageFillContain
	b.photo.SetMinSize(fyne.NewSize(640, 520))

	cyan := color.NRGBA{R: 0, G: 220, B: 255, A: 200}
	for i := 0; i <= jedecN; i++ {
		b.hLines[i] = canvas.NewLine(cyan)
		b.hLines[i].StrokeWidth = 1
		b.vLines[i] = canvas.NewLine(cyan)
		b.vLines[i].StrokeWidth = 1
	}
	for i := 0; i < jedecN; i++ {
		b.rowLbl[i] = canvas.NewText(string(emmcisp.RowsJEDEC[i]), cyan)
		b.rowLbl[i].TextSize = 11
		b.colLbl[i] = canvas.NewText(strconv.Itoa(i+1), cyan)
		b.colLbl[i].TextSize = 11
	}
	yellow := color.NRGBA{R: 255, G: 220, B: 0, A: 255}
	b.a1Ring = canvas.NewCircle(color.NRGBA{})
	b.a1Ring.StrokeColor = yellow
	b.a1Ring.StrokeWidth = 2
	b.a1Fill = canvas.NewCircle(yellow)
	b.a1Text = canvas.NewText("A1", yellow)
	b.a1Text.TextSize = 12
	b.a1Text.TextStyle = fyne.TextStyle{Bold: true}

	b.ispRing = make([]*canvas.Circle, len(emmcisp.ISPPins))
	b.ispFill = make([]*canvas.Circle, len(emmcisp.ISPPins))
	b.ispLabel = make([]*canvas.Text, len(emmcisp.ISPPins))
	b.ispLead = make([]*canvas.Line, len(emmcisp.ISPPins))
	for i, pin := range emmcisp.ISPPins {
		c := color.NRGBA{R: pin.Color.R, G: pin.Color.G, B: pin.Color.B, A: 255}
		b.ispRing[i] = canvas.NewCircle(color.NRGBA{})
		b.ispRing[i].StrokeColor = c
		b.ispRing[i].StrokeWidth = 2
		b.ispFill[i] = canvas.NewCircle(c)
		b.ispLabel[i] = canvas.NewText(pin.Name, c)
		b.ispLabel[i].TextSize = 12
		b.ispLabel[i].TextStyle = fyne.TextStyle{Bold: true}
		b.ispLead[i] = canvas.NewLine(c)
		b.ispLead[i].StrokeWidth = 1.5
	}

	objs := []fyne.CanvasObject{b.photo}
	for i := 0; i <= jedecN; i++ {
		objs = append(objs, b.hLines[i], b.vLines[i])
	}
	for i := 0; i < jedecN; i++ {
		objs = append(objs, b.rowLbl[i], b.colLbl[i])
	}
	objs = append(objs, b.a1Ring, b.a1Fill, b.a1Text)
	for i := range emmcisp.ISPPins {
		objs = append(objs, b.ispLead[i], b.ispRing[i], b.ispFill[i], b.ispLabel[i])
	}
	objs = append(objs, b.pad)
	b.overlay = container.NewWithoutLayout(objs...)
	b.hideOverlay()
	b.ExtendBaseWidget(b)
	return b
}

func (b *boardView) CreateRenderer() fyne.WidgetRenderer {
	return &boardRenderer{b: b, objs: []fyne.CanvasObject{b.overlay}}
}

type boardRenderer struct {
	b    *boardView
	objs []fyne.CanvasObject
}

func (r *boardRenderer) Layout(size fyne.Size) {
	r.b.overlay.Resize(size)
	r.b.photo.Resize(size)
	r.b.photo.Move(fyne.NewPos(0, 0))
	r.b.pad.Resize(size)
	r.b.pad.Move(fyne.NewPos(0, 0))
	r.b.layoutOverlay(size)
}

func (r *boardRenderer) MinSize() fyne.Size             { return fyne.NewSize(640, 520) }
func (r *boardRenderer) Objects() []fyne.CanvasObject   { return r.objs }
func (r *boardRenderer) Destroy()                       {}
func (r *boardRenderer) Refresh() {
	canvas.Refresh(r.b.photo)
	r.b.layoutOverlay(r.b.Size())
}

func (b *boardView) setPhoto(img image.Image) {
	b.photo.Image = img
	bounds := img.Bounds()
	b.imgW = float32(bounds.Dx())
	b.imgH = float32(bounds.Dy())
	b.pad.setImageSize(bounds.Dx(), bounds.Dy())
	b.Refresh()
}

func (b *boardView) mapGeom(size fyne.Size) (scale, ox, oy float32) {
	if b.imgW < 1 || b.imgH < 1 {
		return 1, 0, 0
	}
	scale = float32(math.Min(float64(size.Width/b.imgW), float64(size.Height/b.imgH)))
	dw, dh := b.imgW*scale, b.imgH*scale
	ox = (size.Width - dw) / 2
	oy = (size.Height - dh) / 2
	return
}

func (b *boardView) toWidget(ix, iy float64, scale, ox, oy float32) fyne.Position {
	return fyne.NewPos(ox+float32(ix)*scale, oy+float32(iy)*scale)
}

func (b *boardView) hideOverlay() {
	for i := 0; i <= jedecN; i++ {
		b.hLines[i].Hide()
		b.vLines[i].Hide()
	}
	for i := 0; i < jedecN; i++ {
		b.rowLbl[i].Hide()
		b.colLbl[i].Hide()
	}
	b.a1Ring.Hide()
	b.a1Fill.Hide()
	b.a1Text.Hide()
	for i := range emmcisp.ISPPins {
		b.ispRing[i].Hide()
		b.ispFill[i].Hide()
		b.ispLabel[i].Hide()
		b.ispLead[i].Hide()
	}
}

func (b *boardView) setLattice(fit emmcisp.Lattice, showGrid, showA1, showISP bool) {
	b.fit = fit
	b.showGrid = showGrid && fit.PX > 0 && fit.PY > 0
	b.showA1 = showA1 && b.showGrid
	b.showISP = showISP && b.showGrid
	b.layoutOverlay(b.Size())
}

// setLatticeLive updates overlay without refreshing the photo texture.
func (b *boardView) setLatticeLive(fit emmcisp.Lattice) {
	b.fit = fit
	b.showGrid = fit.PX > 0 && fit.PY > 0
	b.showA1 = b.showGrid
	b.showISP = false
	b.layoutOverlay(b.Size())
}

func (b *boardView) layoutOverlay(size fyne.Size) {
	if size.Width < 1 || size.Height < 1 {
		return
	}
	if !b.showGrid {
		b.hideOverlay()
		return
	}
	scale, ox, oy := b.mapGeom(size)
	fit := b.fit
	x0 := fit.OX - 0.5*fit.PX
	y0 := fit.OY - 0.5*fit.PY
	for i := 0; i <= jedecN; i++ {
		xi := x0 + float64(i)*fit.PX
		yi := y0 + float64(i)*fit.PY
		b.vLines[i].Position1 = b.toWidget(xi, y0, scale, ox, oy)
		b.vLines[i].Position2 = b.toWidget(xi, y0+float64(jedecN)*fit.PY, scale, ox, oy)
		b.vLines[i].Show()
		b.hLines[i].Position1 = b.toWidget(x0, yi, scale, ox, oy)
		b.hLines[i].Position2 = b.toWidget(x0+float64(jedecN)*fit.PX, yi, scale, ox, oy)
		b.hLines[i].Show()
	}
	for i := 0; i < jedecN; i++ {
		cx := fit.OX + float64(i)*fit.PX
		cy := fit.OY + float64(i)*fit.PY
		rp := b.toWidget(fit.OX-0.7*fit.PX, cy, scale, ox, oy)
		b.rowLbl[i].Move(fyne.NewPos(rp.X-10, rp.Y-7))
		b.rowLbl[i].Show()
		cp := b.toWidget(cx, fit.OY-0.65*fit.PY, scale, ox, oy)
		b.colLbl[i].Move(fyne.NewPos(cp.X-5, cp.Y-10))
		b.colLbl[i].Show()
	}
	if b.showA1 {
		p := b.toWidget(fit.OX, fit.OY, scale, ox, oy)
		r := float32(math.Max(8, float64(scale*float32(fit.PX))*0.35))
		b.a1Ring.Resize(fyne.NewSize(r*2, r*2))
		b.a1Ring.Move(fyne.NewPos(p.X-r, p.Y-r))
		b.a1Ring.Show()
		b.a1Fill.Resize(fyne.NewSize(6, 6))
		b.a1Fill.Move(fyne.NewPos(p.X-3, p.Y-3))
		b.a1Fill.Show()
		b.a1Text.Move(fyne.NewPos(p.X+r+2, p.Y-8))
		b.a1Text.Show()
	} else {
		b.a1Ring.Hide()
		b.a1Fill.Hide()
		b.a1Text.Hide()
	}
	if b.showISP {
		labelX := size.Width - 120
		labelY := float32(28)
		for i, pin := range emmcisp.ISPPins {
			pt := emmcisp.BallXY(fit, pin.Row, pin.Col)
			p := b.toWidget(pt.X, pt.Y, scale, ox, oy)
			r := float32(math.Max(10, float64(scale*float32(fit.PX))*0.4))
			b.ispRing[i].Resize(fyne.NewSize(r*2, r*2))
			b.ispRing[i].Move(fyne.NewPos(p.X-r, p.Y-r))
			b.ispFill[i].Resize(fyne.NewSize(6, 6))
			b.ispFill[i].Move(fyne.NewPos(p.X-3, p.Y-3))
			lp := fyne.NewPos(labelX, labelY)
			b.ispLabel[i].Text = string(pin.Row) + strconv.Itoa(pin.Col) + " " + pin.Name
			b.ispLabel[i].Move(lp)
			b.ispLead[i].Position1 = p
			b.ispLead[i].Position2 = fyne.NewPos(lp.X, lp.Y+8)
			b.ispRing[i].Show()
			b.ispFill[i].Show()
			b.ispLabel[i].Show()
			b.ispLead[i].Show()
			labelY += 26
		}
	} else {
		for i := range emmcisp.ISPPins {
			b.ispRing[i].Hide()
			b.ispFill[i].Hide()
			b.ispLabel[i].Hide()
			b.ispLead[i].Hide()
		}
	}
}

type mousePad struct {
	widget.BaseWidget
	imgW, imgH     float32
	mode           string
	dragging       bool
	x0, y0, x1, y1 float64
	onGrid         func(ox, oy, px, py float64)
	onA1           func(x, y float64)
	onDragPreview  func(ox, oy, px, py float64)
}

func newMousePad() *mousePad {
	p := &mousePad{}
	p.ExtendBaseWidget(p)
	return p
}

func (p *mousePad) CreateRenderer() fyne.WidgetRenderer {
	r := canvas.NewRectangle(color.NRGBA{})
	r.SetMinSize(fyne.NewSize(640, 520))
	return widget.NewSimpleRenderer(r)
}

func (p *mousePad) setImageSize(w, h int) {
	p.imgW, p.imgH = float32(w), float32(h)
}

func (p *mousePad) Cursor() desktop.Cursor {
	if p.mode == "grid" || p.mode == "a1" {
		return desktop.CrosshairCursor
	}
	return desktop.DefaultCursor
}

func (p *mousePad) MouseDown(ev *desktop.MouseEvent) {
	ix, iy, ok := p.toImage(ev.Position)
	if !ok {
		return
	}
	switch p.mode {
	case "grid":
		p.dragging = true
		p.x0, p.y0 = ix, iy
		p.x1, p.y1 = ix, iy
	case "a1":
		if p.onA1 != nil {
			p.onA1(ix, iy)
		}
	}
}

func (p *mousePad) MouseMoved(ev *desktop.MouseEvent) {
	if !p.dragging || p.mode != "grid" {
		return
	}
	ix, iy, ok := p.toImage(ev.Position)
	if !ok {
		return
	}
	p.x1, p.y1 = ix, iy
	ox, oy, px, py := latticeFromDrag(p.x0, p.y0, p.x1, p.y1)
	if p.onDragPreview != nil && px > 1 && py > 1 {
		p.onDragPreview(ox, oy, px, py)
	}
}

func (p *mousePad) MouseUp(ev *desktop.MouseEvent) {
	if !p.dragging || p.mode != "grid" {
		return
	}
	p.dragging = false
	if ix, iy, ok := p.toImage(ev.Position); ok {
		p.x1, p.y1 = ix, iy
	}
	ox, oy, px, py := latticeFromDrag(p.x0, p.y0, p.x1, p.y1)
	if px < 2 || py < 2 {
		return
	}
	if p.onGrid != nil {
		p.onGrid(ox, oy, px, py)
	}
}

func (p *mousePad) MouseIn(*desktop.MouseEvent) {}
func (p *mousePad) MouseOut()                   {}

func (p *mousePad) toImage(pos fyne.Position) (float64, float64, bool) {
	sz := p.Size()
	if p.imgW < 1 || p.imgH < 1 || sz.Width < 1 || sz.Height < 1 {
		return 0, 0, false
	}
	scale := float32(math.Min(float64(sz.Width/p.imgW), float64(sz.Height/p.imgH)))
	dw, dh := p.imgW*scale, p.imgH*scale
	ox := (sz.Width - dw) / 2
	oy := (sz.Height - dh) / 2
	if pos.X < ox || pos.Y < oy || pos.X >= ox+dw || pos.Y >= oy+dh {
		return 0, 0, false
	}
	return float64((pos.X - ox) / scale), float64((pos.Y - oy) / scale), true
}

func latticeFromDrag(x0, y0, x1, y1 float64) (ox, oy, px, py float64) {
	ox, oy = math.Min(x0, x1), math.Min(y0, y1)
	px = math.Abs(x1-x0) / 13.0
	py = math.Abs(y1-y0) / 13.0
	return
}
