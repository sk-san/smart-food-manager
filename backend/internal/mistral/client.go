// Package mistral is an HTTP client for the Mistral chat completions API,
// built to the same contract as internal/gemini: bounded retries on transient
// failures, a capped response read, the key in a header, and external-call
// logging that records metadata only — never the prompt or the reply.
package mistral

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/sk-san/smart-food-manager/backend/internal/logging"
)

const (
	defaultBaseURL   = "https://api.mistral.ai/v1"
	defaultModel     = "mistral-small-latest"
	defaultTimeout   = 60 * time.Second
	defaultRetries   = 3
	maxResponseBytes = 4 << 20 // 4 MiB cap on response reads
)

var (
	ErrMissingAPIKey = errors.New("mistral: missing API key")
	ErrNoContent     = errors.New("mistral: no choices returned")
)

// Config holds the tunables for a Client. Zero values fall back to defaults.
type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	Timeout    time.Duration
	MaxRetries int
}

// Usage reports the tokens one completion consumed.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// TextRequest is a single-turn completion. Unset fields fall back to the
// client's defaults; Temperature 0 is sent as "unset", matching the wire
// format's omitempty.
type TextRequest struct {
	System      string
	Prompt      string
	Model       string
	Temperature float64
	MaxTokens   int
}

// Client is an HTTP client for the Mistral chat completions API.
type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
	model      string
	maxRetries int
}

// New builds a Client, applying defaults for any unset Config fields.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultRetries
	}
	return &Client{
		httpClient: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		apiKey:     cfg.APIKey,
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		model:      cfg.Model,
		maxRetries: cfg.MaxRetries,
	}
}

// Model reports the model requests go to when a TextRequest does not name one.
func (c *Client) Model() string { return c.model }

// Generate performs a single-turn completion and returns the reply with its
// token usage.
func (c *Client) Generate(ctx context.Context, req TextRequest) (string, Usage, error) {
	if c.apiKey == "" {
		return "", Usage{}, ErrMissingAPIKey
	}

	model := req.Model
	if model == "" {
		model = c.model
	}

	messages := make([]Message, 0, 2)
	if req.System != "" {
		messages = append(messages, Message{Role: "system", Content: req.System})
	}
	messages = append(messages, Message{Role: "user", Content: req.Prompt})

	payload, err := json.Marshal(chatCompletionRequest{
		Model:       model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	})
	if err != nil {
		return "", Usage{}, fmt.Errorf("mistral: marshal request: %w", err)
	}

	done := logging.StartExternalCall(ctx, logging.Dependency{
		Provider:  "mistral",
		Service:   "mistral-api",
		Operation: "chat.completions",
		Model:     model,
	}, "mistral.chat_completion")

	body, status, err := c.doWithRetry(ctx, payload)
	if err != nil {
		done(status, err, slog.Int("request.size_bytes", len(payload)))
		return "", Usage{}, err
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		done(status, err)
		return "", Usage{}, fmt.Errorf("mistral: decode response: %w", err)
	}

	usage := Usage{
		InputTokens:  parsed.Usage.PromptTokens,
		OutputTokens: parsed.Usage.CompletionTokens,
		TotalTokens:  parsed.Usage.TotalTokens,
	}

	if len(parsed.Choices) == 0 {
		done(status, ErrNoContent)
		return "", usage, ErrNoContent
	}

	done(status, nil,
		slog.Int("tokens.prompt", usage.InputTokens),
		slog.Int("tokens.output", usage.OutputTokens),
		slog.Int("tokens.total", usage.TotalTokens),
	)

	return parsed.Choices[0].Message.Content, usage, nil
}

// doWithRetry POSTs the payload, retrying transient failures (network errors
// and 429/5xx) with exponential backoff. It returns the raw 200 body, the last
// HTTP status seen, and the final error. Context cancellation is never retried.
func (c *Client) doWithRetry(ctx context.Context, payload []byte) ([]byte, int, error) {
	url := c.baseURL + "/chat/completions"

	var lastStatus int
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepBackoff(ctx, attempt); err != nil {
				return nil, lastStatus, err
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, lastStatus, fmt.Errorf("mistral: new request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		// The key travels in a header, never the URL, to keep it out of logs.
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, lastStatus, fmt.Errorf("mistral: request: %w", ctx.Err())
			}
			lastErr = fmt.Errorf("mistral: request: %w", err)
			continue // transient network error: retry
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
		lastStatus = resp.StatusCode

		switch {
		case resp.StatusCode == http.StatusOK:
			return body, resp.StatusCode, nil
		case isRetryable(resp.StatusCode):
			lastErr = apiErrorFrom(resp.StatusCode, body)
			continue
		default:
			return nil, resp.StatusCode, apiErrorFrom(resp.StatusCode, body)
		}
	}

	if lastErr == nil {
		lastErr = errors.New("mistral: retries exhausted")
	}
	return nil, lastStatus, lastErr
}

// isRetryable reports whether an HTTP status warrants a retry.
func isRetryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// apiErrorFrom builds an error from the JSON error envelope, falling back to
// the bare status code when the body is not one of the expected shapes.
func apiErrorFrom(status int, body []byte) error {
	var ae apiError
	if err := json.Unmarshal(body, &ae); err == nil {
		if msg := ae.Error.Message; msg != "" {
			return fmt.Errorf("mistral: status %d: %s", status, msg)
		}
		if ae.Message != "" {
			return fmt.Errorf("mistral: status %d: %s", status, ae.Message)
		}
	}
	return fmt.Errorf("mistral: status %d", status)
}

// sleepBackoff waits before a retry using exponential backoff (200ms, 400ms,
// 800ms, ...), returning early if the context is cancelled.
func sleepBackoff(ctx context.Context, attempt int) error {
	delay := time.Duration(200<<(attempt-1)) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("mistral: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
