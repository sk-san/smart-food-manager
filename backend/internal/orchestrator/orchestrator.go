// Package orchestrator runs one prompt across several agents concurrently and
// merges their answers into one, recording the whole workflow as a single
// LangSmith run tree: a chain run with one llm run per agent plus one for the
// synthesis.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sk-san/smart-food-manager/backend/internal/agent"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

var (
	// ErrNoAgents is returned when a fan-out is attempted with no agents.
	ErrNoAgents = errors.New("orchestrator: no agents configured")
	// ErrNoDrafts is returned by a Synthesizer handed nothing usable to merge.
	ErrNoDrafts = errors.New("orchestrator: no usable agent output to synthesize")
)

// AgentResult is one agent's contribution to a fan-out. Err is set when that
// agent failed; the others still count, which is the point of fanning out.
type AgentResult struct {
	Name string
	// Provider and Model record which model produced the draft, so a caller
	// can tell two drafts apart by more than a name.
	Provider string
	Model    string
	Output   string
	Usage    tracing.Usage
	Err      error
}

// Result is a completed fan-out.
type Result struct {
	// Output is the synthesized answer.
	Output string
	// Agents holds the per-agent results in the order the agents were
	// configured, failures included.
	Agents []AgentResult
	// Usage is every model call the fan-out made, agents plus synthesis.
	Usage tracing.Usage
}

// Orchestrator fans a prompt out to its agents and synthesizes the replies.
type Orchestrator struct {
	name        string
	agents      []agent.Agent
	synthesizer Synthesizer
	recorder    *tracing.Recorder
}

// New builds an Orchestrator. name is the run name the whole fan-out appears
// under in LangSmith. A nil recorder disables tracing without changing
// behaviour; agents are expected to be wrapped with agent.Traced by the caller,
// so each one shows up as its own llm run.
func New(name string, recorder *tracing.Recorder, synthesizer Synthesizer, agents ...agent.Agent) *Orchestrator {
	if name == "" {
		name = "orchestrator.fan_out"
	}
	return &Orchestrator{
		name:        name,
		agents:      agents,
		synthesizer: synthesizer,
		recorder:    recorder,
	}
}

// Describe lists the configured agents, so a caller can report the roster
// without holding a second reference to it.
func (o *Orchestrator) Describe() []agent.Descriptor {
	out := make([]agent.Descriptor, 0, len(o.agents))
	for _, a := range o.agents {
		out = append(out, a.Describe())
	}
	return out
}

// Run fans the prompt out to every agent, then synthesizes their replies.
//
// A partial failure is survivable: as long as one agent answers, the synthesis
// runs on what came back. Run fails only when every agent fails, when there are
// no agents, or when the synthesis itself fails — cases where there is no
// answer to return rather than a thinner one.
func (o *Orchestrator) Run(ctx context.Context, prompt string) (Result, error) {
	if len(o.agents) == 0 {
		return Result{}, ErrNoAgents
	}
	if o.synthesizer == nil {
		return Result{}, errors.New("orchestrator: no synthesizer configured")
	}

	ctx, span := o.start(ctx, prompt)
	defer span.End()

	results := o.fanOut(ctx, prompt)

	usage := tracing.Usage{}
	var failures []error
	succeeded := 0
	for _, r := range results {
		usage = usage.Add(r.Usage)
		if r.Err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", r.Name, r.Err))
			continue
		}
		succeeded++
	}

	if succeeded == 0 {
		err := fmt.Errorf("orchestrator: all %d agents failed: %w", len(results), errors.Join(failures...))
		span.Fail(err)
		return Result{Agents: results, Usage: usage}, err
	}

	synthesis, err := o.synthesizer.Synthesize(ctx, prompt, results)
	usage = usage.Add(synthesis.Usage)
	if err != nil {
		err = fmt.Errorf("orchestrator: synthesize: %w", err)
		span.Fail(err)
		return Result{Agents: results, Usage: usage}, err
	}

	// The chain run carries the totals for the whole fan-out: the child llm
	// runs each report their own, and LangSmith does not sum them upward.
	span.Succeed(synthesis.Text, usage)

	return Result{Output: synthesis.Text, Agents: results, Usage: usage}, nil
}

// start opens the chain run for the fan-out. It is the root of the LLM
// workflow, even when an HTTP span sits above it in the same trace.
func (o *Orchestrator) start(ctx context.Context, prompt string) (context.Context, *tracing.Span) {
	recorder := o.recorder
	if recorder == nil {
		// Content capture is irrelevant to a no-op recorder, which emits
		// spans only if a global tracer provider is installed.
		recorder = tracing.NewRecorder(nil, false)
	}
	return recorder.Start(ctx, tracing.Run{
		Name: o.name,
		Kind: tracing.KindChain,
		Root: true,
	}, prompt)
}

// fanOut runs every agent concurrently on the same prompt and returns their
// results positionally. Ordering is by configuration rather than by completion
// so that the synthesis prompt — and therefore the answer — does not change
// with which model happened to reply first.
func (o *Orchestrator) fanOut(ctx context.Context, prompt string) []AgentResult {
	results := make([]AgentResult, len(o.agents))

	var wg sync.WaitGroup
	for i, a := range o.agents {
		wg.Add(1)
		go func() {
			defer wg.Done()

			resp, err := a.Run(ctx, prompt)
			d := a.Describe()
			// Each goroutine owns one index, so the slice needs no lock.
			results[i] = AgentResult{
				Name:     d.Name,
				Provider: d.Provider,
				Model:    d.Model,
				Output:   resp.Text,
				Usage:    resp.Usage,
				Err:      err,
			}
		}()
	}
	// Waiting for every agent is what keeps the goroutines from outliving the
	// call; they unblock on their own once ctx is cancelled.
	wg.Wait()

	return results
}
