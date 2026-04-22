// Copyright © 2026 ForcePoint Software. All rights reserved.

package tasks

import (
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/sshconn"
	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

// allowedCommands is a security allowlist that restricts which SSH commands can be executed on remote devices.
var allowedCommands = map[string]bool{
	"show hardware":                 true,
	"show mac address-table":        true,
	"show log":                      true,
	"show interfaces status":        true,
	"show running-config":           true,
	"show running-config view full": true,
	"log print":                     true,
	"export show-sensitive":         true,
	"interface print brief":         true,
	"interface ethernet switch host print; interface bridge host print": true,
	"system routerboard print": true,
}

// dqResponse is the top-level response from /api/agent/devicemanager/query-payloads.
type dqResponse struct {
	Status   string      `json:"status"`
	Payloads []dqPayload `json:"payloads"`
	Config   dqConfig    `json:"config"`
}

// dqPayload is a single on-demand device query request issued by the tenant.
type dqPayload struct {
	ID          int64         `json:"id"`
	UUID        string        `json:"uuid"`
	PayloadData dqPayloadData `json:"payload_data"`
}

// dqPayloadData contains the SSH connection details and command for a single query.
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

// dqConfig carries per-request SSH configuration overrides supplied by the tenant.
type dqConfig struct {
	LegacySSHOptions string `json:"legacy_ssh_options"`
}

// DeviceManagerQuery polls the tenant for on-demand device query requests, executes
// the requested SSH commands (validated against a strict allowlist), and reports results back.
// Runs in a polling loop for 5 minutes before returning.
func DeviceManagerQuery() {
	slog.Info("devicequery: starting 5-minute polling loop")

	client := tenant.New()

	// Fetch the symmetric key once; it's shared across all requests in this run.
	key, err := config.Get().Tenant.GetEncryptionKey()
	if err != nil {
		slog.Error("devicequery: failed to get encryption key", "err", err)
		return
	}

	// This task is triggered on-demand via the command queue. It polls every
	// 15 s for new query requests and exits after 5 minutes so the scheduler
	// goroutine does not run indefinitely if the tenant never clears the queue.
	const pollInterval = 15 * time.Second
	const maxRuntime = 5 * time.Minute
	deadline := time.Now().Add(maxRuntime)

	url := tenant.URL("/api/agent/devicemanager/query-payloads")

	for time.Now().Before(deadline) {
		slog.Debug("devicequery: GET", "url", url)

		var result dqResponse
		if err := client.GetEncryptedJSON(url, &result, key); err != nil {
			slog.Error("devicequery: failed to fetch payloads", "err", err)
			// Back off and retry; a transient network error should not
			// abort the entire polling session.
			time.Sleep(pollInterval)
			continue
		}

		slog.Debug("devicequery: poll response", "status", result.Status, "count", len(result.Payloads))

		if result.Status != "success" || len(result.Payloads) == 0 {
			// No work to do this tick; wait and poll again.
			slog.Info("devicequery: no pending payloads")
			time.Sleep(pollInterval)
			continue
		}

		slog.Info("devicequery: dispatching payloads", "count", len(result.Payloads))

		// Run each query concurrently — SSH I/O is the bottleneck, so
		// parallelism keeps total response latency low.
		var wg sync.WaitGroup
		for _, p := range result.Payloads {
			wg.Add(1)
			go func(payload dqPayload, legacyOpts string) {
				defer wg.Done()
				processDeviceQuery(client, payload, legacyOpts, key)
			}(p, result.Config.LegacySSHOptions)
		}
		wg.Wait()

		// Even after successfully processing a batch, sleep before the next
		// poll to give the server time to clear the completed entries.
		time.Sleep(pollInterval)
	}

	slog.Info("devicequery: max runtime reached, exiting")
}

// processDeviceQuery validates the command against the allowlist, executes it over SSH,
// and posts the result (or an error) back to the tenant.
func processDeviceQuery(client *tenant.Client, p dqPayload, legacySSHOpts string, key []byte) {
	data := p.PayloadData

	// Reject commands not in the static allowlist before making any network
	// connection. This prevents the tenant (or anyone who compromises it) from
	// running arbitrary commands on managed devices via the agent.
	if !allowedCommands[data.Command] {
		slog.Error("devicequery: command not in allowlist", "id", p.ID, "command", data.Command)
		postQueryError(client, p.ID, "requested command is not permitted", key)
		return
	}

	// Validate that all required SSH connection fields are present; a missing
	// field would cause a confusing SSH error rather than a clear failure.
	if data.DeviceHostname == "" || data.Username == "" || data.Password == "" || data.Command == "" {
		postQueryError(client, p.ID, "missing required payload fields", key)
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

	// Execute the command over SSH; RunCommand enforces its own timeout.
	output, err := sshconn.RunCommand(cfg, data.Command)
	if err != nil {
		slog.Error("devicequery: SSH command failed", "id", p.ID, "err", err)
		postQueryError(client, p.ID, err.Error(), key)
		return
	}

	slog.Debug("devicequery: SSH output received", "id", p.ID, "bytes", len(output))

	// Return the raw output plus metadata. Both "output" and "data" carry the
	// same text; the duplication is intentional for compatibility with older
	// server response handlers that expect one or the other field name.
	postQueryResult(client, p.ID, map[string]any{
		"status":          "success",
		"output":          output,
		"data":            output,
		"device_hostname": data.DeviceHostname,
		"action":          data.Action,
		"output_size":     len(output),
	}, key)
}

// postQueryResult encrypts and POSTs a query response to /api/agent/devicemanager/query-response.
func postQueryResult(client *tenant.Client, payloadID int64, responseData map[string]any, key []byte) {
	body := map[string]any{
		"payload_id":    payloadID,
		"response_data": responseData,
	}
	resp, err := client.PostEncryptedJSON(tenant.URL("/api/agent/devicemanager/query-response"), body, key)
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

// postQueryError posts an error result for the given payload ID back to the tenant.
func postQueryError(client *tenant.Client, payloadID int64, message string, key []byte) {
	postQueryResult(client, payloadID, map[string]any{
		"status": "error",
		"error":  message,
	}, key)
}
