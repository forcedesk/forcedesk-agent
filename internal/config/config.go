package config

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"golang.org/x/term"

	"github.com/forcedesk/forcedesk-agent/internal/secure"
)

type Tenant struct {
	URL           string `toml:"url"`
	APIKey        string `toml:"api_key"`
	UUID          string `toml:"uuid"`
	VerifySSL     bool   `toml:"verify_ssl"`
	EncryptionKey string `toml:"encryption_key"` // hex-encoded 32-byte ChaCha20-Poly1305 key, set by server
	apiKeySec     *secure.String
	encKeySec     *secure.String
}

// GetAPIKey returns the API key from secure storage, or falls back to the plain text field.
func (t *Tenant) GetAPIKey() string {
	if t.apiKeySec != nil && !t.apiKeySec.IsEmpty() {
		return t.apiKeySec.String()
	}
	return t.APIKey
}

// GetEncryptionKey decodes and returns the 32-byte ChaCha20-Poly1305 key from secure storage.
// Returns an error if the key is not configured or is not a valid 32-byte hex string.
func (t *Tenant) GetEncryptionKey() ([]byte, error) {
	var raw string
	if t.encKeySec != nil && !t.encKeySec.IsEmpty() {
		raw = t.encKeySec.String()
	} else {
		raw = t.EncryptionKey
	}
	if raw == "" {
		return nil, fmt.Errorf("encryption_key not set in [tenant] config")
	}
	key, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("encryption_key is not valid hex: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption_key must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	return key, nil
}

type Papercut struct {
	Enabled   bool   `toml:"enabled"`
	APIURL    string `toml:"api_url"`
	APIKey    string `toml:"api_key"`
	apiKeySec *secure.String
}

// GetAPIKey returns the API key from secure storage, or falls back to the plain text field.
func (p *Papercut) GetAPIKey() string {
	if p.apiKeySec != nil && !p.apiKeySec.IsEmpty() {
		return p.apiKeySec.String()
	}
	return p.APIKey
}

type EduStar struct {
	Enabled      bool   `toml:"enabled"`
	Username     string `toml:"username"`
	Password     string `toml:"password"`
	SchoolCode   string `toml:"school_code"`
	CRTGroupDN   string `toml:"crt_group_dn"`
	CRTGroupName string `toml:"crt_group_name"`
	// AuthMode controls authentication: "ntlm", "form", or "" (auto — tries NTLM then form).
	AuthMode    string `toml:"auth_mode"`
	passwordSec *secure.String
}

// GetPassword returns the STMC password from secure storage, or falls back to the plain text field.
func (e *EduStar) GetPassword() string {
	if e.passwordSec != nil && !e.passwordSec.IsEmpty() {
		return e.passwordSec.String()
	}
	return e.Password
}

type DeviceManager struct {
	LegacySSHOptions string `toml:"legacy_ssh_options"`
}

type Logging struct {
	Level string `toml:"level"`
}

type Config struct {
	Tenant        Tenant        `toml:"tenant"`
	Papercut      Papercut      `toml:"papercut"`
	EduStar       EduStar       `toml:"edustar"`
	DeviceManager DeviceManager `toml:"device_manager"`
	Logging       Logging       `toml:"logging"`
}

var (
	instance *Config
	mu       sync.RWMutex
)

// DataDir returns the persistent data directory for the agent.
func DataDir() string {
	dir := os.Getenv("ProgramData")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "ForceDeskAgent")
}

// ConfigPath returns the path to the agent config file.
func ConfigPath() string {
	return filepath.Join(DataDir(), "config.toml")
}

// Exists reports whether the config file is present on disk.
func Exists() bool {
	_, err := os.Stat(ConfigPath())
	return err == nil
}

// Load reads the config file from disk. If the file does not exist the
// returned Config is populated with defaults and no error is returned.
func Load() (*Config, error) {
	cfg := defaults()

	path := ConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		mu.Lock()
		instance = cfg
		mu.Unlock()
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}

	// Convert sensitive strings to secure storage.
	if cfg.Tenant.APIKey != "" {
		cfg.Tenant.apiKeySec = secure.NewString(cfg.Tenant.APIKey)
		cfg.Tenant.APIKey = ""
	}
	if cfg.Tenant.EncryptionKey != "" {
		cfg.Tenant.encKeySec = secure.NewString(cfg.Tenant.EncryptionKey)
		cfg.Tenant.EncryptionKey = ""
	}
	if cfg.Papercut.APIKey != "" {
		cfg.Papercut.apiKeySec = secure.NewString(cfg.Papercut.APIKey)
		cfg.Papercut.APIKey = ""
	}
	if cfg.EduStar.Password != "" {
		cfg.EduStar.passwordSec = secure.NewString(cfg.EduStar.Password)
		cfg.EduStar.Password = ""
	}

	mu.Lock()
	instance = cfg
	mu.Unlock()
	return cfg, nil
}

// Get returns the current config, loading it if necessary.
func Get() *Config {
	mu.RLock()
	c := instance
	mu.RUnlock()
	if c != nil {
		return c
	}
	c, _ = Load()
	return c
}

// Setup runs an interactive first-time configuration wizard, writes the
// resulting config.toml to disk, and returns the new Config.
func Setup() (*Config, error) {
	cfg := defaults()
	r := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("ForceDesk Agent — First-time Setup")
	fmt.Println("===================================")
	fmt.Printf("Config will be written to: %s\n\n", ConfigPath())

	// Prompt for tenant configuration.
	fmt.Println("[Tenant]")
	cfg.Tenant.URL = promptRequired(r, "Tenant URL (e.g. https://tenant.schooldesk.io)")
	cfg.Tenant.UUID = promptRequired(r, "Agent UUID")
	apiKey := promptPassword(r, "API Key")
	cfg.Tenant.APIKey = apiKey
	cfg.Tenant.apiKeySec = secure.NewString(apiKey)
	cfg.Tenant.VerifySSL = promptBool(r, "Verify SSL certificates?", true)

	// Prompt for optional Papercut integration.
	fmt.Println()
	fmt.Println("[Papercut]")
	if promptBool(r, "Enable Papercut integration?", false) {
		cfg.Papercut.Enabled = true
		cfg.Papercut.APIURL = promptDefault(r, "Papercut API URL", "http://papercut-server:9191/rpc/api/xmlrpc")
		pcAPIKey := promptPassword(r, "Papercut API Key")
		cfg.Papercut.APIKey = pcAPIKey
		cfg.Papercut.apiKeySec = secure.NewString(pcAPIKey)
	}

	// Prompt for optional EduStar STMC integration.
	fmt.Println()
	fmt.Println("[EduStar STMC]")
	if promptBool(r, "Enable EduStar STMC integration?", false) {
		cfg.EduStar.Enabled = true
		cfg.EduStar.Username = promptRequired(r, "STMC Username")
		esPwd := promptPassword(r, "STMC Password")
		cfg.EduStar.Password = esPwd
		cfg.EduStar.passwordSec = secure.NewString(esPwd)
		cfg.EduStar.SchoolCode = promptRequired(r, "School Code")
		cfg.EduStar.CRTGroupDN = promptDefault(r, "CRT Group DN (optional)", "")
		cfg.EduStar.CRTGroupName = promptDefault(r, "CRT Group Name (optional)", "")
		cfg.EduStar.AuthMode = promptDefault(r, "Auth Mode (ntlm/form/auto)", "")
	}

	// Write configuration to disk.
	if err := save(cfg); err != nil {
		return nil, err
	}

	fmt.Printf("\nConfig saved to %s\n\n", ConfigPath())

	mu.Lock()
	instance = cfg
	mu.Unlock()
	return cfg, nil
}

// SaveConfig writes cfg to disk and updates the in-memory singleton.
// Used by platform-specific GUI setup wizards.
func SaveConfig(cfg *Config) error {
	if err := save(cfg); err != nil {
		return err
	}
	mu.Lock()
	instance = cfg
	mu.Unlock()
	return nil
}

// save writes cfg to ConfigPath as TOML, creating the data directory if needed.
func save(cfg *Config) error {
	if err := os.MkdirAll(DataDir(), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	// Create config file with restrictive permissions (owner read/write only).
	// This prevents other users from reading API keys and passwords.
	f, err := os.OpenFile(ConfigPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return nil
}

func defaults() *Config {
	return &Config{
		Tenant:  Tenant{VerifySSL: true},
		Logging: Logging{Level: "info"},
		DeviceManager: DeviceManager{
			LegacySSHOptions: "-o StrictHostKeyChecking=no -oKexAlgorithms=+diffie-hellman-group1-sha1",
		},
	}
}

// promptRequired loops until the user enters a non-empty value.
func promptRequired(r *bufio.Reader, label string) string {
	for {
		fmt.Printf("  %s: ", label)
		val, _ := r.ReadString('\n')
		val = strings.TrimSpace(val)
		if val != "" {
			return val
		}
		fmt.Println("  (value is required)")
	}
}

// promptDefault shows a default value in brackets; an empty response accepts it.
func promptDefault(r *bufio.Reader, label, def string) string {
	fmt.Printf("  %s [%s]: ", label, def)
	val, _ := r.ReadString('\n')
	val = strings.TrimSpace(val)
	if val == "" {
		return def
	}
	return val
}

// promptBool asks a yes/no question. def is the default answer.
func promptBool(r *bufio.Reader, label string, def bool) bool {
	hint := "Y/n"
	if !def {
		hint = "y/N"
	}
	for {
		fmt.Printf("  %s [%s]: ", label, hint)
		val, _ := r.ReadString('\n')
		val = strings.TrimSpace(strings.ToLower(val))
		switch val {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		case "":
			return def
		default:
			fmt.Println("  (please enter y or n)")
		}
	}
}

// promptPassword reads a value with echo suppressed when stdin is a terminal,
// falling back to plain text input if it is not.
func promptPassword(r *bufio.Reader, label string) string {
	for {
		fmt.Printf("  %s: ", label)

		fd := int(os.Stdin.Fd())
		if term.IsTerminal(fd) {
			b, err := term.ReadPassword(fd)
			fmt.Println()
			if err == nil {
				val := strings.TrimSpace(string(b))
				if val != "" {
					return val
				}
				fmt.Println("  (value is required)")
				continue
			}
			// term.ReadPassword failed; fall through to plain text input.
		}

		// Fallback for piped input or non-terminal environments.
		val, _ := r.ReadString('\n')
		val = strings.TrimSpace(val)
		if val != "" {
			return val
		}
		fmt.Println("  (value is required)")
	}
}
