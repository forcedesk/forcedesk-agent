// Copyright © 2026 ForcePoint Software. All rights reserved.

package tasks

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/edustar"
	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

// eduStarConfig holds STMC integration settings, sourced from either local
// config.toml or the tenant API.
type eduStarConfig struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	SchoolCode   string `json:"school_code"`
	CRTGroupDN   string `json:"crt_group_dn"`
	CRTGroupName string `json:"crt_group_name"`
	AuthMode     string `json:"auth_mode"`
}

// UnmarshalJSON handles school_code being sent as either a JSON string or number.
func (c *eduStarConfig) UnmarshalJSON(b []byte) error {
	type raw struct {
		Username     string          `json:"username"`
		Password     string          `json:"password"`
		SchoolCode   json.RawMessage `json:"school_code"`
		CRTGroupDN   string          `json:"crt_group_dn"`
		CRTGroupName string          `json:"crt_group_name"`
		AuthMode     string          `json:"auth_mode"`
	}
	var r raw
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	c.Username = r.Username
	c.Password = r.Password
	c.CRTGroupDN = r.CRTGroupDN
	c.CRTGroupName = r.CRTGroupName
	c.AuthMode = r.AuthMode

	// school_code may arrive as a quoted string or a bare number.
	var s string
	if err := json.Unmarshal(r.SchoolCode, &s); err == nil {
		c.SchoolCode = s
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(r.SchoolCode, &n); err != nil {
		return fmt.Errorf("school_code: %w", err)
	}
	c.SchoolCode = n.String()
	return nil
}

// crtAccount is a CRT service account fetched from the tenant for expire/enable operations.
type crtAccount struct {
	Login  string `json:"login"`
	LdapDN string `json:"ldap_dn"`
}

// serviceAccount is a managed service account fetched from the tenant for expire operations.
type serviceAccount struct {
	Login  string `json:"login"`
	LdapDN string `json:"ldap_dn"`
}

// EduStarCLIOpts holds the optional flags parsed from the CLI for the edustar subcommand.
type EduStarCLIOpts struct {
	School      string // overrides the configured school code
	DN          string // user DN for user/password operations
	GroupDN     string // group DN for group operations
	GroupName   string // group name for group operations
	MemberDN    string // member DN for add/remove-from-group
	NewPassword string // new password for set-password
	Dump        bool   // print JSON payload to stdout instead of sending to tenant
}

// resolveConfig returns EduStar config from local config.toml when enabled,
// otherwise falls back to fetching from the tenant API.
func resolveConfig(tc *tenant.Client) (*eduStarConfig, error) {
	local := config.Get().EduStar
	if local.Enabled && local.Username != "" {
		return &eduStarConfig{
			Username:     local.Username,
			Password:     local.GetPassword(),
			SchoolCode:   local.SchoolCode,
			CRTGroupDN:   local.CRTGroupDN,
			CRTGroupName: local.CRTGroupName,
			AuthMode:     local.AuthMode,
		}, nil
	}
	return fetchEduStarConfig(tc)
}

// fetchEduStarConfig retrieves STMC integration config from the tenant API.
// The response is decrypted using the ChaCha20-Poly1305 key from [tenant] encryption_key in config.toml.
func fetchEduStarConfig(tc *tenant.Client) (*eduStarConfig, error) {
	key, err := config.Get().Tenant.GetEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}

	var cfg eduStarConfig
	if err := tc.GetEncryptedJSON(tenant.URL("/api/agent/edustar-config"), &cfg, key); err != nil {
		return nil, fmt.Errorf("fetch edustar config: %w", err)
	}
	if cfg.Username == "" || cfg.Password == "" {
		return nil, fmt.Errorf("edustar config is incomplete (missing credentials)")
	}
	return &cfg, nil
}

// initClient resolves config and returns an authenticated STMC client.
func initClient(tc *tenant.Client) (*edustar.Client, *eduStarConfig, error) {
	cfg, err := resolveConfig(tc)
	if err != nil {
		return nil, nil, err
	}

	stmc := edustar.New(cfg.AuthMode)
	if err := stmc.Login(cfg.Username, cfg.Password); err != nil {
		return nil, nil, fmt.Errorf("STMC login failed: %w", err)
	}

	slog.Info("edustar: authenticated", "mode", stmc.AuthMode)
	return stmc, cfg, nil
}

// ============================================================
// Scheduled service task
// ============================================================

// EduStarService runs the full population sync: students, staff, and CRT accounts.
// Registered in the scheduler. Only runs when EduStar is enabled in local config.
func EduStarService() {
	if !config.Get().EduStar.Enabled {
		slog.Info("edustar: disabled in config, skipping")
		return
	}

	slog.Info("edustar: starting population sync")

	tc := tenant.New()
	if err := tc.TestConnectivity(); err != nil {
		slog.Error("edustar: connectivity check failed", "err", err)
		return
	}

	stmc, cfg, err := initClient(tc)
	if err != nil {
		slog.Error("edustar: init failed", "err", err)
		return
	}

	populateStudents(tc, stmc, cfg)
	populateStaff(tc, stmc, cfg)
	populateCRT(tc, stmc, cfg)

	slog.Info("edustar: population sync complete")
}

// ============================================================
// Command queue handler
// ============================================================

// EduStarCommand runs a single named action triggered by the command queue,
// posting results back to the tenant.
func EduStarCommand(action string) {
	if action == "" {
		slog.Error("edustar: command received with no action")
		return
	}

	slog.Info("edustar: running command", "action", action)

	tc := tenant.New()
	if err := tc.TestConnectivity(); err != nil {
		slog.Error("edustar: connectivity check failed", "err", err, "action", action)
		return
	}

	stmc, cfg, err := initClient(tc)
	if err != nil {
		slog.Error("edustar: init failed", "err", err, "action", action)
		return
	}

	switch action {
	case "populate-student-accounts":
		populateStudents(tc, stmc, cfg)
	case "populate-staff-accounts":
		populateStaff(tc, stmc, cfg)
	case "populate-crt-accounts":
		populateCRT(tc, stmc, cfg)
	case "expire-crt-accounts":
		expireCRT(tc, stmc, cfg)
	case "enable-crt-accounts":
		enableCRT(tc, stmc, cfg)
	case "expire-service-accounts":
		expireServiceAccounts(tc, stmc, cfg)
	default:
		slog.Warn("edustar: unknown action", "action", action)
	}
}

// ============================================================
// Shared service operations
// ============================================================

// populateStudents fetches all students for the configured school from STMC and posts them to the tenant.
func populateStudents(tc *tenant.Client, stmc *edustar.Client, cfg *eduStarConfig) {
	slog.Info("edustar: fetching students", "school", cfg.SchoolCode)

	students, err := stmc.GetStudents(cfg.SchoolCode)
	if err != nil {
		slog.Error("edustar: GetStudents failed", "err", err)
		return
	}

	resp, err := tc.PostJSON(tenant.URL("/api/agent/ingest/edustar/students"), students)
	if err != nil {
		slog.Error("edustar: failed to post students", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("edustar: students synced", "count", len(students), "status", resp.StatusCode)
}

// populateStaff fetches staff (and technicians) for the configured school from STMC and posts them to the tenant.
func populateStaff(tc *tenant.Client, stmc *edustar.Client, cfg *eduStarConfig) {
	slog.Info("edustar: fetching staff", "school", cfg.SchoolCode)

	staff, err := stmc.GetStaff(cfg.SchoolCode)
	if err != nil {
		slog.Error("edustar: GetStaff failed", "err", err)
		return
	}

	resp, err := tc.PostJSON(tenant.URL("/api/agent/ingest/edustar/staff"), staff)
	if err != nil {
		slog.Error("edustar: failed to post staff", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("edustar: staff synced", "status", resp.StatusCode)
}

// populateCRT fetches the members of the CRT group from STMC and posts them to the tenant.
// Skipped when CRTGroupDN or CRTGroupName is not configured.
func populateCRT(tc *tenant.Client, stmc *edustar.Client, cfg *eduStarConfig) {
	if cfg.CRTGroupDN == "" || cfg.CRTGroupName == "" {
		slog.Warn("edustar: CRT group DN/name not configured, skipping CRT sync")
		return
	}

	slog.Info("edustar: fetching CRT group members", "group", cfg.CRTGroupName)

	members, err := stmc.GetGroup(cfg.SchoolCode, cfg.CRTGroupName, cfg.CRTGroupDN)
	if err != nil {
		slog.Error("edustar: GetGroup failed", "err", err)
		return
	}

	resp, err := tc.PostJSON(tenant.URL("/api/agent/ingest/edustar/crt-accounts"), members)
	if err != nil {
		slog.Error("edustar: failed to post CRT accounts", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("edustar: CRT accounts synced", "count", len(members), "status", resp.StatusCode)
}

// expireCRT disables each CRT account in STMC and scrambles its password so it
// cannot be used even if re-enabled manually outside this system.
func expireCRT(tc *tenant.Client, stmc *edustar.Client, cfg *eduStarConfig) {
	slog.Info("edustar: expiring CRT accounts")

	// Fetch the current CRT account list from the tenant rather than querying
	// STMC directly — the tenant is the authoritative source for which accounts
	// belong to the CRT rotation on a given day.
	accounts, err := fetchCRTAccounts(tc)
	if err != nil {
		slog.Error("edustar: failed to fetch CRT accounts", "err", err)
		return
	}

	for _, acc := range accounts {
		// Step 1: Disable the account in STMC to prevent login immediately.
		if err := stmc.DisableServiceAccount(cfg.SchoolCode, acc.LdapDN); err != nil {
			slog.Error("edustar: disable failed", "login", acc.Login, "err", err)
			continue
		}
		// Step 2: Scramble the password with a random UUID so the account cannot be
		// reactivated simply by re-enabling it — the credential is now unknown to anyone.
		if err := stmc.SetStudentPassword(cfg.SchoolCode, acc.LdapDN, newUUID()); err != nil {
			slog.Error("edustar: password scramble failed", "login", acc.Login, "err", err)
		}
		slog.Info("edustar: CRT account expired", "login", acc.Login)
	}

	slog.Info("edustar: CRT expire complete", "count", len(accounts))
}

// enableCRT re-enables each CRT account in STMC, sets a fresh daily password via
// password.ninja, and posts the updated credentials to the tenant for the daily CRT email.
func enableCRT(tc *tenant.Client, stmc *edustar.Client, cfg *eduStarConfig) {
	slog.Info("edustar: enabling CRT accounts")

	accounts, err := fetchCRTAccounts(tc)
	if err != nil {
		slog.Error("edustar: failed to fetch CRT accounts", "err", err)
		return
	}

	type crtPassword struct {
		Login    string `json:"login"`
		LdapDN   string `json:"ldap_dn"`
		Password string `json:"password"`
	}

	var updated []crtPassword

	// Three-step activation per account. All three steps must succeed for the
	// account to be added to the updated list; partial success (e.g. enabled but
	// password not set) is logged and the account is skipped so it won't appear
	// in the daily CRT email with a stale or unknown credential.
	for _, acc := range accounts {
		// Step 1: Re-enable the AD account so the CRT can log in today.
		if err := stmc.EnableServiceAccount(cfg.SchoolCode, acc.LdapDN); err != nil {
			slog.Error("edustar: enable failed", "login", acc.Login, "err", err)
			continue
		}

		// Step 2: Generate a strong random password from password.ninja.
		// A fresh password is created each day so that yesterday's credential
		// becomes useless after expireCRT scrambles it tonight.
		pwd, err := generatePassword()
		if err != nil {
			slog.Error("edustar: password generation failed", "login", acc.Login, "err", err)
			continue
		}

		// Step 3: Push the new password into STMC so it takes effect immediately.
		if err := stmc.SetStudentPassword(cfg.SchoolCode, acc.LdapDN, pwd); err != nil {
			slog.Error("edustar: set password failed", "login", acc.Login, "err", err)
			continue
		}

		slog.Info("edustar: CRT account enabled", "login", acc.Login)
		updated = append(updated, crtPassword{Login: acc.Login, LdapDN: acc.LdapDN, Password: pwd})
	}

	if len(updated) == 0 {
		return
	}

	// Post updated passwords to the tenant so it can store them and send the daily CRT email.
	resp, err := tc.PostJSON(tenant.URL("/api/agent/ingest/edustar/crt-passwords"), updated)
	if err != nil {
		slog.Error("edustar: failed to post CRT passwords", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("edustar: CRT enable complete", "count", len(updated), "status", resp.StatusCode)
}

// fetchCRTAccounts retrieves the list of CRT accounts stored on the tenant.
func fetchCRTAccounts(tc *tenant.Client) ([]crtAccount, error) {
	var accounts []crtAccount
	if err := tc.GetJSON(tenant.URL("/api/agent/edustar/crt-accounts"), &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

// fetchServiceAccounts retrieves the list of managed service accounts stored on the tenant.
func fetchServiceAccounts(tc *tenant.Client) ([]serviceAccount, error) {
	var accounts []serviceAccount
	if err := tc.GetJSON(tenant.URL("/api/agent/edustar/service-accounts"), &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

// expireServiceAccounts disables each managed service account in STMC and scrambles its password.
func expireServiceAccounts(tc *tenant.Client, stmc *edustar.Client, cfg *eduStarConfig) {
	slog.Info("edustar: expiring service accounts")

	accounts, err := fetchServiceAccounts(tc)
	if err != nil {
		slog.Error("edustar: failed to fetch service accounts", "err", err)
		return
	}

	// Same two-step disable+scramble pattern as expireCRT: disable first to block
	// login immediately, then replace the password with a random UUID so the account
	// cannot be reactivated by simply re-enabling it in the directory.
	for _, acc := range accounts {
		if err := stmc.DisableServiceAccount(cfg.SchoolCode, acc.LdapDN); err != nil {
			slog.Error("edustar: disable service account failed", "login", acc.Login, "err", err)
			continue
		}
		// Scramble the password so the account cannot be used even if manually re-enabled.
		if err := stmc.SetStudentPassword(cfg.SchoolCode, acc.LdapDN, newUUID()); err != nil {
			slog.Error("edustar: password scramble failed", "login", acc.Login, "err", err)
		}
		slog.Info("edustar: service account expired", "login", acc.Login)
	}

	slog.Info("edustar: service account expire complete", "count", len(accounts))
}

// generatePassword fetches a single strong password from the password.ninja API.
// The response is a JSON-quoted string (e.g. `"abc123"`) that is unquoted before returning.
func generatePassword() (string, error) {
	// Request one password with symbols, capitals, and numeric characters.
	// excludeSymbols=f means "false" — symbols ARE included. The API returns a
	// bare JSON string (not a JSON object), e.g.: `"Abc!123xyz"`.
	resp, err := http.Get("https://password.ninja/api/password?symbols=true&capitals=true&numOfPasswords=1&excludeSymbols=f") //nolint:noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// The password.ninja response is a JSON-quoted string literal: `"Abc!123xyz"`.
	// Strip the surrounding double quotes rather than running json.Unmarshal so we
	// don't introduce a dependency on the body being valid JSON in other respects.
	pwd := string(body)
	if len(pwd) >= 2 && pwd[0] == '"' && pwd[len(pwd)-1] == '"' {
		pwd = pwd[1 : len(pwd)-1]
	}
	if pwd == "" {
		return "", fmt.Errorf("empty password from password.ninja")
	}
	return pwd, nil
}

// ============================================================
// CLI
// ============================================================

// RunEduStarCLI executes an EduStar action and prints the result to stdout.
// Config is resolved from local config.toml first; if not configured locally it is
// fetched from the tenant API (encrypted). No connectivity pre-check is performed.
func RunEduStarCLI(action string, opts EduStarCLIOpts) {
	tc := tenant.New()
	stmc, cfg, err := initClient(tc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "edustar: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Authenticated via %s\n\n", stmc.AuthMode)

	// Resolve school: CLI flag > config.
	school := opts.School
	if school == "" {
		school = cfg.SchoolCode
	}

	switch action {
	// ── Info / read-only ────────────────────────────────────────────────────

	case "whoami":
		result, err := stmc.WhoAmI()
		cliPrint(result, err)

	case "schools":
		result, err := stmc.GetSchools()
		cliPrint(result, err)

	case "all-schools":
		result, err := stmc.GetAllSchools()
		cliPrint(result, err)

	case "students":
		result, err := stmc.GetStudents(school)
		cliPrint(result, err)

	case "staff":
		result, err := stmc.GetStaff(school)
		cliPrint(result, err)

	case "technicians":
		result, err := stmc.GetTechnicians(school)
		cliPrint(result, err)

	case "groups":
		result, err := stmc.GetGroups(school)
		cliPrint(result, err)

	case "certificates":
		result, err := stmc.GetCertificates(school)
		cliPrint(result, err)

	case "service-accounts":
		result, err := stmc.GetServiceAccounts(school)
		cliPrint(result, err)

	case "nps":
		result, err := stmc.GetNps(school)
		cliPrint(result, err)

	case "user":
		requireFlag("--dn", opts.DN)
		result, err := stmc.GetUser(opts.DN)
		cliPrint(result, err)

	case "group":
		requireFlag("--group-dn", opts.GroupDN)
		requireFlag("--group-name", opts.GroupName)
		result, err := stmc.GetGroup(school, opts.GroupName, opts.GroupDN)
		cliPrint(result, err)

	// ── Password operations ──────────────────────────────────────────────────

	case "reset-password":
		requireFlag("--dn", opts.DN)
		result, err := stmc.ResetStudentPassword(school, opts.DN)
		cliPrint(result, err)

	case "set-password":
		requireFlag("--dn", opts.DN)
		requireFlag("--new-password", opts.NewPassword)
		if err := stmc.SetStudentPassword(school, opts.DN, opts.NewPassword); err != nil {
			cliError(err)
		}
		fmt.Println("Password set successfully.")

	// ── Group membership ─────────────────────────────────────────────────────

	case "add-to-group":
		requireFlag("--group-dn", opts.GroupDN)
		requireFlag("--member-dn", opts.MemberDN)
		if err := stmc.AddToGroup(school, opts.GroupDN, opts.MemberDN); err != nil {
			cliError(err)
		}
		fmt.Println("Member added successfully.")

	case "remove-from-group":
		requireFlag("--group-dn", opts.GroupDN)
		requireFlag("--member-dn", opts.MemberDN)
		if err := stmc.RemoveFromGroup(school, opts.GroupDN, opts.MemberDN); err != nil {
			cliError(err)
		}
		fmt.Println("Member removed successfully.")

	// ── Sync / service operations (fetch from STMC, POST to tenant) ─────────

	case "populate-student-accounts":
		fmt.Printf("Fetching students for school %s...\n", cfg.SchoolCode)
		students, err := stmc.GetStudents(cfg.SchoolCode)
		if err != nil {
			cliError(fmt.Errorf("GetStudents: %w", err))
		}
		if opts.Dump {
			cliPrint(students, nil)
			break
		}
		fmt.Printf("Fetched %d students. Posting to tenant...\n", len(students))
		resp, err := tc.PostJSON(tenant.URL("/api/agent/ingest/edustar/students"), students)
		if err != nil {
			cliError(fmt.Errorf("post students: %w", err))
		}
		resp.Body.Close()
		fmt.Printf("Done. HTTP %d\n", resp.StatusCode)

	case "populate-staff-accounts":
		fmt.Printf("Fetching staff for school %s...\n", cfg.SchoolCode)
		staff, err := stmc.GetStaff(cfg.SchoolCode)
		if err != nil {
			cliError(fmt.Errorf("GetStaff: %w", err))
		}
		if opts.Dump {
			cliPrint(staff, nil)
			break
		}
		fmt.Printf("Fetched %d staff. Posting to tenant...\n", len(staff))
		resp, err := tc.PostJSON(tenant.URL("/api/agent/ingest/edustar/staff"), staff)
		if err != nil {
			cliError(fmt.Errorf("post staff: %w", err))
		}
		resp.Body.Close()
		fmt.Printf("Done. HTTP %d\n", resp.StatusCode)

	case "populate-crt-accounts":
		requireFlag("--group-dn", cfg.CRTGroupDN)
		requireFlag("--group-name", cfg.CRTGroupName)
		fmt.Printf("Fetching CRT group %q...\n", cfg.CRTGroupName)
		members, err := stmc.GetGroup(cfg.SchoolCode, cfg.CRTGroupName, cfg.CRTGroupDN)
		if err != nil {
			cliError(fmt.Errorf("GetGroup: %w", err))
		}
		if opts.Dump {
			cliPrint(members, nil)
			break
		}
		fmt.Printf("Fetched %d members. Posting to tenant...\n", len(members))
		resp, err := tc.PostJSON(tenant.URL("/api/agent/ingest/edustar/crt-accounts"), members)
		if err != nil {
			cliError(fmt.Errorf("post CRT accounts: %w", err))
		}
		resp.Body.Close()
		fmt.Printf("Done. HTTP %d\n", resp.StatusCode)

	case "expire-crt-accounts":
		accounts, err := fetchCRTAccounts(tc)
		if err != nil {
			cliError(fmt.Errorf("fetch CRT accounts: %w", err))
		}
		if opts.Dump {
			cliPrint(accounts, nil)
			break
		}
		fmt.Printf("Expiring %d CRT accounts...\n", len(accounts))
		for _, acc := range accounts {
			if err := stmc.DisableServiceAccount(cfg.SchoolCode, acc.LdapDN); err != nil {
				fmt.Fprintf(os.Stderr, "  disable %s: %v\n", acc.Login, err)
				continue
			}
			if err := stmc.SetStudentPassword(cfg.SchoolCode, acc.LdapDN, newUUID()); err != nil {
				fmt.Fprintf(os.Stderr, "  scramble password %s: %v\n", acc.Login, err)
			}
			fmt.Printf("  expired: %s\n", acc.Login)
		}
		fmt.Printf("Done. %d accounts expired.\n", len(accounts))

	case "enable-crt-accounts":
		accounts, err := fetchCRTAccounts(tc)
		if err != nil {
			cliError(fmt.Errorf("fetch CRT accounts: %w", err))
		}
		if opts.Dump {
			cliPrint(accounts, nil)
			break
		}
		fmt.Printf("Enabling %d CRT accounts...\n", len(accounts))

		type crtPassword struct {
			Login    string `json:"login"`
			LdapDN   string `json:"ldap_dn"`
			Password string `json:"password"`
		}
		var updated []crtPassword

		for _, acc := range accounts {
			if err := stmc.EnableServiceAccount(cfg.SchoolCode, acc.LdapDN); err != nil {
				fmt.Fprintf(os.Stderr, "  enable %s: %v\n", acc.Login, err)
				continue
			}
			pwd, err := generatePassword()
			if err != nil {
				fmt.Fprintf(os.Stderr, "  generate password %s: %v\n", acc.Login, err)
				continue
			}
			if err := stmc.SetStudentPassword(cfg.SchoolCode, acc.LdapDN, pwd); err != nil {
				fmt.Fprintf(os.Stderr, "  set password %s: %v\n", acc.Login, err)
				continue
			}
			fmt.Printf("  enabled: %s\n", acc.Login)
			updated = append(updated, crtPassword{Login: acc.Login, LdapDN: acc.LdapDN, Password: pwd})
		}

		if len(updated) > 0 {
			fmt.Printf("Posting %d updated passwords to tenant...\n", len(updated))
			resp, err := tc.PostJSON(tenant.URL("/api/agent/ingest/edustar/crt-passwords"), updated)
			if err != nil {
				cliError(fmt.Errorf("post CRT passwords: %w", err))
			}
			resp.Body.Close()
			fmt.Printf("Done. HTTP %d — %d accounts enabled.\n", resp.StatusCode, len(updated))
		} else {
			fmt.Println("No accounts were updated.")
		}

	case "expire-service-accounts":
		accounts, err := fetchServiceAccounts(tc)
		if err != nil {
			cliError(fmt.Errorf("fetch service accounts: %w", err))
		}
		if opts.Dump {
			cliPrint(accounts, nil)
			break
		}
		fmt.Printf("Expiring %d service accounts...\n", len(accounts))
		for _, acc := range accounts {
			if err := stmc.DisableServiceAccount(cfg.SchoolCode, acc.LdapDN); err != nil {
				fmt.Fprintf(os.Stderr, "  disable %s: %v\n", acc.Login, err)
				continue
			}
			if err := stmc.SetStudentPassword(cfg.SchoolCode, acc.LdapDN, newUUID()); err != nil {
				fmt.Fprintf(os.Stderr, "  scramble password %s: %v\n", acc.Login, err)
			}
			fmt.Printf("  expired: %s\n", acc.Login)
		}
		fmt.Printf("Done. %d service accounts expired.\n", len(accounts))

	default:
		fmt.Fprintf(os.Stderr, "edustar: unknown action %q\n", action)
		os.Exit(1)
	}
}

// requireFlag exits with an error if value is empty, used to validate required CLI flags.
func requireFlag(name, value string) {
	if value == "" {
		fmt.Fprintf(os.Stderr, "edustar: %s is required for this action\n", name)
		os.Exit(1)
	}
}

// cliError prints an error to stderr and exits with code 1.
func cliError(err error) {
	fmt.Fprintf(os.Stderr, "edustar: %v\n", err)
	os.Exit(1)
}

// cliPrint marshals v as indented JSON and prints it to stdout, or calls cliError on failure.
func cliPrint(v any, err error) {
	if err != nil {
		cliError(err)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}
