// Package tracing emits OpenTelemetry spans shaped the way LangSmith's OTLP
// ingestion reads them, so an LLM call graph shows up in LangSmith as a run
// tree without a second SDK or a parallel export path.
//
// LangSmith derives a run from a span's attributes rather than its name:
//
//   - langsmith.span.kind picks the run type (chain, llm, tool).
//   - gen_ai.prompt / gen_ai.completion hold the run's inputs and outputs, as
//     JSON {"messages":[{"role":..., "content":...}]}.
//   - langsmith.usage_metadata carries the token counts LangSmith prices the
//     run with; the flat gen_ai.usage.* attributes are the OTel-standard
//     equivalents, kept for Tempo/Prometheus consumers.
//
// The spans are ordinary OTel spans on the app's tracer provider, so they also
// reach the collector configured by OTEL_EXPORTER_OTLP_ENDPOINT whether or not
// LangSmith is enabled. See internal/telemetry for the export wiring.
package tracing

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/sk-san/smart-food-manager/backend/internal/logging"
)

// LangSmith and GenAI attribute keys. These are the names the LangSmith OTLP
// converter looks for; gen_ai.prompt/completion in particular predate the
// per-message semconv events and have no semconv constant.
const (
	spanKindKey  = attribute.Key("langsmith.span.kind")
	traceNameKey = attribute.Key("langsmith.trace.name")
	sessionIDKey = attribute.Key("langsmith.metadata.session_id")
	// LangSmith prices a run by looking the model up under this key. The
	// OTel-standard gen_ai.request.model below is not what the cost
	// calculation reads, so both are set.
	modelNameKey  = attribute.Key("langsmith.metadata.ls_model_name")
	promptKey     = attribute.Key("gen_ai.prompt")
	completionKey = attribute.Key("gen_ai.completion")
	usageMetaKey  = attribute.Key("langsmith.usage_metadata")

	systemKey       = attribute.Key("gen_ai.system")
	requestModelKey = attribute.Key("gen_ai.request.model")
	operationKey    = attribute.Key("gen_ai.operation.name")
	inputTokensKey  = attribute.Key("gen_ai.usage.input_tokens")
	outputTokensKey = attribute.Key("gen_ai.usage.output_tokens")
	totalTokensKey  = attribute.Key("gen_ai.usage.total_tokens")
)

// maxContentBytes caps a single prompt or completion attribute. Span exporters
// reject oversized payloads, and a runaway model reply should never be able to
// stall the batch processor.
const maxContentBytes = 16 << 10

// truncationMarker is appended to content that was cut, so a truncated prompt
// in the LangSmith UI can never be mistaken for the whole one.
const truncationMarker = "…[truncated]"

// Kind is the LangSmith run type a span maps to.
type Kind string

const (
	// KindChain is a step that orchestrates other runs.
	KindChain Kind = "chain"
	// KindLLM is a single model call.
	KindLLM Kind = "llm"
	// KindTool is a non-model call made on the model's behalf.
	KindTool Kind = "tool"
)

// Usage is the token accounting for one model call.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	// ReasoningTokens is the share of OutputTokens the model spent thinking
	// rather than answering. It is reported separately so the cost breakdown
	// shows where the output budget went; it is already counted in
	// OutputTokens, which is what providers bill on.
	ReasoningTokens int
}

// Add returns the element-wise sum, for rolling child calls up onto the run
// that fanned them out.
func (u Usage) Add(other Usage) Usage {
	return Usage{
		InputTokens:     u.InputTokens + other.InputTokens,
		OutputTokens:    u.OutputTokens + other.OutputTokens,
		TotalTokens:     u.TotalTokens + other.TotalTokens,
		ReasoningTokens: u.ReasoningTokens + other.ReasoningTokens,
	}
}

// IsZero reports whether the call recorded no tokens at all, which is how a
// provider that does not report usage looks.
func (u Usage) IsZero() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.TotalTokens == 0
}

// Run describes the LangSmith run one span stands for.
type Run struct {
	// Name is the span name, which is also the run name in LangSmith — there
	// is no separate field for it.
	Name string
	Kind Kind
	// Provider and Model apply to KindLLM runs: the gen_ai.system value
	// (e.g. "gcp.gemini") and the model the request was sent to.
	Provider string
	Model    string
	// Root names the whole trace in the LangSmith UI. Set it on the outermost
	// run of an LLM workflow, not on the HTTP span above it.
	Root bool
}

// Recorder starts LangSmith-shaped spans on a tracer.
//
// captureContent decides whether prompts and completions ride along on the
// spans. Those spans go to every configured exporter, so enabling it puts user
// text in the trace backend as well as in LangSmith — internal/logging
// deliberately keeps prompt text out of logs, and this is the one place that
// choice is reversed on purpose.
type Recorder struct {
	tracer         trace.Tracer
	captureContent bool
}

// NewRecorder builds a Recorder from a tracer provider. A nil provider falls
// back to the global one, which is itself a no-op until telemetry.Setup
// installs the real provider — so a Recorder is always safe to call.
func NewRecorder(tp trace.TracerProvider, captureContent bool) *Recorder {
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	return &Recorder{
		tracer:         tp.Tracer("github.com/sk-san/smart-food-manager/backend/internal/tracing"),
		captureContent: captureContent,
	}
}

// Span is a started run. Exactly one of Succeed or Fail should be called, then
// End — typically deferred.
type Span struct {
	span           trace.Span
	captureContent bool
}

// Start opens a run and returns a context parented to it, so any span started
// underneath nests inside this run.
func (r *Recorder) Start(ctx context.Context, run Run, input string) (context.Context, *Span) {
	attrs := []attribute.KeyValue{
		spanKindKey.String(string(run.Kind)),
		operationKey.String("chat"),
	}
	if run.Root {
		attrs = append(attrs, traceNameKey.String(run.Name))
	}
	if run.Provider != "" {
		attrs = append(attrs, systemKey.String(run.Provider))
	}
	if run.Model != "" {
		attrs = append(attrs, requestModelKey.String(run.Model), modelNameKey.String(run.Model))
	}
	// Groups the trace into a LangSmith thread using the same session
	// identifier the structured logs are keyed by.
	if sessionID := logging.SessionIDFrom(ctx); sessionID != "" {
		attrs = append(attrs, sessionIDKey.String(sessionID))
	}
	if r.captureContent {
		if msg, ok := messagesJSON("user", input); ok {
			attrs = append(attrs, promptKey.String(msg))
		}
	}

	ctx, span := r.tracer.Start(ctx, run.Name, trace.WithAttributes(attrs...))
	return ctx, &Span{span: span, captureContent: r.captureContent}
}

// Succeed records the run's output and token usage and marks it OK.
func (s *Span) Succeed(output string, u Usage) {
	attrs := make([]attribute.KeyValue, 0, 5)

	if s.captureContent {
		if msg, ok := messagesJSON("assistant", output); ok {
			attrs = append(attrs, completionKey.String(msg))
		}
	}

	// LangSmith prices the run from the usage_metadata JSON and ignores the
	// flat keys; the flat keys are the OTel-standard names other backends read.
	if !u.IsZero() {
		meta := map[string]any{
			"input_tokens":  u.InputTokens,
			"output_tokens": u.OutputTokens,
			"total_tokens":  u.TotalTokens,
		}
		// Reported under the details key LangSmith understands, so a thinking
		// model's output is broken down rather than looking like one long
		// answer. It stays inside output_tokens, which is what is billed.
		if u.ReasoningTokens > 0 {
			meta["output_token_details"] = map[string]int{"reasoning": u.ReasoningTokens}
		}
		if encoded, err := json.Marshal(meta); err == nil {
			attrs = append(attrs, usageMetaKey.String(string(encoded)))
		}
		attrs = append(attrs,
			inputTokensKey.Int(u.InputTokens),
			outputTokensKey.Int(u.OutputTokens),
			totalTokensKey.Int(u.TotalTokens),
		)
	}

	s.span.SetAttributes(attrs...)
	s.span.SetStatus(codes.Ok, "")
}

// Fail marks the run as errored. The error text lands on the span, so it must
// stay free of prompt content and credentials — the errors from
// internal/gemini are.
func (s *Span) Fail(err error) {
	if err == nil {
		return
	}
	s.span.RecordError(err)
	s.span.SetStatus(codes.Error, err.Error())
}

// End closes the run.
func (s *Span) End() { s.span.End() }

// SpanContext exposes the underlying span's context, for callers that need to
// correlate a trace id with something outside OTel.
func (s *Span) SpanContext() trace.SpanContext { return s.span.SpanContext() }

// messagesJSON renders one message the way the LangSmith converter parses
// prompts and completions. It reports false for empty content, which would
// otherwise show up as an empty run input in the UI.
func messagesJSON(role, content string) (string, bool) {
	if content == "" {
		return "", false
	}
	if len(content) > maxContentBytes {
		content = logging.Truncate(content, maxContentBytes) + truncationMarker
	}
	out, err := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": role, "content": content}},
	})
	if err != nil {
		return "", false
	}
	return string(out), true
}
