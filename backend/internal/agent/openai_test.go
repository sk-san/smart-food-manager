package agent

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"

	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// fakeResponder stands in for the OpenAI Responses service.
type fakeResponder struct {
	reply string
	usage responses.ResponseUsage
	err   error

	gotParams responses.ResponseNewParams
}

func (f *fakeResponder) New(_ context.Context, body responses.ResponseNewParams, _ ...option.RequestOption) (*responses.Response, error) {
	f.gotParams = body
	if f.err != nil {
		return nil, f.err
	}
	return &responses.Response{
		Usage: f.usage,
		Output: []responses.ResponseOutputItemUnion{{
			Type: "message",
			Role: "assistant",
			Content: []responses.ResponseOutputMessageContentUnion{{
				Type: "output_text",
				Text: f.reply,
			}},
		}},
	}, nil
}

func TestOpenAIAgentSendsRoleAndReturnsUsage(t *testing.T) {
	r := &fakeResponder{
		reply: "lentils are a good source",
		usage: responses.ResponseUsage{InputTokens: 21, OutputTokens: 9, TotalTokens: 30},
	}
	a := NewOpenAI(r, OpenAIConfig{
		Name:      "second-opinion",
		Model:     "gpt-5.4-mini",
		System:    "you are a dietitian",
		MaxTokens: 300,
	})

	got, err := a.Run(context.Background(), "what should I eat?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Text != "lentils are a good source" {
		t.Errorf("Text = %q", got.Text)
	}
	if got.Usage != (tracing.Usage{InputTokens: 21, OutputTokens: 9, TotalTokens: 30}) {
		t.Errorf("Usage = %+v", got.Usage)
	}

	// The role travels as Instructions, not glued onto the prompt.
	if r.gotParams.Instructions.Value != "you are a dietitian" {
		t.Errorf("Instructions = %q", r.gotParams.Instructions.Value)
	}
	if r.gotParams.Input.OfString.Value != "what should I eat?" {
		t.Errorf("Input = %q", r.gotParams.Input.OfString.Value)
	}
	if r.gotParams.Model != "gpt-5.4-mini" || r.gotParams.MaxOutputTokens.Value != 300 {
		t.Errorf("params = %+v", r.gotParams)
	}
}

func TestOpenAIAgentOmitsTemperatureUnlessAsked(t *testing.T) {
	// The GPT-5 reasoning models reject an explicit temperature, so an unset
	// one must not reach the wire.
	r := &fakeResponder{reply: "answer"}
	if _, err := NewOpenAI(r, OpenAIConfig{Name: "a"}).Run(context.Background(), "p"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.gotParams.Temperature.Valid() {
		t.Errorf("temperature sent unasked: %v", r.gotParams.Temperature.Value)
	}

	if _, err := NewOpenAI(r, OpenAIConfig{Name: "a", Temperature: 0.7}).Run(context.Background(), "p"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.gotParams.Temperature.Value != 0.7 {
		t.Errorf("temperature = %v, want the requested 0.7", r.gotParams.Temperature.Value)
	}
}

func TestOpenAIAgentDefaults(t *testing.T) {
	got := NewOpenAI(&fakeResponder{}, OpenAIConfig{}).Describe()
	if got.Name != "openai" {
		t.Errorf("Name = %q, want a fallback so no run is unnamed", got.Name)
	}
	if got.Model != DefaultOpenAIModel {
		t.Errorf("Model = %q, want the default", got.Model)
	}
	if got.Provider != "openai" {
		t.Errorf("Provider = %q", got.Provider)
	}
}

func TestOpenAIAgentEmptyOutputIsAFailure(t *testing.T) {
	// A reasoning model that spends its budget on reasoning returns no text.
	// Counting that as a draft would let an empty answer into the merge.
	r := &fakeResponder{reply: "   ", usage: responses.ResponseUsage{InputTokens: 5, TotalTokens: 5}}

	got, err := NewOpenAI(r, OpenAIConfig{Name: "a"}).Run(context.Background(), "p")
	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("err = %v, want ErrEmptyOutput", err)
	}
	// The call still cost tokens, so the usage is reported regardless.
	if got.Usage.InputTokens != 5 {
		t.Errorf("Usage = %+v, want the tokens the failed call spent", got.Usage)
	}
}

func TestOpenAIAgentPropagatesAPIError(t *testing.T) {
	// Request and Response are populated because openai.Error.Error() reads
	// them, and both the external-call log and the span record err.Error().
	// The SDK only ever builds this error from a real exchange.
	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	apiErr := &openai.Error{
		StatusCode: http.StatusTooManyRequests,
		Request:    req,
		Response:   &http.Response{StatusCode: http.StatusTooManyRequests},
	}
	r := &fakeResponder{err: apiErr}

	_, err = NewOpenAI(r, OpenAIConfig{Name: "a"}).Run(context.Background(), "p")
	if !errors.Is(err, apiErr) {
		t.Errorf("err = %v, want the API error unwrapped", err)
	}
	if got := statusFromOpenAIError(err); got != 429 {
		t.Errorf("statusFromOpenAIError = %d, want 429 for the external-call log", got)
	}
	if got := statusFromOpenAIError(errors.New("dial tcp: connection refused")); got != 0 {
		t.Errorf("statusFromOpenAIError = %d, want 0 for a transport failure", got)
	}
}
