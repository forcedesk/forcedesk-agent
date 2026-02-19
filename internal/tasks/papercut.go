package tasks

import (
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/db"
	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

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

// PapercutService queries the local Papercut XML-RPC API for each known
// staff member and student, then POSTs the collected data to the tenant.
// Runs every 30 minutes.
func PapercutService() {
	slog.Info("papercut: starting")

	cfg := config.Get()
	if !cfg.Papercut.Enabled {
		slog.Info("papercut: disabled in config, skipping")
		return
	}

	client := tenant.New()
	if err := client.TestConnectivity(); err != nil {
		slog.Error("papercut: connectivity check failed", "err", err)
		return
	}

	payload := pcPayload{}

	// --- Staff ---
	staff, err := db.GetStaff()
	if err != nil {
		slog.Error("papercut: failed to query staff", "err", err)
	}
	slog.Debug("papercut: staff records to process", "count", len(staff))
	for _, s := range staff {
		slog.Debug("papercut: querying staff member", "username", s.StaffCode)
		pin, err := pcGetProperty(cfg.Papercut.APIURL, cfg.Papercut.APIKey, s.StaffCode, "secondary-card-number")
		if err != nil {
			slog.Debug("papercut: PIN lookup failed", "username", s.StaffCode, "err", err)
		}
		bal, err := pcGetPropertyFloat(cfg.Papercut.APIURL, cfg.Papercut.APIKey, s.StaffCode, "balance")
		if err != nil {
			slog.Debug("papercut: balance lookup failed", "username", s.StaffCode, "err", err)
		}
		slog.Debug("papercut: staff result", "username", s.StaffCode, "has_pin", pin != nil, "has_balance", bal != nil)

		if pin == nil && bal == nil {
			continue
		}
		rec := pcUserRecord{Username: s.StaffCode, PIN: pin, Balance: bal}
		payload.Staff = append(payload.Staff, rec)
		slog.Info("papercut: processed staff", "username", s.StaffCode)
	}

	// --- Students ---
	students, err := db.GetStudents()
	if err != nil {
		slog.Error("papercut: failed to query students", "err", err)
	}
	slog.Debug("papercut: student records to process", "count", len(students))
	for _, s := range students {
		slog.Debug("papercut: querying student", "login", s.Login)
		pin, err := pcGetProperty(cfg.Papercut.APIURL, cfg.Papercut.APIKey, s.Login, "secondary-card-number")
		if err != nil {
			slog.Debug("papercut: PIN lookup failed", "login", s.Login, "err", err)
		}
		bal, err := pcGetPropertyFloat(cfg.Papercut.APIURL, cfg.Papercut.APIKey, s.Login, "balance")
		if err != nil {
			slog.Debug("papercut: balance lookup failed", "login", s.Login, "err", err)
		}
		slog.Debug("papercut: student result", "login", s.Login, "has_pin", pin != nil, "has_balance", bal != nil)

		if pin == nil && bal == nil {
			continue
		}
		rec := pcUserRecord{Login: s.Login, PIN: pin, Balance: bal}
		payload.Students = append(payload.Students, rec)
		slog.Info("papercut: processed student", "login", s.Login)
	}

	if len(payload.Staff) == 0 && len(payload.Students) == 0 {
		slog.Info("papercut: no data to send")
		return
	}

	resp, err := client.PostJSON(tenant.URL("/api/agent/ingest/papercut-data"), payload)
	if err != nil {
		slog.Error("papercut: failed to send data", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("papercut: data sent", "staff", len(payload.Staff), "students", len(payload.Students))
}

// ---------------------------------------------------------------------------
// Papercut XML-RPC helpers
// ---------------------------------------------------------------------------

// pcXMLRequest builds an XML-RPC getUserProperty request body.
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

// pcValue holds the inner XML of a <value> element so we can handle both
// <value><string>x</string></value> and bare <value>x</value>.
type pcValue struct {
	InnerXML string `xml:",innerxml"`
}

type pcFault struct {
	Value string `xml:"value"`
}

func pcCall(apiURL, apiKey, username, property string) (string, error) {
	body := pcXMLRequest(apiKey, username, property)
	resp, err := http.Post(apiURL, "text/xml; charset=UTF-8", strings.NewReader(body)) //nolint:noctx
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

	// InnerXML may be "<string>1234</string>" or just "1234"
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

// pcGetProperty returns a numeric string value, or nil if not present/numeric.
func pcGetProperty(apiURL, apiKey, username, property string) (*string, error) {
	val, err := pcCall(apiURL, apiKey, username, property)
	if err != nil {
		return nil, err
	}
	if !isNumericString(val) {
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

func isNumericString(s string) bool {
	if s == "" {
		return false
	}
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return err == nil
}
