// Package agent wraps a single LLM call behind one interface, so the
// orchestrator can fan a prompt out across several models or roles without
// knowing which provider answers.
package agent

import (
	"context"

	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// Descriptor identifies an agent on a trace. Name becomes the LangSmith run
// name, Provider the gen_ai.system value, and Model the model the request goes
// to — one source of truth, so a traced agent never disagrees with the call it
// actually made.
type Descriptor struct {
	Name     string
	Provider string
	Model    string
}

// Response is one agent's answer plus what it cost.
type Response struct {
	Text  string
	Usage tracing.Usage
}

// Agent produces a single completion for a prompt. Implementations must honour
// ctx cancellation: the orchestrator relies on it to abandon a fan-out when the
// caller goes away.
type Agent interface {
	Describe() Descriptor
	Run(ctx context.Context, prompt string) (Response, error)
}
