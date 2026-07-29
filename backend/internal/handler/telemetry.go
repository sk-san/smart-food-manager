package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/example/food-app/backend/internal/logging"
	"github.com/example/food-app/backend/internal/middleware"
)

const (
	telemetryMaxBody   = 256 << 10 // 256 KiB per batch
	telemetryMaxEvents = 50
)

var (
	traceIDPattern   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	spanIDPattern    = regexp.MustCompile(`^[0-9a-f]{16}$`)
	sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

// telemetryAttrAllowlist maps accepted event-specific fields to a kind
// ("s" string, "i" integer). Everything else is dropped, so the frontend
// can never push secrets, raw prompts, or PII into the logs.
var telemetryAttrAllowlist = map[string]string{
	"screen.name": "s", "screen.from": "s", "screen.to": "s",
	"client.locale": "s", "client.timezone": "s",
	"client.viewport_width": "i", "client.viewport_height": "i",
	"http.request.method": "s", "http.route": "s",
	"http.response.status_code": "i",
	"error.type":                "s", "error.code": "s", "error.message": "s",
}

// TelemetryHandler ingests structured UX/event logs from the frontend and
// relays them into the shared logging pipeline with source=frontend, per
// the blueprint: the browser never talks to Loki directly.
type TelemetryHandler struct{}

func NewTelemetryHandler() *TelemetryHandler { return &TelemetryHandler{} }

// Ingest accepts {"events": [...]} where each event follows the frontend
// envelope. Events with unknown names, malformed identifiers, or
// disallowed fields are dropped individually; the response reports both
// counts so the frontend can detect schema drift.
func (h *TelemetryHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, telemetryMaxBody)
	var batch struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if len(batch.Events) > telemetryMaxEvents {
		writeError(w, http.StatusBadRequest, "too many events in batch")
		return
	}

	// user_id comes only from a verified token (OptionalAuthenticator),
	// never from the payload; the client address is hashed server-side
	// because the frontend cannot be trusted as a source of IPs.
	userID := logging.UserIDFrom(r.Context())
	addrHash := middleware.ClientAddrHash(r)
	userAgent := r.UserAgent()

	accepted, dropped := 0, 0
	for _, raw := range batch.Events {
		e, ok := frontendEvent(raw, userID, addrHash, userAgent)
		if !ok {
			dropped++
			continue
		}
		// Emit without the ingest request's context so the relayed record
		// carries the browser's trace context, not this endpoint's span.
		logging.Emit(context.Background(), e)
		accepted++
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"accepted": accepted, "dropped": dropped})
}

// frontendEvent validates one raw frontend event against the catalog and
// allowlist and converts it into a logging.Event.
func frontendEvent(raw map[string]any, userID, addrHash, userAgent string) (logging.Event, bool) {
	name := str(raw["event.name"])
	category, known := logging.FrontendEventCategories[name]
	if !known {
		return logging.Event{}, false
	}
	sev, ok := logging.ParseSeverity(str(raw["severity"]))
	if !ok || sev > logging.LevelError {
		// The frontend may not emit CRITICAL or above.
		return logging.Event{}, false
	}
	msg := logging.Truncate(str(raw["message"]), 512)
	if msg == "" {
		return logging.Event{}, false
	}
	outcome := str(raw["event.outcome"])
	switch outcome {
	case logging.OutcomeStarted, logging.OutcomeSuccess, logging.OutcomeFailure:
	default:
		return logging.Event{}, false
	}

	e := logging.Event{
		Severity:       sev,
		Message:        msg,
		Name:           name,
		Category:       category,
		Action:         logging.Truncate(str(raw["event.action"]), 128),
		Outcome:        outcome,
		Source:         "frontend",
		ServiceName:    "frontend-web",
		ServiceVersion: "unknown",
		UserID:         userID,
	}
	if v := str(raw["service.version"]); v != "" {
		e.ServiceVersion = logging.Truncate(v, 32)
	}
	// Trust client timestamps only within a sane window around now;
	// otherwise the ingest time on the record stands.
	if t, err := time.Parse(time.RFC3339Nano, str(raw["time"])); err == nil {
		now := time.Now()
		if t.After(now.Add(-10*time.Minute)) && t.Before(now.Add(time.Minute)) {
			e.Time = t
		}
	}
	if tid := str(raw["trace_id"]); traceIDPattern.MatchString(tid) {
		e.TraceID = tid
		if sid := str(raw["span_id"]); spanIDPattern.MatchString(sid) {
			e.SpanID = sid
		}
		if s, ok := raw["trace_sampled"].(bool); ok {
			e.TraceSampled = &s
		}
	}
	if sid := str(raw["session_id"]); sessionIDPattern.MatchString(sid) {
		e.SessionID = sid
	}
	if d, ok := num(raw["duration_ms"]); ok && d >= 0 && d < 86_400_000 {
		e.Duration = time.Duration(d * float64(time.Millisecond))
	}

	attrs := []slog.Attr{
		slog.String("client.user_agent", userAgent),
		slog.String("client.address_hash", addrHash),
	}
	for key, kind := range telemetryAttrAllowlist {
		v, present := raw[key]
		if !present {
			continue
		}
		switch kind {
		case "s":
			if s := str(v); s != "" {
				attrs = append(attrs, slog.String(key, logging.Truncate(s, 512)))
			}
		case "i":
			if n, ok := num(v); ok {
				attrs = append(attrs, slog.Int64(key, int64(n)))
			}
		}
	}
	e.Attrs = attrs
	return e, true
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) (float64, bool) {
	n, ok := v.(float64)
	return n, ok
}
