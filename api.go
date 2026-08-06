package emmcisp

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
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
	grayROI, gw, gh := roiGray(rgba, roi)
	hint := float64(min(roi.Dx(), roi.Dy())) / 13.0

	var fit Lattice
	var pads []Pt
	if a1 != "" && pitch > 0 {
		ax, ay, err := parseXY(a1)
		if err != nil {
			return nil, err
		}
		fit = Lattice{OX: ax, OY: ay, PX: pitch, PY: pitch}
		pads = padsFromSocket(grayROI, gw, gh, roi, fit)
		n := countFullyInCells(pads, fit.OX, fit.OY, fit.PX, fit.PY)
		fit.Cells, fit.Hits, fit.Score = n, n, float64(n)
	} else {
		fit = fitSocketOnImage(grayROI, gw, gh, roi, hint)
		pads = padsFromSocket(grayROI, gw, gh, roi, fit)
	}

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

// AutoROI returns a default pad window for the image.
func AutoROI(rgba *image.RGBA) image.Rectangle {
	return autoROI(rgba)
}
