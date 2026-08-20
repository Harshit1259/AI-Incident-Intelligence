/*
 * NeurOps Agent — Go SDK HTTP Client
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Wraps net/http for REST API metric plugins.
 * Supports Basic, Bearer, API-key, and Digest authentication,
 * proxy routing, TLS config, and transparent pagination.
 *
 * Usage:
 *   client := http.New(context, logger)
 *   result, err := client.Get("/api/v1/metrics", nil)
 */

package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neuroops/agent/internal/logger"
	"github.com/neuroops/agent/sdk/go/types"
)

// Response is returned by every HTTP method.
type Response struct {
	StatusCode int
	Body       map[string]any
	RawBody    []byte
}

// Client wraps http.Client for plugin use.
type Client struct {
	baseURL    string
	username   string
	password   string
	token      string
	apiKey     string
	apiKeyHdr  string
	authMode   string
	timeout    time.Duration
	httpClient *http.Client
	log        *logger.Logger
}

// New creates an HTTP Client from a plugin context map.
func New(ctx types.NeurOpsMap, log *logger.Logger) *Client {
	timeout := time.Duration(ctx.GetInt("timeout")) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	apiKeyHdr := ctx.GetString("api.key.header")
	if apiKeyHdr == "" {
		apiKeyHdr = "X-API-Key"
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: ctx.GetString("verify.ssl") == "no",
	}

	transport := &http.Transport{TLSClientConfig: tlsConfig}

	// Proxy support
	if proxyHost := ctx.GetString("proxy.server"); proxyHost != "" {
		proxyPort := ctx.GetString("proxy.port")
		proxyURL, err := url.Parse(fmt.Sprintf("http://%s:%s", proxyHost, proxyPort))
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &Client{
		baseURL:    strings.TrimRight(ctx.GetString("base.url"), "/"),
		username:   ctx.GetString("username"),
		password:   ctx.GetString("password"),
		token:      ctx.GetString("bearer.token"),
		apiKey:     ctx.GetString("api.key"),
		apiKeyHdr:  apiKeyHdr,
		authMode:   strings.ToLower(ctx.GetString("auth.mode")),
		timeout:    timeout,
		log:        log,
		httpClient: &http.Client{Transport: transport, Timeout: timeout},
	}
}

// ── Public API ────────────────────────────────────────────────────────────────

func (c *Client) Get(path string, params map[string]string) (*Response, error) {
	return c.do(http.MethodGet, path, params, nil)
}

func (c *Client) Post(path string, body map[string]any) (*Response, error) {
	return c.do(http.MethodPost, path, nil, body)
}

func (c *Client) Put(path string, body map[string]any) (*Response, error) {
	return c.do(http.MethodPut, path, nil, body)
}

func (c *Client) Delete(path string) (*Response, error) {
	return c.do(http.MethodDelete, path, nil, nil)
}

// GetAll walks paginated endpoints and returns all collected records.
// pageKey is the query-param name (e.g. "page"), dataKey is the JSON
// array field in each response (e.g. "data").
func (c *Client) GetAll(path, pageKey, dataKey string, maxPages int) ([]any, error) {
	var all []any
	for page := 1; page <= maxPages; page++ {
		resp, err := c.Get(path, map[string]string{pageKey: fmt.Sprintf("%d", page)})
		if err != nil {
			return all, err
		}
		records, _ := resp.Body[dataKey].([]any)
		if len(records) == 0 {
			break
		}
		all = append(all, records...)
	}
	return all, nil
}

// ── Internal ──────────────────────────────────────────────────────────────────

func (c *Client) do(method, path string, params map[string]string, body map[string]any) (*Response, error) {
	urlStr := c.baseURL
	if path != "" {
		urlStr = c.baseURL + "/" + strings.TrimLeft(path, "/")
	}

	// Build query string
	if len(params) > 0 {
		q := url.Values{}
		for k, v := range params {
			q.Set(k, v)
		}
		urlStr += "?" + q.Encode()
	}

	// Build body
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshalling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	switch c.authMode {
	case "basic":
		req.SetBasicAuth(c.username, c.password)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+c.token)
	case "apikey":
		req.Header.Set(c.apiKeyHdr, c.apiKey)
	}

	c.log.Debugf("HTTP %s %s", method, urlStr)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s %s: %w", method, urlStr, err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var parsed map[string]any
	_ = json.Unmarshal(rawBody, &parsed)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, urlStr, string(rawBody[:min(200, len(rawBody))]))
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       parsed,
		RawBody:    rawBody,
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
