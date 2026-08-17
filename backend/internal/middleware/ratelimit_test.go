package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterBurstAndRefill(t *testing.T) {
	rl := NewRateLimiter(1, 2)
	if !rl.allow("client") || !rl.allow("client") {
		t.Fatal("initial burst was not allowed")
	}
	if rl.allow("client") {
		t.Fatal("request above burst was allowed")
	}

	rl.mu.Lock()
	rl.buckets["client"].last = time.Now().Add(-2 * time.Second)
	rl.mu.Unlock()
	if !rl.allow("client") {
		t.Fatal("refilled request was rejected")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	rl := NewRateLimiter(0, 1)
	called := 0
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))

	first := httptest.NewRequest(http.MethodGet, "/", nil)
	first.RemoteAddr = "192.0.2.10:1234"
	firstRes := httptest.NewRecorder()
	handler.ServeHTTP(firstRes, first)
	if firstRes.Code != http.StatusNoContent {
		t.Errorf("first status = %d, want 204", firstRes.Code)
	}

	second := httptest.NewRequest(http.MethodGet, "/", nil)
	second.RemoteAddr = "192.0.2.10:5678"
	secondRes := httptest.NewRecorder()
	handler.ServeHTTP(secondRes, second)
	if secondRes.Code != http.StatusTooManyRequests {
		t.Errorf("second status = %d, want 429", secondRes.Code)
	}
	if called != 1 {
		t.Errorf("next handler calls = %d, want 1", called)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		want       string
	}{
		{name: "host and port", remoteAddr: "192.0.2.1:1234", want: "192.0.2.1"},
		{name: "IPv6 host and port", remoteAddr: "[2001:db8::1]:1234", want: "2001:db8::1"},
		{name: "unparseable address", remoteAddr: "local-client", want: "local-client"},
		{name: "single forwarded address", remoteAddr: "local", forwarded: "198.51.100.1", want: "198.51.100.1"},
		{name: "forwarded chain", remoteAddr: "local", forwarded: "198.51.100.1, 203.0.113.5", want: "198.51.100.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if got := clientIP(req); got != tt.want {
				t.Errorf("clientIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIndexComma(t *testing.T) {
	if got := indexComma("one,two"); got != 3 {
		t.Errorf("indexComma = %d, want 3", got)
	}
	if got := indexComma("one"); got != -1 {
		t.Errorf("indexComma = %d, want -1", got)
	}
}
