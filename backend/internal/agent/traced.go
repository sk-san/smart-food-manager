package agent

import (
	"context"

	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// traced decorates an Agent with one LangSmith llm run per call.
type traced struct {
	agent    Agent
	recorder *tracing.Recorder
}

// Traced wraps a so every call emits an llm run nested under whatever run is
// already on the context. A nil recorder returns a unchanged, so tracing stays
// an opt-in decoration rather than a constructor argument every caller has to
// satisfy.
func Traced(a Agent, recorder *tracing.Recorder) Agent {
	if recorder == nil {
		return a
	}
	return traced{agent: a, recorder: recorder}
}

// Describe implements Agent.
func (t traced) Describe() Descriptor { return t.agent.Describe() }

// Run implements Agent.
func (t traced) Run(ctx context.Context, prompt string) (Response, error) {
	d := t.agent.Describe()

	ctx, span := t.recorder.Start(ctx, tracing.Run{
		Name:     d.Name,
		Kind:     tracing.KindLLM,
		Provider: d.Provider,
		Model:    d.Model,
	}, prompt)
	defer span.End()

	resp, err := t.agent.Run(ctx, prompt)
	if err != nil {
		span.Fail(err)
		return Response{}, err
	}

	span.Succeed(resp.Text, resp.Usage)
	return resp, nil
}
