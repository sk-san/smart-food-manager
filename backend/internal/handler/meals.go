package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MealsHandler struct {
	pool *pgxpool.Pool
}

func NewMealsHandler(pool *pgxpool.Pool) *MealsHandler {
	return &MealsHandler{pool: pool}
}

type mealRequest struct {
	Name       string  `json:"name"`
	ConsumedAt string  `json:"consumed_at"`
	Calories   float64 `json:"calories"`
	Protein    float64 `json:"protein"`
	Carbs      float64 `json:"carbs"`
	Fat        float64 `json:"fat"`
	Sodium     float64 `json:"sodium"`
	Calcium    float64 `json:"calcium"`
	Iron       float64 `json:"iron"`
}

type mealResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ConsumedAt time.Time `json:"consumed_at"`
	Calories   float64   `json:"calories"`
	Protein    float64   `json:"protein"`
	Carbs      float64   `json:"carbs"`
	Fat        float64   `json:"fat"`
	Sodium     float64   `json:"sodium"`
	Calcium    float64   `json:"calcium"`
	Iron       float64   `json:"iron"`
}

const mealSelect = `
	SELECT m.id::text,
	       COALESCE(NULLIF(m.note, ''), string_agg(DISTINCT f.name, ', ')),
	       m.consumed_at,
	       COALESCE(SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)
	         FILTER (WHERE n.code = 'calories'), 0)::float8,
	       COALESCE(SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)
	         FILTER (WHERE n.code = 'protein'), 0)::float8,
	       COALESCE(SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)
	         FILTER (WHERE n.code = 'carbs'), 0)::float8,
	       COALESCE(SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)
	         FILTER (WHERE n.code = 'fat'), 0)::float8,
	       COALESCE(SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)
	         FILTER (WHERE n.code = 'sodium'), 0)::float8,
	       COALESCE(SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)
	         FILTER (WHERE n.code = 'calcium'), 0)::float8,
	       COALESCE(SUM(mi.quantity_g / 100.0 * fn.amount_per_100g)
	         FILTER (WHERE n.code = 'iron'), 0)::float8
	FROM meals m
	JOIN meal_items mi ON mi.meal_id = m.id
	JOIN foods f ON f.id = mi.food_id
	LEFT JOIN food_nutrients fn ON fn.food_id = f.id
	LEFT JOIN nutrients n ON n.id = fn.nutrient_id`

func scanMeal(row pgx.Row) (mealResponse, error) {
	var meal mealResponse
	err := row.Scan(&meal.ID, &meal.Name, &meal.ConsumedAt, &meal.Calories,
		&meal.Protein, &meal.Carbs, &meal.Fat, &meal.Sodium, &meal.Calcium, &meal.Iron)
	return meal, err
}

func (h *MealsHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	rows, err := h.pool.Query(r.Context(), mealSelect+`
		WHERE m.user_id = $1
		GROUP BY m.id
		ORDER BY m.consumed_at ASC`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load meals")
		return
	}
	defer rows.Close()

	meals := make([]mealResponse, 0)
	for rows.Next() {
		meal, err := scanMeal(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not read meals")
			return
		}
		meals = append(meals, meal)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "could not read meals")
		return
	}
	writeJSON(w, http.StatusOK, meals)
}

func (h *MealsHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	mealID := chi.URLParam(r, "mealID")
	if !validResourceID(mealID) {
		writeError(w, http.StatusBadRequest, "invalid meal id")
		return
	}
	meal, err := scanMeal(h.pool.QueryRow(r.Context(), mealSelect+`
		WHERE m.user_id = $1 AND m.id = $2
		GROUP BY m.id`, userID, mealID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "meal not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load meal")
		return
	}
	writeJSON(w, http.StatusOK, meal)
}

func validateMeal(req *mealRequest, defaultTime time.Time) (time.Time, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 200 {
		return time.Time{}, errors.New("name is required and must be at most 200 characters")
	}
	if !finiteNonNegative(req.Calories, req.Protein, req.Carbs, req.Fat,
		req.Sodium, req.Calcium, req.Iron) {
		return time.Time{}, errors.New("nutrient amounts must be non-negative")
	}
	if strings.TrimSpace(req.ConsumedAt) == "" {
		return defaultTime, nil
	}
	consumedAt, err := time.Parse(time.RFC3339, req.ConsumedAt)
	if err != nil {
		return time.Time{}, errors.New("consumed_at must be an RFC3339 timestamp")
	}
	return consumedAt, nil
}

func mealTypeAt(t time.Time) string {
	switch hour := t.Hour(); {
	case hour >= 5 && hour < 11:
		return "breakfast"
	case hour >= 11 && hour < 16:
		return "lunch"
	case hour >= 16 && hour < 22:
		return "dinner"
	default:
		return "snack"
	}
}

func saveMealNutrients(ctx context.Context, tx pgx.Tx, foodID string, req mealRequest) error {
	command, err := tx.Exec(ctx, `
		INSERT INTO food_nutrients (food_id, nutrient_id, amount_per_100g)
		SELECT $1, n.id, v.amount
		FROM (VALUES
			('calories', $2::numeric), ('protein', $3::numeric),
			('carbs', $4::numeric), ('fat', $5::numeric),
			('sodium', $6::numeric), ('calcium', $7::numeric),
			('iron', $8::numeric)
		) AS v(code, amount)
		JOIN nutrients n ON n.code = v.code
		ON CONFLICT (food_id, nutrient_id) DO UPDATE
		SET amount_per_100g = EXCLUDED.amount_per_100g`,
		foodID, req.Calories, req.Protein, req.Carbs, req.Fat,
		req.Sodium, req.Calcium, req.Iron)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 7 {
		return errors.New("nutrient master is incomplete; apply migration 0002")
	}
	return nil
}

func responseFromMealRequest(id string, consumedAt time.Time, req mealRequest) mealResponse {
	return mealResponse{
		ID: id, Name: req.Name, ConsumedAt: consumedAt, Calories: req.Calories,
		Protein: req.Protein, Carbs: req.Carbs, Fat: req.Fat, Sodium: req.Sodium,
		Calcium: req.Calcium, Iron: req.Iron,
	}
}

// lockInventoryLinkedMeal reports whether a meal belongs to the pantry
// lifecycle. Those meals must only change through consume/waste reconciliation;
// generic meal edits would otherwise desynchronize inventory quantities from
// the nutrition ledger.
func lockInventoryLinkedMeal(ctx context.Context, tx pgx.Tx, userID, mealID string) (bool, error) {
	var inventoryID string
	err := tx.QueryRow(ctx, `
		SELECT i.id::text
		FROM inventory_items i
		JOIN meal_items mi ON mi.inventory_item_id = i.id
		WHERE mi.meal_id = $1 AND i.user_id = $2
		LIMIT 1
		FOR UPDATE OF i`, mealID, userID).Scan(&inventoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (h *MealsHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req mealRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	consumedAt, err := validateMeal(&req, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create meal")
		return
	}
	defer tx.Rollback(r.Context())

	var foodID, mealID string
	if err = tx.QueryRow(r.Context(), `
		INSERT INTO foods (name, food_type, is_global, created_by)
		VALUES ($1, 'prepared_food', false, $2)
		RETURNING id::text`, req.Name, userID).Scan(&foodID); err == nil {
		err = saveMealNutrients(r.Context(), tx, foodID, req)
	}
	if err == nil {
		err = tx.QueryRow(r.Context(), `
			INSERT INTO meals (user_id, meal_type, consumed_at, note)
			VALUES ($1, $2, $3, $4)
			RETURNING id::text`, userID, mealTypeAt(consumedAt), consumedAt, req.Name).Scan(&mealID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO meal_items (meal_id, food_id, quantity_g)
			VALUES ($1, $2, 100)`, mealID, foodID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create meal")
		return
	}
	writeJSON(w, http.StatusCreated, responseFromMealRequest(mealID, consumedAt, req))
}

func (h *MealsHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	mealID := chi.URLParam(r, "mealID")
	if !validResourceID(mealID) {
		writeError(w, http.StatusBadRequest, "invalid meal id")
		return
	}
	var req mealRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	consumedAt, err := validateMeal(&req, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update meal")
		return
	}
	defer tx.Rollback(r.Context())
	linked, err := lockInventoryLinkedMeal(r.Context(), tx, userID, mealID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update meal")
		return
	}
	if linked {
		writeError(w, http.StatusConflict, "inventory-linked meals are managed through pantry consumption")
		return
	}
	var foodID string
	err = tx.QueryRow(r.Context(), `
		SELECT mi.food_id::text
		FROM meals m
		JOIN meal_items mi ON mi.meal_id = m.id
		WHERE m.id = $1 AND m.user_id = $2
		ORDER BY mi.id
		LIMIT 1
		FOR UPDATE OF m, mi`, mealID, userID).Scan(&foodID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "meal not found")
		return
	}
	if err == nil {
		var command pgconn.CommandTag
		command, err = tx.Exec(r.Context(), `
			UPDATE foods SET name = $1
			WHERE id = $2 AND created_by = $3`, req.Name, foodID, userID)
		if err == nil && command.RowsAffected() == 0 {
			err = tx.QueryRow(r.Context(), `
				INSERT INTO foods (name, food_type, is_global, created_by)
				VALUES ($1, 'prepared_food', false, $2)
				RETURNING id::text`, req.Name, userID).Scan(&foodID)
			if err == nil {
				_, err = tx.Exec(r.Context(), `
					UPDATE meal_items SET food_id = $1, quantity_g = 100
					WHERE meal_id = $2`, foodID, mealID)
			}
		}
	}
	if err == nil {
		err = saveMealNutrients(r.Context(), tx, foodID, req)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `
			UPDATE meals SET consumed_at = $1, meal_type = $2, note = $3
			WHERE id = $4 AND user_id = $5`, consumedAt, mealTypeAt(consumedAt), req.Name, mealID, userID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update meal")
		return
	}
	writeJSON(w, http.StatusOK, responseFromMealRequest(mealID, consumedAt, req))
}

func (h *MealsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	mealID := chi.URLParam(r, "mealID")
	if !validResourceID(mealID) {
		writeError(w, http.StatusBadRequest, "invalid meal id")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete meal")
		return
	}
	defer tx.Rollback(r.Context())
	linked, err := lockInventoryLinkedMeal(r.Context(), tx, userID, mealID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete meal")
		return
	}
	if linked {
		writeError(w, http.StatusConflict, "inventory-linked meals are managed through pantry consumption")
		return
	}
	rows, err := tx.Query(r.Context(), `
		DELETE FROM meals
		WHERE id = $1 AND user_id = $2
		RETURNING id::text`, mealID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete meal")
		return
	}
	deleted := rows.Next()
	rows.Close()
	if !deleted {
		writeError(w, http.StatusNotFound, "meal not found")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete meal")
		return
	}
	writeNoContent(w)
}
