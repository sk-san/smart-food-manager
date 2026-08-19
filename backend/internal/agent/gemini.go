package agent

import (
	"context"

	"github.com/sk-san/smart-food-manager/backend/internal/gemini"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// geminiProvider is the gen_ai.system value for the Gemini API, matching the
// OTel semconv value LangSmith recognises.
const geminiProvider = "gcp.gemini"

// Generator is the slice of *gemini.Client an agent needs. Narrowing it here
// keeps the agents testable without an HTTP round trip.
type Generator interface {
	Generate(ctx context.Context, req gemini.TextRequest) (string, gemini.Usage, error)
	Model() string
}

// GeminiConfig describes one Gemini-backed agent. Model may name a different
// model from the client's default, which is what lets a fan-out compare models;
// leaving it empty uses the client's.
type GeminiConfig struct {
	Name        string
	Model       string
	System      string
	Temperature float64
	MaxTokens   int
}

// Gemini is an Agent backed by the Gemini generateContent API. Its role lives
// in the system instruction, so several agents can share one client.
type Gemini struct {
	gen Generator
	cfg GeminiConfig
}

// NewGemini builds a Gemini-backed agent. An empty Name is reported as
// "gemini" so a trace never carries an unnamed run, and an empty Model is
// resolved to the client's so spans record the model actually called.
func NewGemini(gen Generator, cfg GeminiConfig) *Gemini {
	if cfg.Name == "" {
		cfg.Name = "gemini"
	}
	if cfg.Model == "" {
		cfg.Model = gen.Model()
	}
	return &Gemini{gen: gen, cfg: cfg}
}

// Describe implements Agent.
func (g *Gemini) Describe() Descriptor {
	return Descriptor{
		Name:     g.cfg.Name,
		Provider: geminiProvider,
		Model:    g.cfg.Model,
	}
}

// Run implements Agent. It asks for prose rather than JSON: an agent's answer
// is synthesized by another model, not parsed by a handler.
func (g *Gemini) Run(ctx context.Context, prompt string) (Response, error) {
	text, usage, err := g.gen.Generate(ctx, gemini.TextRequest{
		System:          g.cfg.System,
		Prompt:          prompt,
		Model:           g.cfg.Model,
		Temperature:     g.cfg.Temperature,
		MaxOutputTokens: g.cfg.MaxTokens,
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
