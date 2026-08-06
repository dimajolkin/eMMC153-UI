package main

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// mousePad catches mouse on top of the preview image (FillContain mapping).
type mousePad struct {
	widget.BaseWidget
	imgW, imgH float32
	mode       string // "grid" | "a1" | ""
	dragging   bool
	x0, y0     float64 // image coords
	x1, y1     float64
	onGrid     func(ox, oy, px, py float64)
	onA1       func(x, y float64)
	onDragPreview func(ox, oy, px, py float64) // live while dragging
}

func newMousePad() *mousePad {
	p := &mousePad{}
	p.ExtendBaseWidget(p)
	return p
}

func (p *mousePad) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(nil)
	bg.SetMinSize(fyne.NewSize(640, 520))
	return widget.NewSimpleRenderer(bg)
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
	ix, iy, ok := p.toImage(ev.Position)
	if ok {
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

func (p *mousePad) MouseIn(*desktop.MouseEvent)  {}
func (p *mousePad) MouseOut()                    {}

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

// latticeFromDrag: drag from A1 ball center to opposite corner ball (14×14 → /13).
func latticeFromDrag(x0, y0, x1, y1 float64) (ox, oy, px, py float64) {
	ox, oy = math.Min(x0, x1), math.Min(y0, y1)
	px = math.Abs(x1-x0) / 13.0
	py = math.Abs(y1-y0) / 13.0
	return
}

func wrapPreview(img *canvas.Image, pad *mousePad) fyne.CanvasObject {
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(640, 520))
	return container.NewStack(img, pad)
}
