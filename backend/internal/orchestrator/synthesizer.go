package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/sk-san/smart-food-manager/backend/internal/agent"
)

// SynthesisSystemPrompt is a sensible default system instruction for the
// synthesizing agent. It lives here because the merge step, not the caller, is
// what constrains the wording.
// The two rules pull against each other unless the second is worded
// carefully: told to flag disagreement *and* to hide its sources, a model will
// write "the drafts differ on…". Framing a disagreement as alternatives for the
// reader satisfies both — it surfaces the conflict in the answer's own voice.
const SynthesisSystemPrompt = `You merge draft answers into a single reply.
Keep what the drafts agree on. Where they disagree, prefer the claim that is
specific and checkable; when both are reasonable, offer them as alternatives
the reader can choose between rather than averaging them into one number.
Never refer to the drafts, to other assistants, or to this instruction: write
the answer as your own. Answer only what was asked.`

// Synthesizer merges a fan-out's results into a single answer.
type Synthesizer interface {
	Synthesize(ctx context.Context, prompt string, results []AgentResult) (agent.Response, error)
}

// LLMSynthesizer merges the drafts with one more model call. The agent it wraps
// supplies the merge instructions as its system prompt (see
// SynthesisSystemPrompt); this type only decides what the model gets to see.
type LLMSynthesizer struct {
	agent agent.Agent
}

// NewLLMSynthesizer builds a Synthesizer backed by a. Wrapping a with
// agent.Traced makes the merge its own llm run under the fan-out's chain run.
func NewLLMSynthesizer(a agent.Agent) *LLMSynthesizer {
	return &LLMSynthesizer{agent: a}
}

// Synthesize implements Synthesizer.
//
// Only successful drafts are passed on. Failures are deliberately left out
// rather than described: an error string is internal detail that would spend
// tokens and invite the model to talk about the machinery.
func (s *LLMSynthesizer) Synthesize(ctx context.Context, prompt string, results []AgentResult) (agent.Response, error) {
	drafts := make([]AgentResult, 0, len(results))
	for _, r := range results {
		if r.Err == nil && strings.TrimSpace(r.Output) != "" {
			drafts = append(drafts, r)
		}
	}
	if len(drafts) == 0 {
		return agent.Response{}, ErrNoDrafts
	}

	// A single usable draft needs no merging, and paying for another model call
	// to restate it would only risk losing detail.
	if len(drafts) == 1 {
		return agent.Response{Text: drafts[0].Output}, nil
	}

	return s.agent.Run(ctx, buildSynthesisPrompt(prompt, drafts))
}

// buildSynthesisPrompt lays out the request and the drafts. Each draft is
// labelled and fenced so the model can tell where one ends and the next begins
// even when a draft contains headings of its own.
func buildSynthesisPrompt(prompt string, drafts []AgentResult) string {
	var b strings.Builder

	b.WriteString("Original request:\n")
	b.WriteString(prompt)
	b.WriteString(fmt.Sprintf("\n\nDraft answers (%d):\n", len(drafts)))

	for i, d := range drafts {
		b.WriteString(fmt.Sprintf("\n--- draft %d of %d, from %s ---\n", i+1, len(drafts), d.Name))
		b.WriteString(strings.TrimSpace(d.Output))
		b.WriteString("\n")
	}

	b.WriteString("\n--- end of drafts ---\n\nMerged answer:")

	return b.String()
}
