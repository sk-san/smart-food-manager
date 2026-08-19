package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/sk-san/smart-food-manager/backend/internal/agent"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// fnAgent is an Agent defined by a function, for scripting fan-out scenarios.
type fnAgent struct {
	name string
	run  func(ctx context.Context, prompt string) (agent.Response, error)
}

func (f fnAgent) Describe() agent.Descriptor {
	return agent.Descriptor{Name: f.name, Provider: "test", Model: "test-model"}
}

func (f fnAgent) Run(ctx context.Context, prompt string) (agent.Response, error) {
	return f.run(ctx, prompt)
}

// staticAgent answers with a fixed reply and usage.
func staticAgent(name, reply string, usage tracing.Usage) fnAgent {
	return fnAgent{name: name, run: func(context.Context, string) (agent.Response, error) {
		return agent.Response{Text: reply, Usage: usage}, nil
	}}
}

// failingAgent always fails.
func failingAgent(name string, err error) fnAgent {
	return fnAgent{name: name, run: func(context.Context, string) (agent.Response, error) {
		return agent.Response{}, err
	}}
}

// recordingSynthesizer captures what the merge step was handed.
type recordingSynthesizer struct {
	reply   string
	usage   tracing.Usage
	err     error
	calls   int
	prompt  string
	results []AgentResult
}

func (r *recordingSynthesizer) Synthesize(_ context.Context, prompt string, results []AgentResult) (agent.Response, error) {
	r.calls++
	r.prompt = prompt
	r.results = results
	if r.err != nil {
		return agent.Response{}, r.err
	}
	return agent.Response{Text: r.reply, Usage: r.usage}, nil
}

func TestRunFanOutPreservesConfiguredOrder(t *testing.T) {
	// The agents are forced to finish in reverse: each one releases its
	// predecessor before returning, so agent-0 completes last. Results must
	// still come back in configuration order, or the synthesis prompt (and so
	// the answer) would depend on which model happened to be quickest.
	const n = 3
	gate := make([]chan struct{}, n)
	for i := range gate {
		gate[i] = make(chan struct{})
	}

	agents := make([]agent.Agent, n)
	for i := range agents {
		agents[i] = fnAgent{
			name: fmt.Sprintf("agent-%d", i),
			run: func(context.Context, string) (agent.Response, error) {
				<-gate[i]
				if i > 0 {
					close(gate[i-1])
				}
				return agent.Response{Text: fmt.Sprintf("draft %d", i)}, nil
			},
		}
	}
	close(gate[n-1])

	synth := &recordingSynthesizer{reply: "merged"}
	got, err := New("test.fan_out", nil, synth, agents...).Run(context.Background(), "question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got.Output != "merged" {
		t.Errorf("Output = %q", got.Output)
	}
	for i, r := range got.Agents {
		if want := fmt.Sprintf("agent-%d", i); r.Name != want {
			t.Errorf("Agents[%d].Name = %q, want %q", i, r.Name, want)
		}
		if want := fmt.Sprintf("draft %d", i); r.Output != want {
			t.Errorf("Agents[%d].Output = %q, want %q", i, r.Output, want)
		}
	}
	if len(synth.results) != n {
		t.Errorf("synthesizer saw %d results, want %d", len(synth.results), n)
	}
	if synth.prompt != "question" {
		t.Errorf("synthesizer prompt = %q, want the original request", synth.prompt)
	}
}

func TestRunSurvivesPartialFailure(t *testing.T) {
	boom := errors.New("gemini: status 503")
	synth := &recordingSynthesizer{reply: "merged"}

	got, err := New("test.fan_out", nil, synth,
		failingAgent("broken", boom),
		staticAgent("working", "draft", tracing.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}),
	).Run(context.Background(), "question")
	if err != nil {
		t.Fatalf("Run failed despite one healthy agent: %v", err)
	}

	if got.Output != "merged" {
		t.Errorf("Output = %q", got.Output)
	}
	if !errors.Is(got.Agents[0].Err, boom) {
		t.Errorf("Agents[0].Err = %v, want the agent's error preserved for the caller", got.Agents[0].Err)
	}
	if got.Agents[1].Err != nil {
		t.Errorf("Agents[1].Err = %v, want nil", got.Agents[1].Err)
	}
	// The failed call reported no tokens, so only the healthy one counts.
	if got.Usage != (tracing.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}) {
		t.Errorf("Usage = %+v", got.Usage)
	}
	if synth.calls != 1 {
		t.Errorf("synthesizer called %d times, want 1", synth.calls)
	}
}

func TestRunFailsWhenEveryAgentFails(t *testing.T) {
	first := errors.New("gemini: status 503")
	second := errors.New("gemini: status 429")
	synth := &recordingSynthesizer{reply: "merged"}

	got, err := New("test.fan_out", nil, synth,
		failingAgent("a", first),
		failingAgent("b", second),
	).Run(context.Background(), "question")

	if err == nil {
		t.Fatal("Run succeeded with no agent output")
	}
	// Both causes stay reachable, so a caller can tell a quota problem from an
	// outage without parsing strings.
	if !errors.Is(err, first) || !errors.Is(err, second) {
		t.Errorf("err = %v, want both agent errors joined", err)
	}
	if synth.calls != 0 {
		t.Error("synthesizer called with nothing to merge")
	}
	// The per-agent detail is still returned alongside the error.
	if len(got.Agents) != 2 {
		t.Errorf("Agents = %v, want the failed results for diagnostics", got.Agents)
	}
}

func TestRunAggregatesUsageIncludingSynthesis(t *testing.T) {
	synth := &recordingSynthesizer{reply: "merged", usage: tracing.Usage{InputTokens: 30, OutputTokens: 5, TotalTokens: 35}}

	got, err := New("test.fan_out", nil, synth,
		staticAgent("a", "draft a", tracing.Usage{InputTokens: 10, OutputTokens: 4, TotalTokens: 14}),
		staticAgent("b", "draft b", tracing.Usage{InputTokens: 12, OutputTokens: 6, TotalTokens: 18}),
	).Run(context.Background(), "question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := tracing.Usage{InputTokens: 52, OutputTokens: 15, TotalTokens: 67}
	if got.Usage != want {
		t.Errorf("Usage = %+v, want %+v (agents plus the merge call)", got.Usage, want)
	}
}

func TestRunRequiresAgentsAndSynthesizer(t *testing.T) {
	if _, err := New("test", nil, &recordingSynthesizer{}).Run(context.Background(), "q"); !errors.Is(err, ErrNoAgents) {
		t.Errorf("err = %v, want ErrNoAgents", err)
	}
	if _, err := New("test", nil, nil, staticAgent("a", "draft", tracing.Usage{})).Run(context.Background(), "q"); err == nil {
		t.Error("Run succeeded with no synthesizer")
	}
}

func TestRunPropagatesSynthesisFailure(t *testing.T) {
	boom := errors.New("gemini: status 500")
	synth := &recordingSynthesizer{err: boom}

	_, err := New("test", nil, synth, staticAgent("a", "draft", tracing.Usage{})).Run(context.Background(), "q")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the synthesis error", err)
	}
}

func TestRunHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// A well-behaved agent returns as soon as ctx is done; Run must not outlive
	// the caller waiting on the others.
	ctxAgent := fnAgent{name: "ctx", run: func(ctx context.Context, _ string) (agent.Response, error) {
		<-ctx.Done()
		return agent.Response{}, ctx.Err()
	}}

	_, err := New("test", nil, &recordingSynthesizer{}, ctxAgent, ctxAgent).Run(ctx, "q")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestRunEmitsChainRunAroundAgentRuns(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()
	recorder := tracing.NewRecorder(tp, true)

	// Wrapping the agents is the caller's job, which is what makes each one its
	// own llm run under the chain.
	agents := []agent.Agent{
		agent.Traced(staticAgent("a", "draft a", tracing.Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12}), recorder),
		agent.Traced(staticAgent("b", "draft b", tracing.Usage{InputTokens: 8, OutputTokens: 3, TotalTokens: 11}), recorder),
	}
	synth := &recordingSynthesizer{reply: "merged", usage: tracing.Usage{InputTokens: 20, OutputTokens: 4, TotalTokens: 24}}

	if _, err := New("advice.fan_out", recorder, synth, agents...).Run(context.Background(), "question"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ended := sr.Ended()
	if len(ended) != 3 {
		t.Fatalf("recorded %d spans, want 3 (two agents plus the chain)", len(ended))
	}

	// The chain run ends last, so it is the final recorded span.
	chain := ended[len(ended)-1]
	if chain.Name() != "advice.fan_out" {
		t.Fatalf("last span = %q, want the chain run", chain.Name())
	}
	if got := spanAttr(chain, "langsmith.span.kind"); got != "chain" {
		t.Errorf("langsmith.span.kind = %q, want chain", got)
	}
	if got := spanAttr(chain, "langsmith.trace.name"); got != "advice.fan_out" {
		t.Errorf("langsmith.trace.name = %q, want the fan-out named as the trace root", got)
	}
	if got := chain.Attributes(); !hasIntAttr(got, "gen_ai.usage.total_tokens", 47) {
		t.Errorf("chain run token total not the fan-out's sum: %v", got)
	}
	if !strings.Contains(spanAttr(chain, "gen_ai.completion"), "merged") {
		t.Error("chain run output is not the synthesized answer")
	}
	if chain.Status().Code != codes.Ok {
		t.Errorf("chain status = %v", chain.Status().Code)
	}

	for _, child := range ended[:2] {
		if child.Parent().SpanID() != chain.SpanContext().SpanID() {
			t.Errorf("run %q is not nested under the chain run", child.Name())
		}
		if got := spanAttr(child, "langsmith.span.kind"); got != "llm" {
			t.Errorf("run %q kind = %q, want llm", child.Name(), got)
		}
	}
}

func TestRunMarksChainRunFailed(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	_, err := New("advice.fan_out", tracing.NewRecorder(tp, true), &recordingSynthesizer{},
		failingAgent("a", errors.New("gemini: status 503")),
	).Run(context.Background(), "question")
	if err == nil {
		t.Fatal("Run succeeded with no agent output")
	}

	chain := sr.Ended()[0]
	if chain.Status().Code != codes.Error {
		t.Errorf("chain status = %v, want Error", chain.Status().Code)
	}
}

// spanAttr returns a string attribute from a recorded span, or "".
func spanAttr(span sdktrace.ReadOnlySpan, key attribute.Key) string {
	for _, kv := range span.Attributes() {
		if kv.Key == key {
			return kv.Value.AsString()
		}
	}
	return ""
}

// hasIntAttr reports whether the attributes carry key with the given value.
func hasIntAttr(attrs []attribute.KeyValue, key attribute.Key, want int64) bool {
	for _, kv := range attrs {
		if kv.Key == key {
			return kv.Value.AsInt64() == want
		}
	}
	return false
}
