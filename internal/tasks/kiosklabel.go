package tasks

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

type kioskLabelsResponse struct {
	Success bool         `json:"success"`
	Labels  []kioskLabel `json:"labels"`
}

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

	labelFile := filepath.Join(exeDir, "assets", "template.lbx")
	vbsFile := filepath.Join(exeDir, "print_label.vbs")

	for _, label := range pending.Labels {
		slog.Info("kiosk-label: printing label", "id", label.ID, "name", label.NameData)
		if err := printKioskLabel(vbsFile, labelFile, label); err != nil {
			slog.Error("kiosk-label: print failed", "id", label.ID, "err", err)
			continue
		}
		slog.Info("kiosk-label: print succeeded", "id", label.ID)
		markKioskLabelPrinted(client, label.ID)
	}
}

func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

func printKioskLabel(vbsFile, labelFile string, label kioskLabel) error {
	args := []string{vbsFile, labelFile, label.QRData, label.NameData, label.BarcodeData, label.DateData}
	slog.Debug("kiosk-label: cscript", "args", args)

	cmd := exec.Command("cscript", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("kiosk-label: cscript failed", "output", string(out), "err", err)
	} else {
		slog.Debug("kiosk-label: cscript output", "output", string(out))
	}
	return err
}

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
