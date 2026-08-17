package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sk-san/smart-food-manager/backend/internal/logging"
)

// GuestAIQuotaCode is the machine-readable code the frontend matches on to
// tell an exhausted allowance apart from the shared per-second rate limiter,
// which also answers 429.
const GuestAIQuotaCode = "guest_ai_daily_limit"

// Quota headers accompany every guest response from a quota-guarded route,
// so a client learns its remaining allowance from the analysis call itself.
const (
	quotaLimitHeader     = "X-AI-Quota-Limit"
	quotaRemainingHeader = "X-AI-Quota-Remaining"
	quotaResetHeader     = "X-AI-Quota-Reset"
)

// GuestAIQuota caps how many AI analyses an unauthenticated caller — the
// "continue as guest" path in the frontend — may run per day. Each run costs
// a Gemini call, so the cap is enforced here rather than in the client, which
// any visitor can bypass. A valid token lifts the cap entirely.
//
// A guest has no account, so the only identifier available is the client IP:
// visitors behind one NAT share a day's allowance, and a guest who changes
// networks gets a fresh one. Counters live in this process, like RateLimiter;
// swap for a Redis-backed store when running multiple replicas.
//
// The day boundary is UTC midnight. The reset instant travels in the
// response so a client can say when the allowance returns in local time.
type GuestAIQuota struct {
	mu    sync.Mutex
	usage map[string]*dailyUsage
	limit int
	nowFn func() time.Time
}

type dailyUsage struct {
	day  time.Time // UTC midnight of the day this count belongs to
	used int
}

// NewGuestAIQuota returns a quota of limit analyses per UTC day. A negative
// limit disables enforcement; a limit of zero blocks guests outright.
func NewGuestAIQuota(limit int) *GuestAIQuota {
	q := &GuestAIQuota{
		usage: make(map[string]*dailyUsage),
		limit: limit,
		nowFn: time.Now,
	}
	go q.gc()
	return q
}

// Limit is the number of analyses a guest may run per day.
func (q *GuestAIQuota) Limit() int { return q.limit }

func (q *GuestAIQuota) enforced() bool { return q.limit >= 0 }

func (q *GuestAIQuota) today() time.Time {
	n := q.nowFn().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func (q *GuestAIQuota) resetAt() time.Time { return q.today().AddDate(0, 0, 1) }

// entry returns the caller's counter for the current day, resetting a
// counter left over from a previous one. Callers must hold q.mu.
func (q *GuestAIQuota) entry(key string) *dailyUsage {
	day := q.today()
	e, ok := q.usage[key]
	if !ok {
		e = &dailyUsage{day: day}
		q.usage[key] = e
	}
	if !e.day.Equal(day) {
		e.day = day
		e.used = 0
	}
	return e
}

// remaining reports the allowance left without spending any of it.
func (q *GuestAIQuota) remaining(key string) (used, left int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	e := q.entry(key)
	return e.used, max(0, q.limit-e.used)
}

// consume spends one analysis, reporting the allowance left afterwards.
func (q *GuestAIQuota) consume(key string) (left int, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	e := q.entry(key)
	if e.used >= q.limit {
		return 0, false
	}
	e.used++
	return q.limit - e.used, true
}

// refund returns a spent analysis to the caller's allowance.
func (q *GuestAIQuota) refund(key string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	e := q.entry(key)
	if e.used > 0 {
		e.used--
	}
}

// gc drops counters from previous days so the map does not grow unbounded.
func (q *GuestAIQuota) gc() {
	ticker := time.NewTicker(time.Hour)
	for range ticker.C {
		q.mu.Lock()
		day := q.today()
		for k, e := range q.usage {
			if !e.day.Equal(day) {
				delete(q.usage, k)
			}
		}
		q.mu.Unlock()
	}
}

// Middleware rejects a guest request once the day's analyses are spent. It
// must run after OptionalAuthenticator: an authenticated caller passes
// straight through.
//
// An analysis that fails — a rejected payload, a provider outage — is
// refunded, so a guest only pays for results actually delivered. The spend
// happens before the handler runs and is returned afterwards, so concurrent
// requests cannot slip past the cap together.
func (q *GuestAIQuota) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !q.enforced() || isAuthenticated(r) {
			next.ServeHTTP(w, r)
			return
		}

		key := clientIP(r)
		left, ok := q.consume(key)
		if !ok {
			q.writeQuotaHeaders(w, 0)
			logging.Emit(r.Context(), logging.Event{
				Severity: logging.LevelWarning,
				Message:  "Guest AI analysis quota exhausted",
				Name:     logging.EventGuestAIQuotaExceeded,
				Category: logging.CategoryHTTP,
				Action:   r.Method + " " + r.URL.Path,
				Outcome:  logging.OutcomeFailure,
				Attrs: []slog.Attr{
					slog.Int("quota.limit", q.limit),
					slog.String("client.address_hash", ClientAddrHash(r)),
				},
			})
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":     "guest AI analyses are limited to " + strconv.Itoa(q.limit) + " per day; sign in to continue",
				"code":      GuestAIQuotaCode,
				"limit":     q.limit,
				"remaining": 0,
				"resetAt":   q.resetAt().Format(time.RFC3339),
			})
			return
		}

		// Headers are stamped when the status is known: a failing analysis is
		// refunded below, and reporting the spent count would understate what
		// the guest actually has left.
		qw := &quotaResponseWriter{
			ResponseWriter: w,
			stamp: func(status int) {
				if isSuccess(status) {
					q.writeQuotaHeaders(w, left)
				} else {
					q.writeQuotaHeaders(w, left+1)
				}
			},
		}
		next.ServeHTTP(qw, r)
		if !isSuccess(qw.statusCode()) {
			q.refund(key)
		}
	})
}

// Status reports the caller's remaining allowance without spending any of it,
// so the client can show the count before an analysis is attempted. Like the
// analysis route it takes an optional token, and reports an authenticated
// caller as unlimited.
func (q *GuestAIQuota) Status(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{"unlimited": true, "limit": 0, "used": 0, "remaining": 0}
	if q.enforced() && !isAuthenticated(r) {
		used, left := q.remaining(clientIP(r))
		q.writeQuotaHeaders(w, left)
		body = map[string]any{
			"unlimited": false,
			"limit":     q.limit,
			"used":      used,
			"remaining": left,
			"resetAt":   q.resetAt().Format(time.RFC3339),
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

func (q *GuestAIQuota) writeQuotaHeaders(w http.ResponseWriter, remaining int) {
	h := w.Header()
	h.Set(quotaLimitHeader, strconv.Itoa(q.limit))
	h.Set(quotaRemainingHeader, strconv.Itoa(remaining))
	h.Set(quotaResetHeader, q.resetAt().Format(time.RFC3339))
}

func isAuthenticated(r *http.Request) bool {
	_, ok := ClaimsFromContext(r.Context())
	return ok
}

func isSuccess(status int) bool { return status >= 200 && status < 300 }

// quotaResponseWriter runs stamp once, immediately before the status line is
// written, and records the status the handler chose.
type quotaResponseWriter struct {
	http.ResponseWriter
	stamp   func(status int)
	status  int
	written bool
}

func (w *quotaResponseWriter) WriteHeader(status int) {
	if !w.written {
		w.written = true
		w.status = status
		w.stamp(status)
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *quotaResponseWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// statusCode reports the handler's status, treating a handler that wrote a
// body without a status line as the 200 net/http would have sent.
func (w *quotaResponseWriter) statusCode() int {
	if !w.written {
		return http.StatusOK
	}
	return w.status
}
