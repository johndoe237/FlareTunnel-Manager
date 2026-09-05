// Package cloudflare is the manager's Cloudflare logic layer.
//
// It only implements what the manager needs to decide: listing the Workers of
// an account and counting those that follow the FlareTunnel naming convention
// ("flaretunnel-*"). Everything else is delegated to FlareTunnel.
package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// FlareTunnelPrefix is the naming convention used by FlareTunnel for its
// Workers. Only Workers whose name starts with this prefix are counted as
// FlareTunnel Workers.
const FlareTunnelPrefix = "flaretunnel-"

// DefaultBaseURL is the Cloudflare API v4 base URL.
const DefaultBaseURL = "https://api.cloudflare.com/client/v4"

// Client queries the Cloudflare API for a single account.
type Client struct {
	APIToken  string
	AccountID string
	BaseURL   string
	HTTP      *http.Client
}

// New returns a Cloudflare client for one account. The API base URL can be
// overridden with the optional CF_API_BASE_URL environment variable (defaults
// to the real Cloudflare API v4 endpoint).
func New(apiToken, accountID string) *Client {
	baseURL := DefaultBaseURL
	if v := os.Getenv("CF_API_BASE_URL"); v != "" {
		baseURL = v
	}
	return &Client{
		APIToken:  apiToken,
		AccountID: accountID,
		BaseURL:   baseURL,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
	}
}

// ListFlareTunnelWorkers returns the names of the Workers of the account that
// follow the FlareTunnel naming convention ("flaretunnel-*"). Other Workers
// of the account are ignored.
func (c *Client) ListFlareTunnelWorkers(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/accounts/%s/workers/scripts", c.BaseURL, c.AccountID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloudflare list workers failed: status %d: %s", resp.StatusCode, truncate(redact(string(body), c.APIToken), 300))
	}

	var result struct {
		Success bool `json:"success"`
		Result  []struct {
			ID string `json:"id"`
		} `json:"result"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("cloudflare response is invalid: %w", err)
	}
	if !result.Success {
		msg := "unknown error"
		if len(result.Errors) > 0 {
			msg = result.Errors[0].Message
		}
		return nil, fmt.Errorf("cloudflare list workers failed: %s", redact(msg, c.APIToken))
	}

	names := make([]string, 0, len(result.Result))
	for _, w := range result.Result {
		if strings.HasPrefix(w.ID, FlareTunnelPrefix) {
			names = append(names, w.ID)
		}
	}
	return names, nil
}

// CountFlareTunnelWorkers returns the number of FlareTunnel Workers of the
// account.
func (c *Client) CountFlareTunnelWorkers(ctx context.Context) (int, error) {
	names, err := c.ListFlareTunnelWorkers(ctx)
	if err != nil {
		return 0, err
	}
	return len(names), nil
}

func redact(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "[REDACTED]")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
