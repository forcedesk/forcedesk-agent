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
// The RTT group is absent when no packets were received.
var fpingRe = regexp.MustCompile(`xmt/rcv/%loss = \d+/\d+/(\d+)%(?:, min/avg/max = [\d.]+/([\d.]+)/[\d.]+)?`)

// fpingPath returns the absolute path to fping.exe, expected to be in the
// same directory as the running executable.
func fpingPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "fping.exe"
	}
	return filepath.Join(filepath.Dir(exe), "fping.exe")
}

// generatePingMetrics runs fping.exe and returns average RTT (ms) and packet
// loss percentage for the given host. Both are nil on error or if fping cannot
// be found. avg is nil when no packets were received (host unreachable).
func generatePingMetrics(host string) (avg *float64, loss *int) {
	if !isValidHostname(host) {
		slog.Error("monitoring: invalid hostname for ping metrics", "host", host)
		return nil, nil
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
		return nil, nil
	}

	lossVal, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, nil
	}
	loss = &lossVal

	// m[2] is the avg RTT field — only present when packets were received.
	if m[2] != "" {
		avgVal, err := strconv.ParseFloat(m[2], 64)
		if err == nil {
			avg = &avgVal
		}
	}

	return avg, loss
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

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []probeResult
	)

	for _, p := range probes {
		wg.Add(1)
		go func(probe probePayload) {
			defer wg.Done()
			slog.Debug("monitoring: running probe", "probe_id", probe.ProbeID, "host", probe.Host, "check_type", probe.CheckType, "port", probe.Port)

			avg, loss := generatePingMetrics(probe.Host)
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
			results = append(results, probeResult{
				ID:             probe.ProbeID,
				PingData:       avg,
				PacketLossData: loss,
				Status:         status,
			})
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	slog.Info("monitoring: probes completed", "total", len(probes), "results", len(results))

	if len(results) == 0 {
		slog.Info("monitoring: no results to send")
		return
	}

	resp, err := client.PostJSON(tenant.URL("/api/agent/monitoring/response-bulk"), results)
	if err != nil {
		slog.Error("monitoring: failed to send combined results", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Debug("monitoring: POST response", "http_status", resp.StatusCode)
	slog.Info("monitoring: combined results sent", "count", len(results))
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
