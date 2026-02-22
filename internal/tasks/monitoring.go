package tasks

import (
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"runtime"
	"sync"
	"time"

	ping "github.com/go-ping/ping"

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

// MonitoringService fetches monitoring probe configurations from the tenant server
// and executes network checks (TCP connectivity and ICMP ping) concurrently.
// Results are reported back to the tenant. Runs every minute.
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

	var wg sync.WaitGroup
	dispatched := 0

	for _, item := range items {
		slog.Debug("monitoring: dispatching probes", "probe_count", len(item.PayloadData))
		for _, probe := range item.PayloadData {
			wg.Add(1)
			dispatched++
			go func(p probePayload) {
				defer wg.Done()
				runProbe(client, p)
			}(probe)
		}
	}

	wg.Wait()
	slog.Info("monitoring: completed", "dispatched", dispatched)
}

func runProbe(client *tenant.Client, p probePayload) {
	slog.Debug("monitoring: running probe", "probe_id", p.ProbeID, "host", p.Host, "check_type", p.CheckType, "port", p.Port)

	ping, loss := generatePingMetrics(p.Host)
	slog.Info("monitoring: ping metrics", "probe_id", p.ProbeID, "ping_data", ping, "packet_loss_data", loss)

	var status string
	switch p.CheckType {
	case "tcp":
		status = performTCPCheck(p.Host, p.Port)
	case "ping":
		status = performPingCheck(p.Host)
	default:
		slog.Error("monitoring: unknown check type", "probe_id", p.ProbeID, "check_type", p.CheckType)
		return
	}

	slog.Debug("monitoring: probe result", "probe_id", p.ProbeID, "host", p.Host, "status", status)

	result := probeResult{
		ID:             p.ProbeID,
		PingData:       ping,
		PacketLossData: loss,
		Status:         status,
	}

	resp, err := client.PostJSON(tenant.URL("/api/agent/monitoring/response"), result)
	if err != nil {
		slog.Error("monitoring: failed to send probe result", "probe_id", p.ProbeID, "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Debug("monitoring: POST response", "probe_id", p.ProbeID, "http_status", resp.StatusCode)
	slog.Info("monitoring: probe result sent", "probe_id", p.ProbeID, "status", status)
}

func performTCPCheck(host string, port int) string {
	addr := fmt.Sprintf("%s:%d", host, port)
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
	// Validate hostname to prevent command injection.
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
	// Allow alphanumeric, dots, hyphens, colons (IPv6), and brackets (IPv6).
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

// generatePingMetrics sends 5 ICMP echo requests and returns average RTT (ms) and packet loss (%).
// Uses the go-ping library (no external binary or output parsing required).
// Either return value may be nil on error.
func generatePingMetrics(host string) (avg *float64, loss *int) {
	pinger, err := ping.NewPinger(host)
	if err != nil {
		slog.Error("monitoring: failed to create pinger", "host", host, "err", err)
		return nil, nil
	}

	// Windows requires privileged mode for raw ICMP sockets.
	// The service runs as LocalSystem, which has the required privileges.
	pinger.SetPrivileged(isWindows())
	pinger.Count = 5
	pinger.Timeout = 10 * time.Second

	if err := pinger.Run(); err != nil {
		slog.Error("monitoring: ping failed", "host", host, "err", err)
		return nil, nil
	}

	stats := pinger.Statistics()
	slog.Debug("monitoring: ping stats", "host", host,
		"sent", stats.PacketsSent,
		"recv", stats.PacketsRecv,
		"loss_pct", stats.PacketLoss,
		"avg_rtt", stats.AvgRtt,
	)

	avgMS := float64(stats.AvgRtt) / float64(time.Millisecond)
	lossInt := int(stats.PacketLoss)

	if stats.PacketsRecv > 0 {
		avg = &avgMS
	}
	loss = &lossInt

	return avg, loss
}
