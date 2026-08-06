// Package emmcisp labels JEDEC eMMC153 ISP pads on a PCB photo.
package emmcisp

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var rows = []byte("ABCDEFGHJKLMNP") // 14 JEDEC rows, skip I/O

type ispPin struct {
	Name  string
	Row   byte
	Col   int
	Color color.RGBA
}

var ispPins = []ispPin{
	{"DAT0", 'A', 3, color.RGBA{R: 255, G: 220, B: 0, A: 255}},
	{"GND", 'A', 6, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
	{"VCC", 'E', 6, color.RGBA{R: 50, G: 160, B: 255, A: 255}},
	{"VCCQ", 'M', 4, color.RGBA{R: 120, G: 210, B: 255, A: 255}},
	{"CMD", 'M', 5, color.RGBA{R: 255, G: 165, B: 0, A: 255}},
	{"CLK", 'M', 6, color.RGBA{R: 255, G: 40, B: 40, A: 255}},
}

type Pt struct{ X, Y float64 }

type Lattice struct {
	OX, OY, PX, PY float64
	Cells, Hits    int
	Score          float64
}


func LoadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func roiGray(img *image.RGBA, roi image.Rectangle) ([]uint8, int, int) {
	roi = roi.Intersect(img.Bounds())
	w, h := roi.Dx(), roi.Dy()
	gray := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gray[y*w+x] = grayAt(img, roi.Min.X+x, roi.Min.Y+y)
		}
	}
	return gray, w, h
}

// circleScore: how much a dark circular pad sits at (cx,cy) with radius r.
// High when ring is darker than surroundings — matches real BGA pads.
func circleScore(gray []uint8, w, h int, cx, cy, r float64) float64 {
	if r < 2 {
		return 0
	}
	var ringSum, inSum, outSum float64
	var ringN, inN, outN int
	r2 := r * r
	rIn2 := (r * 0.45) * (r * 0.45)
	rOut2 := (r * 1.35) * (r * 1.35)
	x0 := int(cx - r*1.4)
	y0 := int(cy - r*1.4)
	x1 := int(cx + r*1.4)
	y1 := int(cy + r*1.4)
	for y := y0; y <= y1; y++ {
		if y < 0 || y >= h {
			continue
		}
		for x := x0; x <= x1; x++ {
			if x < 0 || x >= w {
				continue
			}
			dx := float64(x) - cx
			dy := float64(y) - cy
			d2 := dx*dx + dy*dy
			v := float64(gray[y*w+x])
			switch {
			case d2 <= rIn2:
				inSum += v
				inN++
			case d2 >= r2*0.65 && d2 <= r2*1.05:
				ringSum += v
				ringN++
			case d2 >= rOut2*0.9 && d2 <= rOut2:
				outSum += v
				outN++
			}
		}
	}
	if ringN < 8 || outN < 8 {
		return 0
	}
	ringMean := ringSum / float64(ringN)
	outMean := outSum / float64(outN)
	inMean := outMean
	if inN > 0 {
		inMean = inSum / float64(inN)
	}
	// pad ring darker than outside copper; inside may be dark hole or copper
	return (outMean - ringMean) + 0.35*(inMean-ringMean)
}

// fitSocketOnImage places JEDEC 14×14 so max cells contain a clear circle.
func fitSocketOnImage(gray []uint8, w, h int, roi image.Rectangle, hint float64) Lattice {
	best := Lattice{Score: -1}
	thresh := 8.0 // min circleScore to count as "circle in cell"
	for _, pitch := range linspace(hint*0.90, hint*1.10, 17) {
		r := pitch * 0.28
		for _, ox := range linspace(float64(roi.Min.X)-0.2*pitch, float64(roi.Min.X)+1.2*pitch, 25) {
			for _, oy := range linspace(float64(roi.Min.Y)-0.2*pitch, float64(roi.Min.Y)+1.2*pitch, 25) {
				n, sum := 0, 0.0
				for ri := 0; ri < 14; ri++ {
					for ci := 0; ci < 14; ci++ {
						cx := ox + float64(ci)*pitch - float64(roi.Min.X)
						cy := oy + float64(ri)*pitch - float64(roi.Min.Y)
						sc := circleScore(gray, w, h, cx, cy, r)
						if sc >= thresh {
							n++
							sum += sc
						}
					}
				}
				score := float64(n)*100 + sum // prefer more cells, then stronger circles
				if score > best.Score {
					best = Lattice{OX: ox, OY: oy, PX: pitch, PY: pitch, Cells: n, Hits: n, Score: score}
				}
			}
		}
	}
	// fine
	if best.Cells > 0 {
		ox, oy, pitch := best.OX, best.OY, best.PX
		for _, dp := range linspace(-0.03*pitch, 0.03*pitch, 7) {
			p2 := pitch + dp
			r := p2 * 0.28
			for _, dx := range linspace(-0.12*p2, 0.12*p2, 9) {
				for _, dy := range linspace(-0.12*p2, 0.12*p2, 9) {
					n, sum := 0, 0.0
					for ri := 0; ri < 14; ri++ {
						for ci := 0; ci < 14; ci++ {
							cx := ox + dx + float64(ci)*p2 - float64(roi.Min.X)
							cy := oy + dy + float64(ri)*p2 - float64(roi.Min.Y)
							sc := circleScore(gray, w, h, cx, cy, r)
							if sc >= thresh {
								n++
								sum += sc
							}
						}
					}
					score := float64(n)*100 + sum
					if score > best.Score {
						best = Lattice{OX: ox + dx, OY: oy + dy, PX: p2, PY: p2, Cells: n, Hits: n, Score: score}
					}
				}
			}
		}
	}
	return best
}

// padsFromSocket: one pad center per socket cell that has a circle.
func padsFromSocket(gray []uint8, w, h int, roi image.Rectangle, fit Lattice) []Pt {
	r := fit.PX * 0.28
	thresh := 8.0
	var pads []Pt
	for ri := 0; ri < 14; ri++ {
		for ci := 0; ci < 14; ci++ {
			cx := fit.OX + float64(ci)*fit.PX - float64(roi.Min.X)
			cy := fit.OY + float64(ri)*fit.PY - float64(roi.Min.Y)
			if circleScore(gray, w, h, cx, cy, r) < thresh {
				continue
			}
			// micro-refine to darkest local point
			bx, by := cx, cy
			best := circleScore(gray, w, h, cx, cy, r)
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					sc := circleScore(gray, w, h, cx+float64(dx), cy+float64(dy), r)
					if sc > best {
						best = sc
						bx, by = cx+float64(dx), cy+float64(dy)
					}
				}
			}
			pads = append(pads, Pt{X: bx + float64(roi.Min.X), Y: by + float64(roi.Min.Y)})
		}
	}
	return pads
}

// saveBWMask draws the socket 14×14 grid and white circles for pads that sit fully in a cell.
// RenderBWMask draws socket grid + pad rings for debug preview.
func RenderBWMask(roi image.Rectangle, pads []Pt, fit Lattice) *image.Gray {
	w, h := roi.Dx(), roi.Dy()
	out := image.NewGray(image.Rect(0, 0, max(1, w), max(1, h)))
	if w <= 0 || h <= 0 {
		return out
	}
	for i := range out.Pix {
		out.Pix[i] = 0
	}

	pitch := fit.PX
	if pitch <= 0 {
		pitch = float64(min(w, h)) / 13.0
	}
	ox := fit.OX - float64(roi.Min.X)
	oy := fit.OY - float64(roi.Min.Y)
	gridV := uint8(50)
	for i := 0; i <= 14; i++ {
		x := int(math.Round(ox + float64(i)*pitch - pitch/2))
		y := int(math.Round(oy + float64(i)*pitch - pitch/2))
		for yy := 0; yy < h; yy++ {
			setGray(out, x, yy, gridV)
		}
		for xx := 0; xx < w; xx++ {
			setGray(out, xx, y, gridV)
		}
	}

	r := pitch * 0.28
	white := uint8(255)
	rad := int(math.Max(4, math.Round(r)))
	for _, p := range pads {
		cx := int(math.Round(p.X)) - roi.Min.X
		cy := int(math.Round(p.Y)) - roi.Min.Y
		drawGrayCircle(out, cx, cy, rad, white, false)
		drawGrayCircle(out, cx, cy, rad-1, white, false)
		drawGrayCircle(out, cx, cy, max(1, rad/3), white, true)
	}
	return out
}

func SaveBWMask(roi image.Rectangle, pads []Pt, fit Lattice, path string) error {
	if roi.Empty() {
		return fmt.Errorf("empty roi")
	}
	return SavePNG(path, RenderBWMask(roi, pads, fit))
}

func drawGrayCircle(img *image.Gray, cx, cy, rad int, v uint8, fill bool) {
	if rad < 1 {
		return
	}
	for y := -rad; y <= rad; y++ {
		for x := -rad; x <= rad; x++ {
			d2 := x*x + y*y
			if fill {
				if d2 <= rad*rad {
					setGray(img, cx+x, cy+y, v)
				}
			} else if d2 >= (rad-1)*(rad-1) && d2 <= rad*rad {
				setGray(img, cx+x, cy+y, v)
			}
		}
	}
}

func setGray(img *image.Gray, x, y int, v uint8) {
	if !image.Pt(x, y).In(img.Bounds()) {
		return
	}
	img.Pix[img.PixOffset(x, y)] = v
}

// fitSocketGrid places a JEDEC 14×14 socket lattice so that as many
// detected circles as possible lie fully inside their cell.
func fitSocketGrid(pads []Pt, roiMinX, roiMinY, hint float64) Lattice {
	if len(pads) < 4 || hint < 5 {
		return Lattice{OX: roiMinX, OY: roiMinY, PX: hint, PY: hint, Score: -1}
	}
	cmin := pads[0]
	for _, p := range pads[1:] {
		if p.X+p.Y < cmin.X+cmin.Y {
			cmin = p
		}
	}

	best := Lattice{Score: -1}
	for _, pitch := range linspace(hint*0.88, hint*1.12, 21) {
		r := pitch * 0.25
		half := pitch * 0.50
		seedsX := []float64{roiMinX, cmin.X, cmin.X - pitch}
		seedsY := []float64{roiMinY, cmin.Y, cmin.Y - pitch}
		for _, sx := range seedsX {
			for _, sy := range seedsY {
				for _, dx := range linspace(-0.5*pitch, 0.5*pitch, 17) {
					for _, dy := range linspace(-0.5*pitch, 0.5*pitch, 17) {
						ox, oy := sx+dx, sy+dy
						n := countFullyInCellsRadius(pads, ox, oy, pitch, pitch, r, half)
						sc := float64(n)
						if sc > best.Score {
							best = Lattice{OX: ox, OY: oy, PX: pitch, PY: pitch, Cells: n, Hits: n, Score: sc}
						}
					}
				}
			}
		}
	}

	// fine refine
	if best.Score >= 0 {
		ox, oy, pitch := best.OX, best.OY, best.PX
		for _, dp := range linspace(-0.04*pitch, 0.04*pitch, 9) {
			p2 := pitch + dp
			if p2 < 5 {
				continue
			}
			r := p2 * 0.25
			half := p2 * 0.50
			for _, dx := range linspace(-0.15*p2, 0.15*p2, 11) {
				for _, dy := range linspace(-0.15*p2, 0.15*p2, 11) {
					n := countFullyInCellsRadius(pads, ox+dx, oy+dy, p2, p2, r, half)
					if float64(n) > best.Score {
						best = Lattice{OX: ox + dx, OY: oy + dy, PX: p2, PY: p2, Cells: n, Hits: n, Score: float64(n)}
					}
				}
			}
		}
		best = snapLatticeToJEDEC(pads, best)
		r := best.PX * 0.25
		half := best.PX * 0.50
		n := countFullyInCellsRadius(pads, best.OX, best.OY, best.PX, best.PY, r, half)
		best.Cells, best.Hits, best.Score = n, n, float64(n)
	}
	return best
}

func filterSocketPads(pads []Pt, pitch float64) []Pt {
	if len(pads) < 8 || pitch < 5 {
		return pads
	}
	tol := 0.35 * pitch
	var out []Pt
	for i, p := range pads {
		n := 0
		for j, q := range pads {
			if i == j {
				continue
			}
			dx, dy := q.X-p.X, q.Y-p.Y
			d := math.Hypot(dx, dy)
			if d < 0.7*pitch || d > 1.3*pitch {
				continue
			}
			// axis-aligned neighbor at ~1 pitch
			if (math.Abs(dy) < tol && math.Abs(dx) > 0.7*pitch) ||
				(math.Abs(dx) < tol && math.Abs(dy) > 0.7*pitch) {
				n++
			}
		}
		if n >= 2 {
			out = append(out, p)
		}
	}
	if len(out) < 12 {
		return pads
	}
	return out
}

func countFullyInCells(pads []Pt, ox, oy, px, py float64) int {
	pitch := math.Min(px, py)
	return countFullyInCellsRadius(pads, ox, oy, px, py, pitch*0.25, pitch*0.50)
}

// countFullyInCellsRadius: circle fully inside its cell (Chebyshev),
// and that cell has exactly one such pad.
func countFullyInCellsRadius(pads []Pt, ox, oy, px, py, r, half float64) int {
	type cell struct{ C, R int }
	type hit struct {
		d    float64
		full bool
	}
	best := map[cell]hit{}
	for _, p := range pads {
		ci := int(math.Round((p.X - ox) / px))
		ri := int(math.Round((p.Y - oy) / py))
		if ci < 0 || ci > 13 || ri < 0 || ri > 13 {
			continue
		}
		cx := ox + float64(ci)*px
		cy := oy + float64(ri)*py
		d := math.Max(math.Abs(p.X-cx), math.Abs(p.Y-cy))
		full := d+r <= half
		k := cell{ci, ri}
		prev, ok := best[k]
		if !ok {
			best[k] = hit{d: d, full: full}
			continue
		}
		best[k] = hit{d: math.Min(prev.d, d), full: false}
	}
	n := 0
	for _, h := range best {
		if h.full {
			n++
		}
	}
	return n
}

func padsFullyInCells(pads []Pt, ox, oy, px, py, r, half float64) []Pt {
	type cell struct{ C, R int }
	type hit struct {
		p    Pt
		d    float64
		full bool
		dup  bool
	}
	best := map[cell]*hit{}
	for _, p := range pads {
		ci := int(math.Round((p.X - ox) / px))
		ri := int(math.Round((p.Y - oy) / py))
		if ci < 0 || ci > 13 || ri < 0 || ri > 13 {
			continue
		}
		cx := ox + float64(ci)*px
		cy := oy + float64(ri)*py
		d := math.Max(math.Abs(p.X-cx), math.Abs(p.Y-cy))
		full := d+r <= half
		k := cell{ci, ri}
		if prev, ok := best[k]; ok {
			prev.dup = true
			prev.full = false
			if d < prev.d {
				prev.p, prev.d = p, d
			}
			continue
		}
		best[k] = &hit{p: p, d: d, full: full}
	}
	var out []Pt
	for _, h := range best {
		if h.full && !h.dup {
			out = append(out, h.p)
		}
	}
	return out
}

func SavePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func ToRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}

func ParseROI(s string, bounds image.Rectangle) (image.Rectangle, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return image.Rectangle{}, fmt.Errorf("roi must be x,y,w,h")
	}
	vals := make([]int, 4)
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return image.Rectangle{}, err
		}
		vals[i] = v
	}
	r := image.Rect(vals[0], vals[1], vals[0]+vals[2], vals[1]+vals[3])
	return r.Intersect(bounds), nil
}

func parseXY(s string) (float64, float64, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected x,y")
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, err
	}
	y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	return x, y, err
}

func autoROI(img *image.RGBA) image.Rectangle {
	b := img.Bounds()
	// Central square fallback (~55% of min side), matching Python default.
	side := int(0.55 * float64(min(b.Dx(), b.Dy())))
	cx, cy := b.Dx()/2, b.Dy()/2
	r := image.Rect(cx-side/2, cy-side/2, cx+side/2, cy+side/2)
	return r.Intersect(b)
}

func grayAt(img *image.RGBA, x, y int) uint8 {
	i := img.PixOffset(x, y)
	r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
	return uint8((int(r)*30 + int(g)*59 + int(b)*11) / 100)
}

func detectPads(img *image.RGBA, roi image.Rectangle) []Pt {
	roi = roi.Intersect(img.Bounds())
	if roi.Empty() {
		return nil
	}
	w, h := roi.Dx(), roi.Dy()
	pitchG := float64(min(w, h)) / 14.5

	// 1) grayscale
	gray := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gray[y*w+x] = grayAt(img, roi.Min.X+x, roi.Min.Y+y)
		}
	}
	blur := boxBlur(gray, w, h, 2)

	// 2) B&W: dark pads on brighter copper (local black-hat)
	mask := make([]bool, w*h)
	for i := range gray {
		diff := int(blur[i]) - int(gray[i])
		if diff > 10 && gray[i] < 170 {
			mask[i] = true
		}
	}
	// Fill hollow rings → solid disks, then open speckles
	mask = morphDilate(mask, w, h, 2)
	mask = morphErode(mask, w, h, 2)
	mask = morphOpen(mask, w, h, 1)

	minArea := max(8.0, (pitchG*0.10)*(pitchG*0.10))
	maxArea := max(minArea+1, (pitchG*0.55)*(pitchG*0.55))
	comps := connectedComponents(mask, w, h)

	var pads []Pt
	for _, c := range comps {
		if float64(c.Area) < minArea || float64(c.Area) > maxArea {
			continue
		}
		bw, bh := c.MaxX-c.MinX+1, c.MaxY-c.MinY+1
		if bw < 2 || bh < 2 {
			continue
		}
		aspect := float64(max(bw, bh)) / float64(max(1, min(bw, bh)))
		if aspect > 1.8 {
			continue
		}
		// circularity: 4πA / P² ≈ 1 for circles; use bbox fill as proxy
		fill := float64(c.Area) / float64(bw*bh)
		if fill < 0.35 {
			continue
		}
		// refine center to darkest pixel in blob bbox (pad center)
		cx, cy := refineDarkCenter(gray, w, h, c)
		pads = append(pads, Pt{X: cx + float64(roi.Min.X), Y: cy + float64(roi.Min.Y)})
	}
	return mergeClose(pads, pitchG*0.30)
}

func morphOpen(mask []bool, w, h, r int) []bool {
	eroded := morphErode(mask, w, h, r)
	return morphDilate(eroded, w, h, r)
}

func morphErode(mask []bool, w, h, r int) []bool {
	out := make([]bool, len(mask))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ok := true
			for dy := -r; dy <= r && ok; dy++ {
				for dx := -r; dx <= r; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h || !mask[ny*w+nx] {
						ok = false
						break
					}
				}
			}
			out[y*w+x] = ok
		}
	}
	return out
}

func morphDilate(mask []bool, w, h, r int) []bool {
	out := make([]bool, len(mask))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !mask[y*w+x] {
				continue
			}
			for dy := -r; dy <= r; dy++ {
				for dx := -r; dx <= r; dx++ {
					nx, ny := x+dx, y+dy
					if nx >= 0 && ny >= 0 && nx < w && ny < h {
						out[ny*w+nx] = true
					}
				}
			}
		}
	}
	return out
}

func refineDarkCenter(gray []uint8, w, h int, c component) (float64, float64) {
	var sx, sy, sw float64
	for y := c.MinY; y <= c.MaxY; y++ {
		for x := c.MinX; x <= c.MaxX; x++ {
			v := gray[y*w+x]
			// weight darker pixels more
			wt := float64(255 - v)
			sx += float64(x) * wt
			sy += float64(y) * wt
			sw += wt
		}
	}
	if sw < 1 {
		return c.CX, c.CY
	}
	return sx / sw, sy / sw
}

type unit2x2 struct {
	TL, TR, BL, BR Pt
	CX, CY         float64
	PX, PY         float64
}

// fitFrom2x2 finds circles linked by the same pitch displacement and
// locks the JEDEC lattice to their centers. hint is expected pitch (ROI/13).
func fitFrom2x2(pads []Pt, hint float64) (Lattice, int) {
	if len(pads) < 8 {
		return Lattice{Score: -1e9}, 0
	}
	pitch := estimatePitch(pads)
	if hint > 0 && (pitch < 5 || pitch > 1.35*hint || pitch < 0.7*hint) {
		pitch = hint
	}
	if pitch < 5 {
		return Lattice{Score: -1e9}, 0
	}
	fmt.Printf("pitch=%.2f (hint=%.2f)\n", pitch, hint)
	units := find2x2(pads, pitch)
	fmt.Printf("2x2 raw=%d\n", len(units))
	// Sweep around hint and nn pitch
	if len(units) < 3 {
		bestN := 0
		bestP := pitch
		seeds := []float64{pitch, hint}
		if hint > 0 {
			for _, s := range []float64{0.85, 0.9, 0.95, 1.0, 1.05, 1.1, 1.15} {
				seeds = append(seeds, hint*s)
			}
		}
		for _, s := range []float64{0.85, 0.9, 0.95, 1.05, 1.1, 1.15, 1.2} {
			seeds = append(seeds, pitch*s)
		}
		for _, p := range seeds {
			if p < 8 {
				continue
			}
			u := find2x2(pads, p)
			if len(u) > bestN {
				bestN = len(u)
				bestP = p
				units = u
			}
		}
		pitch = bestP
		fmt.Printf("2x2 after sweep: %d @ %.2f\n", len(units), pitch)
	}

	if len(units) < 3 {
		return Lattice{Score: -1e9}, len(units)
	}
	pxs := make([]float64, 0, len(units)*2)
	pys := make([]float64, 0, len(units)*2)
	for _, u := range units {
		pxs = append(pxs, u.PX)
		pys = append(pys, u.PY)
	}
	sort.Float64s(pxs)
	sort.Float64s(pys)
	px := percentile(pxs, 0.5)
	py := percentile(pys, 0.5)
	if math.Abs(px-py)/math.Max(px, py) < 0.15 {
		p := (px + py) / 2
		px, py = p, p
	}

	// Origin near top-left of pad cloud / 2×2 TL corners
	cmin := pads[0]
	for _, p := range pads[1:] {
		if p.X+p.Y < cmin.X+cmin.Y {
			cmin = p
		}
	}
	tls := make([]Pt, len(units))
	for i, u := range units {
		tls[i] = u.TL
	}
	tlMed := medianPt(tls)

	best := Lattice{Score: -1e9}
	seeds := []Pt{cmin, tlMed, {tlMed.X - px, tlMed.Y - py}, {cmin.X, cmin.Y - py}}
	for _, seed := range seeds {
		for _, dx := range linspace(-0.55*px, 0.55*px, 15) {
			for _, dy := range linspace(-0.55*py, 0.55*py, 15) {
				for _, s := range linspace(0.97, 1.03, 5) {
					oox, ooy := seed.X+dx, seed.Y+dy
					pxx, pyy := px*s, py*s
					sc, cells, hits := score(pads, oox, ooy, pxx, pyy, 0.32*math.Min(pxx, pyy))
					if sc > best.Score {
						best = Lattice{OX: oox, OY: ooy, PX: pxx, PY: pyy, Cells: cells, Hits: hits, Score: sc}
					}
				}
			}
		}
	}
	if best.Score > -1e8 {
		ox, oy, pxx, pyy := best.OX, best.OY, best.PX, best.PY
		for _, dx := range linspace(-0.2*pxx, 0.2*pxx, 11) {
			for _, dy := range linspace(-0.2*pyy, 0.2*pyy, 11) {
				for _, s := range linspace(0.98, 1.02, 5) {
					sc, cells, hits := score(pads, ox+dx, oy+dy, pxx*s, pyy*s, 0.32*math.Min(pxx, pyy)*s)
					if sc > best.Score {
						best = Lattice{OX: ox + dx, OY: oy + dy, PX: pxx * s, PY: pyy * s, Cells: cells, Hits: hits, Score: sc}
					}
				}
			}
		}
		best = snapLatticeToJEDEC(pads, best)
	}
	return best, len(units)
}

func snapLatticeToJEDEC(pads []Pt, fit Lattice) Lattice {
	tol := 0.32 * math.Min(fit.PX, fit.PY)
	type cell struct{ C, R int }
	occ := map[cell]struct{}{}
	for _, p := range pads {
		ci := int(math.Round((p.X - fit.OX) / fit.PX))
		ri := int(math.Round((p.Y - fit.OY) / fit.PY))
		lx := fit.OX + float64(ci)*fit.PX
		ly := fit.OY + float64(ri)*fit.PY
		if hypot(p.X-lx, p.Y-ly) <= tol {
			occ[cell{ci, ri}] = struct{}{}
		}
	}
	if len(occ) == 0 {
		return fit
	}
	minC, minR := 99, 99
	maxC, maxR := -99, -99
	for c := range occ {
		if c.C < minC {
			minC = c.C
		}
		if c.R < minR {
			minR = c.R
		}
		if c.C > maxC {
			maxC = c.C
		}
		if c.R > maxR {
			maxR = c.R
		}
	}
	shiftC, shiftR := minC, minR
	if maxC-shiftC > 13 {
		shiftC = maxC - 13
	}
	if maxR-shiftR > 13 {
		shiftR = maxR - 13
	}
	fit.OX += float64(shiftC) * fit.PX
	fit.OY += float64(shiftR) * fit.PY
	sc, cells, hits := score(pads, fit.OX, fit.OY, fit.PX, fit.PY, tol)
	fit.Score, fit.Cells, fit.Hits = sc, cells, hits
	return fit
}

func find2x2(pads []Pt, pitch float64) []unit2x2 {
	tol := 0.40 * pitch
	var out []unit2x2
	seen := map[[4]int]bool{}
	for i, tl := range pads {
		tr, okR := nearestInDir(pads, i, pitch, 0, tol)
		bl, okD := nearestInDir(pads, i, 0, pitch, tol)
		if !okR || !okD {
			continue
		}
		// reject if "right" isn't mostly horizontal / "down" mostly vertical
		if math.Abs(tr.Y-tl.Y) > 0.35*pitch || math.Abs(bl.X-tl.X) > 0.35*pitch {
			continue
		}
		if (tr.X-tl.X) < 0.55*pitch || (bl.Y-tl.Y) < 0.55*pitch {
			continue
		}
		brExpect := Pt{X: tl.X + (tr.X - tl.X), Y: tl.Y + (bl.Y - tl.Y)}
		br, okBR := nearestTo(pads, brExpect, tol)
		if !okBR {
			continue
		}
		px := (hypot(tr.X-tl.X, tr.Y-tl.Y) + hypot(br.X-bl.X, br.Y-bl.Y)) / 2
		py := (hypot(bl.X-tl.X, bl.Y-tl.Y) + hypot(br.X-tr.X, br.Y-tr.Y)) / 2
		if px < 0.6*pitch || px > 1.4*pitch || py < 0.6*pitch || py > 1.4*pitch {
			continue
		}
		key := [4]int{
			int(tl.X*10 + tl.Y), int(tr.X*10 + tr.Y),
			int(bl.X*10 + bl.Y), int(br.X*10 + br.Y),
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, unit2x2{
			TL: tl, TR: tr, BL: bl, BR: br,
			CX: (tl.X + tr.X + bl.X + br.X) / 4,
			CY: (tl.Y + tr.Y + bl.Y + br.Y) / 4,
			PX: px, PY: py,
		})
	}
	return out
}

func nearestInDir(pads []Pt, from int, dx, dy, tol float64) (Pt, bool) {
	target := Pt{X: pads[from].X + dx, Y: pads[from].Y + dy}
	return nearestTo(pads, target, tol)
}

func nearestTo(pads []Pt, target Pt, tol float64) (Pt, bool) {
	bestI := -1
	bestD := tol
	for i, p := range pads {
		d := hypot(p.X-target.X, p.Y-target.Y)
		if d <= bestD {
			bestD = d
			bestI = i
		}
	}
	if bestI < 0 {
		return Pt{}, false
	}
	return pads[bestI], true
}

type component struct {
	Area                   int
	MinX, MinY, MaxX, MaxY int
	CX, CY                 float64
}

func connectedComponents(mask []bool, w, h int) []component {
	labels := make([]int, w*h)
	var comps []component
	next := 1
	qx := make([]int, 0, 256)
	qy := make([]int, 0, 256)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if !mask[i] || labels[i] != 0 {
				continue
			}
			qx = qx[:0]
			qy = qy[:0]
			qx = append(qx, x)
			qy = append(qy, y)
			labels[i] = next
			var area int
			minX, minY, maxX, maxY := x, y, x, y
			var sx, sy float64
			for len(qx) > 0 {
				cx, cy := qx[0], qy[0]
				qx, qy = qx[1:], qy[1:]
				area++
				sx += float64(cx)
				sy += float64(cy)
				if cx < minX {
					minX = cx
				}
				if cy < minY {
					minY = cy
				}
				if cx > maxX {
					maxX = cx
				}
				if cy > maxY {
					maxY = cy
				}
				for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					nx, ny := cx+d[0], cy+d[1]
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					ni := ny*w + nx
					if !mask[ni] || labels[ni] != 0 {
						continue
					}
					labels[ni] = next
					qx = append(qx, nx)
					qy = append(qy, ny)
				}
			}
			comps = append(comps, component{
				Area: area, MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY,
				CX: sx / float64(area), CY: sy / float64(area),
			})
			next++
		}
	}
	return comps
}

func boxBlur(src []uint8, w, h, radius int) []uint8 {
	if radius <= 0 {
		out := make([]uint8, len(src))
		copy(out, src)
		return out
	}
	tmp := make([]int, w*h)
	out := make([]uint8, w*h)
	// horizontal
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum, n := 0, 0
			for k := -radius; k <= radius; k++ {
				xx := x + k
				if xx < 0 || xx >= w {
					continue
				}
				sum += int(src[y*w+xx])
				n++
			}
			tmp[y*w+x] = sum / n
		}
	}
	// vertical
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum, n := 0, 0
			for k := -radius; k <= radius; k++ {
				yy := y + k
				if yy < 0 || yy >= h {
					continue
				}
				sum += tmp[yy*w+x]
				n++
			}
			out[y*w+x] = uint8(sum / n)
		}
	}
	return out
}

func mergeClose(pts []Pt, tol float64) []Pt {
	if len(pts) == 0 {
		return pts
	}
	used := make([]bool, len(pts))
	var out []Pt
	tol2 := tol * tol
	for i := range pts {
		if used[i] {
			continue
		}
		sumX, sumY := 0.0, 0.0
		n := 0
		for j := i; j < len(pts); j++ {
			if used[j] {
				continue
			}
			dx := pts[i].X - pts[j].X
			dy := pts[i].Y - pts[j].Y
			if dx*dx+dy*dy <= tol2 {
				used[j] = true
				sumX += pts[j].X
				sumY += pts[j].Y
				n++
			}
		}
		out = append(out, Pt{X: sumX / float64(n), Y: sumY / float64(n)})
	}
	return out
}

func estimatePitch(pts []Pt) float64 {
	if len(pts) < 6 {
		return 0
	}
	// For each pad: nearest right and nearest below neighbor distances
	var horiz, vert []float64
	for i := range pts {
		bestH, bestV := 1e9, 1e9
		for j := range pts {
			if i == j {
				continue
			}
			dx := pts[j].X - pts[i].X
			dy := pts[j].Y - pts[i].Y
			d := math.Hypot(dx, dy)
			if d < 10 || d > 90 {
				continue
			}
			// mostly to the right
			if dx > 0 && math.Abs(dy) < 0.3*d && dx > 0.7*d && d < bestH {
				bestH = d
			}
			// mostly below
			if dy > 0 && math.Abs(dx) < 0.3*d && dy > 0.7*d && d < bestV {
				bestV = d
			}
		}
		if bestH < 1e8 {
			horiz = append(horiz, bestH)
		}
		if bestV < 1e8 {
			vert = append(vert, bestV)
		}
	}
	mode := func(v []float64) float64 {
		if len(v) < 3 {
			return 0
		}
		sort.Float64s(v)
		return percentile(v, 0.5)
	}
	h, v := mode(horiz), mode(vert)
	switch {
	case h > 0 && v > 0:
		return (h + v) / 2
	case h > 0:
		return h
	case v > 0:
		return v
	}
	// fallback: classic NN
	var nn []float64
	for i := range pts {
		best := 1e9
		for j := range pts {
			if i == j {
				continue
			}
			d := hypot(pts[i].X-pts[j].X, pts[i].Y-pts[j].Y)
			if d < best {
				best = d
			}
		}
		if best < 1e8 {
			nn = append(nn, best)
		}
	}
	if len(nn) == 0 {
		return 0
	}
	sort.Float64s(nn)
	return percentile(nn, 0.5)
}

func compactPads(pts []Pt, pitch float64) []Pt {
	if len(pts) < 12 || pitch <= 0 {
		return pts
	}
	var keep []Pt
	for i := range pts {
		n := 0
		for j := range pts {
			if i == j {
				continue
			}
			d := hypot(pts[i].X-pts[j].X, pts[i].Y-pts[j].Y)
			if d < 1.6*pitch {
				n++
			}
		}
		if n >= 1 {
			keep = append(keep, pts[i])
		}
	}
	if len(keep) < 12 {
		return pts
	}
	med := medianPt(keep)
	var out []Pt
	for _, p := range keep {
		if hypot(p.X-med.X, p.Y-med.Y) <= 9*pitch {
			out = append(out, p)
		}
	}
	if len(out) >= 12 {
		return out
	}
	return pts
}

func refineAround(pts []Pt, ox, oy, pitch float64) Lattice {
	best := Lattice{Score: -1e9}
	for _, s := range linspace(0.94, 1.10, 13) {
		px := pitch * s
		tol := 0.33 * px
		for _, dx := range linspace(-1.2*px, 1.2*px, 21) {
			for _, dy := range linspace(-1.2*px, 1.2*px, 21) {
				sc, cells, hits := score(pts, ox+dx, oy+dy, px, px, tol)
				if sc > best.Score {
					best = Lattice{OX: ox + dx, OY: oy + dy, PX: px, PY: px, Cells: cells, Hits: hits, Score: sc}
				}
			}
		}
	}
	if best.Score < 0 {
		// geometric fallback
		return Lattice{OX: ox, OY: oy, PX: pitch, PY: pitch}
	}
	ox, oy, px := best.OX, best.OY, best.PX
	for _, dx := range linspace(-0.25*px, 0.25*px, 11) {
		for _, dy := range linspace(-0.25*px, 0.25*px, 11) {
			for _, s := range linspace(0.98, 1.02, 5) {
				sc, cells, hits := score(pts, ox+dx, oy+dy, px*s, px*s, 0.33*px*s)
				if sc > best.Score {
					best = Lattice{OX: ox + dx, OY: oy + dy, PX: px * s, PY: px * s, Cells: cells, Hits: hits, Score: sc}
				}
			}
		}
	}
	return best
}

func score(pts []Pt, ox, oy, px, py, tol float64) (float64, int, int) {
	if px <= 1 || py <= 1 || len(pts) == 0 {
		return -1e9, 0, 0
	}
	type cell struct{ C, R int }
	cells := map[cell]struct{}{}
	var errSum float64
	hits := 0
	for _, p := range pts {
		ci := int(math.Round((p.X - ox) / px))
		ri := int(math.Round((p.Y - oy) / py))
		lx := ox + float64(ci)*px
		ly := oy + float64(ri)*py
		err := hypot(p.X-lx, p.Y-ly)
		if err <= tol && ci >= 0 && ci <= 13 && ri >= 0 && ri <= 13 {
			cells[cell{ci, ri}] = struct{}{}
			errSum += err
			hits++
		}
	}
	if hits < 6 {
		return -1e9, 0, 0
	}
	outer := 0
	for c := range cells {
		ring := min(min(c.C, c.R), min(13-c.C, 13-c.R))
		if ring <= 2 {
			outer++
		}
	}
	sc := float64(len(cells))*3.0 + float64(outer)*0.75 - errSum/float64(hits)
	return sc, len(cells), hits
}

func rowIndex(row byte) int {
	for i, r := range rows {
		if r == row {
			return i
		}
	}
	return -1
}

func ballXY(fit Lattice, row byte, col int) Pt {
	return Pt{X: fit.OX + float64(col-1)*fit.PX, Y: fit.OY + float64(rowIndex(row))*fit.PY}
}

func DrawAnnotation(src *image.RGBA, fit Lattice, pads []Pt, roi image.Rectangle) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, src, b.Min, draw.Src)

	cyan := color.RGBA{R: 0, G: 255, B: 255, A: 255}
	green := color.RGBA{R: 0, G: 255, B: 0, A: 255}
	amber := color.RGBA{R: 0, G: 200, B: 255, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.RGBA{A: 255}

	drawRect(out, roi, cyan, 2)
	for _, p := range pads {
		drawCircle(out, int(p.X+0.5), int(p.Y+0.5), 3, green, false)
	}
	for ri := 0; ri < 14; ri++ {
		for ci := 0; ci < 14; ci++ {
			x := int(math.Round(fit.OX + float64(ci)*fit.PX))
			y := int(math.Round(fit.OY + float64(ri)*fit.PY))
			drawCircle(out, x, y, 2, amber, true)
		}
		x := int(math.Round(fit.OX - 0.65*fit.PX))
		y := int(math.Round(fit.OY + float64(ri)*fit.PY))
		drawString(out, x-6, y+4, string(rows[ri]), cyan)
	}
	for ci := 0; ci < 14; ci++ {
		x := int(math.Round(fit.OX + float64(ci)*fit.PX))
		y := int(math.Round(fit.OY - 0.55*fit.PY))
		drawString(out, x-5, y, strconv.Itoa(ci+1), cyan)
	}

	ly := 36
	lx := min(b.Dx()-12, max(roi.Max.X+10, int(float64(b.Dx())*0.78)))
	for _, pin := range ispPins {
		p := ballXY(fit, pin.Row, pin.Col)
		ix, iy := int(math.Round(p.X)), int(math.Round(p.Y))
		drawCircle(out, ix, iy, 11, pin.Color, false)
		drawCircle(out, ix, iy, 3, pin.Color, true)
		label := fmt.Sprintf("%c%d %s", pin.Row, pin.Col, pin.Name)
		drawLine(out, ix, iy, lx, ly, pin.Color)
		tw := len(label) * 7
		drawFilledRect(out, image.Rect(lx, ly-12, lx+tw+8, ly+4), black)
		drawString(out, lx+3, ly, label, pin.Color)
		ly += 28
	}

	title := "eMMC153 ISP  |  A1 = top-left (silkscreen notch)"
	drawString(out, 11, 23, title, black)
	drawString(out, 10, 22, title, white)
	meta := fmt.Sprintf("pitch=%.1fx%.1fpx  cells=%d hits=%d score=%.1f pads=%d",
		fit.PX, fit.PY, fit.Cells, fit.Hits, fit.Score, len(pads))
	drawString(out, 10, b.Dy()-12, meta, white)
	return out
}

func BuildJSON(in, out string, roi image.Rectangle, pads []Pt, fit Lattice) ResultJSON {
	isp := map[string]PinJSON{}
	for _, pin := range ispPins {
		p := ballXY(fit, pin.Row, pin.Col)
		isp[pin.Name] = PinJSON{
			Ball: fmt.Sprintf("%c%d", pin.Row, pin.Col),
			X:    round2(p.X),
			Y:    round2(p.Y),
		}
	}
	return ResultJSON{
		Input:    in,
		Output:   out,
		ROI:      []int{roi.Min.X, roi.Min.Y, roi.Max.X, roi.Max.Y},
		PadCount: len(pads),
		Lattice: map[string]any{
			"A1":       []float64{fit.OX, fit.OY},
			"pitch_xy": []float64{fit.PX, fit.PY},
			"cells":    fit.Cells,
			"hits":     fit.Hits,
			"score":    fit.Score,
		},
		ISP:         isp,
		Orientation: "A1 top-left (silkscreen notch); rows A..P skip I,O; cols 1..14",
	}
}

func drawRect(img *image.RGBA, r image.Rectangle, c color.RGBA, thickness int) {
	for t := 0; t < thickness; t++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			setRGBA(img, x, r.Min.Y+t, c)
			setRGBA(img, x, r.Max.Y-1-t, c)
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			setRGBA(img, r.Min.X+t, y, c)
			setRGBA(img, r.Max.X-1-t, y, c)
		}
	}
}

func drawFilledRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			setRGBA(img, x, y, c)
		}
	}
}

func drawCircle(img *image.RGBA, cx, cy, rad int, c color.RGBA, fill bool) {
	for y := -rad; y <= rad; y++ {
		for x := -rad; x <= rad; x++ {
			d2 := x*x + y*y
			if fill {
				if d2 <= rad*rad {
					setRGBA(img, cx+x, cy+y, c)
				}
			} else if d2 >= (rad-1)*(rad-1) && d2 <= rad*rad {
				setRGBA(img, cx+x, cy+y, c)
			}
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		setRGBA(img, x0, y0, c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func drawString(img *image.RGBA, x, y int, s string, c color.RGBA) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
}

func setRGBA(img *image.RGBA, x, y int, c color.RGBA) {
	if !image.Pt(x, y).In(img.Bounds()) {
		return
	}
	i := img.PixOffset(x, y)
	img.Pix[i] = c.R
	img.Pix[i+1] = c.G
	img.Pix[i+2] = c.B
	img.Pix[i+3] = c.A
}

func linspace(a, b float64, n int) []float64 {
	if n <= 1 {
		return []float64{a}
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = a + (b-a)*float64(i)/float64(n-1)
	}
	return out
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	i := int(idx)
	if i >= len(sorted)-1 {
		return sorted[len(sorted)-1]
	}
	f := idx - float64(i)
	return sorted[i]*(1-f) + sorted[i+1]*f
}

func medianPt(pts []Pt) Pt {
	xs := make([]float64, len(pts))
	ys := make([]float64, len(pts))
	for i, p := range pts {
		xs[i], ys[i] = p.X, p.Y
	}
	sort.Float64s(xs)
	sort.Float64s(ys)
	return Pt{X: percentile(xs, 0.5), Y: percentile(ys, 0.5)}
}

func hypot(x, y float64) float64 { return math.Hypot(x, y) }
func round2(v float64) float64   { return math.Round(v*100) / 100 }
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

