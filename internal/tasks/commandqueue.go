// Copyright © 2026 ForcePoint Software. All rights reserved.

package tasks

import (
	"log/slog"

	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

// commandQueueItem is a single entry returned by /api/agent/command-queues.
type commandQueueItem struct {
	Type        string         `json:"type"`
	PayloadData commandPayload `json:"payload_data"`
}

// commandPayload carries the parameters for a queued command. Only fields
// relevant to the specific command Type are populated by the server.
type commandPayload struct {
	Process          bool    `json:"process"`
	Action           string  `json:"action"`
	SharedAccount    string  `json:"shared_account"`
	RequestedBalance float64 `json:"requested_balance"`
	AdjustmentReason string  `json:"adjustment_reason"`
	Snid             string  `json:"snid"`
	ComputerName     string  `json:"computer_name"`
	RequestUUID      string  `json:"request_uuid"`
	DeviceType       string  `json:"device_type"`
	CertName         string  `json:"cert_name"`
	BatchID          string  `json:"batch_id"`
	BatchTotal       int     `json:"batch_total"`
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

		case "run-edustar":
			if item.PayloadData.Process {
				slog.Info("commandqueue: triggering edustar command", "action", item.PayloadData.Action)
				go EduStarCommand(item.PayloadData.Action)
			}

		case "get-papercut-shared-accounts":
			slog.Info("commandqueue: triggering papercut shared accounts fetch")
			go PapercutGetSharedAccounts()

		case "set-papercut-shared-account-balance":
			slog.Info("commandqueue: setting papercut shared account balance",
				"account", item.PayloadData.SharedAccount,
				"balance", item.PayloadData.RequestedBalance)
			go PapercutSetSharedAccountBalance(
				item.PayloadData.SharedAccount,
				item.PayloadData.RequestedBalance,
				item.PayloadData.AdjustmentReason,
			)

		case "request-student-device-certificate":
			if item.PayloadData.Snid == "" || item.PayloadData.ComputerName == "" {
				slog.Warn("commandqueue: request-student-device-certificate missing snid or computer_name")
			} else {
				slog.Info("commandqueue: requesting student device certificate", "snid", item.PayloadData.Snid)
				go RequestStudentDeviceCertificate(item.PayloadData.Snid, item.PayloadData.ComputerName, item.PayloadData.RequestUUID, item.PayloadData.DeviceType)
			}

		case "request-bulk-certificate":
			if item.PayloadData.CertName == "" || item.PayloadData.BatchID == "" {
				slog.Warn("commandqueue: request-bulk-certificate missing cert_name or batch_id")
			} else {
				slog.Info("commandqueue: requesting bulk certificate", "cert_name", item.PayloadData.CertName, "batch_id", item.PayloadData.BatchID)
				go RequestBulkCertificate(item.PayloadData.CertName, item.PayloadData.BatchID, item.PayloadData.BatchTotal)
			}

		default:
			slog.Warn("commandqueue: unknown command type", "type", item.Type)
		}
	}
}
