package telemetry_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/sk-san/smart-food-manager/backend/internal/telemetry"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// This file is an external test package on purpose: internal/logging imports
// telemetry and tracing imports logging, so only a _test package can see both.

func TestSpanKindKeyMatchesTracing(t *testing.T) {
	// telemetry filters on a copied constant because importing tracing would
	// be a cycle. If the two ever diverge, the LangSmith export silently
	// filters out everything, so this is the guard.
	if got := telemetry.LLMSpanKindKeyForTest(); got != tracing.SpanKindKey {
		t.Errorf("telemetry filters on %q but tracing sets %q", got, tracing.SpanKindKey)
	}
}

func TestFilterKeepsWholeTraceThatDidLLMWork(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		telemetry.WithLLMSpanFilterForTest(sdktrace.NewSimpleSpanProcessor(exp)),
	)
	tr := tp.Tracer("test")

	// A request that calls a model: the HTTP span is the parent, the LLM run
	// is its child, and the child ends first — as it always does.
	reqCtx, httpSpan := tr.Start(context.Background(), "POST")
	_, llmSpan := tr.Start(reqCtx, "meal.analyze")
	llmSpan.SetAttributes(attribute.KeyValue{Key: tracing.SpanKindKey, Value: attribute.StringValue("llm")})
	llmSpan.End()
	httpSpan.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Both must travel: LangSmith discards a run whose parent it never
	// received, so exporting the llm span alone would lose it entirely.
	got := exported(exp)
	if len(got) != 2 || got[0] != "meal.analyze" || got[1] != "POST" {
		t.Errorf("exported %v, want the llm run and its HTTP parent", got)
	}
}

func TestFilterDropsTransportSpansInsideAnLLMTrace(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		telemetry.WithLLMSpanFilterForTest(sdktrace.NewSimpleSpanProcessor(exp)),
	)
	tr := tp.Tracer("test")

	// A fan-out: agents run concurrently, so one agent's outbound HTTP span
	// ends after another agent's LLM span has already marked the trace.
	reqCtx, httpSpan := tr.Start(context.Background(), "POST")
	panelCtx, panel := tr.Start(reqCtx, "nutrition.panel")
	panel.SetAttributes(attribute.KeyValue{Key: tracing.SpanKindKey, Value: attribute.StringValue("chain")})

	agentCtx, first := tr.Start(panelCtx, "gemini-draft")
	first.SetAttributes(attribute.KeyValue{Key: tracing.SpanKindKey, Value: attribute.StringValue("llm")})
	first.End()

	// The provider call made on an agent's behalf, plus a retry.
	for range 2 {
		_, outbound := tr.Start(agentCtx, "HTTP POST")
		outbound.End()
	}

	panel.End()
	httpSpan.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	for _, name := range exported(exp) {
		if name == "HTTP POST" {
			t.Errorf("exported transport spans: %v", exported(exp))
			break
		}
	}
	if got := len(exported(exp)); got != 3 {
		t.Errorf("exported %v, want the root, the chain, and the llm run", exported(exp))
	}
}

func TestFilterDropsTracesWithoutLLMWork(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		telemetry.WithLLMSpanFilterForTest(sdktrace.NewSimpleSpanProcessor(exp)),
	)
	tr := tp.Tracer("test")

	// A CORS preflight and a plain read. The browser sends a preflight before
	// roughly every cross-origin call, and these outnumbered the real runs in
	// the LangSmith project four to one.
	_, preflight := tr.Start(context.Background(), "OPTIONS")
	preflight.End()

	readCtx, read := tr.Start(context.Background(), "GET")
	_, db := tr.Start(readCtx, "SELECT nutrients")
	db.End()
	read.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := exported(exp); len(got) != 0 {
		t.Errorf("exported %v, want nothing — no model was called", got)
	}
}

func TestFilterSeparatesConcurrentTraces(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		telemetry.WithLLMSpanFilterForTest(sdktrace.NewSimpleSpanProcessor(exp)),
	)
	tr := tp.Tracer("test")

	// Two requests in flight at once: marking a trace must not leak into
	// another one that never touched a model.
	aiCtx, aiHTTP := tr.Start(context.Background(), "POST /analyze")
	plainCtx, plainHTTP := tr.Start(context.Background(), "GET /nutrients")

	_, llmSpan := tr.Start(aiCtx, "meal.analyze")
	llmSpan.SetAttributes(attribute.KeyValue{Key: tracing.SpanKindKey, Value: attribute.StringValue("llm")})
	llmSpan.End()

	_, db := tr.Start(plainCtx, "SELECT nutrients")
	db.End()

	plainHTTP.End()
	aiHTTP.End()

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := exported(exp)
	for _, name := range got {
		if name == "GET /nutrients" || name == "SELECT nutrients" {
			t.Errorf("exported %q from a trace that called no model: %v", name, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("exported %v, want only the AI request's two spans", got)
	}
}

// exported lists the span names that reached the exporter, in order.
func exported(exp *tracetest.InMemoryExporter) []string {
	var names []string
	for _, s := range exp.GetSpans() {
		names = append(names, s.Name)
	}
	return names
}
