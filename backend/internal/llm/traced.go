// Package llm instruments the model calls the API handlers make.
//
// The handlers depend on narrow interfaces (GenerateText, GenerateFromImage)
// rather than on the Gemini client itself, so a decorator satisfying the same
// interfaces can record each call as a LangSmith llm run without any handler
// knowing. Without this the only trace of a model call is the outbound HTTP
// span, which carries no prompt, no answer, no model, and no token counts —
// exactly the things an LLM trace exists to show.
package llm

import (
	"context"

	"github.com/sk-san/smart-food-manager/backend/internal/gemini"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// geminiProvider is the gen_ai.system value for the Gemini API.
const geminiProvider = "gcp.gemini"

// Generator is the slice of *gemini.Client the decorator needs: the two
// usage-returning calls, and the model to attribute a run to.
type Generator interface {
	Generate(ctx context.Context, req gemini.TextRequest) (string, gemini.Usage, error)
	GenerateImage(ctx context.Context, req gemini.ImageRequest) (string, gemini.Usage, error)
	Model() string
}

// Traced wraps a Gemini client so every call becomes an llm run named after
// the feature that made it.
type Traced struct {
	client   Generator
	recorder *tracing.Recorder
	// name is the feature the calls belong to ("meal.analyze"), so LangSmith
	// shows what the model was asked to do rather than a transport verb.
	name string
}

// NewTraced wraps client, recording runs under name. A nil recorder is not
// accepted: callers always have one, and silently dropping traces is the bug
// this package exists to fix.
func NewTraced(client Generator, recorder *tracing.Recorder, name string) *Traced {
	return &Traced{client: client, recorder: recorder, name: name}
}

// GenerateText records a text generation as an llm run.
func (t *Traced) GenerateText(ctx context.Context, system, prompt string) (string, error) {
	// The settings match gemini.GenerateText, which this replaces for
	// instrumented callers: JSON output at a low temperature.
	text, _, err := t.run(ctx, prompt, func(ctx context.Context) (string, gemini.Usage, error) {
		return t.client.Generate(ctx, gemini.TextRequest{
			System:      system,
			Prompt:      prompt,
			Temperature: 0.2,
			MIMEType:    "application/json",
		})
	})
	return text, err
}

// GenerateFromImage records a vision generation as an llm run.
func (t *Traced) GenerateFromImage(ctx context.Context, system, prompt, mimeType string, image []byte) (string, error) {
	text, _, err := t.run(ctx, prompt, func(ctx context.Context) (string, gemini.Usage, error) {
		return t.client.GenerateImage(ctx, gemini.ImageRequest{
			System:      system,
			Prompt:      prompt,
			MIMEType:    mimeType,
			Image:       image,
			Temperature: 0.1,
		})
	})
	return text, err
}

// run opens the span, calls through, and records the outcome. The image bytes
// are never part of the recorded input — only the prompt text is, and only
// when content capture is on.
func (t *Traced) run(ctx context.Context, input string, call func(context.Context) (string, gemini.Usage, error)) (string, gemini.Usage, error) {
	ctx, span := t.recorder.Start(ctx, tracing.Run{
		Name:     t.name,
		Kind:     tracing.KindLLM,
		Provider: geminiProvider,
		Model:    t.client.Model(),
		// Root: these calls are the top of the LLM work in a request, and the
		// span above them is an HTTP span that would otherwise name the trace
		// "POST".
		Root: true,
	}, input)
	defer span.End()

	text, usage, err := call(ctx)
	if err != nil {
		span.Fail(err)
		return "", usage, err
	}

	span.Succeed(text, tracing.Usage{
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		TotalTokens:     usage.TotalTokens,
		ReasoningTokens: usage.ReasoningTokens,
	})
	return text, usage, nil
}
