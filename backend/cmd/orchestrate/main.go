// Command orchestrate runs one prompt through the multi-agent fan-out and
// prints what each agent said alongside the merged answer. It exists because
// no HTTP route exposes the orchestrator yet: this is how the pipeline — and
// its LangSmith run tree — is exercised end to end.
//
// Usage:
//
//	set -a; . ./.env; set +a          # GEMINI_API_KEY, OPENAI_API_KEY, LANGSMITH_API_KEY
//	go run ./cmd/orchestrate "what should I cook with lentils and spinach?"
//
// Agents join the fan-out only when their provider's key is set, so it runs
// with one provider or both.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sk-san/smart-food-manager/backend/internal/agent"
	"github.com/sk-san/smart-food-manager/backend/internal/config"
	"github.com/sk-san/smart-food-manager/backend/internal/gemini"
	"github.com/sk-san/smart-food-manager/backend/internal/mistral"
	"github.com/sk-san/smart-food-manager/backend/internal/orchestrator"
	"github.com/sk-san/smart-food-manager/backend/internal/telemetry"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

const defaultPrompt = "I have lentils, spinach, and half a jar of tahini that expires this week. What should I cook?"

// runTimeout bounds the whole fan-out, merge included.
const runTimeout = 3 * time.Minute

// The agents share a role so the drafts are comparable; what differs is the
// model behind them, which is the point of fanning out across providers.
const agentSystemPrompt = `You are a practical cooking and nutrition assistant.
Answer in at most six sentences. Be concrete: name dishes, quantities, and
timings rather than general advice. If something in the request is unsafe to
eat, say so first.`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	prompt := strings.TrimSpace(strings.Join(os.Args[1:], " "))
	if prompt == "" {
		prompt = defaultPrompt
	}

	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	// Same telemetry bootstrap as the API, so the runs land in the same
	// LangSmith project the service would write to.
	shutdown, err := telemetry.Setup(ctx, "orchestrate-cli", cfg.ServiceVersion, cfg.Environment,
		telemetry.WithLangSmith(cfg.LangSmithExportKey(), cfg.LangSmithProject, cfg.LangSmithEndpoint))
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: telemetry unavailable:", err)
	} else {
		// Spans are batched, so the flush on shutdown is what actually
		// delivers this run to LangSmith.
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := shutdown(shutdownCtx); err != nil {
				fmt.Fprintln(os.Stderr, "warning: telemetry shutdown:", err)
			}
		}()
	}

	recorder := tracing.NewRecorder(nil, cfg.CaptureLLMContent())

	agents, synthesizer, err := buildAgents(cfg, recorder)
	if err != nil {
		return err
	}

	fmt.Printf("prompt:  %s\n", prompt)
	fmt.Printf("agents:  %s\n", strings.Join(agentNames(agents), ", "))
	fmt.Printf("tracing: langsmith=%t content=%t project=%s\n\n",
		cfg.LLMTracingEnabled(), cfg.CaptureLLMContent(), cfg.LangSmithProject)

	started := time.Now()
	result, runErr := orchestrator.New("nutrition.fan_out", recorder,
		orchestrator.NewLLMSynthesizer(synthesizer), agents...).Run(ctx, prompt)
	elapsed := time.Since(started)

	report(result, elapsed)
	return runErr
}

// buildAgents wires one agent per configured provider, plus the agent that
// merges their drafts.
func buildAgents(cfg config.Config, recorder *tracing.Recorder) ([]agent.Agent, agent.Agent, error) {
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
			Name:        "gemini-cook",
			System:      agentSystemPrompt,
			Temperature: 0.3,
			// Thinking tokens are drawn from this budget, so a tight cap
			// truncates the answer mid-sentence rather than shortening it.
			MaxTokens: 3000,
		}), recorder))

		// A second model on the same client, so the fan-out compares two
		// models even with only one provider configured. One client can serve
		// both because the model is chosen per request.
		if cfg.GeminiAltModel != "" && cfg.GeminiAltModel != client.Model() {
			agents = append(agents, agent.Traced(agent.NewGemini(client, agent.GeminiConfig{
				Name:        "gemini-alt-cook",
				Model:       cfg.GeminiAltModel,
				System:      agentSystemPrompt,
				Temperature: 0.3,
				MaxTokens:   3000,
			}), recorder))
		}

		// Gemini Flash is the cheaper of the two, so it does the merging.
		synthesizer = agent.Traced(agent.NewGemini(client, agent.GeminiConfig{
			Name:        "synthesizer",
			System:      orchestrator.SynthesisSystemPrompt,
			Temperature: 0.2,
			MaxTokens:   3000,
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
			Name:        "mistral-cook",
			System:      agentSystemPrompt,
			Temperature: 0.3,
			MaxTokens:   1500,
		}), recorder))
	}

	if cfg.OpenAIAPIKey != "" {
		responder := agent.NewOpenAIResponder(cfg.OpenAIAPIKey, cfg.OpenAITimeout)
		agents = append(agents, agent.Traced(agent.NewOpenAI(responder, agent.OpenAIConfig{
			Name:   "openai-cook",
			Model:  cfg.OpenAIModel,
			System: agentSystemPrompt,
			// Generous, because reasoning tokens are charged against this
			// budget and a model that spends it all returns no text at all.
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
		return nil, nil, errors.New("no provider keys set: export GEMINI_API_KEY and/or OPENAI_API_KEY")
	}
	return agents, synthesizer, nil
}

// agentNames lists the configured agents for the run header.
func agentNames(agents []agent.Agent) []string {
	names := make([]string, 0, len(agents))
	for _, a := range agents {
		d := a.Describe()
		names = append(names, fmt.Sprintf("%s (%s)", d.Name, d.Model))
	}
	return names
}

// report prints each draft's outcome, then the merged answer and the totals.
func report(result orchestrator.Result, elapsed time.Duration) {
	for _, r := range result.Agents {
		fmt.Printf("── %s ──\n", r.Name)
		if r.Err != nil {
			fmt.Printf("   FAILED: %v\n\n", r.Err)
			continue
		}
		fmt.Printf("   %d in / %d out tokens\n   %s\n\n", r.Usage.InputTokens, r.Usage.OutputTokens, indent(r.Output))
	}

	if result.Output != "" {
		fmt.Printf("── merged ──\n   %s\n\n", indent(result.Output))
	}
	fmt.Printf("total: %d in / %d out / %d tokens in %s\n",
		result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.TotalTokens, elapsed.Round(time.Millisecond))
}

// indent aligns a multi-line answer under its heading.
func indent(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n   ")
}
