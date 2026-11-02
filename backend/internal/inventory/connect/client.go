// Package connect is the "connect" inventory provider: a read-only (GET)
// client for a paged JSON REST API, used to pull Product and Customer
// records for a single branch. The API address comes from configuration
// (INVENTORY_CONNECT_URL), never from code.
package connect

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ecommerce/backend/internal/inventory"
)

type Client struct {
	httpClient  *http.Client
	urlTemplate string
	creds       inventory.Credentials
}

// NewClient builds a client for one branch. urlTemplate is the API's base
// URL with a {host} placeholder for the branch's host.
func NewClient(urlTemplate string, creds inventory.Credentials, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{httpClient: httpClient, urlTemplate: urlTemplate, creds: creds}
}

func (c *Client) baseURL() string {
	u := strings.ReplaceAll(c.urlTemplate, "{host}", url.PathEscape(c.creds.Host))
	if !strings.HasSuffix(u, "/") {
		u += "/"
	}
	return u
}

func (c *Client) authHeader() string {
	raw := c.creds.Username + ":" + c.creds.APIKey
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// envelope is the universal GET response wrapper. The key holding the actual
// records isn't documented consistently, so it's resolved dynamically in
// extractRecords rather than hardcoded to "data".
type envelope struct {
	HTTPCode     int    `json:"http_code"`
	ResponseCode string `json:"response_code"`
	ResponseMsg  string `json:"response_msg"`
	Page         int    `json:"page"`
	PagesTotal   int    `json:"pages_total"`
	PageRecords  int    `json:"page_records"`
	RecordsTotal int    `json:"records_total"`
}

var envelopeKnownKeys = map[string]bool{
	"http_code": true, "response_code": true, "response_msg": true,
	"server_time": true, "page": true, "page_record_limit": true,
	"page_records": true, "pages_total": true, "records_total": true,
	"received_parameters": true,
}

func extractRecords(raw map[string]json.RawMessage) ([]json.RawMessage, error) {
	if d, ok := raw["data"]; ok {
		var records []json.RawMessage
		if err := json.Unmarshal(d, &records); err != nil {
			return nil, fmt.Errorf("unmarshal data array: %w", err)
		}
		return records, nil
	}
	for k, v := range raw {
		if envelopeKnownKeys[k] {
			continue
		}
		trimmed := bytes.TrimSpace(v)
		if len(trimmed) > 0 && trimmed[0] == '[' {
			var records []json.RawMessage
			if err := json.Unmarshal(v, &records); err != nil {
				continue
			}
			return records, nil
		}
	}
	return nil, fmt.Errorf("could not locate a records array in the response")
}

// APIError is a non-transient error returned by the API (4xx other than 429).
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("inventory api error: status %d: %s", e.StatusCode, e.Body)
}

// fetchPage performs a single GET request with retry/backoff on 429 and 5xx.
func (c *Client) fetchPage(ctx context.Context, resource string, params url.Values, page int) (envelope, []json.RawMessage, error) {
	const maxAttempts = 5
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		env, records, retryAfter, err := c.doFetchPage(ctx, resource, params, page)
		if err == nil {
			return env, records, nil
		}
		lastErr = err

		if !isRetryable(err) {
			return envelope{}, nil, err
		}

		if attempt == maxAttempts {
			break
		}

		wait := retryAfter
		if wait <= 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			jitter := time.Duration(rand.Int63n(int64(time.Second)))
			wait = backoff + jitter
			if wait > 60*time.Second {
				wait = 60 * time.Second
			}
		}
		select {
		case <-ctx.Done():
			return envelope{}, nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return envelope{}, nil, fmt.Errorf("giving up after %d attempts: %w", maxAttempts, lastErr)
}

func isRetryable(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		// network-level error (timeout, connection reset, etc.) - retryable
		return true
	}
	return apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= 500
}

func (c *Client) doFetchPage(ctx context.Context, resource string, params url.Values, page int) (envelope, []json.RawMessage, time.Duration, error) {
	q := url.Values{}
	for k, v := range params {
		q[k] = v
	}
	q.Set("page", strconv.Itoa(page))

	reqURL := c.baseURL() + resource + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return envelope{}, nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return envelope{}, nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return envelope{}, nil, 0, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var retryAfter time.Duration
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		return envelope{}, nil, retryAfter, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return envelope{}, nil, 0, fmt.Errorf("unmarshal envelope: %w", err)
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return envelope{}, nil, 0, fmt.Errorf("unmarshal envelope fields: %w", err)
	}

	records, err := extractRecords(raw)
	if err != nil {
		return envelope{}, nil, 0, err
	}

	return env, records, 0, nil
}

// PageHandler processes one page of raw records. Returning an error aborts
// the remaining pages for this resource.
type PageHandler func(ctx context.Context, records []json.RawMessage) error

// ForEachPage walks every page of a resource, starting at page 1, until
// pages_total is reached. params should already include any resource
// filters (e.g. modified_since); "page" is managed internally.
func (c *Client) ForEachPage(ctx context.Context, resource string, params url.Values, handle PageHandler) error {
	page := 1
	for {
		env, records, err := c.fetchPage(ctx, resource, params, page)
		if err != nil {
			return fmt.Errorf("fetch %s page %d: %w", resource, page, err)
		}

		if err := handle(ctx, records); err != nil {
			return fmt.Errorf("handle %s page %d: %w", resource, page, err)
		}

		if env.PagesTotal == 0 || page >= env.PagesTotal {
			return nil
		}
		page++
	}
}
