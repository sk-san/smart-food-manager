// Package logging implements the project's structured logging design
// (see observability/README.md): every entry is a common envelope —
// service identity, event triple, trace context, request/session/user
// context — plus event-specific attributes. Entries flow through the
// default slog handler, which the OTel bridge exports to Loki.
package logging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"time"
	"unicode/utf8"

	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"
)

// Severity levels per the blueprint policy. NOTICE and CRITICAL extend
// slog's built-ins; the explicit "severity" attribute on every entry is
// the authoritative blueprint name regardless of how the OTel bridge
// maps the numeric level.
const (
	LevelDebug    = slog.LevelDebug
	LevelInfo     = slog.LevelInfo
	LevelNotice   = slog.Level(2)
	LevelWarning  = slog.LevelWarn
	LevelError    = slog.LevelError
	LevelCritical = slog.Level(12)
)

// SeverityString returns the blueprint severity name for a slog level.
func SeverityString(l slog.Level) string {
	switch {
	case l >= LevelCritical:
		return "CRITICAL"
	case l >= LevelError:
		return "ERROR"
	case l >= LevelWarning:
		return "WARNING"
	case l >= LevelNotice:
		return "NOTICE"
	case l >= LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}

var severityNames = map[string]slog.Level{
	"DEBUG":    LevelDebug,
	"INFO":     LevelInfo,
	"NOTICE":   LevelNotice,
	"WARNING":  LevelWarning,
	"ERROR":    LevelError,
	"CRITICAL": LevelCritical,
}

// ParseSeverity maps a blueprint severity name to its slog level.
func ParseSeverity(s string) (slog.Level, bool) {
	l, ok := severityNames[s]
	return l, ok
}

var svc = struct {
	source  string
	name    string
	version string
	env     string
	salt    string
}{source: "backend", name: "backend-api", version: "0.0.0", env: "development"}

// SetService configures the identity stamped on every log entry.
// Call once at startup, before the first Emit.
func SetService(name, version, environment string) {
	svc.name = name
	svc.version = version
	svc.env = environment
}

// SetHashSalt sets the salt mixed into HashIdentifier. Configure via
// LOG_HASH_SALT so hashed identifiers cannot be reversed by brute force.
func SetHashSalt(salt string) { svc.salt = salt }

// HashIdentifier pseudonymises a sensitive identifier (client IP, user ID,
// etc.) as "sha256:<hex>" per the blueprint's PII rules.
func HashIdentifier(v string) string {
	sum := sha256.Sum256([]byte(svc.salt + v))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Truncate caps s at max bytes without splitting a UTF-8 rune.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// Event is one structured log entry.
type Event struct {
	Severity slog.Level
	Message  string

	Name     string // event.name — stable machine-readable name from the catalog
	Category string // event.category — ux, http, auth, dependency, file, account
	Action   string // event.action — specific action, e.g. "GET /api/v1/nutrients"
	Outcome  string // event.outcome — started, success, or failure

	// Overrides for entries relayed on behalf of another source (the
	// frontend telemetry endpoint). Zero values fall back to this
	// service's identity and to values carried in ctx.
	Time           time.Time
	Source         string
	ServiceName    string
	ServiceVersion string
	TraceID        string
	SpanID         string
	TraceSampled   *bool
	RequestID      string
	SessionID      string
	UserID         string

	Duration time.Duration // emitted as duration_ms when non-zero
	Attrs    []slog.Attr   // event-specific fields
}

// Emit writes the event through the default slog handler with the common
// envelope applied. Trace context is taken from ctx unless the event
// carries explicit identifiers.
func Emit(ctx context.Context, e Event) {
	h := slog.Default().Handler()
	if !h.Enabled(ctx, e.Severity) {
		return
	}

	source := e.Source
	if source == "" {
		source = svc.source
	}
	name := e.ServiceName
	if name == "" {
		name = svc.name
	}
	version := e.ServiceVersion
	if version == "" {
		version = svc.version
	}

	attrs := make([]slog.Attr, 0, 16+len(e.Attrs))
	attrs = append(attrs,
		slog.String("severity", SeverityString(e.Severity)),
		slog.String("source", source),
		slog.String("service.name", name),
		slog.String("service.version", version),
		slog.String("deployment.environment", svc.env),
		slog.String("event.name", e.Name),
		slog.String("event.category", e.Category),
		slog.String("event.action", e.Action),
		slog.String("event.outcome", e.Outcome),
	)

	traceID, spanID, sampled := e.TraceID, e.SpanID, e.TraceSampled
	if traceID == "" {
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			traceID = sc.TraceID().String()
			spanID = sc.SpanID().String()
			s := sc.IsSampled()
			sampled = &s
		}
	}
	if traceID != "" {
		attrs = append(attrs, slog.String("trace_id", traceID))
		if spanID != "" {
			attrs = append(attrs, slog.String("span_id", spanID))
		}
		if sampled != nil {
			attrs = append(attrs, slog.Bool("trace_sampled", *sampled))
		}
	}

	if rid := firstNonEmpty(e.RequestID, chimw.GetReqID(ctx)); rid != "" {
		attrs = append(attrs, slog.String("request_id", rid))
	}
	if sid := firstNonEmpty(e.SessionID, SessionIDFrom(ctx)); sid != "" {
		attrs = append(attrs, slog.String("session_id", sid))
	}
	if uid := firstNonEmpty(e.UserID, UserIDFrom(ctx)); uid != "" {
		attrs = append(attrs, slog.String("user_id", uid))
	}
	if e.Duration > 0 {
		attrs = append(attrs, slog.Int64("duration_ms", e.Duration.Milliseconds()))
	}
	attrs = append(attrs, e.Attrs...)

	t := e.Time
	if t.IsZero() {
		t = time.Now()
	}
	rec := slog.NewRecord(t, e.Severity, e.Message, 0)
	rec.AddAttrs(attrs...)
	_ = h.Handle(ctx, rec)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
