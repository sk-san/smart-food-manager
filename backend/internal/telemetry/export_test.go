package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Test hooks. The filter and its key are unexported because nothing outside
// this package should build one, but the drift guard against internal/tracing
// has to live in an external test package to avoid an import cycle.

// LLMSpanKindKeyForTest exposes the attribute key the LangSmith export filters on.
func LLMSpanKindKeyForTest() attribute.Key { return llmSpanKindKey }

// WithLLMSpanFilterForTest wraps next in the same filter Setup applies.
func WithLLMSpanFilterForTest(next sdktrace.SpanProcessor) sdktrace.TracerProviderOption {
	return sdktrace.WithSpanProcessor(newLLMTracesOnly(next))
}
