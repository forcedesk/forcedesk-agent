package tasks

import (
	"log/slog"

	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

type heartbeatResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Heartbeat confirms the agent can reach the tenant and that the tenant
// acknowledges this agent. Runs every 5 minutes.
func Heartbeat() {
	slog.Info("heartbeat: starting")

	client := tenant.New()

	url := tenant.URL("/api/agent/heartbeat")
	slog.Debug("heartbeat: GET", "url", url)

	var resp heartbeatResponse
	if err := client.GetJSON(url, &resp); err != nil {
		slog.Error("heartbeat: request failed", "err", err)
		return
	}

	slog.Debug("heartbeat: response", "status", resp.Status, "message", resp.Message)

	if resp.Status == "ok" {
		slog.Info("heartbeat: ok", "message", resp.Message)
	} else {
		slog.Error("heartbeat: tenant returned failure", "message", resp.Message)
	}
}
