package llm

import (
	"errors"

	"github.com/sk-san/smart-food-manager/backend/internal/agent"
	"github.com/sk-san/smart-food-manager/backend/internal/config"
	"github.com/sk-san/smart-food-manager/backend/internal/gemini"
	"github.com/sk-san/smart-food-manager/backend/internal/mistral"
	"github.com/sk-san/smart-food-manager/backend/internal/orchestrator"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// ErrNoProviders is returned when no provider key is configured, so a caller
// can leave the feature switched off rather than serving an endpoint that can
// only fail.
var ErrNoProviders = errors.New("llm: no provider configured for the panel")

// draftMaxTokens bounds one agent's answer. Thinking tokens come out of this
// budget on a reasoning model, so a tight cap truncates the answer mid-sentence
// rather than shortening it.
const draftMaxTokens = 3000

// mistralMaxTokens is lower because Mistral does not spend the budget on
// reasoning, so the same figure would buy a far longer answer than the others.
const mistralMaxTokens = 1500

// NewPanel assembles the multi-model panel: one drafting agent per configured
// provider plus the agent that merges their drafts.
//
// Agents join only when their provider's key is set, so the same roster works
// with one provider or four. name is the chain run's name in LangSmith and
// system is the role every drafting agent is given — what differs between them
// is the model, which is the point of fanning out.
func NewPanel(cfg config.Config, recorder *tracing.Recorder, name, system string) (*orchestrator.Orchestrator, error) {
	var agents []agent.Agent
	var synthesizer agent.Agent

	if cfg.GeminiAPIKey != "" {
		client := gemini.New(gemini.Config{
			APIKey:  cfg.GeminiAPIKey,
			BaseURL: cfg.GeminiBaseURL,
			Model:   cfg.GeminiModel,
			Timeout: cfg.GeminiTimeout,
		})
		agents = append(agents, agent.Traced(agent.NewGemini(client, agent.GeminiConfig{
			Name:        "gemini-draft",
			System:      system,
			Temperature: 0.3,
			MaxTokens:   draftMaxTokens,
		}), recorder))

		// A second model on the same client, so the panel compares two models
		// even with only one provider configured. One client serves both
		// because the model is chosen per request.
		if cfg.GeminiAltModel != "" && cfg.GeminiAltModel != client.Model() {
			agents = append(agents, agent.Traced(agent.NewGemini(client, agent.GeminiConfig{
				Name:        "gemini-alt-draft",
				Model:       cfg.GeminiAltModel,
				System:      system,
				Temperature: 0.3,
				MaxTokens:   draftMaxTokens,
			}), recorder))
		}

		synthesizer = agent.Traced(agent.NewGemini(client, agent.GeminiConfig{
			Name:        "synthesizer",
			System:      orchestrator.SynthesisSystemPrompt,
			Temperature: 0.2,
			MaxTokens:   draftMaxTokens,
		}), recorder)
	}

	if cfg.MistralAPIKey != "" {
		client := mistral.New(mistral.Config{
			APIKey:  cfg.MistralAPIKey,
			BaseURL: cfg.MistralBaseURL,
			Model:   cfg.MistralModel,
			Timeout: cfg.MistralTimeout,
		})
		agents = append(agents, agent.Traced(agent.NewMistral(client, agent.MistralConfig{
			Name:        "mistral-draft",
			System:      system,
			Temperature: 0.3,
			MaxTokens:   mistralMaxTokens,
		}), recorder))

		// Falls back to Mistral for the merge when Gemini is not configured,
		// so the panel still produces one answer rather than a list.
		if synthesizer == nil {
			synthesizer = agent.Traced(agent.NewMistral(client, agent.MistralConfig{
				Name:      "synthesizer",
				System:    orchestrator.SynthesisSystemPrompt,
				MaxTokens: mistralMaxTokens,
			}), recorder)
		}
	}

	if cfg.OpenAIAPIKey != "" {
		responder := agent.NewOpenAIResponder(cfg.OpenAIAPIKey, cfg.OpenAITimeout)
		agents = append(agents, agent.Traced(agent.NewOpenAI(responder, agent.OpenAIConfig{
			Name:      "openai-draft",
			Model:     cfg.OpenAIModel,
			System:    system,
			MaxTokens: 4000,
		}), recorder))

		if synthesizer == nil {
			synthesizer = agent.Traced(agent.NewOpenAI(responder, agent.OpenAIConfig{
				Name:      "synthesizer",
				Model:     cfg.OpenAIModel,
				System:    orchestrator.SynthesisSystemPrompt,
				MaxTokens: 4000,
			}), recorder)
		}
	}

	if len(agents) == 0 {
		return nil, ErrNoProviders
	}
	return orchestrator.New(name, recorder, orchestrator.NewLLMSynthesizer(synthesizer), agents...), nil
}
