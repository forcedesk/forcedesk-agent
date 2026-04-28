// Copyright © 2026 ForcePoint Software. All rights reserved.

package tasks

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"log/slog"
	"strings"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

// bulkCertUpload is posted to the tenant after a pool certificate has been fetched from STMC.
type bulkCertUpload struct {
	CertificateBinary []byte `json:"certificate_binary"`
	CertificateExpiry string `json:"certificate_expiry,omitempty"`
	CertName          string `json:"cert_name"`
	BatchID           string `json:"batch_id"`
	BatchTotal        int    `json:"batch_total"`
}

// RequestBulkCertificate authenticates with STMC, requests a pool certificate for certName,
// downloads and validates the ZIP, extracts the expiry date, then posts the result to the tenant.
// certName is the bare name (e.g. "FD-abc123"); the school prefix is prepended internally.
// batchTotal is forwarded so the ingest endpoint can detect when all certificates in the batch
// have arrived and write the completion log.
func RequestBulkCertificate(certName, batchID string, batchTotal int) {
	slog.Info("bulkcertificates: requesting certificate", "cert_name", certName, "batch_id", batchID)

	tc := tenant.New()
	if err := tc.TestConnectivity(); err != nil {
		slog.Error("bulkcertificates: connectivity check failed", "err", err)
		return
	}

	stmc, cfg, err := initClient(tc)
	if err != nil {
		slog.Error("bulkcertificates: STMC init failed", "cert_name", certName, "err", err)
		return
	}

	slog.Info("bulkcertificates: authenticated with STMC", "mode", stmc.AuthMode, "cert_name", certName)

	if err := stmc.AddCertificate(cfg.SchoolCode, certName, "eduSTAR.NET"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			slog.Error("bulkcertificates: AddCertificate failed", "cert_name", certName, "err", err)
			return
		}
		slog.Info("bulkcertificates: certificate already exists in STMC, proceeding to download", "cert_name", certName)
	} else {
		slog.Info("bulkcertificates: certificate request submitted, waiting for STMC to process", "cert_name", certName)
		time.Sleep(5 * time.Second)
	}

	compName := cfg.SchoolCode + "-" + certName

	// Verify the certificate appears in the school's list.
	certs, err := stmc.GetCertificates(cfg.SchoolCode)
	if err != nil {
		slog.Error("bulkcertificates: GetCertificates failed", "cert_name", certName, "err", err)
		return
	}

	found := false
	for _, cert := range certs {
		if n, ok := cert["name"].(string); ok && n == compName {
			found = true
			break
		}
	}

	if !found {
		slog.Error("bulkcertificates: certificate not found in STMC list after creation", "cert_name", certName, "expected", compName)
		return
	}

	slog.Info("bulkcertificates: certificate verified, downloading", "comp_name", compName)

	b64, err := stmc.GetCertificate(cfg.SchoolCode, compName, "eduSTAR.NET")
	if err != nil {
		slog.Error("bulkcertificates: GetCertificate failed", "cert_name", certName, "err", err)
		return
	}

	zipBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		preview := b64
		if len(preview) > 100 {
			preview = preview[:100]
		}
		slog.Error("bulkcertificates: base64 decode failed", "cert_name", certName, "err", err, "preview", preview)
		return
	}

	if len(zipBytes) == 0 {
		slog.Error("bulkcertificates: decoded certificate is empty", "cert_name", certName)
		return
	}
	if len(zipBytes) > maxCertZipSize {
		slog.Error("bulkcertificates: certificate ZIP too large", "cert_name", certName, "size", len(zipBytes))
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		slog.Error("bulkcertificates: invalid ZIP archive", "cert_name", certName, "err", err)
		return
	}

	expiry := certExpiryFromZip(zr, certName)
	if expiry != "" {
		slog.Info("bulkcertificates: certificate expiry determined", "cert_name", certName, "expiry", expiry)
	} else {
		slog.Warn("bulkcertificates: could not determine certificate expiry", "cert_name", certName)
	}

	payload := bulkCertUpload{
		CertificateBinary: zipBytes,
		CertificateExpiry: expiry,
		CertName:          compName,
		BatchID:           batchID,
		BatchTotal:        batchTotal,
	}
	resp, err := tc.PostJSON(tenant.URL("/api/agent/bulk-certificates/certificate"), payload)
	if err != nil {
		slog.Error("bulkcertificates: failed to post certificate to tenant", "cert_name", certName, "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("bulkcertificates: certificate stored on tenant", "cert_name", certName, "status", resp.StatusCode)
}
