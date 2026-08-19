package mistral

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(baseURL string) *Client {
	return New(Config{APIKey: "test-key", BaseURL: baseURL, Model: "test-model", Timeout: 5 * time.Second})
}

func TestNewAppliesDefaults(t *testing.T) {
	c := New(Config{})
	if c.baseURL != defaultBaseURL || c.model != defaultModel || c.maxRetries != defaultRetries {
		t.Errorf("defaults not applied: %+v", c)
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, defaultTimeout)
	}
	if c.Model() != defaultModel {
		t.Errorf("Model() = %q", c.Model())
	}
}

func TestGenerate(t *testing.T) {
	var captured chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		// The key must travel as a bearer header, never in the URL.
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"model reply"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":22,"completion_tokens":8,"total_tokens":30}}`))
	}))
	defer server.Close()

	text, usage, err := newTestClient(server.URL).Generate(context.Background(), TextRequest{
		System:      "system rules",
		Prompt:      "user prompt",
		Model:       "other-model",
		Temperature: 0.3,
		MaxTokens:   256,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if text != "model reply" {
		t.Errorf("text = %q", text)
	}
	if usage != (Usage{InputTokens: 22, OutputTokens: 8, TotalTokens: 30}) {
		t.Errorf("usage = %+v", usage)
	}

	// The system instruction is a message with role "system", ahead of the user turn.
	if len(captured.Messages) != 2 ||
		captured.Messages[0] != (Message{Role: "system", Content: "system rules"}) ||
		captured.Messages[1] != (Message{Role: "user", Content: "user prompt"}) {
		t.Errorf("messages = %+v", captured.Messages)
	}
	if captured.Model != "other-model" {
		t.Errorf("model = %q, want the per-request override", captured.Model)
	}
	if captured.Temperature != 0.3 || captured.MaxTokens != 256 {
		t.Errorf("request = %+v", captured)
	}
}

func TestGenerateOmitsSystemMessageWhenUnset(t *testing.T) {
	var captured chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{}}`))
	}))
	defer server.Close()

	if _, _, err := newTestClient(server.URL).Generate(context.Background(), TextRequest{Prompt: "p"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(captured.Messages) != 1 || captured.Messages[0].Role != "user" {
		t.Errorf("messages = %+v, want the user turn alone", captured.Messages)
	}
}

func TestGenerateRequiresAPIKey(t *testing.T) {
	c := New(Config{})
	if _, _, err := c.Generate(context.Background(), TextRequest{Prompt: "p"}); err != ErrMissingAPIKey {
		t.Errorf("err = %v, want ErrMissingAPIKey", err)
	}
}

func TestGenerateNoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[],"usage":{"prompt_tokens":3,"total_tokens":3}}`))
	}))
	defer server.Close()

	_, usage, err := newTestClient(server.URL).Generate(context.Background(), TextRequest{Prompt: "p"})
	if err != ErrNoContent {
		t.Errorf("err = %v, want ErrNoContent", err)
	}
	// The call was still billed, so its usage is reported.
	if usage.InputTokens != 3 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestGenerateRetriesTransientFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"rate limited"}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"second try"}}],"usage":{}}`))
	}))
	defer server.Close()

	text, _, err := newTestClient(server.URL).Generate(context.Background(), TextRequest{Prompt: "p"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if text != "second try" || calls.Load() != 2 {
		t.Errorf("text = %q after %d calls", text, calls.Load())
	}
}

func TestGenerateSurfacesAPIErrorMessages(t *testing.T) {
	// Mistral uses two error envelopes; both must reach the caller, because a
	// 401 "unauthorized" and a 400 "invalid model" need different fixes.
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"bare message", `{"message":"Unauthorized"}`, "Unauthorized", http.StatusUnauthorized},
		{"nested error", `{"error":{"message":"Invalid model","type":"invalid_request_error"}}`, "Invalid model", http.StatusBadRequest},
		{"unparsable", `<html>gateway</html>`, "status 400", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			_, _, err := newTestClient(server.URL).Generate(context.Background(), TextRequest{Prompt: "p"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestGenerateHonoursContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := newTestClient(server.URL).Generate(ctx, TextRequest{Prompt: "p"}); err == nil {
		t.Error("Generate succeeded with a cancelled context")
	}
}
