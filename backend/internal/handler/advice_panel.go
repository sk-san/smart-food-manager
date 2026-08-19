package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sk-san/smart-food-manager/backend/internal/agent"
	"github.com/sk-san/smart-food-manager/backend/internal/orchestrator"
)

// Panel runs one prompt across several models and merges their answers. It is
// an interface so the handler can be tested without calling any provider.
type Panel interface {
	Run(ctx context.Context, prompt string) (orchestrator.Result, error)
	Describe() []agent.Descriptor
}

// panelTimeout bounds the whole fan-out. The agents have their own client
// timeouts, but the slowest one decides the response, so the request needs a
// ceiling of its own.
const panelTimeout = 90 * time.Second

// maxPanelPrompt caps the question. A panel sends the prompt to every agent,
// so an oversized one is multiplied by the roster.
const maxPanelPrompt = 4000

// PanelHandler answers nutrition questions with a panel of models rather than
// one, returning the merged answer alongside each model's draft.
type PanelHandler struct {
	panel Panel
}

// NewPanelHandler builds the handler around an assembled panel.
func NewPanelHandler(panel Panel) *PanelHandler {
	return &PanelHandler{panel: panel}
}

type panelRequest struct {
	Prompt string `json:"prompt"`
}

// panelDraft is one agent's contribution. Answer and Error are mutually
// exclusive: a provider that failed is reported rather than hidden, because a
// merged answer built from two of three models is a different thing from one
// built from all three.
type panelDraft struct {
	Agent        string `json:"agent"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Answer       string `json:"answer,omitempty"`
	Error        string `json:"error,omitempty"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
}

type panelResponse struct {
	Answer string `json:"answer"`
	// Merged reports whether Answer came from combining the drafts or from a
	// single one because the merge step failed. The caller is told which,
	// rather than being handed a lone draft dressed up as a consensus.
	Merged      bool         `json:"merged"`
	Drafts      []panelDraft `json:"drafts"`
	TotalTokens int          `json:"totalTokens"`
}

// Advice runs the panel over the caller's question.
func (h *PanelHandler) Advice(w http.ResponseWriter, r *http.Request) {
	var req panelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if len(prompt) > maxPanelPrompt {
		writeError(w, http.StatusBadRequest, "prompt is too long")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), panelTimeout)
	defer cancel()

	result, err := h.panel.Run(ctx, prompt)

	answer, merged := result.Output, true
	if err != nil {
		// The run failed, but that covers two different situations: every
		// agent failed, or the drafts succeeded and only the merge did not —
		// which happens when the synthesizer's provider is the one being rate
		// limited. Falling back to a draft is better than answering 502 while
		// holding a perfectly good answer.
		answer, merged = firstDraft(result), false
		if answer == "" {
			// Detail is on the spans and in the dependency logs, so the
			// client gets the shape of the problem, not the cause.
			writeError(w, http.StatusBadGateway, "the model panel is unavailable")
			return
		}
	}

	writeJSON(w, http.StatusOK, panelResponse{
		Answer:      answer,
		Merged:      merged,
		Drafts:      draftsFrom(result),
		TotalTokens: result.Usage.TotalTokens,
	})
}

// firstDraft returns the first usable answer from the fan-out, for when the
// merge step is the only thing that failed. Results are in configuration
// order, so this is deterministic rather than a race winner.
func firstDraft(result orchestrator.Result) string {
	for _, r := range result.Agents {
		if r.Err == nil && strings.TrimSpace(r.Output) != "" {
			return r.Output
		}
	}
	return ""
}

// draftsFrom flattens the per-agent results for the response body.
func draftsFrom(result orchestrator.Result) []panelDraft {
	drafts := make([]panelDraft, 0, len(result.Agents))
	for _, r := range result.Agents {
		d := panelDraft{
			Agent:        r.Name,
			Provider:     r.Provider,
			Model:        r.Model,
			InputTokens:  r.Usage.InputTokens,
			OutputTokens: r.Usage.OutputTokens,
		}
		if r.Err != nil {
			d.Error = "unavailable"
		} else {
			d.Answer = r.Output
		}
		drafts = append(drafts, d)
	}
	return drafts
}
