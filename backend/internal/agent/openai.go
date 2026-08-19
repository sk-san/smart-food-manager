package agent

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/sk-san/smart-food-manager/backend/internal/logging"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// openAIProvider is the gen_ai.system value for the OpenAI API.
const openAIProvider = "openai"

// DefaultOpenAIModel is used when a config names no model.
const DefaultOpenAIModel = openai.ChatModelGPT5_6Luna

// defaultOpenAITimeout bounds one Responses call. Without it a stalled request
// would pin a fan-out open for as long as the caller's context allows.
const defaultOpenAITimeout = 60 * time.Second

// ErrEmptyOutput is returned when a model answers with no text — which a
// reasoning model can do by spending its whole output budget on reasoning.
// Reporting it as a failure keeps an empty draft from being counted as one.
var ErrEmptyOutput = errors.New("agent: model returned no output text")

// Responder is the slice of the OpenAI client an agent needs: the Responses
// API's create call. Narrowing it keeps the agent testable without HTTP.
type Responder interface {
	New(ctx context.Context, body responses.ResponseNewParams, opts ...option.RequestOption) (*responses.Response, error)
}

// NewOpenAIResponder builds a Responses client. The HTTP client is wrapped
// with otelhttp, as internal/gemini does, so the outbound call is a child span
// of the run that made it.
func NewOpenAIResponder(apiKey string, timeout time.Duration) Responder {
	if timeout <= 0 {
		timeout = defaultOpenAITimeout
	}
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithHTTPClient(&http.Client{
			Timeout:   timeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}),
	)
	return &client.Responses
}

// OpenAIConfig describes one OpenAI-backed agent. As with the Gemini agent the
// role lives in the system instruction, so several agents can share a client.
type OpenAIConfig struct {
	Name        string
	Model       string
	System      string
	Temperature float64
	MaxTokens   int
}

// OpenAI is an Agent backed by the OpenAI Responses API.
type OpenAI struct {
	responses Responder
	cfg       OpenAIConfig
}

// NewOpenAI builds an OpenAI-backed agent, defaulting the name and model so a
// run is never unnamed and a span always records which model answered.
func NewOpenAI(r Responder, cfg OpenAIConfig) *OpenAI {
	if cfg.Name == "" {
		cfg.Name = "openai"
	}
	if cfg.Model == "" {
		cfg.Model = DefaultOpenAIModel
	}
	return &OpenAI{responses: r, cfg: cfg}
}

// Describe implements Agent.
func (o *OpenAI) Describe() Descriptor {
	return Descriptor{Name: o.cfg.Name, Provider: openAIProvider, Model: o.cfg.Model}
}

// Run implements Agent.
func (o *OpenAI) Run(ctx context.Context, prompt string) (Response, error) {
	params := responses.ResponseNewParams{
		Model: o.cfg.Model,
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)},
	}
	if o.cfg.System != "" {
		params.Instructions = openai.String(o.cfg.System)
	}
	// The GPT-5 reasoning models reject an explicit temperature, so it is sent
	// only when a caller deliberately asked for one.
	if o.cfg.Temperature > 0 {
		params.Temperature = openai.Float(o.cfg.Temperature)
	}
	if o.cfg.MaxTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(o.cfg.MaxTokens))
	}

	// Same external-call logging contract as the Gemini client: metadata only,
	// never the prompt or the reply.
	done := logging.StartExternalCall(ctx, logging.Dependency{
		Provider:  "openai",
		Service:   "openai-api",
		Operation: "responses.create",
		Model:     o.cfg.Model,
	}, "openai.responses_create")

	resp, err := o.responses.New(ctx, params)
	if err != nil {
		done(statusFromOpenAIError(err), err)
		return Response{}, err
	}

	usage := tracing.Usage{
		InputTokens:  int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
		TotalTokens:  int(resp.Usage.TotalTokens),
	}
	done(http.StatusOK, nil,
		slog.Int("tokens.prompt", usage.InputTokens),
		slog.Int("tokens.output", usage.OutputTokens),
		slog.Int("tokens.total", usage.TotalTokens),
	)

	text := resp.OutputText()
	if strings.TrimSpace(text) == "" {
		return Response{Usage: usage}, ErrEmptyOutput
	}
	return Response{Text: text, Usage: usage}, nil
}

// statusFromOpenAIError digs the HTTP status out of an API error so the
// external-call log records it, returning 0 for transport-level failures.
func statusFromOpenAIError(err error) int {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return 0
}
