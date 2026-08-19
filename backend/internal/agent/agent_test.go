package agent

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/sk-san/smart-food-manager/backend/internal/gemini"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// fakeGenerator stands in for *gemini.Client.
type fakeGenerator struct {
	model string
	reply string
	usage gemini.Usage
	err   error

	gotRequest gemini.TextRequest
}

func (f *fakeGenerator) Generate(_ context.Context, req gemini.TextRequest) (string, gemini.Usage, error) {
	f.gotRequest = req
	return f.reply, f.usage, f.err
}

func (f *fakeGenerator) Model() string { return f.model }

func TestGeminiAgentPassesRoleAndReturnsUsage(t *testing.T) {
	gen := &fakeGenerator{
		model: "gemini-2.5-flash",
		reply: "lentils are high in protein",
		usage: gemini.Usage{InputTokens: 11, OutputTokens: 7, TotalTokens: 18},
	}
	a := NewGemini(gen, GeminiConfig{
		Name:        "nutrition",
		System:      "you are a dietitian",
		Temperature: 0.4,
		MaxTokens:   256,
	})

	resp, err := a.Run(context.Background(), "what should I eat?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Text != "lentils are high in protein" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.Usage != (tracing.Usage{InputTokens: 11, OutputTokens: 7, TotalTokens: 18}) {
		t.Errorf("Usage = %+v", resp.Usage)
	}

	if gen.gotRequest.System != "you are a dietitian" || gen.gotRequest.Prompt != "what should I eat?" {
		t.Errorf("request = %+v", gen.gotRequest)
	}
	if gen.gotRequest.Temperature != 0.4 || gen.gotRequest.MaxOutputTokens != 256 {
		t.Errorf("generation settings not forwarded: %+v", gen.gotRequest)
	}
	// Prose, not JSON: the answer is merged by another model, not parsed.
	if gen.gotRequest.MIMEType != "" {
		t.Errorf("MIMEType = %q, want the API default (plain text)", gen.gotRequest.MIMEType)
	}
}

func TestGeminiAgentResolvesModelForTraces(t *testing.T) {
	gen := &fakeGenerator{model: "gemini-2.5-flash"}

	// Unset: the client's model is what the call actually uses, so that is what
	// the descriptor must report.
	if got := NewGemini(gen, GeminiConfig{Name: "a"}).Describe(); got.Model != "gemini-2.5-flash" {
		t.Errorf("Model = %q, want the client's", got.Model)
	}
	// Set: the per-agent override wins, and reaches the request.
	a := NewGemini(gen, GeminiConfig{Name: "a", Model: "gemini-2.5-pro"})
	if got := a.Describe(); got.Model != "gemini-2.5-pro" {
		t.Errorf("Model = %q, want the override", got.Model)
	}
	if _, err := a.Run(context.Background(), "p"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gen.gotRequest.Model != "gemini-2.5-pro" {
		t.Errorf("request model = %q, want the override", gen.gotRequest.Model)
	}
}

func TestGeminiAgentDefaultsName(t *testing.T) {
	if got := NewGemini(&fakeGenerator{}, GeminiConfig{}).Describe().Name; got != "gemini" {
		t.Errorf("Name = %q, want a fallback so no run is unnamed", got)
	}
}

func TestGeminiAgentPropagatesError(t *testing.T) {
	gen := &fakeGenerator{err: gemini.ErrMissingAPIKey}
	if _, err := NewGemini(gen, GeminiConfig{Name: "a"}).Run(context.Background(), "p"); !errors.Is(err, gemini.ErrMissingAPIKey) {
		t.Errorf("err = %v, want the client's error unwrapped", err)
	}
}

// stubAgent is a minimal Agent for exercising the decorator.
type stubAgent struct {
	desc Descriptor
	resp Response
	err  error
}

func (s stubAgent) Describe() Descriptor { return s.desc }
func (s stubAgent) Run(context.Context, string) (Response, error) {
	return s.resp, s.err
}

func TestTracedEmitsLLMRun(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	a := Traced(stubAgent{
		desc: Descriptor{Name: "nutrition", Provider: "gcp.gemini", Model: "gemini-2.5-flash"},
		resp: Response{Text: "eat lentils", Usage: tracing.Usage{InputTokens: 5, OutputTokens: 3, TotalTokens: 8}},
	}, tracing.NewRecorder(tp, true))

	if _, err := a.Run(context.Background(), "what should I eat?"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(ended))
	}
	if ended[0].Name() != "nutrition" {
		t.Errorf("run name = %q, want the agent's", ended[0].Name())
	}
	if got := findAttr(ended[0], "langsmith.span.kind"); got != "llm" {
		t.Errorf("langsmith.span.kind = %q, want llm", got)
	}
	if got := findAttr(ended[0], "gen_ai.request.model"); got != "gemini-2.5-flash" {
		t.Errorf("gen_ai.request.model = %q", got)
	}
	if ended[0].Status().Code != codes.Ok {
		t.Errorf("status = %v", ended[0].Status().Code)
	}
}

func TestTracedRecordsFailure(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	wantErr := errors.New("gemini: status 503")
	a := Traced(stubAgent{desc: Descriptor{Name: "nutrition"}, err: wantErr}, tracing.NewRecorder(tp, true))

	if _, err := a.Run(context.Background(), "p"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if got := sr.Ended()[0].Status().Code; got != codes.Error {
		t.Errorf("status = %v, want Error", got)
	}
}

func TestTracedWithoutRecorderIsPassthrough(t *testing.T) {
	stub := stubAgent{desc: Descriptor{Name: "nutrition"}, resp: Response{Text: "out"}}
	if got := Traced(stub, nil); got != Agent(stub) {
		t.Error("a nil recorder should return the agent unchanged")
	}
}

// findAttr returns a recorded span's string attribute, or "" when absent.
func findAttr(span sdktrace.ReadOnlySpan, key attribute.Key) string {
	for _, kv := range span.Attributes() {
		if kv.Key == key {
			return kv.Value.AsString()
		}
	}
	return ""
}
