package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Advisor produces nutrition guidance from a prompt. *gemini.Client satisfies
// this interface; the handler depends on the interface (not the concrete
// client) so it stays unit-testable with a fake.
type Advisor interface {
	GenerateText(ctx context.Context, system, prompt string) (string, error)
}

type NutrientHandler struct {
	pool    *pgxpool.Pool
	advisor Advisor
}

func NewNutrientHandler(pool *pgxpool.Pool, advisor Advisor) *NutrientHandler {
	return &NutrientHandler{pool: pool, advisor: advisor}
}

type nutrient struct {
	ID    int      `json:"id"`
	Code  string   `json:"code"`
	Name  string   `json:"name"`
	Unit  string   `json:"unit"`
	Focus string   `json:"focus"`
	Ref   *float64 `json:"reference_daily_amount"`
}

// List returns the active nutrient master, ordered for display.
func (h *NutrientHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, code, name, unit, focus, reference_daily_amount
		   FROM nutrients
		  WHERE is_active = true
		  ORDER BY sort_order, name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	out := make([]nutrient, 0)
	for rows.Next() {
		var n nutrient
		if err := rows.Scan(&n.ID, &n.Code, &n.Name, &n.Unit, &n.Focus, &n.Ref); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		out = append(out, n)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "row iteration failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type adviceRequest struct {
	Prompt string `json:"prompt"`
}

type adviceResponse struct {
	Advice string `json:"advice"`
}

const adviceSystemPrompt = `You are a nutrition assistant for a food management app.
Given the user's question, respond with concise, practical guidance.
Respond as JSON of the form {"advice": "<text>"}.`

// Advice forwards the user's question to the Advisor (Gemini) and returns the
// generated text.
func (h *NutrientHandler) Advice(w http.ResponseWriter, r *http.Request) {
	if h.advisor == nil {
		writeError(w, http.StatusServiceUnavailable, "advisor not configured")
		return
	}

	var req adviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	text, err := h.advisor.GenerateText(r.Context(), adviceSystemPrompt, req.Prompt)
	if err != nil {
		// Detail is captured in the dependency logs/metrics; keep the client
		// response generic.
		writeError(w, http.StatusBadGateway, "advice generation failed")
		return
	}
	writeJSON(w, http.StatusOK, adviceResponse{Advice: text})
}
