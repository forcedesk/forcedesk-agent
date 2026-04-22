// Copyright © 2026 ForcePoint Software. All rights reserved.

package tasks

import (
	"bytes"
	"context"
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

// fpingRun executes fping against host, sending count pings with intervalMS
// milliseconds between each. Pass intervalMS=0 to use fping's default (1 s).
// Returns avg, min, max RTT (ms) and packet loss. RTT pointers are nil when no
// packets were received. All return values are nil on parse error.
func fpingRun(host string, count, intervalMS int) (avg, minMS, maxMS *float64, loss *int) {
	// Validate the hostname before passing it to an external process to
	// prevent command injection through crafted hostnames.
	if !isValidHostname(host) {
		slog.Error("monitoring: invalid hostname for ping metrics", "host", host)
		return nil, nil, nil, nil
	}

	// -c <count>: send exactly count pings per host.
	// -p <ms>:    pause between pings (omitted when intervalMS=0, letting fping use its default of 1000 ms).
	// -q:         quiet mode — suppress per-ping output; only print the summary line to stderr.
	args := []string{"-c", strconv.Itoa(count)}
	if intervalMS > 0 {
		args = append(args, "-p", strconv.Itoa(intervalMS))
	}
	args = append(args, "-q", host)

	// fping writes its per-host summary to stderr regardless of exit code;
	// a non-zero exit code simply means some or all hosts were unreachable.
	// The context deadline is set to the expected total ping time (count × interval)
	// plus a 5 s buffer so a slow host does not stall the goroutine indefinitely.
	pingDuration := time.Duration(count) * time.Duration(max(intervalMS, 1000)) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), pingDuration+5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, fpingPath(), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Run() //nolint:errcheck // non-zero exit is normal for unreachable hosts

	output := stderr.String()
	slog.Debug("monitoring: fping output", "host", host, "output", output)

	// Parse the summary line; nil return on no match means the probe result
	// will be treated as completely unreachable.
	m := fpingRe.FindStringSubmatch(output)
	if m == nil {
		slog.Error("monitoring: failed to parse fping output", "host", host, "output", output)
		return nil, nil, nil, nil
	}

	// Group 1 is always present: packet loss percentage (0–100).
	lossVal, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, nil, nil, nil
	}
	loss = &lossVal

	// Groups 2–4 (min/avg/max RTT) are only present when at least one packet
	// was received. When all packets are lost the RTT fields are omitted from
	// fping's output, so we leave the pointer fields nil to signal "no data".
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

// generatePingMetrics runs a fast 3-ping check used for server status reporting.
// Returns avg RTT and packet loss only; min/max are discarded.
func generatePingMetrics(host string) (avg *float64, loss *int) {
	a, _, _, l := fpingRun(host, 3, 0)
	return a, l
}

// generateGraphMetrics runs a 20-ping check at 100 ms intervals (~2 s total)
// to capture enough jitter for a meaningful smokeping smoke band.
func generateGraphMetrics(host string) (avg, minMS, maxMS *float64, loss *int) {
	return fpingRun(host, 20, 100)
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

	// Flatten the nested payload structure into a single probe list.
	var probes []probePayload
	for _, item := range items {
		probes = append(probes, item.PayloadData...)
	}
	slog.Debug("monitoring: total probes", "count", len(probes))

	// probeRecord carries two independent measurements for each probe:
	//   result    — the 3-ping "fast" check reported to the server for up/down status.
	//   graph*    — the 20-ping "thorough" check used only for rrdtool graphs.
	// They are intentionally separate: the server status endpoint only needs a
	// quick ping result, while the graph needs enough samples for a meaningful
	// jitter band.
	type probeRecord struct {
		result     probeResult
		graphAvg   *float64
		graphMinMS *float64
		graphMaxMS *float64
		graphLoss  int
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		records []probeRecord
	)

	// Run all probes concurrently. Each probe spawns its own fping process,
	// so there is no shared ICMP socket to contend over.
	for _, p := range probes {
		wg.Add(1)
		go func(probe probePayload) {
			defer wg.Done()
			slog.Debug("monitoring: running probe", "probe_id", probe.ProbeID, "host", probe.Host, "check_type", probe.CheckType, "port", probe.Port)

			// Fast check: 3 pings, result sent to the server for status reporting.
			avg, loss := generatePingMetrics(probe.Host)
			slog.Info("monitoring: ping metrics", "probe_id", probe.ProbeID, "ping_data", avg, "packet_loss_data", loss)

			// Determine up/down status. If avg is nil the host returned no
			// packets at all; skip the protocol-specific check and mark it down.
			var status string
			if avg == nil {
				status = "down"
			} else {
				switch probe.CheckType {
				case "tcp":
					// TCP check verifies that a specific service port is open,
					// not just that the host responds to ICMP.
					status = performTCPCheck(probe.Host, probe.Port)
				case "ping":
					status = performPingCheck(probe.Host)
				default:
					slog.Error("monitoring: unknown check type", "probe_id", probe.ProbeID, "check_type", probe.CheckType)
					status = "down"
				}
			}

			// Thorough check: 20 pings at 100 ms intervals (~2 s total).
			// More samples produce a wider, more accurate jitter band in the graph.
			gAvg, gMin, gMax, gLoss := generateGraphMetrics(probe.Host)
			graphLoss := 0
			if gLoss != nil {
				graphLoss = *gLoss
			}

			slog.Debug("monitoring: probe result", "probe_id", probe.ProbeID, "host", probe.Host, "status", status)

			// Append under a mutex because multiple probe goroutines may finish simultaneously.
			mu.Lock()
			records = append(records, probeRecord{
				result: probeResult{
					ID:             probe.ProbeID,
					PingData:       avg,
					PacketLossData: loss,
					Status:         status,
				},
				graphAvg:   gAvg,
				graphMinMS: gMin,
				graphMaxMS: gMax,
				graphLoss:  graphLoss,
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

	// Extract server-facing results from the combined records and POST them
	// in a single bulk request to reduce HTTP overhead.
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

	// RRD pipeline: for each probe, create the RRD if it doesn't exist,
	// feed in the latest measurement, render a PNG, and upload it to the tenant.
	// Failures are per-probe and do not abort the remaining graphs.
	graphsDir := graph.GraphDir(config.DataDir())
	if err := os.MkdirAll(graphsDir, 0755); err != nil {
		slog.Error("monitoring: failed to create graphs dir", "err", err)
		return
	}

	now := time.Now()
	for _, rec := range records {
		id := rec.result.ID

		// Create the RRD database file on first use; idempotent on subsequent runs.
		if err := graph.EnsureRRD(config.DataDir(), id); err != nil {
			slog.Error("monitoring: failed to ensure RRD", "probe_id", id, "err", err)
			continue
		}

		// Feed this minute's measurement into the round-robin database.
		if err := graph.Update(config.DataDir(), id, now, rec.graphAvg, rec.graphMinMS, rec.graphMaxMS, rec.graphLoss); err != nil {
			slog.Error("monitoring: failed to update RRD", "probe_id", id, "err", err)
			continue
		}

		// Re-render the 24-hour PNG from the updated RRD data.
		outPath := graph.ProbePath(config.DataDir(), id)
		if err := graph.Render(config.DataDir(), id, outPath); err != nil {
			slog.Error("monitoring: failed to render graph", "probe_id", id, "err", err)
			continue
		}

		// Read the rendered PNG and upload it to the tenant for display in the dashboard.
		pngData, err := os.ReadFile(outPath)
		if err != nil {
			slog.Error("monitoring: failed to read rendered graph", "probe_id", id, "err", err)
			continue
		}

		uploadURL := tenant.URL(fmt.Sprintf("/api/agent/monitoring/graph/%d", id))
		uploadResp, err := client.PostFile(uploadURL, fmt.Sprintf("probe_%d.png", id), pngData)
		if err != nil {
			slog.Error("monitoring: failed to upload graph", "probe_id", id, "err", err)
			continue
		}
		uploadResp.Body.Close()
		slog.Debug("monitoring: graph uploaded", "probe_id", id, "http_status", uploadResp.StatusCode)
	}
}

// performTCPCheck attempts a TCP connection to host:port and returns "up" on success, "down" on failure.
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
