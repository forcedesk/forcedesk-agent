package tenant

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"time"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/ratelimit"
)

const AgentVersion = "2.0.44-golang-win32"

// Client is a thin wrapper around http.Client that automatically applies
// agent authentication headers (API key, UUID, version) to every request.
// Includes rate limiting to prevent abuse.
type Client struct {
	http    *http.Client
	limiter *ratelimit.Limiter
}

// New creates a Client using the current agent configuration.
// SSL verification can be disabled via the VerifySSL config option.
func New() *Client {
	cfg := config.Get()

	// Warn if SSL verification is disabled.
	if !cfg.Tenant.VerifySSL {
		slog.Warn("SSL certificate verification is DISABLED - connections are vulnerable to MITM attacks",
			"tenant_url", cfg.Tenant.URL)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !cfg.Tenant.VerifySSL, //nolint:gosec
		},
	}

	// Rate limiter: max 100 requests, refill 1 token every 100ms (600/min max).
	limiter := ratelimit.NewLimiter(100, 100*time.Millisecond)

	return &Client{
		http: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		limiter: limiter,
	}
}

// URL constructs a full URL by prepending the configured tenant base URL to the given path.
func URL(path string) string {
	return config.Get().Tenant.URL + path
}

func (c *Client) applyHeaders(req *http.Request) {
	cfg := config.Get()
	req.Header.Set("Authorization", "Bearer "+cfg.Tenant.GetAPIKey())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ForceDeskAgent\\v"+AgentVersion)
	req.Header.Set("x-forcedesk-agent", cfg.Tenant.UUID)
	req.Header.Set("x-forcedesk-agentversion", AgentVersion)
	req.Header.Set("Accept", "application/json")
}

// Get performs an authenticated GET request to the given URL.
func (c *Client) Get(url string) (*http.Response, error) {
	// Apply rate limiting.
	if !c.limiter.Allow() {
		slog.Warn("rate limit reached, throttling request", "url", url)
		c.limiter.Wait()
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	return c.http.Do(req)
}

// PostJSON performs an authenticated POST request with the provided value serialized as JSON in the request body.
func (c *Client) PostJSON(url string, v any) (*http.Response, error) {
	// Apply rate limiting.
	if !c.limiter.Allow() {
		slog.Warn("rate limit reached, throttling request", "url", url)
		c.limiter.Wait()
	}

	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	// Log request details without exposing sensitive data.
	slog.Debug("tenant: POST request", "url", url, "body_size", len(body))
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	return c.http.Do(req)
}

// GetJSON performs an authenticated GET request and unmarshals the JSON response body into dst.
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

// GetEncryptedJSON performs an authenticated GET request whose response body is encrypted
// with ChaCha20-Poly1305 using the provided 32-byte mutual key.
// Expected wire format: nonce (12 bytes) || ciphertext+tag — the decrypted payload is JSON
// which is unmarshalled into dst.
func (c *Client) GetEncryptedJSON(url string, dst any, key []byte) error {
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
		return fmt.Errorf("read encrypted response: %w", err)
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}

	ns := aead.NonceSize()
	if len(body) < ns {
		return fmt.Errorf("encrypted response too short (%d bytes)", len(body))
	}

	plaintext, err := aead.Open(nil, body[:ns], body[ns:], nil)
	if err != nil {
		return fmt.Errorf("decrypt response: %w", err)
	}

	return json.Unmarshal(plaintext, dst)
}

// PostEncryptedJSON marshals v as JSON, encrypts it with ChaCha20-Poly1305 using
// the provided 32-byte key, and POSTs the result as application/octet-stream.
// Wire format: nonce (12 bytes) || ciphertext+tag — the inverse of GetEncryptedJSON.
func (c *Client) PostEncryptedJSON(url string, v any, key []byte) (*http.Response, error) {
	if !c.limiter.Allow() {
		slog.Warn("rate limit reached, throttling request", "url", url)
		c.limiter.Wait()
	}

	plaintext, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(ciphertext))
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	req.Header.Set("Content-Type", "application/octet-stream")
	return c.http.Do(req)
}

// PostFile uploads raw bytes as a multipart/form-data POST. The file is sent
// under the field name "file" with the supplied filename.
func (c *Client) PostFile(url, filename string, data []byte) (*http.Response, error) {
	if !c.limiter.Allow() {
		slog.Warn("rate limit reached, throttling request", "url", url)
		c.limiter.Wait()
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := fw.Write(data); err != nil {
		return nil, fmt.Errorf("write file data: %w", err)
	}
	mw.Close()

	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return nil, err
	}
	// Apply auth headers then override Content-Type for multipart.
	c.applyHeaders(req)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return c.http.Do(req)
}

// TestConnectivity verifies that the agent can successfully reach the tenant API server.
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
