// Copyright © 2026 ForcePoint Software. All rights reserved.

package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const logRetentionDays = 7

// Init configures the global slog logger with rolling file output.
// If console is true, logs are also written to stdout (useful for interactive/debug mode).
// If verbose is true, the log level is set to Debug; otherwise Info.
// A background goroutine prunes log files older than 7 days, running once at startup
// and then every 24 hours so long-running services don't accumulate stale files.
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

	// Prune old logs at startup, then daily. Run in a goroutine so a slow
	// filesystem scan never delays the caller.
	go func() {
		pruneOldLogs(logsDir)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			pruneOldLogs(logsDir)
		}
	}()

	return nil
}

// pruneOldLogs deletes agent-YYYY-MM-DD.log files whose date is more than
// logRetentionDays days in the past. Files that don't match the naming
// pattern are left untouched.
func pruneOldLogs(logsDir string) {
	cutoff := time.Now().AddDate(0, 0, -logRetentionDays).Truncate(24 * time.Hour)

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		slog.Warn("logger: failed to read logs directory", "err", err)
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Only touch files matching our own naming convention.
		if !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		// Parse the date from "agent-YYYY-MM-DD.log".
		datePart := strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".log")
		t, err := time.Parse("2006-01-02", datePart)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			path := filepath.Join(logsDir, name)
			if err := os.Remove(path); err != nil {
				slog.Warn("logger: failed to remove old log file", "file", name, "err", err)
			} else {
				slog.Info("logger: removed old log file", "file", name)
			}
		}
	}
}

// rollingFile is an io.Writer that creates a new log file each day.
// Files are named agent-YYYY-MM-DD.log.
type rollingFile struct {
	dir  string
	mu   sync.Mutex
	day  string
	file *os.File
}

// Write implements io.Writer, automatically rolling to a new file when the day changes.
func (r *rollingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if r.day != today {
		// Close the previous day's log file if open.
		if r.file != nil {
			_ = r.file.Close()
		}
		// Open or create today's log file.
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
