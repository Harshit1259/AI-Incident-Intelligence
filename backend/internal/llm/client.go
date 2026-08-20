// Package llm provides a provider-agnostic LLM client supporting OpenAI, Anthropic,
// and any OpenAI-compatible local endpoint (Ollama, vLLM, LM Studio, etc.).
//
// Deployment modes:
//   - cloud   (default): calls OpenAI or Anthropic public APIs
//   - private: calls a custom LLM_BASE_URL endpoint (BYOC / on-prem)
//   - offline: all LLM calls return ErrOfflineMode; callers fall back to templates
package llm

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

const (
	defaultTimeout   = 30 * time.Second
	defaultMaxTokens = 1024
)

// ErrNoAPIKey is returned when no API key is configured.
var ErrNoAPIKey = fmt.Errorf("LLM_API_KEY is not configured")

// ErrOfflineMode is returned when DataMode is "offline" — callers must use template fallback.
var ErrOfflineMode = fmt.Errorf("LLM is in offline mode — using rule-based fallback")

// Provider identifies which LLM backend to use.
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderLocal     Provider = "local" // any OpenAI-compatible endpoint
)

// DataMode controls whether data is sent to cloud providers.
type DataMode string

const (
	DataModeCloud   DataMode = "cloud"   // default: use cloud API
	DataModePrivate DataMode = "private" // custom base URL, no public API
	DataModeOffline DataMode = "offline" // no LLM calls at all
)

// CallStats holds lightweight runtime stats for the /ai/status endpoint.
type CallStats struct {
	TotalCalls    int64  `json:"total_calls"`
	FailedCalls   int64  `json:"failed_calls"`
	LastInputHash string `json:"last_input_hash,omitempty"` // sha256[:8] of last prompt — no content leaked
	LastCalledAt  string `json:"last_called_at,omitempty"`
}

// Client is a provider-agnostic LLM client.
type Client struct {
	apiKey   string
	model    string
	provider Provider
	baseURL  string   // custom base URL for local/BYOC providers
	dataMode DataMode // cloud | private | offline
	httpCli  *http.Client

	// atomic stats — safe for concurrent use
	totalCalls  int64
	failedCalls int64
	lastHash    atomic.Value // stores string
	lastCalledAt atomic.Value // stores string (RFC3339)
}

// New creates a ready-to-use Client.
// provider is auto-detected from the key prefix if empty:
//   - "sk-ant-…" → Anthropic
//   - anything else → OpenAI
//
// baseURL overrides the default API endpoint (for private/local providers).
// dataMode is one of "cloud" | "private" | "offline".
func New(apiKey, model, provider string) *Client {
	return NewWithOptions(apiKey, model, provider, "", "")
}

// NewWithOptions creates a Client with custom base URL and data mode.
func NewWithOptions(apiKey, model, provider, baseURL, dataMode string) *Client {
	p := detectProvider(apiKey, provider, baseURL)
	if model == "" {
		model = defaultModel(p)
	}
	dm := DataModeCloud
	switch strings.ToLower(dataMode) {
	case "private":
		dm = DataModePrivate
	case "offline":
		dm = DataModeOffline
	}
	c := &Client{
		apiKey:   apiKey,
		model:    model,
		provider: p,
		baseURL:  baseURL,
		dataMode: dm,
		httpCli:  &http.Client{Timeout: defaultTimeout},
	}
	c.lastHash.Store("")
	c.lastCalledAt.Store("")
	return c
}

// Complete sends a single user message and returns the assistant text reply.
func (c *Client) Complete(userPrompt string) (string, error) {
	return c.CompleteWithSystem("", userPrompt)
}

// CompleteWithSystem sends a system prompt + user message.
// Returns ErrOfflineMode immediately when DataMode is "offline" — callers must use template fallback.
func (c *Client) CompleteWithSystem(systemPrompt, userPrompt string) (string, error) {
	if c.dataMode == DataModeOffline {
		return "", ErrOfflineMode
	}
	if c.apiKey == "" && c.provider != ProviderLocal {
		return "", ErrNoAPIKey
	}

	// Track call stats
	atomic.AddInt64(&c.totalCalls, 1)
	h := sha256.Sum256([]byte(userPrompt))
	c.lastHash.Store(fmt.Sprintf("%x", h[:4]))
	c.lastCalledAt.Store(time.Now().Format(time.RFC3339))

	// Scrub PII before sending to cloud providers
	if c.dataMode == DataModeCloud {
		systemPrompt = scrubPII(systemPrompt)
		userPrompt = scrubPII(userPrompt)
	}

	var result string
	var err error
	switch c.provider {
	case ProviderAnthropic:
		result, err = c.callAnthropic(systemPrompt, userPrompt)
	case ProviderLocal:
		result, err = c.callLocal(systemPrompt, userPrompt)
	default:
		result, err = c.callOpenAI(systemPrompt, userPrompt)
	}
	if err != nil {
		atomic.AddInt64(&c.failedCalls, 1)
	}
	return result, err
}

// Stats returns a snapshot of call statistics (safe for concurrent use).
func (c *Client) Stats() CallStats {
	return CallStats{
		TotalCalls:    atomic.LoadInt64(&c.totalCalls),
		FailedCalls:   atomic.LoadInt64(&c.failedCalls),
		LastInputHash: c.lastHash.Load().(string),
		LastCalledAt:  c.lastCalledAt.Load().(string),
	}
}

// IsConfigured returns true when the client has a non-empty API key (or local provider).
func (c *Client) IsConfigured() bool { return c.apiKey != "" || c.provider == ProviderLocal }

// ProviderName returns the detected provider name.
func (c *Client) ProviderName() string { return string(c.provider) }

// DataModeName returns the current data mode (cloud | private | offline).
func (c *Client) DataModeName() string { return string(c.dataMode) }

// ─────────────────────────────────────────────────────
// OpenAI
// ─────────────────────────────────────────────────────

type openAIRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Client) callOpenAI(systemPrompt, userPrompt string) (string, error) {
	msgs := []openAIMessage{}
	if strings.TrimSpace(systemPrompt) != "" {
		msgs = append(msgs, openAIMessage{Role: "system", Content: systemPrompt})
	}
	msgs = append(msgs, openAIMessage{Role: "user", Content: userPrompt})

	payload, err := json.Marshal(openAIRequest{
		Model:     c.model,
		MaxTokens: defaultMaxTokens,
		Messages:  msgs,
	})
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	body, status, err := doRequest(c.httpCli, req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("llm: openai error %d: %s", status, truncate(string(body), 300))
	}

	var resp openAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("llm: unmarshal response: %w", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("llm: empty response from openai")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// ─────────────────────────────────────────────────────
// Anthropic
// ─────────────────────────────────────────────────────

type anthropicRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	System    string            `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (c *Client) callAnthropic(systemPrompt, userPrompt string) (string, error) {
	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: defaultMaxTokens,
		Messages:  []anthropicMessage{{Role: "user", Content: userPrompt}},
	}
	if strings.TrimSpace(systemPrompt) != "" {
		reqBody.System = systemPrompt
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	body, status, err := doRequest(c.httpCli, req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("llm: anthropic error %d: %s", status, truncate(string(body), 300))
	}

	var resp anthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("llm: unmarshal response: %w", err)
	}
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			return strings.TrimSpace(block.Text), nil
		}
	}
	return "", fmt.Errorf("llm: empty response from anthropic")
}

// ─────────────────────────────────────────────────────
// Local / BYOC (OpenAI-compatible endpoint)
// ─────────────────────────────────────────────────────

func (c *Client) callLocal(systemPrompt, userPrompt string) (string, error) {
	msgs := []openAIMessage{}
	if strings.TrimSpace(systemPrompt) != "" {
		msgs = append(msgs, openAIMessage{Role: "system", Content: systemPrompt})
	}
	msgs = append(msgs, openAIMessage{Role: "user", Content: userPrompt})

	payload, err := json.Marshal(openAIRequest{
		Model:     c.model,
		MaxTokens: defaultMaxTokens,
		Messages:  msgs,
	})
	if err != nil {
		return "", fmt.Errorf("llm: marshal request: %w", err)
	}

	endpoint := strings.TrimRight(c.baseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("llm: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	body, status, err := doRequest(c.httpCli, req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("llm: local error %d: %s", status, truncate(string(body), 300))
	}

	var resp openAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("llm: unmarshal response: %w", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("llm: empty response from local endpoint")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// ─────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────

func doRequest(cli *http.Client, req *http.Request) ([]byte, int, error) {
	resp, err := cli.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("llm: http call: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("llm: read response: %w", err)
	}
	return body, resp.StatusCode, nil
}

func detectProvider(apiKey, hint, baseURL string) Provider {
	switch strings.ToLower(hint) {
	case "anthropic":
		return ProviderAnthropic
	case "openai":
		return ProviderOpenAI
	case "local":
		return ProviderLocal
	}
	// Non-empty custom base URL with no explicit provider hint → local
	if baseURL != "" {
		return ProviderLocal
	}
	if strings.HasPrefix(apiKey, "sk-ant-") {
		return ProviderAnthropic
	}
	return ProviderOpenAI
}

func defaultModel(p Provider) string {
	switch p {
	case ProviderAnthropic:
		return "claude-sonnet-4-6"
	case ProviderLocal:
		return "llama3" // sensible default for local Ollama/vLLM
	}
	return "gpt-4o"
}

// scrubPII masks emails, IPv4 addresses, and bearer/password patterns before cloud send.
func scrubPII(s string) string {
	s = piiEmail.ReplaceAllString(s, "[email]")
	s = piiIPv4.ReplaceAllString(s, "[ip]")
	s = piiBearer.ReplaceAllStringFunc(s, func(m string) string {
		parts := piiBearer.FindStringSubmatch(m)
		if len(parts) < 2 {
			return m
		}
		return parts[0][:len(parts[0])-len(parts[1])] + "[redacted]"
	})
	return s
}

var (
	piiEmail  = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	piiIPv4   = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	piiBearer = regexp.MustCompile(`(?i)(bearer\s+|password[=:\s]+|token[=:\s]+|api[_-]?key[=:\s]+)(\S+)`)
)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
