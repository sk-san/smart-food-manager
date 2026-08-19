package agent

import (
	"context"

	"github.com/sk-san/smart-food-manager/backend/internal/mistral"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// mistralProvider is the gen_ai.system value for the Mistral API.
const mistralProvider = "mistral"

// Completer is the slice of *mistral.Client an agent needs. It mirrors
// Generator so the two providers stay interchangeable behind Agent.
type Completer interface {
	Generate(ctx context.Context, req mistral.TextRequest) (string, mistral.Usage, error)
	Model() string
}

// MistralConfig describes one Mistral-backed agent. Model may name a different
// model from the client's default; leaving it empty uses the client's.
type MistralConfig struct {
	Name        string
	Model       string
	System      string
	Temperature float64
	MaxTokens   int
}

// Mistral is an Agent backed by the Mistral chat completions API.
type Mistral struct {
	client Completer
	cfg    MistralConfig
}

// NewMistral builds a Mistral-backed agent, defaulting the name and resolving
// the model so a span records the model actually called.
func NewMistral(client Completer, cfg MistralConfig) *Mistral {
	if cfg.Name == "" {
		cfg.Name = "mistral"
	}
	if cfg.Model == "" {
		cfg.Model = client.Model()
	}
	return &Mistral{client: client, cfg: cfg}
}

// Describe implements Agent.
func (m *Mistral) Describe() Descriptor {
	return Descriptor{Name: m.cfg.Name, Provider: mistralProvider, Model: m.cfg.Model}
}

// Run implements Agent.
func (m *Mistral) Run(ctx context.Context, prompt string) (Response, error) {
	text, usage, err := m.client.Generate(ctx, mistral.TextRequest{
		System:      m.cfg.System,
		Prompt:      prompt,
		Model:       m.cfg.Model,
		Temperature: m.cfg.Temperature,
		MaxTokens:   m.cfg.MaxTokens,
	})
	if err != nil {
		return Response{}, err
	}
	return Response{
		Text: text,
		Usage: tracing.Usage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.TotalTokens,
		},
	}, nil
}
