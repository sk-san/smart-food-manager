package telemetry

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Recommended application metrics from the structured logging blueprint
// (section 17). Dimensions must stay low-cardinality: route templates,
// methods, status codes, providers, and outcomes — never trace/request/
// session/user identifiers.
//
// Instruments are created against the global meter provider at package
// init; the OTel globals delegate them to the real provider once Setup
// installs it, and degrade to no-ops if telemetry is unavailable.
var (
	HTTPRequestsTotal        metric.Int64Counter
	HTTPRequestDurationMS    metric.Float64Histogram
	AuthLoginAttemptsTotal   metric.Int64Counter
	ExternalAPIRequestsTotal metric.Int64Counter
	ExternalAPIDurationMS    metric.Float64Histogram
	LLMInputTokensTotal      metric.Int64Counter
	LLMOutputTokensTotal     metric.Int64Counter
	FileUploadsTotal         metric.Int64Counter
	FileUploadDurationMS     metric.Float64Histogram
)

func init() {
	m := otel.Meter("github.com/sk-san/smart-food-manager/backend/internal/telemetry")
	durationBuckets := metric.WithExplicitBucketBoundaries(5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000)

	HTTPRequestsTotal, _ = m.Int64Counter("http_server_requests_total",
		metric.WithDescription("HTTP requests served"))
	HTTPRequestDurationMS, _ = m.Float64Histogram("http_server_request_duration_ms",
		metric.WithDescription("HTTP request duration"),
		metric.WithUnit("ms"), durationBuckets)
	AuthLoginAttemptsTotal, _ = m.Int64Counter("auth_login_attempts_total",
		metric.WithDescription("Login attempts by outcome"))
	ExternalAPIRequestsTotal, _ = m.Int64Counter("external_api_requests_total",
		metric.WithDescription("External API calls by provider, service, operation, and outcome"))
	ExternalAPIDurationMS, _ = m.Float64Histogram("external_api_duration_ms",
		metric.WithDescription("External API call duration"),
		metric.WithUnit("ms"), durationBuckets)
	LLMInputTokensTotal, _ = m.Int64Counter("llm_input_tokens_total",
		metric.WithDescription("LLM input tokens consumed"))
	LLMOutputTokensTotal, _ = m.Int64Counter("llm_output_tokens_total",
		metric.WithDescription("LLM output tokens produced"))
	FileUploadsTotal, _ = m.Int64Counter("file_uploads_total",
		metric.WithDescription("File uploads by outcome"))
	FileUploadDurationMS, _ = m.Float64Histogram("file_upload_duration_ms",
		metric.WithDescription("File upload processing duration"),
		metric.WithUnit("ms"), durationBuckets)
}
