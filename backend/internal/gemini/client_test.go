package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewAppliesDefaults(t *testing.T) {
	c := New(Config{})
	if c.baseURL != defaultBaseURL || c.model != defaultModel || c.maxRetries != defaultRetries {
		t.Errorf("defaults not applied: %+v", c)
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, defaultTimeout)
	}
}

func TestGenerateText(t *testing.T) {
	var captured generateContentRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/test-model:generateContent" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Errorf("API key header = %q", r.Header.Get("x-goog-api-key"))
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"model reply"}]}}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"totalTokenCount":6}}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	reply, err := c.GenerateText(context.Background(), "system rules", "user prompt")
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if reply != "model reply" {
		t.Errorf("reply = %q", reply)
	}
	if captured.SystemInstruction == nil || captured.SystemInstruction.Parts[0].Text != "system rules" {
		t.Errorf("system instruction = %+v", captured.SystemInstruction)
	}
	if len(captured.Contents) != 1 || captured.Contents[0].Parts[0].Text != "user prompt" {
		t.Errorf("contents = %+v", captured.Contents)
	}
	if captured.GenerationConfig == nil || captured.GenerationConfig.ResponseMIMEType != "application/json" {
		t.Errorf("generation config = %+v", captured.GenerationConfig)
	}
}

func TestGenerateFromImage(t *testing.T) {
	image := []byte("image bytes")
	var captured generateContentRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"image reply"}]}}]}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	reply, err := c.GenerateFromImage(context.Background(), "", "inspect this", "image/png", image)
	if err != nil {
		t.Fatalf("GenerateFromImage: %v", err)
	}
	if reply != "image reply" {
		t.Errorf("reply = %q", reply)
	}
	if captured.SystemInstruction != nil {
		t.Errorf("unexpected system instruction: %+v", captured.SystemInstruction)
	}
	parts := captured.Contents[0].Parts
	if len(parts) != 2 || parts[1].InlineData == nil {
		t.Fatalf("parts = %+v", parts)
	}
	if parts[1].InlineData.MIMEType != "image/png" || parts[1].InlineData.Data != base64.StdEncoding.EncodeToString(image) {
		t.Errorf("inline data = %+v", parts[1].InlineData)
	}
}

func TestGenerateRequiresAPIKey(t *testing.T) {
	c := New(Config{})
	if _, err := c.GenerateText(context.Background(), "", "prompt"); !errors.Is(err, ErrMissingAPIKey) {
		t.Errorf("GenerateText error = %v, want ErrMissingAPIKey", err)
	}
	if _, err := c.GenerateFromImage(context.Background(), "", "prompt", "image/png", []byte("x")); !errors.Is(err, ErrMissingAPIKey) {
		t.Errorf("GenerateFromImage error = %v, want ErrMissingAPIKey", err)
	}
}

func TestGenerateResponseFailures(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantError   string
		targetError error
	}{
		{name: "API error", status: http.StatusBadRequest, body: `{"error":{"message":"bad request"}}`, wantError: "gemini: status 400: bad request"},
		{name: "invalid JSON", status: http.StatusOK, body: `{`, wantError: "gemini: decode response"},
		{name: "blocked prompt", status: http.StatusOK, body: `{"promptFeedback":{"blockReason":"SAFETY"}}`, targetError: ErrBlocked},
		{name: "missing candidate", status: http.StatusOK, body: `{}`, targetError: ErrNoContent},
		{name: "candidate without parts", status: http.StatusOK, body: `{"candidates":[{"content":{"parts":[]}}]}`, targetError: ErrNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			_, err := newTestClient(server.URL).GenerateText(context.Background(), "", "prompt")
			if tt.targetError != nil && !errors.Is(err, tt.targetError) {
				t.Errorf("error = %v, want %v", err, tt.targetError)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantError)
			}
		})
	}
}

func TestGenerateRetriesTransientFailure(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			http.Error(w, "try again", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"recovered"}]}}]}`))
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	c.maxRetries = 1
	reply, err := c.GenerateText(context.Background(), "", "prompt")
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if reply != "recovered" || attempts.Load() != 2 {
		t.Errorf("reply = %q, attempts = %d", reply, attempts.Load())
	}
}

func TestClientHelpers(t *testing.T) {
	for _, status := range []int{429, 500, 502, 503, 504} {
		if !isRetryable(status) {
			t.Errorf("status %d should be retryable", status)
		}
	}
	if isRetryable(http.StatusBadRequest) || isRetryable(http.StatusNotFound) {
		t.Error("4xx status unexpectedly retryable")
	}
	if got := apiErrorFrom(http.StatusTeapot, []byte("not JSON")).Error(); got != "gemini: status 418" {
		t.Errorf("fallback API error = %q", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepBackoff(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Errorf("sleepBackoff error = %v, want context canceled", err)
	}
}

func newTestClient(baseURL string) *Client {
	c := New(Config{
		APIKey:     "test-key",
		BaseURL:    baseURL + "/",
		Model:      "test-model",
		Timeout:    time.Second,
		MaxRetries: 1,
	})
	c.maxRetries = 0
	return c
}

func TestGenerateReportsUsageIncludingThinkingTokens(t *testing.T) {
	var captured generateContentRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A per-request model overrides the client's.
		if r.URL.Path != "/models/other-model:generateContent" {
			t.Errorf("path = %q, want the per-request model", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"prose reply"}]}}],` +
			`"usageMetadata":{"promptTokenCount":73,"candidatesTokenCount":74,"thoughtsTokenCount":722,"totalTokenCount":869}}`))
	}))
	defer server.Close()

	text, usage, err := newTestClient(server.URL).Generate(context.Background(), TextRequest{
		System:          "system rules",
		Prompt:          "user prompt",
		Model:           "other-model",
		Temperature:     0.3,
		MaxOutputTokens: 3000,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if text != "prose reply" {
		t.Errorf("text = %q", text)
	}

	// Thinking is billed at the output rate and drawn from the output budget,
	// so it has to land in OutputTokens or the run looks ten times cheaper
	// than it was.
	want := Usage{InputTokens: 73, OutputTokens: 796, TotalTokens: 869}
	if usage != want {
		t.Errorf("usage = %+v, want %+v", usage, want)
	}

	// Generate leaves the output format to the caller, unlike GenerateText.
	if captured.GenerationConfig.ResponseMIMEType != "" {
		t.Errorf("MIME type = %q, want unset", captured.GenerationConfig.ResponseMIMEType)
	}
	if captured.GenerationConfig.Temperature != 0.3 || captured.GenerationConfig.MaxOutputTokens != 3000 {
		t.Errorf("generation config = %+v", captured.GenerationConfig)
	}
}
