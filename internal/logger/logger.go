package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Init configures the global slog logger.
//
//   - console: also write to stdout (useful for interactive / debug mode)
//   - verbose: set log level to Debug; otherwise Info
func Init(dataDir string, console, verbose bool) error {
	logsDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	lf := &rollingFile{dir: logsDir}

	var w io.Writer = lf
	if console {
		w = io.MultiWriter(os.Stdout, lf)
	}

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
	return nil
}

// rollingFile writes to a new file each day: agent-YYYY-MM-DD.log
type rollingFile struct {
	dir  string
	mu   sync.Mutex
	day  string
	file *os.File
}

func (r *rollingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if r.day != today {
		if r.file != nil {
			_ = r.file.Close()
		}
		name := filepath.Join(r.dir, "agent-"+today+".log")
		f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return 0, err
		}
		r.file = f
		r.day = today
	}

	return r.file.Write(p)
}
