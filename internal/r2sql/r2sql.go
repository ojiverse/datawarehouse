// Package r2sql is a minimal client for the Cloudflare R2 SQL query API.
package r2sql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config identifies the account, bucket (warehouse) and bearer token.
type Config struct {
	AccountID string
	Bucket    string
	Token     string
	// BaseURL defaults to the public R2 SQL endpoint.
	BaseURL string
}

// Client executes queries.
type Client struct {
	cfg  Config
	http *http.Client
}

// New validates the config and returns a client.
func New(cfg Config) (*Client, error) {
	if cfg.AccountID == "" || cfg.Bucket == "" || cfg.Token == "" {
		return nil, errors.New("r2sql: account id, bucket and token are required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.sql.cloudflarestorage.com"
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: 120 * time.Second}}, nil
}

// Result is the decoded query response. Rows are kept as generic JSON objects
// because R2 SQL is in beta and its column typing may change.
type Result struct {
	Rows       []map[string]any
	Raw        json.RawMessage
	Elapsed    time.Duration
	HTTPStatus int
}

// envelope mirrors the Cloudflare v4 response wrapper; the row payload shape is
// probed defensively (result.rows, result.results or a bare array).
type envelope struct {
	Success bool            `json:"success"`
	Errors  []any           `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

// Query runs one SQL statement.
func (c *Client) Query(ctx context.Context, sql string) (Result, error) {
	body, _ := json.Marshal(map[string]string{"query": sql})
	url := fmt.Sprintf("%s/api/v1/accounts/%s/r2-sql/query/%s", c.cfg.BaseURL, c.cfg.AccountID, c.cfg.Bucket)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("r2 sql request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("r2 sql read: %w", err)
	}
	res := Result{Raw: raw, Elapsed: time.Since(start), HTTPStatus: resp.StatusCode}
	if resp.StatusCode/100 != 2 {
		return res, fmt.Errorf("r2 sql http %d: %s", resp.StatusCode, truncate(raw))
	}
	rows, err := extractRows(raw)
	if err != nil {
		return res, err
	}
	res.Rows = rows
	return res, nil
}

func extractRows(raw []byte) ([]map[string]any, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err == nil && len(env.Result) > 0 {
		if len(env.Errors) > 0 && !env.Success {
			return nil, fmt.Errorf("r2 sql error: %s", truncate(raw))
		}
		return rowsFromResult(env.Result)
	}
	return rowsFromResult(raw)
}

func rowsFromResult(res json.RawMessage) ([]map[string]any, error) {
	var arr []map[string]any
	if err := json.Unmarshal(res, &arr); err == nil {
		return arr, nil
	}
	var obj struct {
		Rows    []map[string]any `json:"rows"`
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(res, &obj); err != nil {
		return nil, fmt.Errorf("r2 sql: unrecognised result shape: %s", truncate(res))
	}
	if obj.Rows != nil {
		return obj.Rows, nil
	}
	return obj.Results, nil
}

func truncate(b []byte) string {
	if len(b) > 512 {
		return string(b[:512]) + "..."
	}
	return string(b)
}
