package tasks

import (
	"log/slog"

	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

type commandQueueItem struct {
	Type        string         `json:"type"`
	PayloadData commandPayload `json:"payload_data"`
}

type commandPayload struct {
	Process bool `json:"process"`
}

// CommandQueueService polls the ForceDesk server for pending commands and executes them.
// Supports commands like forcing a Papercut sync or triggering device manager queries.
// Runs every minute.
func CommandQueueService() {
	slog.Info("commandqueue: starting")

	client := tenant.New()
	if err := client.TestConnectivity(); err != nil {
		slog.Error("commandqueue: connectivity check failed", "err", err)
		return
	}

	url := tenant.URL("/api/agent/command-queues")
	slog.Debug("commandqueue: GET", "url", url)

	var items []commandQueueItem
	if err := client.GetJSON(url, &items); err != nil {
		slog.Error("commandqueue: failed to fetch queue", "err", err)
		return
	}

	slog.Debug("commandqueue: items received", "count", len(items))

	for _, item := range items {
		slog.Debug("commandqueue: processing item", "type", item.Type, "process", item.PayloadData.Process)
		switch item.Type {
		case "force-sync-papercutsvc":
			if item.PayloadData.Process {
				slog.Info("commandqueue: triggering papercut sync")
				PapercutService()
			}

		case "force-devicemanager-query":
			slog.Info("commandqueue: triggering device manager query loop")
			go DeviceManagerQuery()

		default:
			slog.Warn("commandqueue: unknown command type", "type", item.Type)
		}
	}
}
