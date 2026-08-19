package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/sk-san/smart-food-manager/backend/internal/mistral"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// fakeCompleter stands in for *mistral.Client.
type fakeCompleter struct {
	model string
	reply string
	usage mistral.Usage
	err   error

	gotRequest mistral.TextRequest
}

func (f *fakeCompleter) Generate(_ context.Context, req mistral.TextRequest) (string, mistral.Usage, error) {
	f.gotRequest = req
	return f.reply, f.usage, f.err
}

func (f *fakeCompleter) Model() string { return f.model }

func TestMistralAgentPassesRoleAndReturnsUsage(t *testing.T) {
	c := &fakeCompleter{
		model: "mistral-small-latest",
		reply: "try a lentil stew",
		usage: mistral.Usage{InputTokens: 30, OutputTokens: 12, TotalTokens: 42},
	}
	a := NewMistral(c, MistralConfig{
		Name:        "mistral-cook",
		System:      "you are a cook",
		Temperature: 0.3,
		MaxTokens:   500,
	})

	got, err := a.Run(context.Background(), "what should I cook?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Text != "try a lentil stew" {
		t.Errorf("Text = %q", got.Text)
	}
	if got.Usage != (tracing.Usage{InputTokens: 30, OutputTokens: 12, TotalTokens: 42}) {
		t.Errorf("Usage = %+v", got.Usage)
	}
	if c.gotRequest.System != "you are a cook" || c.gotRequest.Prompt != "what should I cook?" {
		t.Errorf("request = %+v", c.gotRequest)
	}
	if c.gotRequest.Temperature != 0.3 || c.gotRequest.MaxTokens != 500 {
		t.Errorf("generation settings not forwarded: %+v", c.gotRequest)
	}
}

func TestMistralAgentResolvesModelForTraces(t *testing.T) {
	c := &fakeCompleter{model: "mistral-small-latest"}

	if got := NewMistral(c, MistralConfig{Name: "a"}).Describe(); got.Model != "mistral-small-latest" {
		t.Errorf("Model = %q, want the client's", got.Model)
	}

	a := NewMistral(c, MistralConfig{Name: "a", Model: "mistral-medium-latest"})
	if got := a.Describe(); got.Model != "mistral-medium-latest" || got.Provider != "mistral" {
		t.Errorf("Describe = %+v", got)
	}
	if _, err := a.Run(context.Background(), "p"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if c.gotRequest.Model != "mistral-medium-latest" {
		t.Errorf("request model = %q, want the override", c.gotRequest.Model)
	}
}

func TestMistralAgentDefaultsName(t *testing.T) {
	if got := NewMistral(&fakeCompleter{}, MistralConfig{}).Describe().Name; got != "mistral" {
		t.Errorf("Name = %q, want a fallback so no run is unnamed", got)
	}
}

func TestMistralAgentPropagatesError(t *testing.T) {
	c := &fakeCompleter{err: mistral.ErrMissingAPIKey}
	if _, err := NewMistral(c, MistralConfig{Name: "a"}).Run(context.Background(), "p"); !errors.Is(err, mistral.ErrMissingAPIKey) {
		t.Errorf("err = %v, want the client's error unwrapped", err)
	}
}
