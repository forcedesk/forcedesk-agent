package tasks

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/tenant"
)

// pcCallWithBody sends a raw XML-RPC body to apiURL and returns the scalar string value
// from the response, stripping any XML-RPC type wrapper tags (string, double, int, boolean).
func pcCallWithBody(apiURL, body string) (string, error) {
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

	inner := strings.TrimSpace(result.Params.Param.Value.InnerXML)
	inner = stripXMLTag(inner, "string")
	inner = stripXMLTag(inner, "double")
	inner = stripXMLTag(inner, "int")
	inner = stripXMLTag(inner, "boolean")
	return strings.TrimSpace(inner), nil
}

// pcSharedAccountsListResponse is the XML-RPC response envelope for api.listSharedAccounts,
// which returns an array of account name strings.
type pcSharedAccountsListResponse struct {
	Params *pcSharedAccountsListParams `xml:"params"`
	Fault  *pcFault                    `xml:"fault"`
}

type pcSharedAccountsListParams struct {
	Param struct {
		Value struct {
			Array struct {
				Items []pcSharedAccountItem `xml:"data>value"`
			} `xml:"array"`
		} `xml:"value"`
	} `xml:"param"`
}

// pcSharedAccountItem holds a single <value> element from the array response.
// InnerXML captures both bare values and type-wrapped values (e.g. <string>…</string>).
type pcSharedAccountItem struct {
	InnerXML string `xml:",innerxml"`
}

// pcListSharedAccounts calls api.listSharedAccounts and returns all account names.
func pcListSharedAccounts(apiURL, apiKey string) ([]string, error) {
	body := fmt.Sprintf(`<?xml version="1.0"?>
<methodCall>
  <methodName>api.listSharedAccounts</methodName>
  <params>
    <param><value><string>%s</string></value></param>
    <param><value><int>0</int></value></param>
    <param><value><int>9999</int></value></param>
  </params>
</methodCall>`, xmlEscape(apiKey))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=UTF-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result pcSharedAccountsListResponse
	if err := xml.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("xml parse: %w", err)
	}
	if result.Fault != nil {
		return nil, fmt.Errorf("XML-RPC fault: %s", result.Fault.Value)
	}
	if result.Params == nil {
		return nil, fmt.Errorf("empty XML-RPC response")
	}

	var accounts []string
	for _, item := range result.Params.Param.Value.Array.Items {
		s := strings.TrimSpace(item.InnerXML)
		s = stripXMLTag(s, "string")
		s = html.UnescapeString(strings.TrimSpace(s))
		if s != "" {
			accounts = append(accounts, s)
		}
	}
	return accounts, nil
}

// pcGetSharedAccountBalance calls api.getSharedAccountAccountBalance and returns the balance.
func pcGetSharedAccountBalance(apiURL, apiKey, accountName string) (float64, error) {
	body := fmt.Sprintf(`<?xml version="1.0"?>
<methodCall>
  <methodName>api.getSharedAccountAccountBalance</methodName>
  <params>
    <param><value><string>%s</string></value></param>
    <param><value><string>%s</string></value></param>
  </params>
</methodCall>`, xmlEscape(apiKey), xmlEscape(accountName))

	val, err := pcCallWithBody(apiURL, body)
	if err != nil {
		return 0, err
	}
	var f float64
	if _, err := fmt.Sscanf(val, "%f", &f); err != nil {
		return 0, fmt.Errorf("parse balance %q: %w", val, err)
	}
	return f, nil
}

// pcSetSharedAccountBalance calls api.setSharedAccountAccountBalance.
// Returns an error if the server does not confirm success.
func pcSetSharedAccountBalance(apiURL, apiKey, accountName string, balance float64, reason string) error {
	body := fmt.Sprintf(`<?xml version="1.0"?>
<methodCall>
  <methodName>api.setSharedAccountAccountBalance</methodName>
  <params>
    <param><value><string>%s</string></value></param>
    <param><value><string>%s</string></value></param>
    <param><value><double>%g</double></value></param>
    <param><value><string>%s</string></value></param>
  </params>
</methodCall>`, xmlEscape(apiKey), xmlEscape(accountName), balance, xmlEscape(reason))

	val, err := pcCallWithBody(apiURL, body)
	if err != nil {
		return err
	}
	if val != "1" {
		return fmt.Errorf("unexpected response: %q", val)
	}
	return nil
}

// pcSharedAccountRecord is used in the payload sent to the ForceDesk server.
type pcSharedAccountRecord struct {
	Name    string  `json:"name"`
	Balance float64 `json:"balance"`
}

type pcSharedAccountsPayload struct {
	Accounts []pcSharedAccountRecord `json:"accounts"`
}

type pcSharedAccountBalancePayload struct {
	SharedAccount string  `json:"shared_account"`
	Balance       float64 `json:"balance"`
}

// PapercutGetSharedAccounts lists all PaperCut shared accounts, fetches each account's
// balance, then POSTs the collated result to the ForceDesk server.
// Triggered by the "get-papercut-shared-accounts" command queue entry.
func PapercutGetSharedAccounts() {
	slog.Info("papercut: fetching shared accounts")

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

	accounts, err := pcListSharedAccounts(pcCfg.APIURL, pcCfg.APIKey)
	if err != nil {
		slog.Error("papercut: failed to list shared accounts", "err", err)
		return
	}
	slog.Debug("papercut: shared accounts listed", "count", len(accounts))

	payload := pcSharedAccountsPayload{}
	for _, name := range accounts {
		bal, err := pcGetSharedAccountBalance(pcCfg.APIURL, pcCfg.APIKey, name)
		if err != nil {
			slog.Warn("papercut: failed to get balance", "account", name, "err", err)
			continue
		}
		payload.Accounts = append(payload.Accounts, pcSharedAccountRecord{Name: name, Balance: bal})
		slog.Debug("papercut: shared account balance", "account", name, "balance", bal)
	}

	key, err := config.Get().Tenant.GetEncryptionKey()
	if err != nil {
		slog.Error("papercut: failed to get encryption key", "err", err)
		return
	}

	resp, err := client.PostEncryptedJSON(tenant.URL("/api/agent/ingest/papercut-shared-accounts"), payload, key)
	if err != nil {
		slog.Error("papercut: failed to send shared accounts", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("papercut: shared accounts sent", "count", len(payload.Accounts))
}

// PapercutSetSharedAccountBalance sets the balance on a PaperCut shared account,
// then fetches the updated balance and POSTs it back to the ForceDesk server.
// Triggered by the "set-papercut-shared-account-balance" command queue entry.
func PapercutSetSharedAccountBalance(accountName string, requestedBalance float64, adjustmentReason string) {
	slog.Info("papercut: setting shared account balance", "account", accountName, "balance", requestedBalance)

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

	currentBalance, err := pcGetSharedAccountBalance(pcCfg.APIURL, pcCfg.APIKey, accountName)
	if err != nil {
		slog.Error("papercut: failed to get current balance before top-up", "account", accountName, "err", err)
		return
	}
	slog.Debug("papercut: current balance before top-up", "account", accountName, "current", currentBalance, "topup", requestedBalance)

	newBalance := currentBalance + requestedBalance

	if err := pcSetSharedAccountBalance(pcCfg.APIURL, pcCfg.APIKey, accountName, newBalance, adjustmentReason); err != nil {
		slog.Error("papercut: failed to set shared account balance", "account", accountName, "err", err)
		return
	}
	slog.Info("papercut: shared account balance set", "account", accountName, "old", currentBalance, "topup", requestedBalance, "new", newBalance)

	bal, err := pcGetSharedAccountBalance(pcCfg.APIURL, pcCfg.APIKey, accountName)
	if err != nil {
		slog.Error("papercut: failed to get updated balance", "account", accountName, "err", err)
		return
	}

	key, err := config.Get().Tenant.GetEncryptionKey()
	if err != nil {
		slog.Error("papercut: failed to get encryption key", "err", err)
		return
	}

	payload := pcSharedAccountBalancePayload{SharedAccount: accountName, Balance: bal}
	resp, err := client.PostEncryptedJSON(tenant.URL("/api/agent/ingest/papercut-shared-account-balance"), payload, key)
	if err != nil {
		slog.Error("papercut: failed to send updated balance", "err", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("papercut: shared account balance updated and sent", "account", accountName, "balance", bal)
}
