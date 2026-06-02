package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimiter is a simple per-client-IP token bucket. It avoids external
// dependencies; swap for a Redis-backed limiter when running multiple
// instances behind the API gateway.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rps     float64
	burst   float64
}

func NewRateLimiter(rps, burst float64) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rps:     rps,
		burst:   burst,
	}
	go rl.gc()
	return rl
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &bucket{tokens: rl.burst - 1, last: now}
		return true
	}
	// Refill based on elapsed time, capped at burst.
	b.tokens += now.Sub(b.last).Seconds() * rl.rps
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// gc periodically drops idle buckets so the map does not grow unbounded.
func (rl *RateLimiter) gc() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		for k, b := range rl.buckets {
			if time.Since(b.last) > 10*time.Minute {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware enforces the rate limit per client IP.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := indexComma(fwd); i >= 0 {
			return fwd[:i]
		}
		return fwd
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func indexComma(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			return i
		}
	}
	return -1
}
