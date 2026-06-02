package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type NutrientHandler struct {
	pool *pgxpool.Pool
}

func NewNutrientHandler(pool *pgxpool.Pool) *NutrientHandler {
	return &NutrientHandler{pool: pool}
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
