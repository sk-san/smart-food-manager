package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/example/food-app/backend/internal/logging"
)

const (
	defaultBaseURL   = "https://generativelanguage.googleapis.com/v1beta"
	defaultModel     = "gemini-2.5-flash"
	defaultTimeout   = 30 * time.Second
	defaultRetries   = 3
	maxResponseBytes = 4 << 20 // 4 MiB cap on response reads
)

var (
	ErrMissingAPIKey = errors.New("gemini: missing API key")
	ErrNoContent     = errors.New("gemini: no candidates returned")
	ErrBlocked       = errors.New("gemini: prompt blocked by safety filters")
)

// Config holds the tunables for a Client. Zero values fall back to defaults.
type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	Timeout    time.Duration
	MaxRetries int
}

// Client is an HTTP client for the Google Gemini generateContent API.
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
			Timeout: cfg.Timeout,
			// otelhttp makes the outbound call a child span of the request,
			// matching the tracing already wired up in internal/server.
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		apiKey:     cfg.APIKey,
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		model:      cfg.Model,
		maxRetries: cfg.MaxRetries,
	}
}

// GenerateText sends a single-turn prompt (with an optional system
// instruction) and returns the model's text reply.
func (c *Client) GenerateText(ctx context.Context, system, prompt string) (string, error) {
	if c.apiKey == "" {
		return "", ErrMissingAPIKey
	}
	reqBody := generateContentRequest{
		Contents: []Content{{Role: "user", Parts: []Part{{Text: prompt}}}},
		GenerationConfig: &GenerationConfig{
			Temperature:      0.2,
			ResponseMIMEType: "application/json",
		},
	}
	if system != "" {
		reqBody.SystemInstruction = &Content{Parts: []Part{{Text: system}}}
	}
	return c.generate(ctx, reqBody, "gemini.generate_text")
}

// GenerateFromImage sends an image (inline base64) plus a prompt and returns
// the model's text reply. Used for photo/label vision extraction. A lower
// temperature keeps extraction deterministic.
func (c *Client) GenerateFromImage(ctx context.Context, system, prompt, mimeType string, image []byte) (string, error) {
	if c.apiKey == "" {
		return "", ErrMissingAPIKey
	}
	reqBody := generateContentRequest{
		Contents: []Content{{Role: "user", Parts: []Part{
			{Text: prompt},
			{InlineData: &Blob{MIMEType: mimeType, Data: base64.StdEncoding.EncodeToString(image)}},
		}}},
		GenerationConfig: &GenerationConfig{
			Temperature:      0.1,
			ResponseMIMEType: "application/json",
		},
	}
	if system != "" {
		reqBody.SystemInstruction = &Content{Parts: []Part{{Text: system}}}
	}
	return c.generate(ctx, reqBody, "gemini.generate_from_image")
}

// generate marshals the request, performs the instrumented HTTP call with
// retries, and returns the first candidate's text. The call is wrapped with
// logging.StartExternalCall, so only metadata (token counts, sizes) is logged
// — never the prompt or response text.
func (c *Client) generate(ctx context.Context, reqBody generateContentRequest, action string) (string, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("gemini: marshal request: %w", err)
	}

	done := logging.StartExternalCall(ctx, logging.Dependency{
		Provider:  "google",
		Service:   "gemini-api",
		Operation: "generateContent",
		Model:     c.model,
	}, action)

	body, status, err := c.doWithRetry(ctx, payload)
	if err != nil {
		done(status, err, slog.Int("request.size_bytes", len(payload)))
		return "", err
	}

	var parsed generateContentResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		done(status, err)
		return "", fmt.Errorf("gemini: decode response: %w", err)
	}

	if parsed.PromptFeedback != nil && parsed.PromptFeedback.BlockReason != "" {
		done(status, ErrBlocked, slog.String("block_reason", parsed.PromptFeedback.BlockReason))
		return "", ErrBlocked
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		done(status, ErrNoContent)
		return "", ErrNoContent
	}

	done(status, nil,
		slog.Int("tokens.prompt", parsed.UsageMetadata.PromptTokenCount),
		slog.Int("tokens.output", parsed.UsageMetadata.CandidatesTokenCount),
		slog.Int("tokens.total", parsed.UsageMetadata.TotalTokenCount),
	)

	return parsed.Candidates[0].Content.Parts[0].Text, nil
}

// doWithRetry POSTs the payload, retrying transient failures (network errors
// and 429/5xx) with exponential backoff. It returns the raw 200 body, the last
// HTTP status seen, and the final error (if any). Context cancellation is never
// retried.
func (c *Client) doWithRetry(ctx context.Context, payload []byte) ([]byte, int, error) {
	url := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, c.model)

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
			return nil, lastStatus, fmt.Errorf("gemini: new request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		// API key travels in a header, never the URL, to keep it out of logs.
		req.Header.Set("x-goog-api-key", c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, lastStatus, fmt.Errorf("gemini: request: %w", ctx.Err())
			}
			lastErr = fmt.Errorf("gemini: request: %w", err)
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
		lastErr = errors.New("gemini: retries exhausted")
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

// apiErrorFrom builds an error from the API's JSON error envelope, falling back
// to the bare status code when the body is not the expected shape.
func apiErrorFrom(status int, body []byte) error {
	var ae apiError
	if err := json.Unmarshal(body, &ae); err == nil && ae.Error.Message != "" {
		return fmt.Errorf("gemini: status %d: %s", status, ae.Error.Message)
	}
	return fmt.Errorf("gemini: status %d", status)
}

// sleepBackoff waits before a retry using exponential backoff (200ms, 400ms,
// 800ms, ...), returning early if the context is cancelled.
func sleepBackoff(ctx context.Context, attempt int) error {
	delay := time.Duration(200<<(attempt-1)) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("gemini: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
