// Package graph manages per-probe RRD databases and renders Smokeping-style
// latency graphs as PNG files via rrdtool.
//
// Each probe gets its own RRD file storing avg/min/max RTT and packet loss at
// one-minute resolution. Graphs are rendered with a translucent "smoke" band
// between min and max RTT, a bright average line, and a loss summary in the
// legend.
//
// # Windows path note
//
// rrdtool's DEF parser splits arguments on ':'. On Windows, absolute paths
// contain a drive-letter colon (C:\...) that breaks parsing — even with forward
// slashes (C:/...). The fix used throughout this package is to set cmd.Dir to
// the graphs directory and pass only bare filenames in all rrdtool arguments,
// so no argument ever contains a colon from a drive letter.
package graph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

const rrdStep = 60 // seconds; must match the probe run interval

// rrdtoolPath returns the absolute path to the rrdtool binary.
// It first checks the directory of the running executable (bundled deploy),
// then falls back to the system PATH.
func rrdtoolPath() string {
	bin := "rrdtool"
	if runtime.GOOS == "windows" {
		bin = "rrdtool.exe"
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), bin)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return bin
}

// rrdFilename returns the bare RRD filename for a probe (no directory).
func rrdFilename(probeID int64) string {
	return fmt.Sprintf("probe_%d.rrd", probeID)
}

// GraphDir returns the directory where RRD databases and PNG graphs are stored.
func GraphDir(dataDir string) string {
	return filepath.Join(dataDir, "graphs")
}

// RRDPath returns the full path to the RRD database for a probe.
func RRDPath(dataDir string, probeID int64) string {
	return filepath.Join(GraphDir(dataDir), rrdFilename(probeID))
}

// ProbePath returns the full path to the rendered PNG graph for a probe.
func ProbePath(dataDir string, probeID int64) string {
	return filepath.Join(GraphDir(dataDir), fmt.Sprintf("probe_%d.png", probeID))
}

// EnsureRRD creates the RRD database for a probe if it does not already exist.
// The database stores four data sources at one-minute resolution:
//   - avg: mean round-trip time (ms)
//   - min: minimum RTT (ms)
//   - max: maximum RTT (ms)
//   - loss: packet loss percentage (0–100)
func EnsureRRD(dataDir string, probeID int64) error {
	if _, err := os.Stat(RRDPath(dataDir, probeID)); err == nil {
		return nil // already exists
	}
	cmd := exec.Command(rrdtoolPath(),
		"create", rrdFilename(probeID),
		"--step", strconv.Itoa(rrdStep),
		// Heartbeat = 2× step; U (unknown) stored when host is unreachable.
		"DS:avg:GAUGE:120:0:U",
		"DS:min:GAUGE:120:0:U",
		"DS:max:GAUGE:120:0:U",
		"DS:loss:GAUGE:120:0:100",
		// 1-min resolution: 24 h of raw data.
		"RRA:AVERAGE:0.5:1:1440",
		"RRA:MIN:0.5:1:1440",
		"RRA:MAX:0.5:1:1440",
		// 5-min resolution: 1 week of consolidated data.
		"RRA:AVERAGE:0.5:5:2016",
		"RRA:MIN:0.5:5:2016",
		"RRA:MAX:0.5:5:2016",
	)
	cmd.Dir = GraphDir(dataDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rrdtool create: %w\noutput: %s", err, out)
	}
	return nil
}

// Update feeds one probe measurement into the RRD database.
// avg, min, and max are nil when the host was unreachable; they are stored as
// "U" (unknown) so RRDtool can handle gaps cleanly.
//
// The timestamp is passed to rrdtool as "N" (current time at invocation) rather
// than a pre-captured Unix value. This avoids "minimum one second step" errors
// when multiple probes finish within the same second and share the same ts.
func Update(dataDir string, probeID int64, _ time.Time, avg, min, max *float64, loss int) error {
	f := func(v *float64) string {
		if v == nil {
			return "U"
		}
		return strconv.FormatFloat(*v, 'f', 4, 64)
	}
	value := fmt.Sprintf("N:%s:%s:%s:%d", f(avg), f(min), f(max), loss)
	cmd := exec.Command(rrdtoolPath(), "update", rrdFilename(probeID), value)
	cmd.Dir = GraphDir(dataDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rrdtool update: %w\noutput: %s", err, out)
	}
	return nil
}

// Render generates a Smokeping-style PNG graph covering the last 24 hours and
// writes it to outputPath.
func Render(dataDir string, probeID int64, outputPath string) error {
	rrdFile := rrdFilename(probeID)
	pngFile := filepath.Base(outputPath)
	cmd := exec.Command(rrdtoolPath(), "graph", pngFile,
		"--imgformat", "PNG",
		"--width", "850",
		"--height", "150",
		"--start", "-86400",
		"--end", "now",
		"--title", fmt.Sprintf("Probe %d – last 24 h", probeID),
		"--vertical-label", "ms",
		"--lower-limit", "0",
		"--slope-mode",
		// Light theme.
		"--color", "BACK#ffffff",
		"--color", "CANVAS#ffffff",
		"--color", "FONT#333333",
		"--color", "GRID#cccccc",
		"--color", "MGRID#999999",
		"--color", "FRAME#999999",
		"--color", "ARROW#333333",
		// Data sources — bare filenames avoid the drive-letter colon on Windows.
		fmt.Sprintf("DEF:avg=%s:avg:AVERAGE", rrdFile),
		fmt.Sprintf("DEF:min=%s:min:MIN", rrdFile),
		fmt.Sprintf("DEF:max=%s:max:MAX", rrdFile),
		fmt.Sprintf("DEF:loss=%s:loss:AVERAGE", rrdFile),
		// Smoke band: transparent base + translucent fill from min→max.
		"CDEF:smoke=max,min,-",
		"AREA:min#00000000",
		"AREA:smoke#0099cc55:Smoke ",
		// Average RTT line.
		"LINE1:avg#00cc00:Average",
		// Legend.
		`GPRINT:avg:LAST: Last\: %6.2lf ms`,
		`GPRINT:avg:AVERAGE: Avg\: %6.2lf ms`,
		`GPRINT:avg:MAX: Max\: %6.2lf ms\n`,
		`GPRINT:loss:AVERAGE:Loss\: %.1lf%%\n`,
	)
	cmd.Dir = GraphDir(dataDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rrdtool graph: %w\noutput: %s", err, out)
	}
	return nil
}
