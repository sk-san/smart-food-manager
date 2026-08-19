package tracing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/sk-san/smart-food-manager/backend/internal/logging"
)

// newTestRecorder returns a Recorder writing to an in-memory span recorder.
func newTestRecorder(t *testing.T, captureContent bool) (*Recorder, *tracetest.SpanRecorder) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})
	return NewRecorder(tp, captureContent), sr
}

// attrs flattens a recorded span's attributes for lookup by key.
func attrs(t *testing.T, span sdktrace.ReadOnlySpan) map[attribute.Key]attribute.Value {
	t.Helper()
	out := map[attribute.Key]attribute.Value{}
	for _, kv := range span.Attributes() {
		out[kv.Key] = kv.Value
	}
	return out
}

func TestStartLLMRunAttributes(t *testing.T) {
	rec, sr := newTestRecorder(t, true)

	_, span := rec.Start(context.Background(), Run{
		Name:     "nutrition-agent",
		Kind:     KindLLM,
		Provider: "gcp.gemini",
		Model:    "gemini-2.5-flash",
	}, "how much protein is in lentils?")
	span.Succeed("about 9g per 100g cooked", Usage{InputTokens: 12, OutputTokens: 8, TotalTokens: 20})
	span.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(ended))
	}
	got := ended[0]
	if got.Name() != "nutrition-agent" {
		t.Errorf("span name = %q, want the run name", got.Name())
	}

	a := attrs(t, got)
	if v := a[spanKindKey].AsString(); v != "llm" {
		t.Errorf("langsmith.span.kind = %q, want llm", v)
	}
	if _, ok := a[traceNameKey]; ok {
		t.Error("langsmith.trace.name set on a non-root run")
	}
	if v := a[systemKey].AsString(); v != "gcp.gemini" {
		t.Errorf("gen_ai.system = %q", v)
	}
	if v := a[requestModelKey].AsString(); v != "gemini-2.5-flash" {
		t.Errorf("gen_ai.request.model = %q", v)
	}

	// LangSmith reads inputs and outputs as a JSON message list.
	if role, content := decodeMessage(t, a[promptKey].AsString()); role != "user" || content != "how much protein is in lentils?" {
		t.Errorf("gen_ai.prompt = %q/%q", role, content)
	}
	if role, content := decodeMessage(t, a[completionKey].AsString()); role != "assistant" || content != "about 9g per 100g cooked" {
		t.Errorf("gen_ai.completion = %q/%q", role, content)
	}

	var usage map[string]int
	if err := json.Unmarshal([]byte(a[usageMetaKey].AsString()), &usage); err != nil {
		t.Fatalf("langsmith.usage_metadata is not JSON: %v", err)
	}
	if usage["input_tokens"] != 12 || usage["output_tokens"] != 8 || usage["total_tokens"] != 20 {
		t.Errorf("usage_metadata = %v", usage)
	}
	if a[inputTokensKey].AsInt64() != 12 || a[totalTokensKey].AsInt64() != 20 {
		t.Error("flat gen_ai.usage.* attributes not recorded")
	}
	if got.Status().Code != codes.Ok {
		t.Errorf("status = %v, want Ok", got.Status().Code)
	}
}

func TestStartRootRunIsNamedAndThreaded(t *testing.T) {
	rec, sr := newTestRecorder(t, true)

	ctx := logging.WithSessionID(context.Background(), "session-abc")
	_, span := rec.Start(ctx, Run{Name: "advice.fan_out", Kind: KindChain, Root: true}, "what should I eat?")
	span.Succeed("lentils", Usage{})
	span.End()

	a := attrs(t, sr.Ended()[0])
	if v := a[spanKindKey].AsString(); v != "chain" {
		t.Errorf("langsmith.span.kind = %q, want chain", v)
	}
	if v := a[traceNameKey].AsString(); v != "advice.fan_out" {
		t.Errorf("langsmith.trace.name = %q, want the run name on a root run", v)
	}
	// The trace joins the same LangSmith thread the app's logs are keyed by.
	if v := a[sessionIDKey].AsString(); v != "session-abc" {
		t.Errorf("langsmith.metadata.session_id = %q", v)
	}
	// Zero usage must not be reported as a real (free) call.
	if _, ok := a[usageMetaKey]; ok {
		t.Error("usage_metadata set for a run with no token counts")
	}
}

func TestCaptureContentDisabledKeepsTextOffSpans(t *testing.T) {
	rec, sr := newTestRecorder(t, false)

	_, span := rec.Start(context.Background(), Run{Name: "agent", Kind: KindLLM}, "a private prompt")
	span.Succeed("a private answer", Usage{InputTokens: 3, OutputTokens: 4, TotalTokens: 7})
	span.End()

	a := attrs(t, sr.Ended()[0])
	if _, ok := a[promptKey]; ok {
		t.Error("prompt text recorded with content capture off")
	}
	if _, ok := a[completionKey]; ok {
		t.Error("completion text recorded with content capture off")
	}
	// Metadata is still useful without the text, so usage must survive.
	if a[inputTokensKey].AsInt64() != 3 {
		t.Error("token usage dropped along with the content")
	}
}

func TestContentIsTruncatedAndMarked(t *testing.T) {
	rec, sr := newTestRecorder(t, true)

	long := strings.Repeat("x", maxContentBytes+500)
	_, span := rec.Start(context.Background(), Run{Name: "agent", Kind: KindLLM}, long)
	span.Succeed("ok", Usage{})
	span.End()

	_, content := decodeMessage(t, attrs(t, sr.Ended()[0])[promptKey].AsString())
	if len(content) > maxContentBytes+len(truncationMarker) {
		t.Errorf("prompt attribute is %d bytes, want it capped near %d", len(content), maxContentBytes)
	}
	if !strings.HasSuffix(content, truncationMarker) {
		t.Error("truncated prompt is not marked as truncated")
	}
}

func TestFailRecordsError(t *testing.T) {
	rec, sr := newTestRecorder(t, true)

	_, span := rec.Start(context.Background(), Run{Name: "agent", Kind: KindLLM}, "prompt")
	span.Fail(errors.New("gemini: status 429"))
	span.End()

	got := sr.Ended()[0]
	if got.Status().Code != codes.Error {
		t.Errorf("status = %v, want Error", got.Status().Code)
	}
	if got.Status().Description != "gemini: status 429" {
		t.Errorf("status description = %q", got.Status().Description)
	}
	if len(got.Events()) == 0 {
		t.Error("error not recorded as a span event")
	}
}

func TestChildRunNestsUnderParent(t *testing.T) {
	rec, sr := newTestRecorder(t, true)

	ctx, parent := rec.Start(context.Background(), Run{Name: "chain", Kind: KindChain, Root: true}, "prompt")
	_, child := rec.Start(ctx, Run{Name: "llm", Kind: KindLLM}, "prompt")
	child.Succeed("out", Usage{})
	child.End()
	parent.Succeed("out", Usage{})
	parent.End()

	ended := sr.Ended()
	if len(ended) != 2 {
		t.Fatalf("recorded %d spans, want 2", len(ended))
	}
	// Ended in child-then-parent order, which is what the run tree needs.
	if ended[0].Parent().SpanID() != ended[1].SpanContext().SpanID() {
		t.Error("llm run is not a child of the chain run")
	}
	if ended[0].SpanContext().TraceID() != ended[1].SpanContext().TraceID() {
		t.Error("runs are not in the same trace")
	}
}

func TestUsageAdd(t *testing.T) {
	got := Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}.
		Add(Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30})
	want := Usage{InputTokens: 11, OutputTokens: 22, TotalTokens: 33}
	if got != want {
		t.Errorf("Add = %+v, want %+v", got, want)
	}
	if !(Usage{}).IsZero() || (Usage{TotalTokens: 1}).IsZero() {
		t.Error("IsZero is wrong")
	}
}

// decodeMessage unwraps the {"messages":[{role,content}]} envelope.
func decodeMessage(t *testing.T, raw string) (role, content string) {
	t.Helper()
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("attribute is not a message envelope: %v (%q)", err, raw)
	}
	if len(payload.Messages) != 1 {
		t.Fatalf("envelope holds %d messages, want 1", len(payload.Messages))
	}
	return payload.Messages[0].Role, payload.Messages[0].Content
}
