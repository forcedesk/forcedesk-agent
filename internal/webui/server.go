// Copyright © 2026 ForcePoint Software. All rights reserved.

package webui

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/scheduler"
	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

//go:embed assets/dashboard.html
var dashboardHTML string

// agentStart captures the process start time for the uptime calculation.
var agentStart = time.Now()

// Start launches the WebUI HTTP server in a background goroutine.
// addr should be a "host:port" string; defaults to "127.0.0.1:8888" if empty.
func Start(addr string, sched *scheduler.Scheduler) {
	if addr == "" {
		addr = "127.0.0.1:8888"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleIndex)
	mux.HandleFunc("GET /api/status", handleStatus(sched))

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	slog.Info("webui: listening", "url", "http://"+addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("webui: server stopped unexpectedly", "err", err)
		}
	}()
}

// handleIndex serves the single-page dashboard HTML.
func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, dashboardHTML)
}

// agentInfo holds static and dynamic agent metadata for the status response.
type agentInfo struct {
	Version   string `json:"version"`
	Uptime    string `json:"uptime"`
	StartedAt string `json:"started_at"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	TenantURL string `json:"tenant_url"`
	DataDir   string `json:"data_dir"`
}

type statusResponse struct {
	Agent agentInfo             `json:"agent"`
	Tasks []scheduler.TaskState `json:"tasks"`
	Logs  []map[string]any      `json:"logs"`
}

// handleStatus returns JSON with agent info, task states, and recent log lines.
func handleStatus(sched *scheduler.Scheduler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := statusResponse{
			Agent: agentInfo{
				Version:   tenant.AgentVersion,
				Uptime:    formatUptime(time.Since(agentStart).Round(time.Second)),
				StartedAt: agentStart.Format(time.RFC3339),
				GoVersion: runtime.Version(),
				OS:        runtime.GOOS,
				Arch:      runtime.GOARCH,
				TenantURL: config.Get().Tenant.URL,
				DataDir:   config.DataDir(),
			},
			Tasks: sched.States(),
			Logs:  readRecentLogs(200),
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(resp)
	}
}

func formatUptime(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// readRecentLogs reads the last n JSON log lines from today's log file,
// falling back to yesterday's file if today's doesn't exist yet.
func readRecentLogs(n int) []map[string]any {
	logsDir := filepath.Join(config.DataDir(), "logs")

	path := filepath.Join(logsDir, "agent-"+time.Now().Format("2006-01-02")+".log")
	f, err := os.Open(path)
	if err != nil {
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		f, err = os.Open(filepath.Join(logsDir, "agent-"+yesterday+".log"))
		if err != nil {
			return nil
		}
	}
	defer f.Close()

	// Collect all non-empty lines then take the last n.
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil {
			entries = append(entries, m)
		}
	}
	return entries
}
