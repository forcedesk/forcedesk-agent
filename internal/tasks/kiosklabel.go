// Copyright © 2026 ForcePoint Software. All rights reserved.

package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

// kioskLabelsResponse is the response from /api/agent/remote-labels/pending.
type kioskLabelsResponse struct {
	Success bool         `json:"success"`
	Labels  []kioskLabel `json:"labels"`
}

// kioskLabel holds the data fields for a single label to be printed.
type kioskLabel struct {
	ID          int    `json:"id"`
	QRData      string `json:"qr_data"`
	NameData    string `json:"name_data"`
	BarcodeData string `json:"barcode_data"`
	DateData    string `json:"date_data"`
}

// KioskLabelService fetches pending kiosk labels from the ForceDesk server,
// prints each one via the bundled VBScript and label template, then marks
// them as printed. Runs on a scheduled interval.
func KioskLabelService() {
	slog.Info("kiosk-label: starting")

	client := tenant.New()

	url := tenant.URL("/api/agent/remote-labels/pending")
	slog.Debug("kiosk-label: GET", "url", url)

	var pending kioskLabelsResponse
	if err := client.GetJSON(url, &pending); err != nil {
		slog.Error("kiosk-label: failed to fetch pending labels", "err", err)
		return
	}

	if !pending.Success {
		slog.Warn("kiosk-label: API returned success=false")
		return
	}

	slog.Info("kiosk-label: labels received", "count", len(pending.Labels))

	if len(pending.Labels) == 0 {
		return
	}

	exeDir, err := executableDir()
	if err != nil {
		slog.Error("kiosk-label: failed to resolve exe directory", "err", err)
		return
	}

	// The label template and VBScript are bundled alongside the executable
	// by the Inno Setup installer and must remain in their expected relative locations.
	labelFile := filepath.Join(exeDir, "assets", "template.lbx")
	vbsFile := filepath.Join(exeDir, "print_label.vbs")

	// Print labels sequentially. Brother printers typically queue jobs so
	// parallel printing would cause overlapping output.
	for _, label := range pending.Labels {
		slog.Info("kiosk-label: printing label", "id", label.ID, "name", label.NameData)
		if err := printKioskLabel(vbsFile, labelFile, label); err != nil {
			slog.Error("kiosk-label: print failed", "id", label.ID, "err", err)
			// Continue to the next label rather than aborting the whole batch.
			continue
		}
		slog.Info("kiosk-label: print succeeded", "id", label.ID)
		// Notify the server only after a successful print so that a failed
		// label is retried on the next poll cycle.
		markKioskLabelPrinted(client, label.ID)
	}
}

// executableDir returns the directory containing the running executable,
// used to locate bundled assets (template.lbx, print_label.vbs).
func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// printKioskLabel invokes the bundled VBScript via cscript to print a label
// on the Brother printer using the given template and label data fields.
func printKioskLabel(vbsFile, labelFile string, label kioskLabel) error {
	// The VBScript expects positional arguments in this exact order:
	//   1. label template path (.lbx)
	//   2. QR code data
	//   3. name text
	//   4. barcode data
	//   5. date text
	args := []string{vbsFile, labelFile, label.QRData, label.NameData, label.BarcodeData, label.DateData}
	slog.Debug("kiosk-label: cscript", "args", args)

	// Give the print job a generous 30 s window; slow printers or USB
	// enumeration can add several seconds of latency.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// cscript runs VBScript in a non-interactive (headless) console host,
	// which is required when running as a Windows Service with no desktop.
	cmd := exec.CommandContext(ctx, "cscript", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("kiosk-label: cscript failed", "output", string(out), "err", err)
	} else {
		slog.Debug("kiosk-label: cscript output", "output", string(out))
	}
	return err
}

// markKioskLabelPrinted notifies the tenant that the label has been printed
// so it is not re-queued on the next poll.
func markKioskLabelPrinted(client *tenant.Client, id int) {
	url := tenant.URL(fmt.Sprintf("/api/agent/remote-labels/%d/printed", id))
	slog.Debug("kiosk-label: POST", "url", url)

	resp, err := client.PostJSON(url, nil)
	if err != nil {
		slog.Error("kiosk-label: failed to mark label as printed", "id", id, "err", err)
		return
	}
	defer resp.Body.Close()
	slog.Debug("kiosk-label: marked as printed", "id", id, "http_status", resp.StatusCode)
}
