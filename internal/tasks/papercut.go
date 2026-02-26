package tasks

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

// papercutConfig holds PaperCut connection settings fetched from the tenant API.
type papercutConfig struct {
	APIURL string `json:"papercut_api_url"`
	APIKey string `json:"papercut_api_key"`
}

// fetchPapercutConfig retrieves PaperCut connection config from the tenant API.
// The response is decrypted using the ChaCha20-Poly1305 key from [tenant] encryption_key in config.toml.
func fetchPapercutConfig(tc *tenant.Client) (*papercutConfig, error) {
	key, err := config.Get().Tenant.GetEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}

	var cfg papercutConfig
	if err := tc.GetEncryptedJSON(tenant.URL("/api/agent/papercut-config"), &cfg, key); err != nil {
		return nil, fmt.Errorf("fetch papercut config: %w", err)
	}
	if cfg.APIURL == "" || cfg.APIKey == "" {
		return nil, fmt.Errorf("papercut config is incomplete (missing api_url or api_key)")
	}
	return &cfg, nil
}

// pcServerUser is a user entry returned by the tenant API.
type pcServerUser struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

// pcServerPayload is the response from the tenant's user list endpoint.
type pcServerPayload struct {
	Staff    []pcServerUser `json:"staff"`
	Students []pcServerUser `json:"students"`
}

// fetchPapercutUsers retrieves the staff and student lists from the tenant API.
// The response is decrypted using the ChaCha20-Poly1305 key from [tenant] encryption_key in config.toml.
func fetchPapercutUsers(tc *tenant.Client) (*pcServerPayload, error) {
	key, err := config.Get().Tenant.GetEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}

	var payload pcServerPayload
	if err := tc.GetEncryptedJSON(tenant.URL("/api/agent/ingest/papercut-data"), &payload, key); err != nil {
		return nil, fmt.Errorf("fetch papercut users: %w", err)
	}
	return &payload, nil
}

type pcUserRecord struct {
	Username string   `json:"username,omitempty"`
	Login    string   `json:"login,omitempty"`
	PIN      *string  `json:"pin"`
	Balance  *float64 `json:"balance"`
}

type pcPayload struct {
	Staff    []pcUserRecord `json:"staff"`
	Students []pcUserRecord `json:"students"`
}

// PapercutService queries the local PaperCut print management server via XML-RPC API
// to retrieve user PINs and account balances for staff and students, then sends the data to the tenant.
// Runs every 30 minutes.
func PapercutService() {
	slog.Info("papercut: starting")

	client := tenant.New()
	if err := client.TestConnectivity(); err != nil {
		slog.Error("papercut: connectivity check failed", "err", err)
		return
	}

	pcCfg, err := fetchPapercutConfig(client)
	if err != nil {
		slog.Error("papercut: failed to resolve config", "err", err)
		return
	}

	users, err := fetchPapercutUsers(client)
	if err != nil {
		slog.Error("papercut: failed to fetch users", "err", err)
		return
	}
	slog.Debug("papercut: users received", "staff", len(users.Staff), "students", len(users.Students))

	payload := pcPayload{}

	for _, s := range users.Staff {
		slog.Debug("papercut: querying staff member", "username", s.Username)
		pin, err := pcGetProperty(pcCfg.APIURL, pcCfg.APIKey, s.Username, "secondary-card-number")
		if err != nil {
			slog.Debug("papercut: PIN lookup failed", "username", s.Username, "err", err)
		}
		bal, err := pcGetPropertyFloat(pcCfg.APIURL, pcCfg.APIKey, s.Username, "balance")
		if err != nil {
			slog.Debug("papercut: balance lookup failed", "username", s.Username, "err", err)
		}
		slog.Debug("papercut: staff result", "username", s.Username, "has_pin", pin != nil, "has_balance", bal != nil)

		if pin == nil && bal == nil {
			continue
		}
		payload.Staff = append(payload.Staff, pcUserRecord{Username: s.Username, PIN: pin, Balance: bal})
		slog.Info("papercut: processed staff", "username", s.Username)
	}

	for _, s := range users.Students {
		slog.Debug("papercut: querying student", "username", s.Username)
		pin, err := pcGetProperty(pcCfg.APIURL, pcCfg.APIKey, s.Username, "secondary-card-number")
		if err != nil {
			slog.Debug("papercut: PIN lookup failed", "username", s.Username, "err", err)
		}
		bal, err := pcGetPropertyFloat(pcCfg.APIURL, pcCfg.APIKey, s.Username, "balance")
		if err != nil {
			slog.Debug("papercut: balance lookup failed", "username", s.Username, "err", err)
		}
		slog.Debug("papercut: student result", "username", s.Username, "has_pin", pin != nil, "has_balance", bal != nil)

		if pin == nil && bal == nil {
			continue
		}
		payload.Students = append(payload.Students, pcUserRecord{Login: s.Username, PIN: pin, Balance: bal})
		slog.Info("papercut: processed student", "username", s.Username)
	}

	if len(payload.Staff) == 0 && len(payload.Students) == 0 {
		slog.Info("papercut: no data to send")
		return
	}

	key, err := config.Get().Tenant.GetEncryptionKey()
	if err != nil {
		slog.Error("papercut: failed to get encryption key", "err", err)
		return
	}

	resp, err := client.PostEncryptedJSON(tenant.URL("/api/agent/ingest/papercut-data"), payload, key)
	if err != nil {
		slog.Error("papercut: failed to send data", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("papercut: data sent", "staff", len(payload.Staff), "students", len(payload.Students))
}

// pcXMLRequest builds an XML-RPC getUserProperty request body for PaperCut API calls.
func pcXMLRequest(apiKey, username, property string) string {
	return fmt.Sprintf(`<?xml version="1.0"?>
<methodCall>
  <methodName>api.getUserProperty</methodName>
  <params>
    <param><value><string>%s</string></value></param>
    <param><value><string>%s</string></value></param>
    <param><value><string>%s</string></value></param>
  </params>
</methodCall>`, xmlEscape(apiKey), xmlEscape(username), xmlEscape(property))
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// pcMethodResponse is the top-level XML-RPC response envelope.
type pcMethodResponse struct {
	Params *pcParams `xml:"params"`
	Fault  *pcFault  `xml:"fault"`
}

type pcParams struct {
	Param pcParam `xml:"param"`
}

type pcParam struct {
	Value pcValue `xml:"value"`
}

// pcValue holds the inner XML of a <value> element to handle both
// wrapped values like <value><string>x</string></value> and bare values like <value>x</value>.
type pcValue struct {
	InnerXML string `xml:",innerxml"`
}

type pcFault struct {
	Value string `xml:"value"`
}

func pcCall(apiURL, apiKey, username, property string) (string, error) {
	body := pcXMLRequest(apiKey, username, property)

	// Create request with context and timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/xml; charset=UTF-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result pcMethodResponse
	if err := xml.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("xml parse: %w", err)
	}

	if result.Fault != nil {
		return "", fmt.Errorf("XML-RPC fault: %s", result.Fault.Value)
	}
	if result.Params == nil {
		return "", fmt.Errorf("empty XML-RPC response")
	}

	// Extract the value from InnerXML, which may be wrapped (e.g., "<string>1234</string>") or bare (e.g., "1234").
	inner := strings.TrimSpace(result.Params.Param.Value.InnerXML)
	inner = stripXMLTag(inner, "string")
	inner = stripXMLTag(inner, "double")
	inner = stripXMLTag(inner, "int")
	return strings.TrimSpace(inner), nil
}

func stripXMLTag(s, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	if strings.HasPrefix(s, open) && strings.HasSuffix(s, close) {
		return s[len(open) : len(s)-len(close)]
	}
	return s
}

// pcGetProperty returns the property value, or nil if empty.
func pcGetProperty(apiURL, apiKey, username, property string) (*string, error) {
	val, err := pcCall(apiURL, apiKey, username, property)
	if err != nil {
		return nil, err
	}
	if val == "" {
		return nil, nil
	}
	return &val, nil
}

// pcGetPropertyFloat returns a float64 balance, or nil if not present/numeric.
func pcGetPropertyFloat(apiURL, apiKey, username, property string) (*float64, error) {
	val, err := pcCall(apiURL, apiKey, username, property)
	if err != nil {
		return nil, err
	}
	var f float64
	if _, err := fmt.Sscanf(val, "%f", &f); err != nil {
		return nil, nil
	}
	return &f, nil
}
