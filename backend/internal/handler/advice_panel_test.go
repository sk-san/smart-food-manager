package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sk-san/smart-food-manager/backend/internal/agent"
	"github.com/sk-san/smart-food-manager/backend/internal/orchestrator"
	"github.com/sk-san/smart-food-manager/backend/internal/tracing"
)

// fakePanel stands in for the assembled orchestrator.
type fakePanel struct {
	result orchestrator.Result
	err    error

	gotPrompt string
}

func (f *fakePanel) Run(_ context.Context, prompt string) (orchestrator.Result, error) {
	f.gotPrompt = prompt
	return f.result, f.err
}

func (f *fakePanel) Describe() []agent.Descriptor { return nil }

func postPanel(t *testing.T, p Panel, body string) *httptest.ResponseRecorder {
	t.Helper()
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nutrients/advice/panel", strings.NewReader(body))
	NewPanelHandler(p).Advice(res, req)
	return res
}

func TestPanelReturnsMergedAnswerWithEachDraft(t *testing.T) {
	p := &fakePanel{result: orchestrator.Result{
		Output: "merged answer",
		Usage:  tracing.Usage{TotalTokens: 900},
		Agents: []orchestrator.AgentResult{
			{Name: "gemini-draft", Provider: "gcp.gemini", Model: "gemini-2.5-flash",
				Output: "first draft", Usage: tracing.Usage{InputTokens: 30, OutputTokens: 200}},
			{Name: "mistral-draft", Provider: "mistral", Model: "mistral-small-latest",
				Output: "second draft", Usage: tracing.Usage{InputTokens: 30, OutputTokens: 120}},
		},
	}}

	res := postPanel(t, p, `{"prompt":"how do I get more iron?"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body)
	}

	var got panelResponse
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Answer != "merged answer" || got.TotalTokens != 900 || !got.Merged {
		t.Errorf("response = %+v", got)
	}
	if len(got.Drafts) != 2 {
		t.Fatalf("drafts = %+v, want one per agent", got.Drafts)
	}
	// The model matters as much as the name: two drafts are only comparable
	// if the caller can tell which model produced each.
	if got.Drafts[0].Model != "gemini-2.5-flash" || got.Drafts[1].Model != "mistral-small-latest" {
		t.Errorf("drafts = %+v, want the model on each", got.Drafts)
	}
	if p.gotPrompt != "how do I get more iron?" {
		t.Errorf("prompt = %q", p.gotPrompt)
	}
}

func TestPanelReportsAFailedAgentWithoutItsError(t *testing.T) {
	p := &fakePanel{result: orchestrator.Result{
		Output: "merged from what answered",
		Agents: []orchestrator.AgentResult{
			{Name: "gemini-draft", Model: "gemini-2.5-flash", Output: "a draft"},
			{Name: "mistral-draft", Model: "mistral-small-latest", Err: errors.New("mistral: status 429: quota exceeded")},
		},
	}}

	res := postPanel(t, p, `{"prompt":"question"}`)
	var got panelResponse
	_ = json.Unmarshal(res.Body.Bytes(), &got)

	// A partial panel is a different thing from a full one, so the caller is
	// told which agent dropped out...
	if got.Drafts[1].Error == "" {
		t.Error("failed agent not reported")
	}
	// ...but the provider's message stays server-side, where the dependency
	// logs and spans already carry it.
	if strings.Contains(res.Body.String(), "quota exceeded") {
		t.Errorf("provider error leaked to the client: %s", res.Body)
	}
	if got.Drafts[1].Answer != "" {
		t.Error("a failed agent reported an answer")
	}
}

func TestPanelRejectsBadRequests(t *testing.T) {
	p := &fakePanel{}
	for name, body := range map[string]string{
		"not json":      `{`,
		"empty prompt":  `{"prompt":"   "}`,
		"missing field": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if res := postPanel(t, p, body); res.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", res.Code)
			}
		})
	}

	// A panel sends the prompt to every agent, so an oversized one is
	// multiplied by the roster before anything rejects it downstream.
	long, _ := json.Marshal(panelRequest{Prompt: strings.Repeat("a", maxPanelPrompt+1)})
	if res := postPanel(t, p, string(long)); res.Code != http.StatusBadRequest {
		t.Errorf("status = %d for an oversized prompt, want 400", res.Code)
	}
}

func TestPanelReportsTotalFailureAsBadGateway(t *testing.T) {
	p := &fakePanel{err: errors.New("orchestrator: every agent failed")}

	res := postPanel(t, p, `{"prompt":"question"}`)
	if res.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", res.Code)
	}
}

func TestPanelFallsBackToADraftWhenOnlyTheMergeFails(t *testing.T) {
	// The synthesizer shares a provider with some drafting agents, so a rate
	// limit can take out the merge while leaving usable drafts behind.
	// Answering 502 while holding a good answer would be the wrong trade.
	p := &fakePanel{
		err: errors.New("orchestrator: synthesize: gemini: status 429"),
		result: orchestrator.Result{
			Agents: []orchestrator.AgentResult{
				{Name: "gemini-draft", Model: "gemini-2.5-flash", Err: errors.New("gemini: status 429")},
				{Name: "mistral-draft", Model: "mistral-small-latest", Output: "eat lentils and spinach"},
			},
		},
	}

	res := postPanel(t, p, `{"prompt":"question"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with the surviving draft", res.Code)
	}

	var got panelResponse
	_ = json.Unmarshal(res.Body.Bytes(), &got)
	if got.Answer != "eat lentils and spinach" {
		t.Errorf("answer = %q, want the surviving draft", got.Answer)
	}
	// The caller must be able to tell a merged answer from a single draft.
	if got.Merged {
		t.Error("reported as merged when the merge failed")
	}
}

func TestPanelStillFailsWhenNoDraftSurvives(t *testing.T) {
	p := &fakePanel{
		err: errors.New("orchestrator: all 2 agents failed"),
		result: orchestrator.Result{Agents: []orchestrator.AgentResult{
			{Name: "gemini-draft", Err: errors.New("gemini: status 429")},
			{Name: "mistral-draft", Err: errors.New("mistral: status 429")},
		}},
	}
	if res := postPanel(t, p, `{"prompt":"question"}`); res.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", res.Code)
	}
}
