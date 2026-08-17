package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// guestRequest is an analysis call from an unauthenticated visitor.
func guestRequest(ip string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/nutrition/analyze", nil)
	r.RemoteAddr = ip + ":1234"
	return r
}

// okHandler stands in for a successful analysis.
func okHandler(calls *int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	})
}

func TestGuestAIQuotaAllowsThreeAnalysesPerDay(t *testing.T) {
	q := NewGuestAIQuota(3)
	calls := 0
	handler := q.Middleware(okHandler(&calls))

	for i := 1; i <= 3; i++ {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, guestRequest("198.51.100.7"))
		if res.Code != http.StatusOK {
			t.Fatalf("analysis %d status = %d, want 200", i, res.Code)
		}
		if got, want := res.Header().Get(quotaRemainingHeader), []string{"2", "1", "0"}[i-1]; got != want {
			t.Errorf("analysis %d remaining header = %q, want %q", i, got, want)
		}
		if got := res.Header().Get(quotaLimitHeader); got != "3" {
			t.Errorf("analysis %d limit header = %q, want \"3\"", i, got)
		}
	}

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, guestRequest("198.51.100.7"))
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("fourth analysis status = %d, want 429", res.Code)
	}
	if calls != 3 {
		t.Errorf("handler calls = %d, want 3", calls)
	}

	var body struct {
		Error     string `json:"error"`
		Code      string `json:"code"`
		Limit     int    `json:"limit"`
		Remaining int    `json:"remaining"`
		ResetAt   string `json:"resetAt"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding rejection body: %v", err)
	}
	if body.Code != GuestAIQuotaCode {
		t.Errorf("code = %q, want %q", body.Code, GuestAIQuotaCode)
	}
	if body.Limit != 3 || body.Remaining != 0 {
		t.Errorf("limit/remaining = %d/%d, want 3/0", body.Limit, body.Remaining)
	}
	if _, err := time.Parse(time.RFC3339, body.ResetAt); err != nil {
		t.Errorf("resetAt = %q, want RFC3339: %v", body.ResetAt, err)
	}
}

func TestGuestAIQuotaIsPerClient(t *testing.T) {
	q := NewGuestAIQuota(3)
	calls := 0
	handler := q.Middleware(okHandler(&calls))

	for i := 0; i < 3; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), guestRequest("198.51.100.7"))
	}

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, guestRequest("203.0.113.9"))
	if res.Code != http.StatusOK {
		t.Fatalf("other client status = %d, want 200", res.Code)
	}
	if got := res.Header().Get(quotaRemainingHeader); got != "2" {
		t.Errorf("other client remaining = %q, want \"2\"", got)
	}
}

func TestGuestAIQuotaExemptsAuthenticatedCallers(t *testing.T) {
	q := NewGuestAIQuota(3)
	calls := 0
	handler := q.Middleware(okHandler(&calls))

	for i := 0; i < 5; i++ {
		r := guestRequest("198.51.100.7")
		ctx := context.WithValue(r.Context(), claimsContextKey, &Claims{UserID: "user-1"})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, r.WithContext(ctx))
		if res.Code != http.StatusOK {
			t.Fatalf("authenticated analysis %d status = %d, want 200", i+1, res.Code)
		}
		if got := res.Header().Get(quotaLimitHeader); got != "" {
			t.Errorf("authenticated response carried quota header %q", got)
		}
	}
	if calls != 5 {
		t.Errorf("handler calls = %d, want 5", calls)
	}
}

func TestGuestAIQuotaRefundsFailedAnalyses(t *testing.T) {
	q := NewGuestAIQuota(3)
	failing := q.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "analysis failed", http.StatusBadGateway)
	}))

	for i := 0; i < 4; i++ {
		res := httptest.NewRecorder()
		failing.ServeHTTP(res, guestRequest("198.51.100.7"))
		if res.Code != http.StatusBadGateway {
			t.Fatalf("failed analysis %d status = %d, want 502", i+1, res.Code)
		}
		// The run was refunded, so the guest still holds the full allowance.
		if got := res.Header().Get(quotaRemainingHeader); got != "3" {
			t.Errorf("failed analysis %d remaining = %q, want \"3\"", i+1, got)
		}
	}

	if used, left := q.remaining("198.51.100.7"); used != 0 || left != 3 {
		t.Errorf("after failures used/left = %d/%d, want 0/3", used, left)
	}
}

func TestGuestAIQuotaResetsOnTheNextDay(t *testing.T) {
	q := NewGuestAIQuota(3)
	now := time.Date(2026, 8, 17, 23, 30, 0, 0, time.UTC)
	q.nowFn = func() time.Time { return now }

	calls := 0
	handler := q.Middleware(okHandler(&calls))
	for i := 0; i < 3; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), guestRequest("198.51.100.7"))
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, guestRequest("198.51.100.7"))
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("status before midnight = %d, want 429", res.Code)
	}

	now = now.Add(time.Hour) // past UTC midnight
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, guestRequest("198.51.100.7"))
	if res.Code != http.StatusOK {
		t.Fatalf("status after midnight = %d, want 200", res.Code)
	}
	if got := res.Header().Get(quotaRemainingHeader); got != "2" {
		t.Errorf("remaining after reset = %q, want \"2\"", got)
	}
}

func TestGuestAIQuotaNegativeLimitDisablesEnforcement(t *testing.T) {
	q := NewGuestAIQuota(-1)
	calls := 0
	handler := q.Middleware(okHandler(&calls))
	for i := 0; i < 10; i++ {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, guestRequest("198.51.100.7"))
		if res.Code != http.StatusOK {
			t.Fatalf("analysis %d status = %d, want 200", i+1, res.Code)
		}
	}
	if calls != 10 {
		t.Errorf("handler calls = %d, want 10", calls)
	}
}

func TestGuestAIQuotaStatusDoesNotSpendTheAllowance(t *testing.T) {
	q := NewGuestAIQuota(3)
	handler := q.Middleware(okHandler(new(int)))
	handler.ServeHTTP(httptest.NewRecorder(), guestRequest("198.51.100.7"))

	var status struct {
		Unlimited bool   `json:"unlimited"`
		Limit     int    `json:"limit"`
		Used      int    `json:"used"`
		Remaining int    `json:"remaining"`
		ResetAt   string `json:"resetAt"`
	}
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nutrition/quota", nil)
		req.RemoteAddr = "198.51.100.7:1234"
		res := httptest.NewRecorder()
		q.Status(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status code = %d, want 200", res.Code)
		}
		if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
			t.Fatalf("decoding status body: %v", err)
		}
		if status.Unlimited || status.Limit != 3 || status.Used != 1 || status.Remaining != 2 {
			t.Fatalf("status = %+v, want limit 3, used 1, remaining 2", status)
		}
	}
}

func TestGuestAIQuotaStatusReportsAuthenticatedCallersUnlimited(t *testing.T) {
	q := NewGuestAIQuota(3)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nutrition/quota", nil)
	req = req.WithContext(context.WithValue(req.Context(), claimsContextKey, &Claims{UserID: "user-1"}))
	res := httptest.NewRecorder()
	q.Status(res, req)

	var status struct {
		Unlimited bool `json:"unlimited"`
	}
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatalf("decoding status body: %v", err)
	}
	if !status.Unlimited {
		t.Error("authenticated status reported a limit")
	}
}
