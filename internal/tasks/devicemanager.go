package tasks

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

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

type dmPayloadItem struct {
	PayloadData []devicePayload `json:"payload_data"`
}

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

	url := tenant.URL("/api/agent/devicemanager/payloads")
	slog.Debug("devicemanager: GET", "url", url)

	var items []dmPayloadItem
	if err := client.GetJSON(url, &items); err != nil {
		slog.Error("devicemanager: failed to fetch payloads", "err", err)
		return
	}

	slog.Debug("devicemanager: payload items received", "count", len(items))

	if len(items) == 0 {
		slog.Info("devicemanager: no payloads received")
		return
	}

	batchID := newUUID()
	slog.Debug("devicemanager: batch", "batch_id", batchID)

	var wg sync.WaitGroup
	dispatched := 0

	for _, item := range items {
		for _, dev := range item.PayloadData {
			if dev.DeviceUsername == "" || dev.DevicePassword == "" {
				slog.Debug("devicemanager: skipping device with no credentials", "device", dev.Name)
				continue
			}
			wg.Add(1)
			dispatched++
			go func(d devicePayload) {
				defer wg.Done()
				runDeviceBackup(client, d, batchID)
			}(dev)
		}
	}

	wg.Wait()
	slog.Info("devicemanager: completed", "dispatched", dispatched, "batch", batchID)
}

func runDeviceBackup(client *tenant.Client, dev devicePayload, batchID string) {
	cmd := deviceCommand(dev)
	if cmd == "" {
		slog.Error("devicemanager: unsupported device type", "type", dev.Type, "name", dev.Name)
		return
	}

	slog.Debug("devicemanager: SSH backup", "device", dev.Name, "host", dev.Hostname, "port", dev.Port, "type", dev.Type, "legacy", dev.IsCiscoLegacy, "command", cmd)

	cfg := sshconn.Config{
		Host:     dev.Hostname,
		Port:     dev.Port,
		Username: dev.DeviceUsername,
		Password: dev.DevicePassword,
		Legacy:   bool(dev.IsCiscoLegacy),
	}

	output, err := sshconn.RunCommand(cfg, cmd)
	if err != nil || len(output) < 10 {
		slog.Error("devicemanager: backup failed", "device", dev.Name, "err", err)
		return
	}

	slog.Debug("devicemanager: SSH output received", "device", dev.Name, "bytes", len(output))

	configData := parseDeviceOutput(dev.Type, output)
	slog.Debug("devicemanager: config parsed", "device", dev.Name, "bytes", len(configData))

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

	resp, err := client.PostJSON(tenant.URL("/api/agent/devicemanager/response"), result)
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

func deviceCommand(dev devicePayload) string {
	switch dev.Type {
	case "cisco":
		return "show running-config view full"
	case "mikrotik":
		return "export show-sensitive verbose"
	}
	return ""
}

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
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
