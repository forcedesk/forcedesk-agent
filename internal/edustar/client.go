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

// New creates an unauthenticated STMC client.
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

func decode[T any](b []byte) (T, error) {
	var v T
	return v, json.Unmarshal(b, &v)
}

// ============================================================
// API methods — mirrors EduStarService.php
// ============================================================

func (c *Client) WhoAmI() (map[string]any, error) {
	b, err := c.request("GET", "/UserGet", "", nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

func (c *Client) GetUser(id string) (map[string]any, error) {
	b, err := c.request("GET", "/UserGetByLogin/"+url.PathEscape(id), "", nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

func (c *Client) GetSchools() ([]map[string]any, error) {
	b, err := c.request("GET", "/GetAllSchools", "", nil)
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}

func (c *Client) GetAllSchools() ([]string, error) {
	b, err := c.request("GET", "/SchGetAllEnabledIds", "", nil)
	if err != nil {
		return nil, err
	}
	return decode[[]string](b)
}

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

func (c *Client) GetTechnicians(school string) (map[string]any, error) {
	b, err := c.request("GET", "/SchGetTechs?includeStaff=false", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

func (c *Client) GetGroups(school string) (map[string]any, error) {
	b, err := c.request("GET", "/GpGetForSch", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

func (c *Client) GetGroup(school, name, dn string) ([]map[string]any, error) {
	path := fmt.Sprintf("/GpGetMems?gpDn=%s&gpName=%s", url.QueryEscape(dn), url.QueryEscape(name))
	b, err := c.request("GET", path, school, nil)
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}

func (c *Client) GetCertificates(school string) ([]map[string]any, error) {
	b, err := c.request("GET", "/CompGetMg", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}

func (c *Client) GetServiceAccounts(school string) ([]map[string]any, error) {
	b, err := c.request("GET", "/SvcAccGetForSch", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}

func (c *Client) GetNps(school string) (map[string]any, error) {
	b, err := c.request("GET", "/NpsMappingGetForSch", school, nil)
	if err != nil {
		return nil, err
	}
	return decode[map[string]any](b)
}

func (c *Client) SetStudentPassword(school, dn, password string) error {
	_, err := c.request("POST", "/StudResetPwd", school, map[string]string{"dn": dn, "newPwd": password})
	return err
}

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

func (c *Client) AddToGroup(school, groupDN, memberDN string) error {
	_, err := c.request("POST", "/GpAddMem", school, map[string]string{"gpDn": groupDN, "memDn": memberDN})
	return err
}

func (c *Client) RemoveFromGroup(school, groupDN, memberDN string) error {
	_, err := c.request("POST", "/GpRemoveMem", school, map[string]string{"gpDn": groupDN, "memDn": memberDN})
	return err
}

func (c *Client) DisableServiceAccount(school, dn string) error {
	_, err := c.request("POST", "/SvcAccDisable", school, map[string]string{"dn": dn})
	return err
}

func (c *Client) EnableServiceAccount(school, dn string) error {
	_, err := c.request("POST", "/SvcAccEnable", school, map[string]string{"dn": dn})
	return err
}
