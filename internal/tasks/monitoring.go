package tasks

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/db"
	"github.com/forcedesk/forcedesk-agent/internal/graph"
	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

func isWindows() bool { return runtime.GOOS == "windows" }

type monitoringPayloadItem struct {
	PayloadData []probePayload `json:"payload_data"`
}

type probePayload struct {
	ProbeID   int64  `json:"probeid"`
	Host      string `json:"host"`
	CheckType string `json:"check_type"`
	Port      int    `json:"port"`
}

type probeResult struct {
	ID             int64    `json:"id"`
	PingData       *float64 `json:"ping_data"`
	PacketLossData *int     `json:"packet_loss_data"`
	Status         string   `json:"status"`
}

// fpingRe parses fping's per-host summary line:
// "host : xmt/rcv/%loss = 3/3/0%, min/avg/max = 0.14/0.15/0.16"
// Groups: 1=loss%, 2=min, 3=avg, 4=max. RTT groups absent when no packets received.
var fpingRe = regexp.MustCompile(`xmt/rcv/%loss = \d+/\d+/(\d+)%(?:, min/avg/max = ([\d.]+)/([\d.]+)/([\d.]+))?`)

// fpingPath returns the absolute path to fping.exe, expected to be in the
// same directory as the running executable.
func fpingPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "fping.exe"
	}
	return filepath.Join(filepath.Dir(exe), "fping.exe")
}

// generatePingMetrics runs fping.exe and returns min, avg, max RTT (ms) and
// packet loss percentage for the given host. All are nil on error or if fping
// cannot be found. RTT values are nil when no packets were received (host down).
func generatePingMetrics(host string) (avg, minMS, maxMS *float64, loss *int) {
	if !isValidHostname(host) {
		slog.Error("monitoring: invalid hostname for ping metrics", "host", host)
		return nil, nil, nil, nil
	}

	// fping writes its per-host summary to stderr regardless of exit code.
	// A non-zero exit simply means some/all hosts were unreachable.
	cmd := exec.Command(fpingPath(), "-c", "3", "-q", host)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Run() //nolint:errcheck

	output := stderr.String()
	slog.Debug("monitoring: fping output", "host", host, "output", output)

	m := fpingRe.FindStringSubmatch(output)
	if m == nil {
		slog.Error("monitoring: failed to parse fping output", "host", host, "output", output)
		return nil, nil, nil, nil
	}

	lossVal, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, nil, nil, nil
	}
	loss = &lossVal

	// m[2]=min, m[3]=avg, m[4]=max — only present when packets were received.
	if m[3] != "" {
		if v, err := strconv.ParseFloat(m[3], 64); err == nil {
			avg = &v
		}
		if v, err := strconv.ParseFloat(m[2], 64); err == nil {
			minMS = &v
		}
		if v, err := strconv.ParseFloat(m[4], 64); err == nil {
			maxMS = &v
		}
	}

	return avg, minMS, maxMS, loss
}

// MonitoringService fetches monitoring probe configurations from the ForceDesk
// server and runs all checks concurrently. fping.exe handles ping metrics as a
// separate process per probe, so goroutines have no ICMP socket contention.
// All results are combined and reported back to the tenant in a single payload.
// Runs every minute.
func MonitoringService() {
	slog.Info("monitoring: starting")

	client := tenant.New()
	if err := client.TestConnectivity(); err != nil {
		slog.Error("monitoring: connectivity check failed", "err", err)
		return
	}

	url := tenant.URL("/api/agent/monitoring/getpayloads")
	slog.Debug("monitoring: GET", "url", url)

	var items []monitoringPayloadItem
	if err := client.GetJSON(url, &items); err != nil {
		slog.Error("monitoring: failed to fetch payloads", "err", err)
		return
	}

	slog.Debug("monitoring: payload items received", "count", len(items))

	if len(items) == 0 {
		slog.Info("monitoring: no payloads received")
		return
	}

	var probes []probePayload
	for _, item := range items {
		probes = append(probes, item.PayloadData...)
	}
	slog.Debug("monitoring: total probes", "count", len(probes))

	// probeRecord carries both the server-facing result and the RTT values
	// needed for local graph rendering.
	type probeRecord struct {
		result probeResult
		minMS  *float64
		maxMS  *float64
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		records []probeRecord
	)

	for _, p := range probes {
		wg.Add(1)
		go func(probe probePayload) {
			defer wg.Done()
			slog.Debug("monitoring: running probe", "probe_id", probe.ProbeID, "host", probe.Host, "check_type", probe.CheckType, "port", probe.Port)

			avg, minMS, maxMS, loss := generatePingMetrics(probe.Host)
			slog.Info("monitoring: ping metrics", "probe_id", probe.ProbeID, "ping_data", avg, "packet_loss_data", loss)

			var status string
			if avg == nil {
				status = "down"
			} else {
				switch probe.CheckType {
				case "tcp":
					status = performTCPCheck(probe.Host, probe.Port)
				case "ping":
					status = performPingCheck(probe.Host)
				default:
					slog.Error("monitoring: unknown check type", "probe_id", probe.ProbeID, "check_type", probe.CheckType)
					status = "down"
				}
			}

			slog.Debug("monitoring: probe result", "probe_id", probe.ProbeID, "host", probe.Host, "status", status)

			mu.Lock()
			records = append(records, probeRecord{
				result: probeResult{
					ID:             probe.ProbeID,
					PingData:       avg,
					PacketLossData: loss,
					Status:         status,
				},
				minMS: minMS,
				maxMS: maxMS,
			})
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	slog.Info("monitoring: probes completed", "total", len(probes), "results", len(records))

	if len(records) == 0 {
		slog.Info("monitoring: no results to send")
		return
	}

	// Extract server-facing results.
	results := make([]probeResult, len(records))
	for i, r := range records {
		results[i] = r.result
	}

	resp, err := client.PostJSON(tenant.URL("/api/agent/monitoring/response-bulk"), results)
	if err != nil {
		slog.Error("monitoring: failed to send combined results", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Debug("monitoring: POST response", "http_status", resp.StatusCode)
	slog.Info("monitoring: combined results sent", "count", len(results))

	// Persist measurements and regenerate graphs.
	graphsDir := graph.GraphDir(config.DataDir())
	if err := os.MkdirAll(graphsDir, 0755); err != nil {
		slog.Error("monitoring: failed to create graphs dir", "err", err)
		return
	}

	now := time.Now()
	for _, rec := range records {
		id := rec.result.ID
		loss := 0
		if rec.result.PacketLossData != nil {
			loss = *rec.result.PacketLossData
		}

		if err := db.SaveProbeHistory(id, now, rec.result.PingData, rec.minMS, rec.maxMS, loss); err != nil {
			slog.Error("monitoring: failed to save probe history", "probe_id", id, "err", err)
			continue
		}

		samples, err := db.GetProbeHistory(id, graph.PlotW)
		if err != nil {
			slog.Error("monitoring: failed to fetch probe history", "probe_id", id, "err", err)
			continue
		}

		gSamples := make([]graph.Sample, len(samples))
		for i, s := range samples {
			gSamples[i] = graph.Sample{
				AvgMS:      s.AvgMS,
				MinMS:      s.MinMS,
				MaxMS:      s.MaxMS,
				PacketLoss: s.PacketLoss,
			}
		}

		outPath := graph.ProbePath(config.DataDir(), id)
		if err := graph.Render(gSamples, outPath); err != nil {
			slog.Error("monitoring: failed to render graph", "probe_id", id, "err", err)
		}
	}
}

func performTCPCheck(host string, port int) string {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		slog.Info("monitoring: TCP check down", "host", host, "port", port, "err", err)
		return "down"
	}
	conn.Close()
	return "up"
}

// performPingCheck executes a single ping check using the system ping command.
// Returns "up" if the host responds, "down" otherwise.
func performPingCheck(host string) string {
	if !isValidHostname(host) {
		slog.Error("monitoring: invalid hostname in ping check", "host", host)
		return "down"
	}
	// Use ping.exe on Windows; a non-zero exit code means the host is down.
	cmd := exec.Command("ping", "-n", "1", "-w", "5000", host)
	if err := cmd.Run(); err != nil {
		return "down"
	}
	return "up"
}

// isValidHostname validates that a hostname contains only safe characters.
// Allows alphanumeric characters, dots, hyphens, and IPv6 colons/brackets.
func isValidHostname(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, c := range host {
		if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '.' || c == '-' || c == ':' || c == '[' || c == ']') {
			return false
		}
	}
	return true
}
