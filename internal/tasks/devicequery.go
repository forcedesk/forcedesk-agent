package tasks

import (
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/sshconn"
	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

// allowedCommands is the security whitelist of commands the tenant may request.
var allowedCommands = map[string]bool{
	"show hardware":                                            true,
	"show mac address-table":                                   true,
	"show log":                                                 true,
	"show interfaces status":                                   true,
	"show running-config":                                      true,
	"show running-config view full":                            true,
	"log print":                                                true,
	"export show-sensitive":                                    true,
	"interface print brief":                                    true,
	"interface ethernet switch host print; interface bridge host print": true,
	"system routerboard print":                                 true,
}

type dqResponse struct {
	Status   string       `json:"status"`
	Payloads []dqPayload  `json:"payloads"`
	Config   dqConfig     `json:"config"`
}

type dqPayload struct {
	ID          int64         `json:"id"`
	UUID        string        `json:"uuid"`
	PayloadData dqPayloadData `json:"payload_data"`
}

type dqPayloadData struct {
	DeviceID       int64  `json:"device_id"`
	DeviceHostname string `json:"device_hostname"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	Port           int    `json:"port"`
	Command        string `json:"command"`
	DeviceType     string `json:"device_type"`
	IsCiscoLegacy  int    `json:"is_cisco_legacy"`
	Action         string `json:"action"`
	RequestUUID    string `json:"request_uuid"`
}

type dqConfig struct {
	LegacySSHOptions string `json:"legacy_ssh_options"`
}

// DeviceManagerQuery polls for on-demand device query payloads, executes the
// requested SSH command (against a strict allowlist), and posts the result
// back to the tenant. The loop runs for 5 minutes before returning.
func DeviceManagerQuery() {
	slog.Info("devicequery: starting 5-minute polling loop")

	client := tenant.New()
	const pollInterval = 15 * time.Second
	const maxRuntime = 5 * time.Minute
	deadline := time.Now().Add(maxRuntime)

	url := tenant.URL("/api/agent/devicemanager/query-payloads")

	for time.Now().Before(deadline) {
		slog.Debug("devicequery: GET", "url", url)

		var result dqResponse
		if err := client.GetJSON(url, &result); err != nil {
			slog.Error("devicequery: failed to fetch payloads", "err", err)
			time.Sleep(pollInterval)
			continue
		}

		slog.Debug("devicequery: poll response", "status", result.Status, "count", len(result.Payloads))

		if result.Status != "success" || len(result.Payloads) == 0 {
			slog.Info("devicequery: no pending payloads")
			time.Sleep(pollInterval)
			continue
		}

		slog.Info("devicequery: dispatching payloads", "count", len(result.Payloads))

		var wg sync.WaitGroup
		for _, p := range result.Payloads {
			wg.Add(1)
			go func(payload dqPayload, legacyOpts string) {
				defer wg.Done()
				processDeviceQuery(client, payload, legacyOpts)
			}(p, result.Config.LegacySSHOptions)
		}
		wg.Wait()

		time.Sleep(pollInterval)
	}

	slog.Info("devicequery: max runtime reached, exiting")
}

func processDeviceQuery(client *tenant.Client, p dqPayload, legacySSHOpts string) {
	data := p.PayloadData

	if !allowedCommands[data.Command] {
		slog.Error("devicequery: command not in allowlist", "id", p.ID, "command", data.Command)
		postQueryError(client, p.ID, "requested command is not permitted")
		return
	}

	if data.DeviceHostname == "" || data.Username == "" || data.Password == "" || data.Command == "" {
		postQueryError(client, p.ID, "missing required payload fields")
		return
	}

	slog.Debug("devicequery: SSH command",
		"id", p.ID,
		"host", data.DeviceHostname,
		"port", data.Port,
		"command", data.Command,
		"legacy", data.IsCiscoLegacy != 0,
	)

	cfg := sshconn.Config{
		Host:     data.DeviceHostname,
		Port:     data.Port,
		Username: data.Username,
		Password: data.Password,
		Legacy:   data.IsCiscoLegacy != 0,
	}

	output, err := sshconn.RunCommand(cfg, data.Command)
	if err != nil {
		slog.Error("devicequery: SSH command failed", "id", p.ID, "err", err)
		postQueryError(client, p.ID, err.Error())
		return
	}

	slog.Debug("devicequery: SSH output received", "id", p.ID, "bytes", len(output))

	postQueryResult(client, p.ID, map[string]any{
		"status":          "success",
		"output":          output,
		"data":            output,
		"device_hostname": data.DeviceHostname,
		"action":          data.Action,
		"output_size":     len(output),
	})
}

func postQueryResult(client *tenant.Client, payloadID int64, responseData map[string]any) {
	body := map[string]any{
		"payload_id":    payloadID,
		"response_data": responseData,
	}
	resp, err := client.PostJSON(tenant.URL("/api/agent/devicemanager/query-response"), body)
	if err != nil {
		slog.Error("devicequery: failed to post result", "id", payloadID, "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("devicequery: non-success response", "id", payloadID, "status", resp.StatusCode, "body", string(body))
	}
}

func postQueryError(client *tenant.Client, payloadID int64, message string) {
	postQueryResult(client, payloadID, map[string]any{
		"status": "error",
		"error":  message,
	})
}
