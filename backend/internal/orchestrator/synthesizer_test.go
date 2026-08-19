package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sk-san/smart-food-manager/backend/internal/agent"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// capturingAgent records the prompt it was asked to answer.
type capturingAgent struct {
	prompt string
	reply  string
	usage  tracing.Usage
	err    error
	calls  int
}

func (c *capturingAgent) Describe() agent.Descriptor {
	return agent.Descriptor{Name: "synthesizer", Provider: "test", Model: "test-model"}
}

func (c *capturingAgent) Run(_ context.Context, prompt string) (agent.Response, error) {
	c.calls++
	c.prompt = prompt
	if c.err != nil {
		return agent.Response{}, c.err
	}
	return agent.Response{Text: c.reply, Usage: c.usage}, nil
}

func TestSynthesizeBuildsPromptFromDrafts(t *testing.T) {
	a := &capturingAgent{reply: "merged", usage: tracing.Usage{InputTokens: 40, OutputTokens: 6, TotalTokens: 46}}

	got, err := NewLLMSynthesizer(a).Synthesize(context.Background(), "what should I eat?", []AgentResult{
		{Name: "nutrition", Output: "eat lentils"},
		{Name: "inventory", Output: "you have spinach"},
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if got.Text != "merged" || got.Usage.TotalTokens != 46 {
		t.Errorf("response = %+v", got)
	}

	// The merge call must see the original request and every draft, each
	// attributed, or the model cannot resolve a disagreement.
	for _, want := range []string{"what should I eat?", "nutrition", "eat lentils", "inventory", "you have spinach"} {
		if !strings.Contains(a.prompt, want) {
			t.Errorf("synthesis prompt is missing %q:\n%s", want, a.prompt)
		}
	}
}

func TestSynthesizeSkipsFailedAndEmptyDrafts(t *testing.T) {
	a := &capturingAgent{reply: "merged"}

	_, err := NewLLMSynthesizer(a).Synthesize(context.Background(), "question", []AgentResult{
		{Name: "broken", Err: errors.New("gemini: status 503 quota exhausted")},
		{Name: "blank", Output: "   "},
		{Name: "good-a", Output: "draft a"},
		{Name: "good-b", Output: "draft b"},
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Failure detail is internal: it would spend tokens and invite the model to
	// discuss the machinery.
	if strings.Contains(a.prompt, "503") || strings.Contains(a.prompt, "broken") {
		t.Errorf("failed agent leaked into the synthesis prompt:\n%s", a.prompt)
	}
	if strings.Contains(a.prompt, "blank") {
		t.Errorf("empty draft included in the synthesis prompt:\n%s", a.prompt)
	}
	if !strings.Contains(a.prompt, "(2)") {
		t.Errorf("draft count does not match the usable drafts:\n%s", a.prompt)
	}
}

func TestSynthesizeSkipsModelCallForASingleDraft(t *testing.T) {
	a := &capturingAgent{reply: "merged"}

	got, err := NewLLMSynthesizer(a).Synthesize(context.Background(), "question", []AgentResult{
		{Name: "broken", Err: errors.New("boom")},
		{Name: "good", Output: "the only draft"},
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Merging one draft is a paid round trip that can only lose detail.
	if a.calls != 0 {
		t.Errorf("synthesizer agent called %d times for a single draft", a.calls)
	}
	if got.Text != "the only draft" {
		t.Errorf("Text = %q, want the draft passed through", got.Text)
	}
}

func TestSynthesizeWithoutUsableDrafts(t *testing.T) {
	a := &capturingAgent{reply: "merged"}

	_, err := NewLLMSynthesizer(a).Synthesize(context.Background(), "question", []AgentResult{
		{Name: "broken", Err: errors.New("boom")},
	})
	if !errors.Is(err, ErrNoDrafts) {
		t.Errorf("err = %v, want ErrNoDrafts", err)
	}
	if a.calls != 0 {
		t.Error("synthesizer agent called with nothing to merge")
	}
}

func TestSynthesizePropagatesAgentError(t *testing.T) {
	boom := errors.New("gemini: status 500")
	a := &capturingAgent{err: boom}

	_, err := NewLLMSynthesizer(a).Synthesize(context.Background(), "question", []AgentResult{
		{Name: "a", Output: "draft a"},
		{Name: "b", Output: "draft b"},
	})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the agent's error", err)
	}
}
