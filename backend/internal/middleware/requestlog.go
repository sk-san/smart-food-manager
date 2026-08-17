package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/sk-san/smart-food-manager/backend/internal/logging"
	"github.com/sk-san/smart-food-manager/backend/internal/telemetry"
)

// RequestLogger emits the blueprint's HTTP lifecycle events
// (request_received, request_completed, request_failed) and records the
// http_server_* metrics. Logs carry the chi route template, never the raw
// path, to keep cardinality bounded. Paths in skip (health probes) are
// neither logged nor measured.
func RequestLogger(skip ...string) func(http.Handler) http.Handler {
	skipped := make(map[string]struct{}, len(skip))
	for _, p := range skip {
		skipped[p] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := skipped[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			ctx := r.Context()
			clientAttrs := []slog.Attr{
				slog.String("client.user_agent", r.UserAgent()),
				slog.String("client.address_hash", ClientAddrHash(r)),
			}

			logging.Emit(ctx, logging.Event{
				Severity: logging.LevelInfo,
				Message:  "Request received",
				Name:     logging.EventRequestReceived,
				Category: logging.CategoryHTTP,
				// The route template resolves during routing, so the
				// received event carries only the method.
				Action:  r.Method,
				Outcome: logging.OutcomeStarted,
				Attrs: append([]slog.Attr{
					slog.String("http.request.method", r.Method),
				}, clientAttrs...),
			})

			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			route := chi.RouteContext(ctx).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			duration := time.Since(start)

			e := logging.Event{
				Severity: logging.LevelInfo,
				Message:  "Request completed",
				Name:     logging.EventRequestCompleted,
				Category: logging.CategoryHTTP,
				Action:   r.Method + " " + route,
				Outcome:  logging.OutcomeSuccess,
				Duration: duration,
				Attrs: append([]slog.Attr{
					slog.String("http.request.method", r.Method),
					slog.String("http.route", route),
					slog.Int("http.response.status_code", status),
				}, clientAttrs...),
			}
			switch {
			case status >= 500:
				e.Severity = logging.LevelError
				e.Message = "Request failed"
				e.Name = logging.EventRequestFailed
				e.Outcome = logging.OutcomeFailure
			case status >= 400:
				e.Severity = logging.LevelWarning
				e.Outcome = logging.OutcomeFailure
			}
			logging.Emit(ctx, e)

			mattrs := metric.WithAttributes(
				attribute.String("http.request.method", r.Method),
				attribute.String("http.route", route),
				attribute.Int("http.response.status_code", status),
				attribute.String("event.outcome", e.Outcome),
			)
			telemetry.HTTPRequestsTotal.Add(ctx, 1, mattrs)
			telemetry.HTTPRequestDurationMS.Record(ctx, float64(duration)/float64(time.Millisecond), mattrs)
		})
	}
}

// ClientAddrHash returns the pseudonymised client IP (set by RealIP) for
// log fields — the blueprint forbids logging raw addresses.
func ClientAddrHash(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return logging.HashIdentifier(host)
}
