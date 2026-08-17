package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GoalsHandler struct {
	pool *pgxpool.Pool
}

func NewGoalsHandler(pool *pgxpool.Pool) *GoalsHandler {
	return &GoalsHandler{pool: pool}
}

type dailyGoals struct {
	Calories float64 `json:"calories"`
	Protein  float64 `json:"protein"`
	Carbs    float64 `json:"carbs"`
	Fat      float64 `json:"fat"`
	Sodium   float64 `json:"sodium"`
	Calcium  float64 `json:"calcium"`
	Iron     float64 `json:"iron"`
}

var suggestedGoals = dailyGoals{
	Calories: 2200,
	Protein:  150,
	Carbs:    250,
	Fat:      70,
	Sodium:   2300,
	Calcium:  1000,
	Iron:     18,
}

func (h *GoalsHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	goals := suggestedGoals
	err := h.pool.QueryRow(r.Context(), `
		SELECT calories, protein, carbs, fat, sodium, calcium, iron
		FROM daily_goals
		WHERE user_id = $1`, userID).Scan(
		&goals.Calories, &goals.Protein, &goals.Carbs, &goals.Fat,
		&goals.Sodium, &goals.Calcium, &goals.Iron,
	)
	if err != nil && err != pgx.ErrNoRows {
		writeError(w, http.StatusInternalServerError, "could not load goals")
		return
	}
	writeJSON(w, http.StatusOK, goals)
}

func (h *GoalsHandler) Put(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var goals dailyGoals
	if !decodeJSON(w, r, &goals) {
		return
	}
	if !finitePositive(goals.Calories, goals.Protein, goals.Carbs, goals.Fat,
		goals.Sodium, goals.Calcium, goals.Iron) {
		writeError(w, http.StatusBadRequest, "all goals must be greater than zero")
		return
	}

	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO daily_goals
			(user_id, calories, protein, carbs, fat, sodium, calcium, iron)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id) DO UPDATE SET
			calories = EXCLUDED.calories,
			protein = EXCLUDED.protein,
			carbs = EXCLUDED.carbs,
			fat = EXCLUDED.fat,
			sodium = EXCLUDED.sodium,
			calcium = EXCLUDED.calcium,
			iron = EXCLUDED.iron,
			updated_at = now()
		RETURNING calories, protein, carbs, fat, sodium, calcium, iron`,
		userID, goals.Calories, goals.Protein, goals.Carbs, goals.Fat,
		goals.Sodium, goals.Calcium, goals.Iron,
	).Scan(&goals.Calories, &goals.Protein, &goals.Carbs, &goals.Fat,
		&goals.Sodium, &goals.Calcium, &goals.Iron)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save goals")
		return
	}
	writeJSON(w, http.StatusOK, goals)
}

func (h *GoalsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if _, err := h.pool.Exec(r.Context(), `DELETE FROM daily_goals WHERE user_id = $1`, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not reset goals")
		return
	}
	writeNoContent(w)
}
