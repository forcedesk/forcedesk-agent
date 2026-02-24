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

// crtAccount is a CRT service account fetched from the tenant for expire/enable operations.
type crtAccount struct {
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
func fetchEduStarConfig(tc *tenant.Client) (*eduStarConfig, error) {
	var cfg eduStarConfig
	if err := tc.GetJSON(tenant.URL("/api/agent/edustar/config"), &cfg); err != nil {
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
	default:
		slog.Warn("edustar: unknown action", "action", action)
	}
}

// ============================================================
// Shared service operations
// ============================================================

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

func expireCRT(tc *tenant.Client, stmc *edustar.Client, cfg *eduStarConfig) {
	slog.Info("edustar: expiring CRT accounts")

	accounts, err := fetchCRTAccounts(tc)
	if err != nil {
		slog.Error("edustar: failed to fetch CRT accounts", "err", err)
		return
	}

	for _, acc := range accounts {
		if err := stmc.DisableServiceAccount(cfg.SchoolCode, acc.LdapDN); err != nil {
			slog.Error("edustar: disable failed", "login", acc.Login, "err", err)
			continue
		}
		// Scramble the password so the account cannot be used even if manually re-enabled.
		if err := stmc.SetStudentPassword(cfg.SchoolCode, acc.LdapDN, newUUID()); err != nil {
			slog.Error("edustar: password scramble failed", "login", acc.Login, "err", err)
		}
		slog.Info("edustar: CRT account expired", "login", acc.Login)
	}

	slog.Info("edustar: CRT expire complete", "count", len(accounts))
}

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

	for _, acc := range accounts {
		if err := stmc.EnableServiceAccount(cfg.SchoolCode, acc.LdapDN); err != nil {
			slog.Error("edustar: enable failed", "login", acc.Login, "err", err)
			continue
		}

		pwd, err := generatePassword()
		if err != nil {
			slog.Error("edustar: password generation failed", "login", acc.Login, "err", err)
			continue
		}

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

func fetchCRTAccounts(tc *tenant.Client) ([]crtAccount, error) {
	var accounts []crtAccount
	if err := tc.GetJSON(tenant.URL("/api/agent/edustar/crt-accounts"), &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

func generatePassword() (string, error) {
	resp, err := http.Get("https://password.ninja/api/password?symbols=true&capitals=true&numOfPasswords=1&excludeSymbols=f") //nolint:noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Response is a JSON-quoted string: `"password123"` — strip the surrounding quotes.
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
// Config is read from local config.toml only — no tenant API calls are made
// unless the action explicitly syncs data back to the server.
func RunEduStarCLI(action string, opts EduStarCLIOpts) {
	local := config.Get().EduStar
	if !local.Enabled || local.Username == "" {
		fmt.Fprintln(os.Stderr, "edustar: not configured — set [edustar] in config.toml")
		os.Exit(1)
	}

	cfg := &eduStarConfig{
		Username:     local.Username,
		Password:     local.GetPassword(),
		SchoolCode:   local.SchoolCode,
		CRTGroupDN:   local.CRTGroupDN,
		CRTGroupName: local.CRTGroupName,
		AuthMode:     local.AuthMode,
	}

	stmc := edustar.New(cfg.AuthMode)
	if err := stmc.Login(cfg.Username, cfg.Password); err != nil {
		fmt.Fprintf(os.Stderr, "edustar: STMC login failed: %v\n", err)
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

	// ── Sync / service operations (these post data back to the tenant) ────────

	case "populate-student-accounts":
		populateStudents(tenant.New(), stmc, cfg)
		fmt.Println("Done.")

	case "populate-staff-accounts":
		populateStaff(tenant.New(), stmc, cfg)
		fmt.Println("Done.")

	case "populate-crt-accounts":
		populateCRT(tenant.New(), stmc, cfg)
		fmt.Println("Done.")

	case "expire-crt-accounts":
		expireCRT(tenant.New(), stmc, cfg)
		fmt.Println("Done.")

	case "enable-crt-accounts":
		enableCRT(tenant.New(), stmc, cfg)
		fmt.Println("Done.")

	default:
		fmt.Fprintf(os.Stderr, "edustar: unknown action %q\n", action)
		os.Exit(1)
	}
}

func requireFlag(name, value string) {
	if value == "" {
		fmt.Fprintf(os.Stderr, "edustar: %s is required for this action\n", name)
		os.Exit(1)
	}
}

func cliError(err error) {
	fmt.Fprintf(os.Stderr, "edustar: %v\n", err)
	os.Exit(1)
}

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
