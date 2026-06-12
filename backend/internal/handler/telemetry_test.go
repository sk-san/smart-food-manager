package handler

import (
	"testing"
	"time"
)

func validRawEvent() map[string]any {
	return map[string]any{
		"time":          time.Now().UTC().Format(time.RFC3339Nano),
		"severity":      "INFO",
		"message":       "Screen viewed: home",
		"event.name":    "screen_view",
		"event.action":  "home",
		"event.outcome": "success",
		"trace_id":      "4bf92f3577b34da6a3ce929d0e0e4736",
		"span_id":       "00f067aa0ba902b7",
		"trace_sampled": true,
		"session_id":    "sess-1a2b3c4d",
		"screen.name":   "home",
		"duration_ms":   float64(42),
	}
}

func TestFrontendEventAccepted(t *testing.T) {
	e, ok := frontendEvent(validRawEvent(), "sha256:user", "sha256:addr", "UA")
	if !ok {
		t.Fatal("valid event rejected")
	}
	if e.Source != "frontend" || e.ServiceName != "frontend-web" {
		t.Errorf("wrong relay identity: %q %q", e.Source, e.ServiceName)
	}
	if e.Category != "ux" {
		t.Errorf("category not canonicalised: %q", e.Category)
	}
	if e.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || e.SpanID != "00f067aa0ba902b7" {
		t.Errorf("trace context lost: %q %q", e.TraceID, e.SpanID)
	}
	if e.UserID != "sha256:user" {
		t.Errorf("user_id must come from the verified token, got %q", e.UserID)
	}
	if e.Duration != 42*time.Millisecond {
		t.Errorf("duration: %v", e.Duration)
	}
}

func TestFrontendEventRejected(t *testing.T) {
	cases := map[string]func(m map[string]any){
		"unknown event name":   func(m map[string]any) { m["event.name"] = "made_up_event" },
		"missing message":      func(m map[string]any) { delete(m, "message") },
		"bad outcome":          func(m map[string]any) { m["event.outcome"] = "partial" },
		"critical severity":    func(m map[string]any) { m["severity"] = "CRITICAL" },
		"unparseable severity": func(m map[string]any) { m["severity"] = "verbose" },
	}
	for name, mutate := range cases {
		raw := validRawEvent()
		mutate(raw)
		if _, ok := frontendEvent(raw, "", "h", "UA"); ok {
			t.Errorf("%s: event accepted, want rejected", name)
		}
	}
}

func TestFrontendEventDropsDisallowedFields(t *testing.T) {
	raw := validRawEvent()
	raw["password"] = "hunter2"
	raw["user_id"] = "spoofed-user"
	raw["trace_id"] = "ZZ92f3577b34da6a3ce929d0e0e4736" // malformed hex

	e, ok := frontendEvent(raw, "", "sha256:addr", "UA")
	if !ok {
		t.Fatal("event rejected")
	}
	if e.TraceID != "" {
		t.Errorf("malformed trace_id kept: %q", e.TraceID)
	}
	if e.UserID != "" {
		t.Error("payload user_id must be ignored")
	}
	for _, a := range e.Attrs {
		if a.Key == "password" || a.Key == "user_id" {
			t.Errorf("disallowed field relayed: %s", a.Key)
		}
	}
}
