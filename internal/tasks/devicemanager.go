// Copyright © 2026 ForcePoint Software. All rights reserved.

package tasks

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/sshconn"
	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

// flexBool is a custom type that unmarshals JSON booleans (true/false) and integers (0/1) as boolean values.
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "true", "1":
		*f = true
	default:
		*f = false
	}
	return nil
}

// devicePayload describes a single network device whose configuration should be backed up.
type devicePayload struct {
	ID             json.Number `json:"id"`
	Type           string      `json:"type"`
	Hostname       string      `json:"hostname"`
	Port           int         `json:"port"`
	DeviceUsername string      `json:"device_username"`
	DevicePassword string      `json:"device_password"`
	IsCiscoLegacy  flexBool    `json:"is_cisco_legacy"`
	Name           string      `json:"name"`
	IsForced       flexBool    `json:"is_forced"`
}

// dmBackupResult is the JSON payload posted to /api/agent/devicemanager/response
// after a backup attempt, whether successful or not.
type dmBackupResult struct {
	DeviceID json.Number `json:"device_id"`
	Data     string      `json:"data"`
	UUID     string      `json:"uuid"`
	Batch    string      `json:"batch"`
	IsForced bool        `json:"is_forced"`
	Size     int         `json:"size"`
	Status   string      `json:"status"`
	Log      string      `json:"log"`
}

// DeviceManagerService fetches network device backup configurations from the tenant,
// connects to each device via SSH to capture running configurations, and reports results back.
// Runs every minute.
func DeviceManagerService() {
	slog.Info("devicemanager: starting")

	client := tenant.New()
	if err := client.TestConnectivity(); err != nil {
		slog.Error("devicemanager: connectivity check failed", "err", err)
		return
	}

	// The device payload is encrypted end-to-end; fetch the symmetric key
	// before requesting the payload so we can decrypt it inline.
	key, err := config.Get().Tenant.GetEncryptionKey()
	if err != nil {
		slog.Error("devicemanager: failed to get encryption key", "err", err)
		return
	}

	url := tenant.URL("/api/agent/devicemanager/payloads")
	slog.Debug("devicemanager: GET", "url", url)

	// The response wraps device lists inside a "payloads" array to allow the
	// server to group devices into separate backup jobs if needed.
	var item struct {
		Payloads []struct {
			PayloadData []devicePayload `json:"payload_data"`
		} `json:"payloads"`
	}
	if err := client.GetEncryptedJSON(url, &item, key); err != nil {
		slog.Error("devicemanager: failed to fetch payloads", "err", err)
		return
	}

	slog.Debug("devicemanager: payload items received", "count", len(item.Payloads))

	if len(item.Payloads) == 0 {
		slog.Info("devicemanager: no payloads received")
		return
	}

	// A single batch ID groups all backups captured in this run together on
	// the server side, making it easy to correlate results in the dashboard.
	batchID := newUUID()
	slog.Debug("devicemanager: batch", "batch_id", batchID)

	var wg sync.WaitGroup
	dispatched := 0

	// Dispatch each device backup concurrently. SSH is I/O-bound so running
	// devices in parallel significantly reduces the total wall-clock time.
	for _, p := range item.Payloads {
		for _, dev := range p.PayloadData {
			if dev.DeviceUsername == "" || dev.DevicePassword == "" {
				slog.Warn("devicemanager: skipping device with no credentials", "device", dev.Name, "type", dev.Type)
				continue
			}
			wg.Add(1)
			dispatched++
			slog.Info("devicemanager: dispatching backup", "device", dev.Name, "type", dev.Type, "host", dev.Hostname)
			go func(d devicePayload) {
				defer wg.Done()
				runDeviceBackup(client, d, batchID, key)
			}(dev)
		}
	}

	// Wait for all goroutines to finish before returning so the scheduler
	// does not start another run while backups are still in progress.
	wg.Wait()
	slog.Info("devicemanager: completed", "dispatched", dispatched, "total", len(item.Payloads), "batch", batchID)
}

// runDeviceBackup SSHes into a single device, captures its running config, and uploads
// the result to the tenant. Called concurrently for each device in a batch.
func runDeviceBackup(client *tenant.Client, dev devicePayload, batchID string, key []byte) {
	// Resolve the CLI command for this device type before opening the SSH
	// connection; bail early if the type is unrecognised.
	cmd := deviceCommand(dev)
	if cmd == "" {
		slog.Error("devicemanager: unsupported device type", "type", dev.Type, "name", dev.Name)
		return
	}

	slog.Info("devicemanager: SSH backup starting", "device", dev.Name, "host", dev.Hostname, "port", dev.Port, "type", dev.Type, "legacy", dev.IsCiscoLegacy, "command", cmd)

	cfg := sshconn.Config{
		Host:     dev.Hostname,
		Port:     dev.Port,
		Username: dev.DeviceUsername,
		Password: dev.DevicePassword,
		// IsCiscoLegacy enables older SSH key-exchange algorithms required
		// by hardware that pre-dates curve25519.
		Legacy: bool(dev.IsCiscoLegacy),
	}

	output, err := sshconn.RunCommand(cfg, cmd)
	// Require at least 10 bytes to guard against devices that accept the
	// connection but return empty or truncated output (e.g. auth failures
	// that yield a short error banner instead of a config dump).
	if err != nil || len(output) < 10 {
		slog.Error("devicemanager: backup failed", "device", dev.Name, "err", err)
		return
	}

	slog.Debug("devicemanager: SSH output received", "device", dev.Name, "bytes", len(output))

	// Strip device-specific preamble (login banners, command echo) so only
	// the configuration text is stored and compared on the server.
	configData := parseDeviceOutput(dev.Type, output)
	slog.Debug("devicemanager: config parsed", "device", dev.Name, "bytes", len(configData))

	// Assemble the result payload. A per-result UUID allows the server to
	// deduplicate in case of retry, while batchID links all results from
	// this scheduler run together.
	result := dmBackupResult{
		DeviceID: dev.ID,
		Data:     configData,
		UUID:     newUUID(),
		Batch:    batchID,
		IsForced: bool(dev.IsForced),
		Size:     len(configData),
		Status:   "success",
		Log:      fmt.Sprintf("Backup for Device: %s was successful.", dev.Name),
	}

	// Encrypt the result before transmission so credentials embedded in
	// device configs are not exposed in transit.
	resp, err := client.PostEncryptedJSON(tenant.URL("/api/agent/devicemanager/response"), result, key)
	if err != nil {
		slog.Error("devicemanager: failed to send backup", "device", dev.Name, "err", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		slog.Error("devicemanager: non-success response", "device", dev.Name, "status", resp.StatusCode)
		return
	}

	slog.Info("devicemanager: backup sent", "device", dev.Name, "size", result.Size)
}

// deviceCommand returns the SSH command used to export a device's running configuration.
// Returns an empty string for unsupported device types.
func deviceCommand(dev devicePayload) string {
	switch dev.Type {
	case "cisco":
		return "show running-config view full"
	case "mikrotik":
		return "export show-sensitive verbose"
	}
	return ""
}

// parseDeviceOutput strips device-specific preamble from SSH output, returning
// the relevant configuration text starting at a known anchor line.
func parseDeviceOutput(deviceType, output string) string {
	switch deviceType {
	case "cisco":
		// Keep everything from the line containing "version" onwards.
		if idx := strings.Index(output, "version"); idx >= 0 {
			return output[idx:]
		}
	case "mikrotik":
		// Keep everything from the first "/interface" onwards.
		if idx := strings.Index(output, "/interface"); idx >= 0 {
			return output[idx:]
		}
	}
	return output
}

// newUUID generates a random UUID v4.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// RFC 4122 §4.4: set version bits (byte 6, top nibble) to 0100 (version 4).
	b[6] = (b[6] & 0x0f) | 0x40
	// RFC 4122 §4.4: set variant bits (byte 8, top two bits) to 10 (RFC 4122 variant).
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
