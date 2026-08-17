package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sk-san/smart-food-manager/backend/internal/config"
)

func TestNewWiresAuthenticationAndAIAnalysis(t *testing.T) {
	cfg := config.Config{
		JWTSecret:         "server-test-secret",
		JWTExpiry:         time.Minute,
		RateLimitRPS:      100,
		RateLimitBurst:    100,
		AllowedOrigin:     "https://app.example.com",
		ServiceName:       "test-api",
		GeminiTimeout:     time.Second,
		GuestAIDailyLimit: 3,
	}
	handler := New(cfg, nil)

	t.Run("protected route requires token", func(t *testing.T) {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
		if res.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", res.Code)
		}
	})

	t.Run("AI route is wired", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/nutrition/analyze",
			strings.NewReader(`{"type":"text","text":"apple"}`))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502; body = %s", res.Code, res.Body.String())
		}
	})

	t.Run("guest AI quota route reports the daily allowance", func(t *testing.T) {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/nutrition/quota", nil))
		if res.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.Code)
		}
		// The failed analysis above was refunded, so the whole allowance is
		// still available.
		body := res.Body.String()
		if !strings.Contains(body, `"limit":3`) || !strings.Contains(body, `"remaining":3`) {
			t.Errorf("body = %s, want limit 3 and remaining 3", body)
		}
	})

	t.Run("admin route requires token", func(t *testing.T) {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/admin/ping", nil))
		if res.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", res.Code)
		}
	})
}

func TestCORS(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	})
	handler := cors("https://app.example.com")(next)

	t.Run("preflight", func(t *testing.T) {
		called = false
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/meals", nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", res.Code)
		}
		if called {
			t.Error("preflight reached next handler")
		}
		if res.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
			t.Errorf("allow origin = %q", res.Header().Get("Access-Control-Allow-Origin"))
		}
		if !strings.Contains(res.Header().Get("Access-Control-Allow-Methods"), "DELETE") {
			t.Errorf("allow methods = %q", res.Header().Get("Access-Control-Allow-Methods"))
		}
	})

	t.Run("ordinary request", func(t *testing.T) {
		called = false
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
		if res.Code != http.StatusCreated || !called {
			t.Errorf("status = %d, called = %v", res.Code, called)
		}
	})
}
