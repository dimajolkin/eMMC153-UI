package emmcisp

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Options configures annotation from a PCB photo.
type Options struct {
	Input    string
	Output   string
	JSONPath string
	BWPath   string
	ROI      string  // "x,y,w,h"; empty = auto
	A1       string  // "x,y"; requires Pitch > 0
	Pitch    float64 // ball pitch in pixels
}

// Result holds in-memory annotation outputs.
type Result struct {
	Input     string
	Output    string
	JSONPath  string
	BWPath    string
	ROI       image.Rectangle
	Lattice   Lattice
	Pads      []Pt
	Annotated *image.RGBA
	Mask      *image.Gray
	Payload   ResultJSON
}

// ResultJSON is the sidecar metadata written next to the annotated PNG.
type ResultJSON struct {
	Input       string             `json:"input"`
	Output      string             `json:"output"`
	ROI         []int              `json:"roi"`
	PadCount    int                `json:"pad_count"`
	Lattice     map[string]any     `json:"lattice"`
	ISP         map[string]PinJSON `json:"isp"`
	Orientation string             `json:"orientation"`
}

// PinJSON is one ISP ball coordinate.
type PinJSON struct {
	Ball string  `json:"ball"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

// Run loads the input image and fits the JEDEC socket lattice (in memory).
func Run(opts Options) (*Result, error) {
	if opts.Input == "" {
		return nil, fmt.Errorf("input image path required")
	}
	img, err := LoadImage(opts.Input)
	if err != nil {
		return nil, err
	}
	rgba := ToRGBA(img)
	bounds := rgba.Bounds()

	var roi image.Rectangle
	if opts.ROI != "" {
		roi, err = ParseROI(opts.ROI, bounds)
		if err != nil {
			return nil, err
		}
	} else {
		roi = autoROI(rgba)
	}

	res, err := Annotate(rgba, roi, opts.A1, opts.Pitch)
	if err != nil {
		return nil, err
	}
	res.Input = opts.Input
	res.Output = opts.Output
	if res.Output == "" {
		ext := filepath.Ext(opts.Input)
		res.Output = strings.TrimSuffix(opts.Input, ext) + "-isp-annotated.png"
	}
	res.JSONPath = opts.JSONPath
	if res.JSONPath == "" {
		res.JSONPath = strings.TrimSuffix(res.Output, filepath.Ext(res.Output)) + ".json"
	}
	res.BWPath = opts.BWPath
	res.Payload = BuildJSON(opts.Input, res.Output, roi, res.Pads, res.Lattice)
	return res, nil
}

// WriteFiles saves annotated PNG, JSON, and optional B&W mask.
func (r *Result) WriteFiles() error {
	if r.Annotated == nil {
		return fmt.Errorf("no annotated image")
	}
	if r.Output == "" {
		return fmt.Errorf("output path empty")
	}
	if err := SavePNG(r.Output, r.Annotated); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(r.Payload, "", "  ")
	if err != nil {
		return err
	}
	if r.JSONPath == "" {
		r.JSONPath = strings.TrimSuffix(r.Output, filepath.Ext(r.Output)) + ".json"
	}
	if err := os.WriteFile(r.JSONPath, raw, 0o644); err != nil {
		return err
	}
	if r.BWPath != "" {
		if r.Mask == nil {
			r.Mask = RenderBWMask(r.ROI, r.Pads, r.Lattice)
		}
		if err := SavePNG(r.BWPath, r.Mask); err != nil {
			return err
		}
	}
	return nil
}

// Annotate fits socket lattice on an already-loaded RGBA image.
// a1 is "x,y" (optional); pitch in pixels (required if a1 set).
func Annotate(rgba *image.RGBA, roi image.Rectangle, a1 string, pitch float64) (*Result, error) {
	if a1 != "" && pitch > 0 {
		ax, ay, err := parseXY(a1)
		if err != nil {
			return nil, err
		}
		return AnnotateLattice(rgba, Lattice{OX: ax, OY: ay, PX: pitch, PY: pitch})
	}
	grayROI, gw, gh := roiGray(rgba, roi)
	hint := float64(min(roi.Dx(), roi.Dy())) / 13.0
	fit := fitSocketOnImage(grayROI, gw, gh, roi, hint)
	return AnnotateLattice(rgba, fit)
}

// AnnotateLattice scores pads on a fixed JEDEC lattice (manual or auto fit).
func AnnotateLattice(rgba *image.RGBA, fit Lattice) (*Result, error) {
	if fit.PX <= 0 || fit.PY <= 0 {
		return nil, fmt.Errorf("pitch must be > 0")
	}
	roi := ROIFromLattice(rgba.Bounds(), fit)
	grayROI, gw, gh := roiGray(rgba, roi)
	pads := padsFromSocket(grayROI, gw, gh, roi, fit)
	n := countFullyInCells(pads, fit.OX, fit.OY, fit.PX, fit.PY)
	fit.Cells, fit.Hits, fit.Score = n, n, float64(n)

	annotated := DrawAnnotation(rgba, fit, pads, roi)
	mask := RenderBWMask(roi, pads, fit)
	return &Result{
		ROI:       roi,
		Lattice:   fit,
		Pads:      pads,
		Annotated: annotated,
		Mask:      mask,
		Payload:   BuildJSON("", "", roi, pads, fit),
	}, nil
}

// ROIFromLattice expands a pad window around the 14×14 lattice.
func ROIFromLattice(bounds image.Rectangle, fit Lattice) image.Rectangle {
	pad := fit.PX * 0.75
	if fit.PY*0.75 > pad {
		pad = fit.PY * 0.75
	}
	minX := int(fit.OX - pad)
	minY := int(fit.OY - pad)
	maxX := int(fit.OX + 13*fit.PX + pad + 1)
	maxY := int(fit.OY + 13*fit.PY + pad + 1)
	return image.Rect(minX, minY, maxX, maxY).Intersect(bounds)
}

// DrawGridPreview draws only the JEDEC grid + optional A1 marker (no ISP labels).
func DrawGridPreview(src *image.RGBA, fit Lattice, a1Set bool) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, src, b.Min, draw.Src)
	if fit.PX <= 0 || fit.PY <= 0 {
		return out
	}
	cyan := color.RGBA{R: 0, G: 220, B: 255, A: 255}
	amber := color.RGBA{R: 0, G: 200, B: 255, A: 255}
	yellow := color.RGBA{R: 255, G: 220, B: 0, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.RGBA{A: 255}

	roi := ROIFromLattice(b, fit)
	drawRect(out, roi, cyan, 2)
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
	if a1Set {
		ix, iy := int(math.Round(fit.OX)), int(math.Round(fit.OY))
		drawCircle(out, ix, iy, 12, yellow, false)
		drawCircle(out, ix, iy, 4, yellow, true)
		drawString(out, ix+14, iy+4, "A1 KEY", yellow)
	}
	title := "этап: сетка — подгони pitch / укажи A1"
	drawString(out, 11, 23, title, black)
	drawString(out, 10, 22, title, white)
	meta := fmt.Sprintf("A1=(%.1f,%.1f) pitch=%.2fx%.2f", fit.OX, fit.OY, fit.PX, fit.PY)
	drawString(out, 10, b.Dy()-12, meta, white)
	return out
}

// AutoROI returns a default pad window for the image.
func AutoROI(rgba *image.RGBA) image.Rectangle {
	return autoROI(rgba)
}
