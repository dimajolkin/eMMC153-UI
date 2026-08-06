// Command annotate labels JEDEC eMMC153 ISP pads on a PCB photo.
//
//	go run ./cmd/annotate -in ../../docs/photos/14-emmc-footprint-closeup.png \
//	  -roi 240,240,400,400 -out ../../docs/photos/22-emmc153-isp-go.png
package main

import (
	"flag"
	"fmt"
	"os"

	emmcisp "github.com/dimajolkin/eMMC153-UI"
)

func main() {
	inPath := flag.String("in", "", "input PCB photo (PNG/JPEG)")
	outPath := flag.String("out", "", "annotated PNG (default: <in>-isp-annotated.png)")
	jsonPath := flag.String("json", "", "JSON output (default: beside -out)")
	roiStr := flag.String("roi", "", "pad ROI as x,y,w,h")
	a1Str := flag.String("a1", "", "A1 pixel as x,y")
	pitch := flag.Float64("pitch", 0, "ball pitch in pixels")
	bwPath := flag.String("bw", "", "save B&W pad mask PNG for debug")
	flag.Parse()

	if *inPath == "" {
		if flag.NArg() > 0 {
			*inPath = flag.Arg(0)
		} else {
			fmt.Fprintln(os.Stderr, "usage: annotate -in photo.png [-roi x,y,w,h] [-a1 x,y] [-pitch px] [-out out.png]")
			os.Exit(2)
		}
	}

	res, err := emmcisp.Run(emmcisp.Options{
		Input:    *inPath,
		Output:   *outPath,
		JSONPath: *jsonPath,
		BWPath:   *bwPath,
		ROI:      *roiStr,
		A1:       *a1Str,
		Pitch:    *pitch,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := res.WriteFiles(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("ROI: %v\n", res.ROI)
	fmt.Printf("lattice A1=(%.1f,%.1f) pitch=(%.2f,%.2f) cells=%d pads=%d\n",
		res.Lattice.OX, res.Lattice.OY, res.Lattice.PX, res.Lattice.PY, res.Lattice.Cells, len(res.Pads))
	fmt.Printf("wrote %s\n", res.Output)
	fmt.Printf("wrote %s\n", res.JSONPath)
	if res.BWPath != "" {
		fmt.Printf("wrote %s\n", res.BWPath)
	}
}
