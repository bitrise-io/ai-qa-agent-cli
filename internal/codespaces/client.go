package codespaces

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a thin HTTP wrapper around the bitrise-codespaces gRPC-gateway.
// Requests and responses are plain Go structs with json tags that match
// protojson's lowerCamelCase wire format — see types.go.
type Client struct {
	baseURL string
	pat     string
	http    *http.Client
}

// NewClient constructs a Client. baseURL must be an absolute URL with scheme
// (e.g. https://codespaces-api.services.bitrise.io or http://localhost:8081).
func NewClient(baseURL, pat string) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if pat == "" {
		return nil, fmt.Errorf("PAT is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid base URL %q (need scheme://host[:port])", baseURL)
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		pat:     pat,
		http:    &http.Client{Timeout: 10 * time.Minute},
	}, nil
}

// Close is a no-op; kept so callers can defer it without caring whether the
// underlying transport is gRPC or HTTP.
func (c *Client) Close() error { return nil }

// do issues a JSON request to a path relative to the base URL. body and resp
// are optional; pass nil to skip either side. Non-2xx responses are wrapped
// in *httpError so FormatError can expand them.
func (c *Client) do(ctx context.Context, method, relPath string, body, resp any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal %s body: %w", relPath, err)
		}
		rdr = bytes.NewReader(raw)
	}

	fullURL := c.baseURL + relPath
	req, err := http.NewRequestWithContext(ctx, method, fullURL, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.pat)

	httpResp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, fullURL, err)
	}
	defer httpResp.Body.Close()

	raw, readErr := io.ReadAll(httpResp.Body)
	if readErr != nil {
		return fmt.Errorf("%s %s: read response: %w", method, fullURL, readErr)
	}
	if httpResp.StatusCode/100 != 2 {
		return &httpError{
			Method:     method,
			URL:        fullURL,
			StatusCode: httpResp.StatusCode,
			Body:       raw,
		}
	}
	if resp != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, resp); err != nil {
			return fmt.Errorf("%s %s: unmarshal response: %w", method, fullURL, err)
		}
	}
	return nil
}
