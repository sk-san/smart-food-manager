package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Option configures Setup. Options exist so extra span destinations can be
// added without every caller of Setup having to name them.
type Option func(*options)

type options struct {
	langsmithAPIKey   string
	langsmithProject  string
	langsmithEndpoint string
}

// WithLangSmith also exports spans to LangSmith, for reading LLM runs as a
// trace tree. An empty apiKey turns it off, which is the default: without a key
// the app traces exactly as before, to the collector alone. An empty endpoint
// uses the SDK's (api.smith.langchain.com).
func WithLangSmith(apiKey, project, endpoint string) Option {
	return func(o *options) {
		o.langsmithAPIKey = apiKey
		o.langsmithProject = project
		o.langsmithEndpoint = endpoint
	}
}

// LangSmith OTLP ingestion details.
const (
	defaultLangSmithHost  = "eu.api.smith.langchain.com"
	langsmithTracesPath   = "/otel/v1/traces"
	langsmithBatchTimeout = time.Second
)

// langsmithExporter builds the OTLP/HTTP exporter that ships spans to
// LangSmith.
//
// The exporter is assembled here rather than through langsmith.NewOTel because
// that helper configures it with otlptracehttp.WithEndpoint, which leaves the
// transport scheme to the environment. OTEL_EXPORTER_OTLP_INSECURE is global
// and this service sets it for its local collector, so the LangSmith export
// would silently downgrade to http:// — which the API answers with 405. Only
// WithEndpointURL pins the scheme per exporter.
func langsmithExporter(ctx context.Context, cfg options) (sdktrace.SpanExporter, error) {
	host := normalizeOTLPEndpoint(cfg.langsmithEndpoint)
	if host == "" {
		host = defaultLangSmithHost
	}
	return otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL("https://"+host+langsmithTracesPath),
		otlptracehttp.WithHeaders(map[string]string{
			"x-api-key":         cfg.langsmithAPIKey,
			"Langsmith-Project": cfg.langsmithProject,
		}),
	)
}

// llmSpanKindKey duplicates tracing.SpanKindKey rather than importing it:
// internal/logging imports this package and internal/tracing imports logging,
// so the import would be a cycle. TestSpanKindKeyMatchesTracing keeps the two
// from drifting.
const llmSpanKindKey = attribute.Key("langsmith.span.kind")

// llmTraceTTL bounds how long a trace stays marked as containing LLM work.
// Every span of a request ends within seconds of the others, so this only has
// to outlive a single request; it exists to keep the map from growing.
const llmTraceTTL = 5 * time.Minute

// llmTracesOnly forwards to LangSmith only the spans belonging to a trace that
// contains LLM work.
//
// Both destinations share one tracer provider, so unfiltered every span the
// service produces is exported — and because the browser preflights each
// cross-origin call, CORS OPTIONS runs outnumbered real ones sixteen to four.
// That buries the LLM runs the tool exists to show and spends the trace quota
// on transport.
//
// Filtering to the LLM spans alone does not work: LangSmith discards a run
// whose parent span it never received, so the llm run disappears along with
// the HTTP span above it. The whole trace has to travel together. Keeping the
// HTTP parent is not merely a workaround either — LangSmith rolls the child's
// tokens and cost up onto it, so it is the run that shows what a request cost.
//
// A span is forwarded when it carries the LLM attribute, or when it is the
// root of a trace already marked as LLM work. Children end before their
// parents, so by the time the HTTP root ends the trace is marked.
//
// Restricting the second case to the root matters: a fan-out runs its agents
// concurrently, so one agent's outbound HTTP span often ends after another
// agent's LLM span has marked the trace. Forwarding everything in a marked
// trace let nine transport spans through for a single panel request.
type llmTracesOnly struct {
	sdktrace.SpanProcessor

	mu   sync.Mutex
	seen map[trace.TraceID]time.Time
}

// newLLMTracesOnly wraps next with the trace filter.
func newLLMTracesOnly(next sdktrace.SpanProcessor) *llmTracesOnly {
	return &llmTracesOnly{SpanProcessor: next, seen: map[trace.TraceID]time.Time{}}
}

// OnEnd forwards the span if its trace involves LLM work.
func (p *llmTracesOnly) OnEnd(span sdktrace.ReadOnlySpan) {
	traceID := span.SpanContext().TraceID()

	if isLLMSpan(span) {
		p.mark(traceID)
		p.SpanProcessor.OnEnd(span)
		return
	}
	// The root is kept because LangSmith discards a run whose parent it never
	// received, and because it is the run the child costs roll up onto.
	if (!span.Parent().IsValid() || span.Parent().IsRemote()) && p.marked(traceID) {
		p.SpanProcessor.OnEnd(span)
	}
}

// isLLMSpan reports whether the span was produced by internal/tracing.
func isLLMSpan(span sdktrace.ReadOnlySpan) bool {
	for _, attr := range span.Attributes() {
		if attr.Key == llmSpanKindKey {
			return true
		}
	}
	return false
}

// mark records the trace as carrying LLM work, sweeping expired entries so a
// long-running service does not accumulate them.
func (p *llmTracesOnly) mark(id trace.TraceID) {
	now := time.Now()

	p.mu.Lock()
	defer p.mu.Unlock()
	for seenID, at := range p.seen {
		if now.Sub(at) > llmTraceTTL {
			delete(p.seen, seenID)
		}
	}
	p.seen[id] = now
}

// marked reports whether this trace was already seen doing LLM work.
func (p *llmTracesOnly) marked(id trace.TraceID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	at, ok := p.seen[id]
	return ok && time.Since(at) <= llmTraceTTL
}

// normalizeOTLPEndpoint reduces an endpoint to the "host[:port]" form the OTLP
// HTTP exporter requires. LangSmith documents its regional endpoints as URLs
// (https://eu.api.smith.langchain.com), but otlptracehttp.WithEndpoint takes a
// host and appends its own path, so a scheme left in place would be treated as
// part of the hostname and every export would fail DNS resolution.
//
// Only TLS endpoints are reachable this way: langsmith-go builds the exporter
// without exposing WithInsecure, so a plaintext host is not expressible.
func normalizeOTLPEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if scheme, rest, found := strings.Cut(endpoint, "://"); found {
		if !strings.EqualFold(scheme, "https") {
			slog.Warn("langsmith endpoint scheme ignored; the exporter always uses TLS",
				"scheme", scheme, "endpoint", endpoint)
		}
		endpoint = rest
	}
	// The exporter appends its own /otel/v1/traces path.
	host, _, _ := strings.Cut(endpoint, "/")
	return host
}

// Setup initialises the global OTel trace, metric, and log providers.
// The returned shutdown function must be called on process exit.
// All providers export via OTLP gRPC; the endpoint is controlled by the
// standard OTEL_EXPORTER_OTLP_ENDPOINT environment variable (default: localhost:4317).
//
// Additional span destinations (see WithLangSmith) are registered as extra
// processors on this one tracer provider, rather than as a second provider, so
// every exporter sees whole traces and the resource attributes stay consistent.
func Setup(ctx context.Context, serviceName, serviceVersion, environment string, opts ...Option) (*Telemetry, error) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Error("otel", "error", err)
	}))
	var cfg options
	for _, opt := range opts {
		opt(&cfg)
	}

	collector := collectorConfigured()

	// NewSchemaless (rather than NewWithAttributes + semconv.SchemaURL) avoids a
	// "conflicting Schema URL" error from resource.Merge when the SDK's
	// resource.Default() carries a newer semconv schema than the one imported
	// here. The merged resource keeps the default's schema.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			semconv.DeploymentEnvironment(environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	}
	if collector {
		traceExp, err := otlptracegrpc.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("trace exporter: %w", err)
		}
		tpOpts = append(tpOpts, sdktrace.WithBatcher(traceExp))
	}
	// The provider is built either way: LangSmith registers on it below, and
	// that export does not go through the collector.
	tp := sdktrace.NewTracerProvider(tpOpts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// LangSmith attaches as one more span processor on tp, so both
	// destinations see whole traces and share the resource attributes above.
	// (langsmith-go's NewOTelTracer would instead build its own provider and
	// install it globally, replacing this one and dropping the collector
	// export.)
	//
	// A misconfigured LangSmith key must not cost the app its telemetry, so a
	// failure here is a warning rather than an error. slog is still writing to
	// stderr at this point in startup.
	if cfg.langsmithAPIKey != "" {
		exp, err := langsmithExporter(ctx, cfg)
		if err != nil {
			slog.Warn("langsmith tracing unavailable, exporting spans to the collector only", "error", err)
		} else {
			tp.RegisterSpanProcessor(newLLMTracesOnly(sdktrace.NewBatchSpanProcessor(exp,
				sdktrace.WithBatchTimeout(langsmithBatchTimeout),
			)))
			slog.Info("langsmith tracing enabled", "project", cfg.langsmithProject)
		}
	}

	shutdowns := []interface{ Shutdown(context.Context) error }{tp}

	// Metrics and logs have no destination but the collector, so without one
	// there is nothing to build. Leaving the globals as their no-op defaults
	// is what keeps the app writing logs to stderr, where the platform picks
	// them up — an OTel logger bridged to an unreachable collector would
	// swallow every entry instead.
	if collector {
		metricExp, err := otlpmetricgrpc.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("metric exporter: %w", err)
		}
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp,
				sdkmetric.WithInterval(15*time.Second),
			)),
			sdkmetric.WithResource(res),
		)
		otel.SetMeterProvider(mp)

		logExp, err := otlploggrpc.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("log exporter: %w", err)
		}
		lp := sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
			sdklog.WithResource(res),
		)
		global.SetLoggerProvider(lp)

		shutdowns = append(shutdowns, mp, lp)
	} else {
		slog.Info("no OTLP collector configured; logs stay on stderr and metrics are not exported")
	}

	return &Telemetry{
		CollectorEnabled: collector,
		shutdown: func(ctx context.Context) error {
			var errs []error
			for _, s := range shutdowns {
				if err := s.Shutdown(ctx); err != nil {
					errs = append(errs, err)
				}
			}
			if len(errs) > 0 {
				return fmt.Errorf("otel shutdown: %v", errs)
			}
			return nil
		},
	}, nil
}

// Telemetry is a configured OTel stack. Shutdown must be called on exit.
type Telemetry struct {
	// CollectorEnabled reports whether an OTLP collector was configured.
	// When false nothing exports logs, so the caller must keep writing them
	// to stderr rather than installing the OTel log bridge.
	CollectorEnabled bool

	shutdown func(context.Context) error
}

// Shutdown flushes and stops every provider Setup installed.
func (t *Telemetry) Shutdown(ctx context.Context) error { return t.shutdown(ctx) }

// collectorConfigured reports whether an OTLP endpoint was given.
//
// The OTel SDK defaults an unset endpoint to localhost:4317, which is right
// for a developer running the local stack and wrong everywhere else: a
// deployed service with no collector would retry that address forever and,
// worse, lose every log to a bridge that cannot deliver. Treating "unset" as
// "no collector" makes the local case explicit — it is configured in .env —
// and the deployed case quiet.
func collectorConfigured() bool {
	for _, key := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
	} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}
