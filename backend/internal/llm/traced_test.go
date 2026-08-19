package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/sk-san/smart-food-manager/backend/internal/gemini"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// fakeGenerator stands in for *gemini.Client.
type fakeGenerator struct {
	reply string
	usage gemini.Usage
	err   error

	gotText  gemini.TextRequest
	gotImage gemini.ImageRequest
}

func (f *fakeGenerator) Generate(_ context.Context, req gemini.TextRequest) (string, gemini.Usage, error) {
	f.gotText = req
	return f.reply, f.usage, f.err
}

func (f *fakeGenerator) GenerateImage(_ context.Context, req gemini.ImageRequest) (string, gemini.Usage, error) {
	f.gotImage = req
	return f.reply, f.usage, f.err
}

func (f *fakeGenerator) Model() string { return "gemini-2.5-flash" }

// record runs fn against a recorder writing to an in-memory exporter.
func record(t *testing.T, captureContent bool, fn func(*Traced)) tracetest.SpanStub {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))

	fn(NewTraced(&fakeGenerator{reply: "answer", usage: gemini.Usage{
		InputTokens: 120, OutputTokens: 800, TotalTokens: 920, ReasoningTokens: 700,
	}}, tracing.NewRecorder(tp, captureContent), "meal.analyze"))

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want exactly one llm run", len(spans))
	}
	return spans[0]
}

func attrs(s tracetest.SpanStub) map[attribute.Key]string {
	out := map[attribute.Key]string{}
	for _, a := range s.Attributes {
		out[a.Key] = a.Value.Emit()
	}
	return out
}

func TestTracedTextCallIsRecordedAsAnLLMRun(t *testing.T) {
	span := record(t, true, func(tr *Traced) {
		if _, err := tr.GenerateText(context.Background(), "system", "what is in this meal?"); err != nil {
			t.Fatalf("GenerateText: %v", err)
		}
	})

	if span.Name != "meal.analyze" {
		t.Errorf("span name = %q, want the feature name", span.Name)
	}
	a := attrs(span)
	if a["langsmith.span.kind"] != "llm" {
		t.Errorf("span kind = %q, want llm", a["langsmith.span.kind"])
	}
	// Cost attribution depends on this key specifically: LangSmith looks the
	// model up under ls_model_name, not under gen_ai.request.model.
	if a["langsmith.metadata.ls_model_name"] != "gemini-2.5-flash" {
		t.Errorf("ls_model_name = %q, want the model", a["langsmith.metadata.ls_model_name"])
	}
	if a["gen_ai.system"] != "gcp.gemini" {
		t.Errorf("gen_ai.system = %q", a["gen_ai.system"])
	}
}

func TestTracedCallReportsUsageAndReasoningSplit(t *testing.T) {
	span := record(t, true, func(tr *Traced) {
		_, _ = tr.GenerateText(context.Background(), "system", "prompt")
	})

	var meta struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
		OutputDetail struct {
			Reasoning int `json:"reasoning"`
		} `json:"output_token_details"`
	}
	raw := attrs(span)["langsmith.usage_metadata"]
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("usage_metadata is not JSON: %v (%q)", err, raw)
	}

	if meta.InputTokens != 120 || meta.OutputTokens != 800 || meta.TotalTokens != 920 {
		t.Errorf("usage = %+v, want the provider's counts", meta)
	}
	// Thinking is billed as output, so it stays inside output_tokens; the
	// detail is what shows where the budget actually went.
	if meta.OutputDetail.Reasoning != 700 {
		t.Errorf("reasoning detail = %d, want 700", meta.OutputDetail.Reasoning)
	}
}

func TestTracedImageCallDoesNotRecordImageBytes(t *testing.T) {
	span := record(t, true, func(tr *Traced) {
		_, _ = tr.GenerateFromImage(context.Background(), "system", "read this label", "image/jpeg", []byte{0xFF, 0xD8, 0xFF})
	})

	a := attrs(span)
	if a["gen_ai.prompt"] == "" {
		t.Error("prompt not recorded")
	}
	// The photo must never ride along on the span: it is user data, it is
	// large, and exporters reject oversized attributes.
	for k, v := range a {
		if len(v) > 4096 {
			t.Errorf("attribute %s is %d bytes — image data may be leaking onto the span", k, len(v))
		}
	}
}

func TestTracedRespectsContentCapture(t *testing.T) {
	span := record(t, false, func(tr *Traced) {
		_, _ = tr.GenerateText(context.Background(), "system", "a private meal description")
	})

	a := attrs(span)
	if a["gen_ai.prompt"] != "" || a["gen_ai.completion"] != "" {
		t.Error("prompt or completion captured with content capture disabled")
	}
	// Usage still travels: cost tracking must not depend on storing user text.
	if a["langsmith.usage_metadata"] == "" {
		t.Error("usage dropped along with the content")
	}
}

func TestTracedRecordsFailures(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	wantErr := errors.New("gemini: status 429")

	tr := NewTraced(&fakeGenerator{err: wantErr}, tracing.NewRecorder(tp, true), "meal.analyze")
	if _, err := tr.GenerateText(context.Background(), "system", "prompt"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want it passed through", err)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("got %d spans", len(spans))
	}
	// A failed call still has to appear: a run that vanishes on error hides
	// exactly the calls worth looking at.
	if spans[0].Status.Code.String() != "Error" {
		t.Errorf("status = %s, want Error", spans[0].Status.Code)
	}
}

func TestTracedForwardsGenerationSettings(t *testing.T) {
	f := &fakeGenerator{reply: "answer"}
	tr := NewTraced(f, tracing.NewRecorder(nil, false), "label.extract")

	if _, err := tr.GenerateText(context.Background(), "sys", "p"); err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	// Same settings the un-decorated client used, so instrumenting a call
	// cannot change its output format.
	if f.gotText.MIMEType != "application/json" || f.gotText.Temperature != 0.2 {
		t.Errorf("text request = %+v", f.gotText)
	}

	if _, err := tr.GenerateFromImage(context.Background(), "sys", "p", "image/png", []byte{1}); err != nil {
		t.Fatalf("GenerateFromImage: %v", err)
	}
	if f.gotImage.MIMEType != "image/png" || f.gotImage.Temperature != 0.1 {
		t.Errorf("image request = %+v", f.gotImage)
	}
}
