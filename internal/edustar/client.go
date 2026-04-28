// Copyright © 2026 ForcePoint Software. All rights reserved.

// Package edustar implements an authenticated HTTP client for the
// F5 eduSTAR School Technology Management Centre (STMC) API at
// https://stmc.education.vic.gov.au.
//
// Two authentication modes are supported, mirroring EduStarService.php:
//
//   - NTLM  — used from outside the eduSTAR subnet (e.g. Citrix VPN).
//     Credentials are sent with every request.
//   - Form  — used from within the eduSTAR subnet (school network).
//     A session cookie is obtained via a login handshake.
//
// Login() tries NTLM first and falls back to form-based authentication.
package edustar

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"

	"github.com/Azure/go-ntlmssp"
)

const (
	baseURL   = "https://stmc.education.vic.gov.au"
	policyURL = "https://stmc.education.vic.gov.au/my.policy"
	apiBase   = "https://stmc.education.vic.gov.au/api"
	domain    = "EDU001"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
)

// Client is an authenticated STMC API client.
type Client struct {
	formClient *http.Client // cookie-based session client for form auth
	ntlmClient *http.Client // transport-level NTLM client
	AuthMode   string       // "ntlm" or "form" — set after successful Login
	forcedMode string       // "" (auto), "ntlm", or "form" — set at construction
	username   string
	password   string
}

// New creates an unauthenticated STMC client..
// authMode controls which authentication method Login uses:
// "ntlm" or "form" force that method; "" (empty) tries NTLM first then falls back to form.
// Call Login before making API requests.
func New(authMode string) *Client {
	jar, _ := cookiejar.New(nil)

	tlsCfg := &tls.Config{InsecureSkipVerify: true} //nolint:gosec // STMC uses self-signed certs

	return &Client{
		forcedMode: authMode,
		formClient: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
			Jar:       jar,
		},
		ntlmClient: &http.Client{
			Transport: ntlmssp.Negotiator{
				RoundTripper: &http.Transport{TLSClientConfig: tlsCfg},
			},
		},
	}
}

// Login authenticates with STMC using the auth mode specified at construction.
// With "" (auto) it tries NTLM first and falls back to form-based auth.
func (c *Client) Login(username, password string) error {
	switch c.forcedMode {
	case "ntlm":
		return c.ntlmLogin(username, password)
	case "form":
		return c.formLogin(username, password)
	default:
		if err := c.ntlmLogin(username, password); err == nil {
			return nil
		}
		return c.formLogin(username, password)
	}
}

// ntlmLogin authenticates by performing an NTLM-negotiated GET to the STMC base URL.
// Credentials are prepended with the domain ("EDU001\") as required by the F5 gateway.
func (c *Client) ntlmLogin(username, password string) error {
	req, err := http.NewRequest(http.MethodGet, baseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.SetBasicAuth(domain+`\`+username, password)

	resp, err := c.ntlmClient.Do(req)
	if err != nil {
		return fmt.Errorf("NTLM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("NTLM: server returned %d", resp.StatusCode)
	}

	c.username = username
	c.password = password
	c.AuthMode = "ntlm"
	return nil
}

// formLogin authenticates via a two-step form handshake: GET base URL to seed session
// cookies, then POST credentials to /my.policy. Only works from within the eduSTAR subnet.
func (c *Client) formLogin(username, password string) error {
	// Step 1: GET base URL to initialise session cookies.
	initReq, _ := http.NewRequest(http.MethodGet, baseURL, nil)
	initReq.Header.Set("User-Agent", userAgent)

	initResp, err := c.formClient.Do(initReq)
	if err != nil {
		return fmt.Errorf("form init: %w", err)
	}
	body, _ := io.ReadAll(initResp.Body)
	initResp.Body.Close()

	if strings.Contains(string(body), "403 Forbidden") {
		return fmt.Errorf("form: 403 Forbidden — access only allowed from Citrix VPN or the eduSTAR subnet")
	}
	if !strings.Contains(string(body), "Department of Education") {
		return fmt.Errorf("form: unexpected response from STMC")
	}

	// Step 2: POST credentials to /my.policy.
	formData := url.Values{"username": {username}, "password": {password}}
	loginReq, _ := http.NewRequest(http.MethodPost, policyURL, strings.NewReader(formData.Encode()))
	loginReq.Header.Set("User-Agent", userAgent)
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("Referer", baseURL)

	loginResp, err := c.formClient.Do(loginReq)
	if err != nil {
		return fmt.Errorf("form login POST: %w", err)
	}
	loginBody, _ := io.ReadAll(loginResp.Body)
	loginResp.Body.Close()

	data := string(loginBody)

	retryRe := regexp.MustCompile(`"retry"\s*:\s*\{[^}]*?"message"\s*:\s*"([^"]+)"`)
	if m := retryRe.FindStringSubmatch(data); len(m) > 1 {
		return fmt.Errorf("form login failed: %s", m[1])
	}
	if !strings.Contains(data, "prism") {
		return fmt.Errorf("form login failed: unspecified failure")
	}

	c.username = username
	c.password = password
	c.AuthMode = "form"
	return nil
}

// request sends an authenticated request to the STMC API and returns the raw response body.
// The school ID, when non-empty, is sent as the emc-sch-id request header.
func (c *Client) request(method, path, school string, data any) ([]byte, error) {
	var bodyReader io.Reader
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(strings.ToUpper(method), apiBase+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	if school != "" {
		req.Header.Set("emc-sch-id", school)
	}

	var resp *http.Response
	if c.AuthMode == "ntlm" {
		req.SetBasicAuth(domain+`\`+c.username, c.password)
		resp, err = c.ntlmClient.Do(req)
	} else {
		resp, err = c.formClient.Do(req)
	}
	if err != nil {
		return nil, fmt.Errorf("STMC %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("STMC %s %s returned %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// decode unmarshals b into a new value of type T.
func decode[T any](b []byte) (T, error) {
	var v T
	return v, json.Unmarshal(b, &v)
}

// ============================================================
// API methods — mirrors EduStarService.php
// ============================================================

// WhoAmI returns the currently authenticated user's profile. (GET /UserGet)
func (c *Client) WhoAmI() (map[string]any, error) {
	b, err := c.request("GET", "/UserGet", "", nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

// GetUser returns a user by their TO number or alias. (GET /UserGetByLogin/{id})
func (c *Client) GetUser(id string) (map[string]any, error) {
	b, err := c.request("GET", "/UserGetByLogin/"+url.PathEscape(id), "", nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

// GetSchools returns all schools the authenticated user can manage. (GET /GetAllSchools)
func (c *Client) GetSchools() ([]map[string]any, error) {
	b, err := c.request("GET", "/GetAllSchools", "", nil)
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}

// GetAllSchools returns the IDs of all enabled schools. (GET /SchGetAllEnabledIds)
func (c *Client) GetAllSchools() ([]string, error) {
	b, err := c.request("GET", "/SchGetAllEnabledIds", "", nil)
	if err != nil {
		return nil, err
	}
	return decode[[]string](b)
}

// GetStudents returns all students with full properties for the given school. (GET /SchGetStuds?fullProps=true)
func (c *Client) GetStudents(school string) ([]map[string]any, error) {
	b, err := c.request("GET", "/SchGetStuds?fullProps=true", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}

// GetStaff returns both technicians and staff. The response contains "techs" and "staff" keys.
func (c *Client) GetStaff(school string) (map[string]any, error) {
	b, err := c.request("GET", "/SchGetTechs?includeStaff=true", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

// GetTechnicians returns only the technicians (not staff) for the given school. (GET /SchGetTechs?includeStaff=false)
func (c *Client) GetTechnicians(school string) (map[string]any, error) {
	b, err := c.request("GET", "/SchGetTechs?includeStaff=false", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

// GetGroups returns all groups for the given school. (GET /GpGetForSch)
func (c *Client) GetGroups(school string) (map[string]any, error) {
	b, err := c.request("GET", "/GpGetForSch", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

// GetGroup returns the members of a specific group identified by DN and name. (GET /GpGetMems)
func (c *Client) GetGroup(school, name, dn string) ([]map[string]any, error) {
	path := fmt.Sprintf("/GpGetMems?gpDn=%s&gpName=%s", url.QueryEscape(dn), url.QueryEscape(name))
	b, err := c.request("GET", path, school, nil)
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}

// GetCertificates returns computer group certificates for the given school. (GET /CompGetMg)
func (c *Client) GetCertificates(school string) ([]map[string]any, error) {
	b, err := c.request("GET", "/CompGetMg", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}

// GetServiceAccounts returns all managed service accounts for the given school. (GET /SvcAccGetForSch)
func (c *Client) GetServiceAccounts(school string) ([]map[string]any, error) {
	b, err := c.request("GET", "/SvcAccGetForSch", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}

// GetNps returns the NPS (Network Policy Server) mapping for the given school. (GET /NpsMappingGetForSch)
func (c *Client) GetNps(school string) (map[string]any, error) {
	b, err := c.request("GET", "/NpsMappingGetForSch", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

// SetStudentPassword sets an explicit password on the student identified by DN. (POST /StudResetPwd)
func (c *Client) SetStudentPassword(school, dn, password string) error {
	_, err := c.request("POST", "/StudResetPwd", school, map[string]string{"dn": dn, "newPwd": password})
	return err
}

// ResetStudentPassword resets the student's password to an auto-generated value and
// returns the result details. (POST /StudBulkSetPwd with mode=auto)
func (c *Client) ResetStudentPassword(school, dn string) ([]map[string]any, error) {
	b, err := c.request("POST", "/StudBulkSetPwd", school, map[string]any{
		"mode": "auto",
		"dns":  []string{dn},
	})
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}

// AddToGroup adds a member to a group by their respective DNs. (POST /GpAddMem)
func (c *Client) AddToGroup(school, groupDN, memberDN string) error {
	_, err := c.request("POST", "/GpAddMem", school, map[string]string{"gpDn": groupDN, "memDn": memberDN})
	return err
}

// RemoveFromGroup removes a member from a group by their respective DNs. (POST /GpRemoveMem)
func (c *Client) RemoveFromGroup(school, groupDN, memberDN string) error {
	_, err := c.request("POST", "/GpRemoveMem", school, map[string]string{"gpDn": groupDN, "memDn": memberDN})
	return err
}

// AddCertificate requests a managed computer certificate for name at school.
// The computer name is stored in STMC as "{school}-{name}" (e.g. "8185-COMPUTERNAME").
// domain is used as the encryption password (pass "eduSTAR.NET"). (POST /CompAddMg)
func (c *Client) AddCertificate(school, name, domain string) error {
	compName := school + "-" + name
	_, err := c.request("POST", "/CompAddMg", school, map[string]string{"compName": compName, "pwd": domain})
	return err
}

// GetCertificate downloads the managed computer certificate for compName as a base64-encoded ZIP.
// compName must include the school prefix (e.g. "8185-COMPUTERNAME").
// domain is the decryption password (pass "eduSTAR.NET"). (POST /CompGetCert)
// The STMC API may return the base64 string wrapped in a JSON object or as a bare string;
// both forms are handled transparently.
func (c *Client) GetCertificate(school, compName, domain string) (string, error) {
	b, err := c.request("POST", "/CompGetCert", school, map[string]string{"compName": compName, "pwd": domain})
	if err != nil {
		return "", err
	}
	// Try a bare JSON string literal first: the API returns "base64..." (quotes included).
	var s string
	if json.Unmarshal(b, &s) == nil && s != "" {
		return s, nil
	}
	// Try a JSON object envelope — the API may return {"data": "..."} or similar.
	var wrapper map[string]any
	if json.Unmarshal(b, &wrapper) == nil {
		for _, key := range []string{"data", "cert", "certificate", "content"} {
			if v, ok := wrapper[key].(string); ok && v != "" {
				return v, nil
			}
		}
		// Single-key object: return its string value.
		if len(wrapper) == 1 {
			for _, v := range wrapper {
				if v, ok := v.(string); ok {
					return v, nil
				}
			}
		}
	}
	// Plain text response — return as-is.
	return string(b), nil
}

// DisableServiceAccount disables the service account identified by DN. (POST /SvcAccDisable)
func (c *Client) DisableServiceAccount(school, dn string) error {
	_, err := c.request("POST", "/SvcAccDisable", school, map[string]string{"dn": dn})
	return err
}

// EnableServiceAccount re-enables the service account identified by DN. (POST /SvcAccEnable)
func (c *Client) EnableServiceAccount(school, dn string) error {
	_, err := c.request("POST", "/SvcAccEnable", school, map[string]string{"dn": dn})
	return err
}
