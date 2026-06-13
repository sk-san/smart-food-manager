package logging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/example/food-app/backend/internal/telemetry"
)

// Dependency identifies an external service call (e.g. the Gemini API).
type Dependency struct {
	Provider  string // "google"
	Service   string // "gemini-api"
	Operation string // "generateContent"
	Model     string // optional model name for LLM calls
}

func (d Dependency) attrs() []slog.Attr {
	a := []slog.Attr{
		slog.String("dependency.provider", d.Provider),
		slog.String("dependency.service", d.Service),
		slog.String("dependency.operation", d.Operation),
	}
	if d.Model != "" {
		a = append(a, slog.String("dependency.model", d.Model))
	}
	return a
}

// StartExternalCall emits external_api_call_started and returns a function
// to call once the dependency responds: it emits external_api_call_completed
// or external_api_call_failed with duration, error details, and the
// external_api_* metrics. Never log the raw prompt or response — pass
// metadata (token counts, hashes, sizes) via extra.
func StartExternalCall(ctx context.Context, dep Dependency, action string) func(httpStatus int, err error, extra ...slog.Attr) {
	start := time.Now()
	Emit(ctx, Event{
		Severity: LevelInfo,
		Message:  "External API call started",
		Name:     EventExternalAPICallStarted,
		Category: CategoryDependency,
		Action:   action,
		Outcome:  OutcomeStarted,
		Attrs:    dep.attrs(),
	})
	return func(httpStatus int, err error, extra ...slog.Attr) {
		duration := time.Since(start)
		attrs := dep.attrs()
		if httpStatus > 0 {
			attrs = append(attrs, slog.Int("dependency.http_status_code", httpStatus))
		}
		attrs = append(attrs, extra...)

		e := Event{
			Severity: LevelInfo,
			Message:  "External API call completed",
			Name:     EventExternalAPICallCompleted,
			Category: CategoryDependency,
			Action:   action,
			Outcome:  OutcomeSuccess,
			Duration: duration,
			Attrs:    attrs,
		}
		if err != nil {
			e.Severity = LevelError
			e.Message = "External API call failed"
			e.Name = EventExternalAPICallFailed
			e.Outcome = OutcomeFailure
			e.Attrs = append(e.Attrs,
				slog.String("error.type", fmt.Sprintf("%T", err)),
				slog.String("error.message", Truncate(err.Error(), 512)),
				slog.String("error.source", "external_service"),
			)
		}
		Emit(ctx, e)

		mattrs := metric.WithAttributes(
			attribute.String("dependency.provider", dep.Provider),
			attribute.String("dependency.service", dep.Service),
			attribute.String("dependency.operation", dep.Operation),
			attribute.String("event.outcome", e.Outcome),
		)
		telemetry.ExternalAPIRequestsTotal.Add(ctx, 1, mattrs)
		telemetry.ExternalAPIDurationMS.Record(ctx, float64(duration)/float64(time.Millisecond), mattrs)
	}
}
