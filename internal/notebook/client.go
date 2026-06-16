// Copyright © 2026 ForcePoint Software. All rights reserved.

// Package notebook implements an authenticated HTTP client for the
// DET Notebooks API at https://apps.edustar.vic.edu.au/notebooks.
//
// Two authentication modes are supported, mirroring DETNotebookService.php:
//
//   - NTLM  — used from outside the eduSTAR subnet (e.g. Citrix VPN).
//     Credentials are sent with every request.
//   - Form  — used from within the eduSTAR subnet (school network).
//     A session cookie is obtained via a BigIP login handshake.
//
// Login() tries NTLM first and falls back to form-based authentication.
// The auth logic mirrors internal/edustar/client.go — only the URLs differ.
package notebook

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"

	"github.com/Azure/go-ntlmssp"
)

const (
	baseURL   = "https://apps.edustar.vic.edu.au/notebooks"
	policyURL = "https://apps.edustar.vic.edu.au/my.policy"
	apiBase   = "https://apps.edustar.vic.edu.au/notebooks/api"
	domain    = "EDU001"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
)

// Client is an authenticated DET Notebooks API client.
type Client struct {
	formClient *http.Client // cookie-based session client for form auth
	ntlmClient *http.Client // transport-level NTLM client (no jar — credentials sent per-request)
	AuthMode   string       // "ntlm" or "form" — set after successful Login
	forcedMode string       // "" (auto), "ntlm", or "form" — set at construction
	username   string
	password   string
}

// New creates an unauthenticated DET Notebooks client.
// authMode controls which authentication method Login uses:
// "ntlm" or "form" force that method; "" (empty) tries NTLM first then falls back to form.
// Call Login before making API requests.
func New(authMode string) *Client {
	jar, _ := cookiejar.New(nil)

	tlsCfg := &tls.Config{InsecureSkipVerify: true} //nolint:gosec // eduSTAR uses self-signed/F5 certs

	// ntlmClient needs its own separate jar. The notebooks F5 BigIP sets a session
	// cookie during the NTLM handshake that must accompany subsequent API requests —
	// unlike STMC which accepts per-request NTLM with no session cookie. The jars
	// must be separate so that a failed NTLM attempt's partial F5 APM cookie does
	// not bleed into the form auth flow and trigger "access policy already in progress".
	ntlmJar, _ := cookiejar.New(nil)

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
			Jar: ntlmJar,
		},
	}
}

// Login authenticates with the DET Notebooks service using the auth mode specified at construction.
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

// ntlmLogin authenticates by performing an NTLM-negotiated GET to the notebooks base URL.
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

// formLogin authenticates via the F5 BigIP handshake: GET base URL to seed session cookies,
// then POST credentials to /my.policy.
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
		return fmt.Errorf("form: unexpected response from DET Notebooks")
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

	// The F5 BigIP redirects to the notebooks app after a successful login, so the
	// response body is the notebooks app HTML — not the STMC app — and will not
	// contain "prism". Only check for explicit failure indicators.
	retryRe := regexp.MustCompile(`"retry"\s*:\s*\{[^}]*?"message"\s*:\s*"([^"]+)"`)
	if m := retryRe.FindStringSubmatch(data); len(m) > 1 {
		return fmt.Errorf("form login failed: %s", m[1])
	}
	if strings.Contains(data, "Invalid credentials") ||
		strings.Contains(data, "Login failed") ||
		strings.Contains(data, "Authentication failed") {
		return fmt.Errorf("form login failed: invalid credentials")
	}

	c.username = username
	c.password = password
	c.AuthMode = "form"
	return nil
}

// request sends an authenticated request to the DET Notebooks API and returns the raw response body.
func (c *Client) request(method, path string, data any) ([]byte, error) {
	var bodyReader io.Reader
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	fullURL := apiBase + path
	slog.Debug("notebooks: request", "method", strings.ToUpper(method), "url", fullURL)

	req, err := http.NewRequest(strings.ToUpper(method), fullURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")

	var resp *http.Response
	if c.AuthMode == "ntlm" {
		req.SetBasicAuth(domain+`\`+c.username, c.password)
		resp, err = c.ntlmClient.Do(req)
	} else {
		resp, err = c.formClient.Do(req)
	}
	if err != nil {
		return nil, fmt.Errorf("notebooks %s %s: %w", method, fullURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("notebooks %s %s returned %d: %s", method, fullURL, resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// decode unmarshals b into a new value of type T.
func decode[T any](b []byte) (T, error) {
	var v T
	return v, json.Unmarshal(b, &v)
}

// FleetRecord represents a single device record from the DET Notebooks current-fleet endpoint.
type FleetRecord struct {
	DeviceID            any    `json:"deviceId"`
	ModelID             any    `json:"modelId"`
	ModelName           string `json:"modelName"`
	Emplid              string `json:"emplid"`
	EmployeeName        string `json:"employeeName"`
	IsEligible          any    `json:"isEligible"`
	WaitListed          any    `json:"waitListed"`
	HasDevice           any    `json:"hasDevice"`
	GrantDevice         any    `json:"grantDevice"`
	HasTrancheEnded     any    `json:"hasTrancheEnded"`
	TrancheID           any    `json:"trancheId"`
	TrancheName         string `json:"trancheName"`
	TrancheEndDate      string `json:"trancheEndDate"`
	SerialNumber        string `json:"serialNumber"`
	CommentID           any    `json:"commentId"`
	Comment             string `json:"comment"`
	CommentCreatedOn    any    `json:"commentCreatedOn"`
	CommentCreatedBy    any    `json:"commentCreatedBy"`
	PreferredPlatformID any    `json:"preferredPlatformId"`
	PreferredPlatform   string `json:"preferredPlatform"`
	SchoolNumber        any    `json:"schoolNumber"`
	EmpSchoolNumber     any    `json:"empSchoolNumber"`
	StatusID            any    `json:"statusId"`
	StatusName          string `json:"statusName"`
	ProvisionDeviceID   any    `json:"provisionDeviceId"`
	ProvisionDeviceName any    `json:"provisionDeviceName"`
	CurrentDeviceCount  any    `json:"currentDeviceCount"`
}

// GetCurrentFleet returns all notebook/device records for the given school.
// Maps to GET /api/schools/{schoolId}/current-fleet.
func (c *Client) GetCurrentFleet(schoolID string) ([]FleetRecord, error) {
	b, err := c.request("GET", "/schools/"+url.PathEscape(schoolID)+"/current-fleet", nil)
	if err != nil {
		return nil, err
	}
	return decode[[]FleetRecord](b)
}

// GetSchools returns the schools available to the authenticated user.
// Maps to GET /api/schools.
func (c *Client) GetSchools() ([]map[string]any, error) {
	b, err := c.request("GET", "/schools", nil)
	if err != nil {
		return nil, err
	}
	return decode[[]map[string]any](b)
}
