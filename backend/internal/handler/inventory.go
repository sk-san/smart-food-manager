package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InventoryHandler struct {
	pool *pgxpool.Pool
}

func NewInventoryHandler(pool *pgxpool.Pool) *InventoryHandler {
	return &InventoryHandler{pool: pool}
}

type inventoryRequest struct {
	Name              string  `json:"name"`
	QuantityPurchased float64 `json:"quantity_purchased"`
	QuantityConsumed  float64 `json:"quantity_consumed"`
	PurchaseDate      string  `json:"purchase_date"`
	BestBeforeDate    string  `json:"best_before_date"`
	UseByDate         string  `json:"use_by_date"`
	DateLabel         string  `json:"date_label"`
	Storage           string  `json:"storage"`
	Package           string  `json:"package"`
}

type inventoryResponse struct {
	ID                string           `json:"id"`
	FoodID            string           `json:"food_id"`
	Name              string           `json:"name"`
	Category          string           `json:"category"`
	SourceType        string           `json:"source_type"`
	ProvisionalMealID string           `json:"provisional_meal_id,omitempty"`
	ExpiryIsEstimated bool             `json:"expiry_is_estimated"`
	QuantityPurchased float64          `json:"quantity_purchased"`
	QuantityConsumed  float64          `json:"quantity_consumed"`
	QuantityWasted    float64          `json:"quantity_wasted"`
	PurchaseDate      string           `json:"purchase_date,omitempty"`
	BestBeforeDate    string           `json:"best_before_date,omitempty"`
	UseByDate         string           `json:"use_by_date,omitempty"`
	DateLabel         string           `json:"date_label"`
	Storage           string           `json:"storage"`
	Package           string           `json:"package"`
	IsWasted          bool             `json:"is_wasted"`
	IsResolved        bool             `json:"is_resolved"`
	ConsumedPct       float64          `json:"consumed_pct"`
	WastedPct         float64          `json:"wasted_pct"`
	NutritionPer100g  nutritionAmounts `json:"nutrition_per_100g"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

// nutritionAmounts is the fixed nutrient vocabulary shared by scan requests,
// pantry responses, and meal responses. Macro values are grams, mineral values
// are milligrams, and calories are kcal.
type nutritionAmounts struct {
	Calories float64 `json:"calories"`
	Protein  float64 `json:"protein"`
	Carbs    float64 `json:"carbs"`
	Fat      float64 `json:"fat"`
	Sodium   float64 `json:"sodium"`
	Calcium  float64 `json:"calcium"`
	Iron     float64 `json:"iron"`
}

const inventorySelect = `
	SELECT i.id::text, i.food_id::text, f.name, COALESCE(f.category, ''),
	       i.source_type, COALESCE(i.provisional_meal_id::text, ''),
	       i.expiry_is_estimated,
	       i.quantity_purchased::float8, i.quantity_consumed::float8,
	       i.quantity_wasted::float8,
	       COALESCE(to_char(i.purchase_date, 'YYYY-MM-DD'), ''),
	       COALESCE(to_char(i.best_before_date, 'YYYY-MM-DD'), ''),
	       COALESCE(to_char(i.use_by_date, 'YYYY-MM-DD'), ''),
	       i.date_label::text, i.storage::text, i.package::text,
	       i.is_wasted, i.is_resolved,
	       i.consumed_pct::float8, i.wasted_pct::float8,
	       COALESCE((SELECT fn.amount_per_100g::float8 FROM food_nutrients fn
	                 JOIN nutrients n ON n.id = fn.nutrient_id
	                 WHERE fn.food_id = i.food_id AND n.code = 'calories'), 0),
	       COALESCE((SELECT fn.amount_per_100g::float8 FROM food_nutrients fn
	                 JOIN nutrients n ON n.id = fn.nutrient_id
	                 WHERE fn.food_id = i.food_id AND n.code = 'protein'), 0),
	       COALESCE((SELECT fn.amount_per_100g::float8 FROM food_nutrients fn
	                 JOIN nutrients n ON n.id = fn.nutrient_id
	                 WHERE fn.food_id = i.food_id AND n.code = 'carbs'), 0),
	       COALESCE((SELECT fn.amount_per_100g::float8 FROM food_nutrients fn
	                 JOIN nutrients n ON n.id = fn.nutrient_id
	                 WHERE fn.food_id = i.food_id AND n.code = 'fat'), 0),
	       COALESCE((SELECT fn.amount_per_100g::float8 FROM food_nutrients fn
	                 JOIN nutrients n ON n.id = fn.nutrient_id
	                 WHERE fn.food_id = i.food_id AND n.code = 'sodium'), 0),
	       COALESCE((SELECT fn.amount_per_100g::float8 FROM food_nutrients fn
	                 JOIN nutrients n ON n.id = fn.nutrient_id
	                 WHERE fn.food_id = i.food_id AND n.code = 'calcium'), 0),
	       COALESCE((SELECT fn.amount_per_100g::float8 FROM food_nutrients fn
	                 JOIN nutrients n ON n.id = fn.nutrient_id
	                 WHERE fn.food_id = i.food_id AND n.code = 'iron'), 0),
	       i.created_at, i.updated_at
	FROM inventory_items i
	JOIN foods f ON f.id = i.food_id`

func scanInventory(row pgx.Row) (inventoryResponse, error) {
	var item inventoryResponse
	err := row.Scan(&item.ID, &item.FoodID, &item.Name, &item.Category,
		&item.SourceType, &item.ProvisionalMealID, &item.ExpiryIsEstimated,
		&item.QuantityPurchased, &item.QuantityConsumed, &item.QuantityWasted,
		&item.PurchaseDate, &item.BestBeforeDate, &item.UseByDate,
		&item.DateLabel, &item.Storage, &item.Package, &item.IsWasted,
		&item.IsResolved, &item.ConsumedPct, &item.WastedPct,
		&item.NutritionPer100g.Calories, &item.NutritionPer100g.Protein,
		&item.NutritionPer100g.Carbs, &item.NutritionPer100g.Fat,
		&item.NutritionPer100g.Sodium, &item.NutritionPer100g.Calcium,
		&item.NutritionPer100g.Iron,
		&item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (h *InventoryHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := reconcileExpiredInventory(r.Context(), h.pool, userID, time.Now()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not reconcile expired pantry items")
		return
	}
	rows, err := h.pool.Query(r.Context(), inventorySelect+`
		WHERE i.user_id = $1 AND i.is_resolved = false
		ORDER BY COALESCE(i.use_by_date, i.best_before_date, 'infinity'::date), i.created_at`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load pantry")
		return
	}
	defer rows.Close()
	items := make([]inventoryResponse, 0)
	for rows.Next() {
		item, err := scanInventory(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not read pantry")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "could not read pantry")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *InventoryHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	itemID := chi.URLParam(r, "itemID")
	if !validResourceID(itemID) {
		writeError(w, http.StatusBadRequest, "invalid pantry item id")
		return
	}
	if err := reconcileExpiredInventory(r.Context(), h.pool, userID, time.Now()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not reconcile expired pantry items")
		return
	}
	item, err := scanInventory(h.pool.QueryRow(r.Context(), inventorySelect+`
		WHERE i.user_id = $1 AND i.id = $2 AND i.is_resolved = false`, userID, itemID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pantry item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load pantry item")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

var (
	storageValues   = map[string]bool{"fridge": true, "freezer": true, "pantry": true, "other": true}
	packageValues   = map[string]bool{"unopened": true, "opened": true, "cooked": true, "leftover": true, "unknown": true}
	dateLabelValues = map[string]bool{
		"best_before": true, "use_by": true, "display_until": true,
		"no_date_label": true, "unknown": true,
	}
)

type validatedInventory struct {
	purchaseDate   *time.Time
	bestBeforeDate *time.Time
	useByDate      *time.Time
}

func validateInventory(req *inventoryRequest, wasted float64, businessDate time.Time) (validatedInventory, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 200 {
		return validatedInventory{}, errors.New("name is required and must be at most 200 characters")
	}
	if !validGramQuantity(req.QuantityPurchased, false) ||
		!validGramQuantity(req.QuantityConsumed, true) ||
		!validGramQuantity(wasted, true) {
		return validatedInventory{}, errors.New("quantities must be non-negative grams with at most two decimal places")
	}
	if req.QuantityConsumed+wasted > req.QuantityPurchased {
		return validatedInventory{}, errors.New("consumed and wasted quantities cannot exceed purchased quantity")
	}
	if req.Storage == "" {
		req.Storage = "other"
	}
	if req.Package == "" {
		req.Package = "unopened"
	}
	if req.DateLabel == "" {
		req.DateLabel = "unknown"
	}
	if !storageValues[req.Storage] || !packageValues[req.Package] || !dateLabelValues[req.DateLabel] {
		return validatedInventory{}, errors.New("invalid storage, package, or date label value")
	}
	purchase, err := parseOptionalDate(req.PurchaseDate)
	if err != nil {
		return validatedInventory{}, err
	}
	bestBefore, err := parseOptionalDate(req.BestBeforeDate)
	if err != nil {
		return validatedInventory{}, err
	}
	useBy, err := parseOptionalDate(req.UseByDate)
	if err != nil {
		return validatedInventory{}, err
	}
	if isExpiredOn(bestBefore, businessDate) || isExpiredOn(useBy, businessDate) {
		return validatedInventory{}, errors.New("expiration dates cannot be before today")
	}
	return validatedInventory{purchaseDate: purchase, bestBeforeDate: bestBefore, useByDate: useBy}, nil
}

func (h *InventoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req inventoryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	now := time.Now()
	businessDate, err := userBusinessDate(r.Context(), h.pool, userID, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve the pantry expiration date")
		return
	}
	validated, err := validateInventory(&req, 0, businessDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not add pantry item")
		return
	}
	defer tx.Rollback(r.Context())
	var foodID, itemID string
	if err = tx.QueryRow(r.Context(), `
		INSERT INTO foods (name, food_type, is_global, created_by)
		VALUES ($1, 'raw_material', false, $2)
		RETURNING id::text`, req.Name, userID).Scan(&foodID); err == nil {
		err = tx.QueryRow(r.Context(), `
			INSERT INTO inventory_items
				(user_id, food_id, quantity_purchased, quantity_consumed,
				 purchase_date, best_before_date, use_by_date, date_label, storage, package,
				 is_resolved)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id::text`, userID, foodID, req.QuantityPurchased, req.QuantityConsumed,
			validated.purchaseDate, validated.bestBeforeDate, validated.useByDate,
			req.DateLabel, req.Storage, req.Package,
			req.QuantityConsumed >= req.QuantityPurchased).Scan(&itemID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not add pantry item")
		return
	}
	item, err := scanInventory(h.pool.QueryRow(r.Context(), inventorySelect+`
		WHERE i.user_id = $1 AND i.id = $2`, userID, itemID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pantry item was saved but could not be loaded")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *InventoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	itemID := chi.URLParam(r, "itemID")
	if !validResourceID(itemID) {
		writeError(w, http.StatusBadRequest, "invalid pantry item id")
		return
	}
	var req inventoryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update pantry item")
		return
	}
	defer tx.Rollback(r.Context())
	var foodID, sourceType string
	var purchased, consumed, wasted float64
	err = tx.QueryRow(r.Context(), `
		SELECT food_id::text, source_type, quantity_purchased::float8,
		       quantity_consumed::float8, quantity_wasted::float8
		FROM inventory_items
		WHERE id = $1 AND user_id = $2
		FOR UPDATE`, itemID, userID).Scan(&foodID, &sourceType, &purchased, &consumed, &wasted)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pantry item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update pantry item")
		return
	}
	businessDate, err := userBusinessDate(r.Context(), tx, userID, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve the pantry expiration date")
		return
	}
	validated, err := validateInventory(&req, wasted, businessDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.QuantityConsumed != consumed {
		writeError(w, http.StatusConflict, "quantity_consumed is read-only; use the consume endpoint")
		return
	}
	if sourceType != "ingredient" && req.QuantityPurchased != purchased {
		writeError(w, http.StatusConflict, "quantity_purchased cannot be changed for scanned food or product")
		return
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE foods SET name = $1 WHERE id = $2 AND created_by = $3`, req.Name, foodID, userID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `
			UPDATE inventory_items SET
				quantity_purchased = $1, quantity_consumed = $2,
				purchase_date = $3, best_before_date = $4, use_by_date = $5,
				date_label = $6, storage = $7, package = $8,
				is_resolved = ($2 + quantity_wasted >= $1), updated_at = now()
			WHERE id = $9 AND user_id = $10`, req.QuantityPurchased, req.QuantityConsumed,
			validated.purchaseDate, validated.bestBeforeDate, validated.useByDate,
			req.DateLabel, req.Storage, req.Package, itemID, userID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update pantry item")
		return
	}
	item, err := scanInventory(h.pool.QueryRow(r.Context(), inventorySelect+`
		WHERE i.user_id = $1 AND i.id = $2`, userID, itemID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pantry item was saved but could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *InventoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	itemID := chi.URLParam(r, "itemID")
	if !validResourceID(itemID) {
		writeError(w, http.StatusBadRequest, "invalid pantry item id")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete pantry item")
		return
	}
	defer tx.Rollback(r.Context())

	var provisionalMealID string
	err = tx.QueryRow(r.Context(), `
		SELECT COALESCE(provisional_meal_id::text, '')
		FROM inventory_items
		WHERE id = $1 AND user_id = $2
		FOR UPDATE`, itemID, userID).Scan(&provisionalMealID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pantry item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete pantry item")
		return
	}
	if provisionalMealID != "" {
		_, err = tx.Exec(r.Context(), `
			DELETE FROM meals WHERE id = $1 AND user_id = $2`, provisionalMealID, userID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `
			DELETE FROM inventory_items WHERE id = $1 AND user_id = $2`, itemID, userID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete pantry item")
		return
	}
	writeNoContent(w)
}
