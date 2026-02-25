// Package graph renders Smokeping-style latency graphs as PNG files using only
// the Go standard library (image/png). No external dependencies are required.
//
// Each graph shows a configurable window of probe measurements as:
//   - A coloured smoke band from min RTT to max RTT
//   - A bright dot at the average RTT
//   - A dark-red tick at the bottom when the host was unreachable
//
// Colour encodes packet loss: green (0%) → yellow → orange → red (100%).
package graph

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	CanvasW = 900
	CanvasH = 180
	padL    = 8
	padR    = 8
	padT    = 8
	padB    = 8

	// PlotW is the usable plot width in pixels — also the maximum number of
	// samples that can be rendered (one column per sample).
	PlotW = CanvasW - padL - padR // 884
	plotH = CanvasH - padT - padB // 164
)

var (
	bgColor   = color.RGBA{0x1a, 0x1a, 0x2a, 0xff}
	gridColor = color.RGBA{0x2a, 0x2a, 0x45, 0xff}
	axisColor = color.RGBA{0x55, 0x55, 0x88, 0xff}
	downColor = color.RGBA{0x88, 0x00, 0x00, 0xff}
)

// Sample holds one probe measurement for graph rendering.
type Sample struct {
	AvgMS      *float64 // nil when the host was unreachable
	MinMS      *float64
	MaxMS      *float64
	PacketLoss int // 0–100
}

// Render writes a Smokeping-style PNG to outputPath.
// samples must be sorted oldest-first; at most PlotW samples are rendered.
func Render(samples []Sample, outputPath string) error {
	img := image.NewRGBA(image.Rect(0, 0, CanvasW, CanvasH))

	fillRect(img, 0, 0, CanvasW, CanvasH, bgColor)

	// Horizontal grid lines at 25%, 50%, 75% of the Y axis.
	for _, f := range []float64{0.25, 0.50, 0.75} {
		hline(img, padL, padL+PlotW-1, padT+int(float64(plotH)*(1-f)), gridColor)
	}

	// Axis frame.
	hline(img, padL, padL+PlotW-1, padT, axisColor)
	hline(img, padL, padL+PlotW-1, padT+plotH, axisColor)
	vline(img, padL, padT, padT+plotH, axisColor)
	vline(img, padL+PlotW-1, padT, padT+plotH, axisColor)

	if len(samples) == 0 {
		return encode(img, outputPath)
	}

	// Clamp to PlotW samples (take the most recent ones).
	if len(samples) > PlotW {
		samples = samples[len(samples)-PlotW:]
	}

	// Auto-scale Y: find the peak max RTT across all samples.
	peakMS := 10.0
	for _, s := range samples {
		if s.MaxMS != nil && *s.MaxMS > peakMS {
			peakMS = *s.MaxMS
		}
	}
	peakMS *= 1.2 // 20% headroom above the tallest spike

	toY := func(ms float64) int {
		f := ms / peakMS
		if f > 1 {
			f = 1
		}
		if f < 0 {
			f = 0
		}
		return padT + int(float64(plotH)*(1-f))
	}

	n := len(samples)
	toX := func(i int) int {
		if n <= 1 {
			return padL + PlotW/2
		}
		return padL + i*(PlotW-1)/(n-1)
	}

	for i, s := range samples {
		x := toX(i)

		if s.AvgMS == nil {
			// Host unreachable: draw a short dark-red tick at the bottom.
			vline(img, x, padT+plotH-5, padT+plotH, downColor)
			continue
		}

		bright, smoke := palette(s.PacketLoss)

		// Smoke band: shaded region from min to max RTT.
		if s.MinMS != nil && s.MaxMS != nil {
			vline(img, x, toY(*s.MaxMS), toY(*s.MinMS), smoke)
		}

		// Average: two-pixel bright marker.
		ya := toY(*s.AvgMS)
		setpx(img, x, ya, bright)
		setpx(img, x, ya+1, bright)
	}

	return encode(img, outputPath)
}

// GraphDir returns the path where probe PNGs are written.
func GraphDir(dataDir string) string {
	return dataDir + string(os.PathSeparator) + "graphs"
}

// ProbePath returns the full output path for a probe's graph PNG.
func ProbePath(dataDir string, probeID int64) string {
	return fmt.Sprintf("%s%cprobe_%d.png", GraphDir(dataDir), os.PathSeparator, probeID)
}

// palette returns a bright opaque and a semi-transparent smoke colour for
// a given packet-loss percentage.
func palette(loss int) (bright, smoke color.RGBA) {
	var r, g, b uint8
	switch {
	case loss == 0:
		r, g, b = 0x00, 0xcc, 0x00 // green
	case loss <= 5:
		r, g, b = 0x88, 0xcc, 0x00 // yellow-green
	case loss <= 20:
		r, g, b = 0xff, 0xcc, 0x00 // yellow
	case loss <= 50:
		r, g, b = 0xff, 0x66, 0x00 // orange
	default:
		r, g, b = 0xff, 0x00, 0x00 // red
	}
	return color.RGBA{r, g, b, 0xff}, color.RGBA{r, g, b, 0x60}
}

// ── primitive drawing helpers ────────────────────────────────────────────────

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func hline(img *image.RGBA, x0, x1, y int, c color.RGBA) {
	b := img.Bounds()
	if y < b.Min.Y || y >= b.Max.Y {
		return
	}
	for x := x0; x <= x1; x++ {
		if x >= b.Min.X && x < b.Max.X {
			img.SetRGBA(x, y, c)
		}
	}
}

func vline(img *image.RGBA, x, y0, y1 int, c color.RGBA) {
	b := img.Bounds()
	if x < b.Min.X || x >= b.Max.X {
		return
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		if y >= b.Min.Y && y < b.Max.Y {
			img.SetRGBA(x, y, c)
		}
	}
}

func setpx(img *image.RGBA, x, y int, c color.RGBA) {
	b := img.Bounds()
	if x >= b.Min.X && x < b.Max.X && y >= b.Min.Y && y < b.Max.Y {
		img.SetRGBA(x, y, c)
	}
}

func encode(img *image.RGBA, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
