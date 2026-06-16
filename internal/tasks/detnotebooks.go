// Copyright © 2026 ForcePoint Software. All rights reserved.

package tasks

import (
	"log/slog"

	"github.com/forcedesk/forcedesk-agent/internal/notebook"
	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

// SyncDETNotebooks fetches the current notebook fleet from the DET Notebooks API
// (apps.edustar.vic.edu.au/notebooks) and posts it to the tenant for storage.
// Credentials are shared with the STMC integration (same eduStarConfig).
func SyncDETNotebooks() {
	slog.Info("detnotebooks: starting fleet sync")

	tc := tenant.New()
	if err := tc.TestConnectivity(); err != nil {
		slog.Info("detnotebooks: connectivity check failed", "err", err)
		return
	}

	cfg, err := resolveConfig(tc)
	if err != nil {
		slog.Info("detnotebooks: failed to resolve config", "err", err)
		return
	}

	if cfg.Username == "" || cfg.Password == "" || cfg.SchoolCode == "" {
		slog.Info("detnotebooks: config is incomplete (missing username, password, or school_code)")
		return
	}

	nb := notebook.New("form")
	if err := nb.Login(cfg.Username, cfg.Password); err != nil {
		slog.Info("detnotebooks: login failed", "err", err)
		return
	}

	slog.Info("detnotebooks: authenticated", "mode", nb.AuthMode, "school", cfg.SchoolCode)

	fleet, err := nb.GetCurrentFleet(cfg.SchoolCode)
	if err != nil {
		slog.Info("detnotebooks: GetCurrentFleet failed", "err", err)
		return
	}

	slog.Info("detnotebooks: fleet fetched", "count", len(fleet))

	resp, err := tc.PostJSON(tenant.URL("/api/agent/ingest/det-notebooks/fleet"), fleet)
	if err != nil {
		slog.Info("detnotebooks: failed to post fleet data", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("detnotebooks: fleet sync complete", "count", len(fleet), "status", resp.StatusCode)
}
