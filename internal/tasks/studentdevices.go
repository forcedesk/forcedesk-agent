// Copyright © 2026 ForcePoint Software. All rights reserved.

package tasks

import (
	"archive/zip"
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

const maxCertZipSize = 10 * 1024 * 1024

// certUpload is posted to the tenant once the certificate ZIP has been fetched and validated.
type certUpload struct {
	CertificateBinary []byte `json:"certificate_binary"`
	CertificateExpiry string `json:"certificate_expiry,omitempty"`
	RequestUUID       string `json:"request_uuid,omitempty"`
	DeviceType        string `json:"device_type,omitempty"`
}

// RequestStudentDeviceCertificate authenticates with STMC, requests a managed computer
// certificate for the given device, downloads and validates the ZIP, extracts the expiry date
// from the enclosed .cer file, then posts the result to the tenant. Triggered via the command queue.
// requestUUID is passed back in the POST body so the server can correlate integration logs.
func RequestStudentDeviceCertificate(snid, computerName, requestUUID, deviceType string) {
	slog.Info("studentdevices: requesting certificate", "snid", snid, "computer", computerName)

	tc := tenant.New()
	if err := tc.TestConnectivity(); err != nil {
		slog.Error("studentdevices: connectivity check failed", "err", err)
		return
	}

	stmc, cfg, err := initClient(tc)
	if err != nil {
		slog.Error("studentdevices: STMC init failed", "snid", snid, "err", err)
		return
	}

	slog.Info("studentdevices: authenticated with STMC", "mode", stmc.AuthMode, "snid", snid)

	// Request the certificate in STMC (stored as "{schoolcode}-{computerName}").
	slog.Info("studentdevices: submitting certificate request to STMC", "snid", snid, "computer", computerName)
	if err := stmc.AddCertificate(cfg.SchoolCode, computerName, "eduSTAR.NET"); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			slog.Error("studentdevices: AddCertificate failed", "snid", snid, "err", err)
			return
		}
		slog.Info("studentdevices: certificate already exists in STMC, proceeding to download", "snid", snid)
	} else {
		slog.Info("studentdevices: certificate request submitted, waiting for STMC to process", "snid", snid)
		time.Sleep(5 * time.Second)
	}

	// Verify the certificate appears in the school's certificate list.
	certs, err := stmc.GetCertificates(cfg.SchoolCode)
	if err != nil {
		slog.Error("studentdevices: GetCertificates failed", "snid", snid, "err", err)
		return
	}

	expectedName := cfg.SchoolCode + "-" + computerName
	found := false
	names := make([]string, 0, len(certs))
	for _, cert := range certs {
		if n, ok := cert["name"].(string); ok {
			names = append(names, n)
			if n == expectedName {
				found = true
			}
		}
	}
	slog.Info("studentdevices: certificate list retrieved", "snid", snid, "count", len(certs), "names", strings.Join(names, ", "))

	if !found {
		slog.Error("studentdevices: certificate not found in STMC list after creation", "snid", snid, "expected", expectedName)
		return
	}

	slog.Info("studentdevices: certificate verified, downloading", "snid", snid, "compName", expectedName)

	// Download the certificate as a base64-encoded ZIP.
	b64, err := stmc.GetCertificate(cfg.SchoolCode, expectedName, "eduSTAR.NET")
	if err != nil {
		slog.Error("studentdevices: GetCertificate failed", "snid", snid, "err", err)
		return
	}

	slog.Info("studentdevices: certificate data received", "snid", snid, "b64_len", len(b64))

	zipBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		preview := b64
		if len(preview) > 100 {
			preview = preview[:100]
		}
		slog.Error("studentdevices: base64 decode failed", "snid", snid, "err", err, "preview", preview)
		return
	}

	if len(zipBytes) == 0 {
		slog.Error("studentdevices: decoded certificate is empty", "snid", snid)
		return
	}
	if len(zipBytes) > maxCertZipSize {
		slog.Error("studentdevices: certificate ZIP too large", "snid", snid, "size", len(zipBytes))
		return
	}

	slog.Info("studentdevices: certificate decoded successfully", "snid", snid, "zip_size", len(zipBytes))

	// Validate the ZIP structure and parse the enclosed .cer for the expiry date.
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		slog.Error("studentdevices: invalid ZIP archive", "snid", snid, "err", err)
		return
	}

	expiry := certExpiryFromZip(zr, computerName)
	if expiry != "" {
		slog.Info("studentdevices: certificate expiry determined", "snid", snid, "expiry", expiry)
	} else {
		slog.Warn("studentdevices: could not determine certificate expiry", "snid", snid)
	}

	// Post the ZIP and expiry to the tenant for storage.
	payload := certUpload{
		CertificateBinary: zipBytes,
		CertificateExpiry: expiry,
		RequestUUID:       requestUUID,
		DeviceType:        deviceType,
	}
	resp, err := tc.PostJSON(tenant.URL(fmt.Sprintf("/api/agent/student-devices/%s/certificate", snid)), payload)
	if err != nil {
		slog.Error("studentdevices: failed to post certificate to tenant", "snid", snid, "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("studentdevices: certificate stored on tenant", "snid", snid, "status", resp.StatusCode)
}

// certExpiryFromZip scans a ZIP archive for a .cer file whose name contains computerName,
// parses the X.509 certificate (DER or PEM), and returns the NotAfter date as "YYYY-MM-DD".
// Returns "" if no matching file is found or the certificate cannot be parsed.
func certExpiryFromZip(zr *zip.Reader, computerName string) string {
	for _, f := range zr.File {
		// Use only the base name to guard against any path-traversal in the ZIP entry.
		safeName := filepath.Base(f.Name)
		if !strings.Contains(safeName, computerName+".cer") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			slog.Warn("studentdevices: could not open .cer file in ZIP", "file", safeName, "err", err)
			return ""
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			slog.Warn("studentdevices: could not read .cer file", "file", safeName, "err", err)
			return ""
		}

		// Try DER encoding first, then fall back to PEM.
		cert, parseErr := x509.ParseCertificate(data)
		if parseErr != nil {
			if block, _ := pem.Decode(data); block != nil {
				cert, parseErr = x509.ParseCertificate(block.Bytes)
			}
		}
		if parseErr != nil {
			slog.Warn("studentdevices: could not parse .cer certificate", "file", safeName, "err", parseErr)
			return ""
		}

		return cert.NotAfter.Format("2006-01-02")
	}
	return ""
}
