package tenant

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/config"
)

const AgentVersion = "2.0.44-golang-win32"

// Client is a thin wrapper around http.Client that automatically applies
// the agent authentication headers to every request.
type Client struct {
	http *http.Client
}

// New creates a Client using the current agent configuration.
func New() *Client {
	cfg := config.Get()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !cfg.Tenant.VerifySSL, //nolint:gosec
		},
	}
	return &Client{
		http: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// URL prepends the configured tenant base URL to path.
func URL(path string) string {
	return config.Get().Tenant.URL + path
}

func (c *Client) applyHeaders(req *http.Request) {
	cfg := config.Get()
	req.Header.Set("Authorization", "Bearer "+cfg.Tenant.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-forcedesk-agent", cfg.Tenant.UUID)
	req.Header.Set("x-forcedesk-agentversion", AgentVersion)
	req.Header.Set("Accept", "application/json")
}

// Get performs an authenticated GET request to the given URL.
func (c *Client) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	return c.http.Do(req)
}

// PostJSON performs an authenticated POST request with v serialised as the
// JSON body.
func (c *Client) PostJSON(url string, v any) (*http.Response, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	slog.Debug("tenant: POST payload", "url", url, "body", string(body))
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	return c.http.Do(req)
}

// GetJSON performs a GET and unmarshals the JSON response body into dst.
func (c *Client) GetJSON(url string, dst any) error {
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

// TestConnectivity verifies that the agent can reach the tenant API.
func (c *Client) TestConnectivity() error {
	var result struct {
		Status string `json:"status"`
	}
	if err := c.GetJSON(URL("/api/agent/test"), &result); err != nil {
		return fmt.Errorf("connectivity test: %w", err)
	}
	if result.Status != "ok" {
		return fmt.Errorf("connectivity test returned status %q", result.Status)
	}
	return nil
}
