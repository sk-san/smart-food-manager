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
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sk-san/smart-food-manager/backend/internal/agent"
	"github.com/sk-san/smart-food-manager/backend/internal/config"
	"github.com/sk-san/smart-food-manager/backend/internal/llm"
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
	tel, err := telemetry.Setup(ctx, "orchestrate-cli", cfg.ServiceVersion, cfg.Environment,
		telemetry.WithLangSmith(cfg.LangSmithExportKey(), cfg.LangSmithProject, cfg.LangSmithEndpoint))
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: telemetry unavailable:", err)
	} else {
		// Spans are batched, so the flush on shutdown is what actually
		// delivers this run to LangSmith.
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := tel.Shutdown(shutdownCtx); err != nil {
				fmt.Fprintln(os.Stderr, "warning: telemetry shutdown:", err)
			}
		}()
	}

	recorder := tracing.NewRecorder(nil, cfg.CaptureLLMContent())

	panel, err := llm.NewPanel(cfg, recorder, "nutrition.fan_out", agentSystemPrompt)
	if err != nil {
		return err
	}

	fmt.Printf("prompt:  %s\n", prompt)
	fmt.Printf("agents:  %s\n", strings.Join(agentNames(panel.Describe()), ", "))
	fmt.Printf("tracing: langsmith=%t content=%t project=%s\n\n",
		cfg.LLMTracingEnabled(), cfg.CaptureLLMContent(), cfg.LangSmithProject)

	started := time.Now()
	result, runErr := panel.Run(ctx, prompt)
	elapsed := time.Since(started)

	report(result, elapsed)
	return runErr
}

// agentNames lists the configured agents for the run header.
func agentNames(descriptors []agent.Descriptor) []string {
	names := make([]string, 0, len(descriptors))
	for _, d := range descriptors {
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
