package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var sourceTypeValues = map[string]bool{
	"food": true, "product": true, "ingredient": true,
}

type inventoryScanRequest struct {
	SourceType        string           `json:"source_type"`
	Name              string           `json:"name"`
	Category          string           `json:"category"`
	Quantity          float64          `json:"quantity_g"`
	ExpiryDate        string           `json:"expiry_date"`
	ExpiryIsEstimated bool             `json:"expiry_is_estimated"`
	DateLabel         string           `json:"date_label"`
	Storage           string           `json:"storage"`
	Package           string           `json:"package"`
	ConsumedAt        string           `json:"consumed_at"`
	Nutrients         nutritionAmounts `json:"nutrients"`
}

type inventoryScanResponse struct {
	Inventory inventoryResponse `json:"inventory"`
	Meal      *mealResponse     `json:"meal,omitempty"`
}

type consumeInventoryRequest struct {
	Quantity         float64 `json:"quantity_g"`
	DiscardRemaining bool    `json:"discard_remaining"`
	WasteReason      string  `json:"waste_reason"`
}

type consumeInventoryResponse struct {
	Inventory     inventoryResponse `json:"inventory"`
	Meal          *mealResponse     `json:"meal,omitempty"`
	DeletedMealID string            `json:"deleted_meal_id,omitempty"`
	WasteEvent    *wasteResponse    `json:"waste_event,omitempty"`
}

type validatedInventoryScan struct {
	expiry     *time.Time
	consumedAt time.Time
}

func validateInventoryScan(req *inventoryScanRequest, defaultTime time.Time) (validatedInventoryScan, error) {
	req.SourceType = strings.ToLower(strings.TrimSpace(req.SourceType))
	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.TrimSpace(req.Category)
	if !sourceTypeValues[req.SourceType] {
		return validatedInventoryScan{}, errors.New("source_type must be food, product, or ingredient")
	}
	if req.Name == "" || len(req.Name) > 200 {
		return validatedInventoryScan{}, errors.New("name is required and must be at most 200 characters")
	}
	if len(req.Category) > 100 {
		return validatedInventoryScan{}, errors.New("category must be at most 100 characters")
	}
	if !validGramQuantity(req.Quantity, false) {
		return validatedInventoryScan{}, errors.New("quantity_g must be positive grams with at most two decimal places")
	}
	if !finiteNonNegative(
		req.Nutrients.Calories, req.Nutrients.Protein, req.Nutrients.Carbs,
		req.Nutrients.Fat, req.Nutrients.Sodium, req.Nutrients.Calcium,
		req.Nutrients.Iron,
	) {
		return validatedInventoryScan{}, errors.New("nutrient amounts must be non-negative")
	}
	if req.Storage == "" {
		req.Storage = "other"
	}
	if req.Package == "" {
		req.Package = "unopened"
	}
	if req.DateLabel == "" {
		if strings.TrimSpace(req.ExpiryDate) == "" {
			req.DateLabel = "unknown"
		} else {
			req.DateLabel = "best_before"
		}
	}
	if !storageValues[req.Storage] || !packageValues[req.Package] || !dateLabelValues[req.DateLabel] {
		return validatedInventoryScan{}, errors.New("invalid storage, package, or date label value")
	}
	expiry, err := parseOptionalDate(req.ExpiryDate)
	if err != nil {
		return validatedInventoryScan{}, err
	}
	consumedAt := defaultTime
	if strings.TrimSpace(req.ConsumedAt) != "" {
		consumedAt, err = time.Parse(time.RFC3339, req.ConsumedAt)
		if err != nil {
			return validatedInventoryScan{}, errors.New("consumed_at must be an RFC3339 timestamp")
		}
	}
	return validatedInventoryScan{expiry: expiry, consumedAt: consumedAt}, nil
}

func validateConsumeInventory(req consumeInventoryRequest) error {
	if !validGramQuantity(req.Quantity, true) {
		return errors.New("quantity_g must be non-negative grams with at most two decimal places")
	}
	if req.Quantity == 0 && !req.DiscardRemaining {
		return errors.New("quantity_g must be greater than zero unless discarding the remainder")
	}
	if req.DiscardRemaining && req.WasteReason != "" && !reasonValues[req.WasteReason] {
		return errors.New("invalid discard reason")
	}
	return nil
}

func nutritionScale(values nutritionAmounts, factor float64) nutritionAmounts {
	return nutritionAmounts{
		Calories: values.Calories * factor,
		Protein:  values.Protein * factor,
		Carbs:    values.Carbs * factor,
		Fat:      values.Fat * factor,
		Sodium:   values.Sodium * factor,
		Calcium:  values.Calcium * factor,
		Iron:     values.Iron * factor,
	}
}

func mealRequestFromNutrition(name string, values nutritionAmounts) mealRequest {
	return mealRequest{
		Name: name, Calories: values.Calories, Protein: values.Protein,
		Carbs: values.Carbs, Fat: values.Fat, Sodium: values.Sodium,
		Calcium: values.Calcium, Iron: values.Iron,
	}
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// userBusinessDate resolves expiration boundaries using the user's profile
// timezone. PostgreSQL performs the timezone conversion so the static API
// image does not need an operating-system zoneinfo database.
func userBusinessDate(ctx context.Context, q rowQuerier, userID string, at time.Time) (time.Time, error) {
	var raw string
	err := q.QueryRow(ctx, `
		SELECT to_char(
			($2::timestamptz AT TIME ZONE COALESCE(
				(SELECT NULLIF(up.timezone, '') FROM user_profiles up WHERE up.user_id = $1),
				'Asia/Tokyo'
			))::date,
			'YYYY-MM-DD'
		)`, userID, at).Scan(&raw)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse("2006-01-02", raw)
}

func isExpiredOn(expiry *time.Time, businessDate time.Time) bool {
	return expiry != nil && expiry.Before(businessDate)
}

func createLinkedMeal(
	ctx context.Context,
	tx pgx.Tx,
	userID, foodID, inventoryID, name string,
	quantity float64,
	consumedAt time.Time,
) (string, error) {
	var mealID string
	err := tx.QueryRow(ctx, `
		INSERT INTO meals (user_id, meal_type, consumed_at, note)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text`,
		userID, mealTypeAt(consumedAt), consumedAt, name,
	).Scan(&mealID)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO meal_items (meal_id, food_id, inventory_item_id, quantity_g)
		VALUES ($1, $2, $3, $4)`, mealID, foodID, inventoryID, quantity)
	return mealID, err
}

func loadMealResponse(ctx context.Context, q rowQuerier, userID, mealID string) (mealResponse, error) {
	return scanMeal(q.QueryRow(ctx, mealSelect+`
		WHERE m.user_id = $1 AND m.id = $2
		GROUP BY m.id`, userID, mealID))
}

func (h *InventoryHandler) CreateScan(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req inventoryScanRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	now := time.Now()
	validated, err := validateInventoryScan(&req, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	businessDate, err := userBusinessDate(r.Context(), h.pool, userID, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve the pantry expiration date")
		return
	}
	if isExpiredOn(validated.expiry, businessDate) {
		writeError(w, http.StatusBadRequest, "expiry_date cannot be before today")
		return
	}

	per100g := nutritionScale(req.Nutrients, 100/req.Quantity)
	foodType := "prepared_food"
	if req.SourceType == "ingredient" {
		foodType = "raw_material"
	}
	var bestBefore, useBy *time.Time
	if req.DateLabel == "use_by" {
		useBy = validated.expiry
	} else {
		bestBefore = validated.expiry
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save scanned pantry item")
		return
	}
	defer tx.Rollback(r.Context())

	var foodID, itemID, mealID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO foods (name, food_type, category, is_global, created_by)
		VALUES ($1, $2, $3, false, $4)
		RETURNING id::text`,
		req.Name, foodType, nullString(req.Category), userID,
	).Scan(&foodID)
	if err == nil {
		err = saveMealNutrients(r.Context(), tx, foodID, mealRequestFromNutrition(req.Name, per100g))
	}
	if err == nil {
		err = tx.QueryRow(r.Context(), `
			INSERT INTO inventory_items
				(user_id, food_id, quantity_purchased, quantity_consumed,
				 purchase_date, best_before_date, use_by_date, date_label, storage,
				 package, source_type, expiry_is_estimated, is_resolved)
			VALUES ($1, $2, $3, 0, $4, $5, $6, $7, $8, $9, $10, $11, false)
			RETURNING id::text`,
			userID, foodID, req.Quantity, now, bestBefore, useBy,
			req.DateLabel, req.Storage, req.Package, req.SourceType,
			req.ExpiryIsEstimated,
		).Scan(&itemID)
	}
	if err == nil && req.SourceType != "ingredient" {
		mealID, err = createLinkedMeal(
			r.Context(), tx, userID, foodID, itemID, req.Name,
			req.Quantity, validated.consumedAt,
		)
		if err == nil {
			_, err = tx.Exec(r.Context(), `
				UPDATE inventory_items SET provisional_meal_id = $1, updated_at = now()
				WHERE id = $2 AND user_id = $3`, mealID, itemID, userID)
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save scanned pantry item")
		return
	}

	item, err := scanInventory(h.pool.QueryRow(r.Context(), inventorySelect+`
		WHERE i.user_id = $1 AND i.id = $2`, userID, itemID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scanned pantry item was saved but could not be loaded")
		return
	}
	response := inventoryScanResponse{Inventory: item}
	if mealID != "" {
		meal, loadErr := loadMealResponse(r.Context(), h.pool, userID, mealID)
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "scanned pantry item was saved but its meal could not be loaded")
			return
		}
		response.Meal = &meal
	}
	writeJSON(w, http.StatusCreated, response)
}

func (h *InventoryHandler) Consume(w http.ResponseWriter, r *http.Request) {
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
	var req consumeInventoryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateConsumeInventory(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now()
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "pantry consumption transaction failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not record pantry consumption")
		return
	}
	defer tx.Rollback(r.Context())
	businessDate, err := userBusinessDate(r.Context(), tx, userID, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve the pantry expiration date")
		return
	}

	var foodID, name, sourceType, provisionalMealID, dateLabel, packageStatus, expiryRaw string
	var purchased, consumed, wasted float64
	var resolved bool
	err = tx.QueryRow(r.Context(), `
		SELECT i.food_id::text, f.name, i.source_type,
		       COALESCE(i.provisional_meal_id::text, ''),
		       i.quantity_purchased::float8, i.quantity_consumed::float8,
		       i.quantity_wasted::float8, i.is_resolved, i.date_label::text,
		       i.package::text,
		       COALESCE(to_char(COALESCE(i.use_by_date, i.best_before_date), 'YYYY-MM-DD'), '')
		FROM inventory_items i
		JOIN foods f ON f.id = i.food_id
		WHERE i.id = $1 AND i.user_id = $2
		FOR UPDATE OF i`, itemID, userID,
	).Scan(
		&foodID, &name, &sourceType, &provisionalMealID, &purchased,
		&consumed, &wasted, &resolved, &dateLabel, &packageStatus, &expiryRaw,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pantry item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record pantry consumption")
		return
	}
	if resolved {
		writeError(w, http.StatusConflict, "pantry item is already resolved")
		return
	}
	remaining := purchased - consumed - wasted
	expiry, _ := parseOptionalDate(expiryRaw)
	if isExpiredOn(expiry, businessDate) {
		wasteEventID, deletedMealID, reconcileErr := resolveExpiredInventoryItem(r.Context(), tx, userID, expiredInventoryItem{
			id: itemID, foodID: foodID, remaining: remaining, dateLabel: dateLabel,
			packageStatus: packageStatus, expiryDate: expiryRaw,
			provisionalMealID: provisionalMealID,
		}, businessDate, now)
		if reconcileErr == nil {
			reconcileErr = tx.Commit(r.Context())
		}
		if reconcileErr != nil {
			slog.ErrorContext(r.Context(), "expired pantry reconciliation failed", "error", reconcileErr)
			writeError(w, http.StatusInternalServerError, "could not reconcile expired pantry item")
			return
		}
		item, loadErr := scanInventory(h.pool.QueryRow(r.Context(), inventorySelect+`
			WHERE i.user_id = $1 AND i.id = $2`, userID, itemID))
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "expired pantry item was reconciled but could not be loaded")
			return
		}
		response := consumeInventoryResponse{Inventory: item, DeletedMealID: deletedMealID}
		if wasteEventID != "" {
			event, wasteLoadErr := scanWaste(h.pool.QueryRow(r.Context(), wasteSelect+`
				WHERE w.user_id = $1 AND w.id = $2`, userID, wasteEventID))
			if wasteLoadErr != nil {
				writeError(w, http.StatusInternalServerError, "expired pantry item was reconciled but its waste event could not be loaded")
				return
			}
			response.WasteEvent = &event
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	if req.Quantity > remaining {
		writeError(w, http.StatusConflict, errInvalidQuantity.Error())
		return
	}
	remainingAfterConsumption := remaining - req.Quantity
	if req.DiscardRemaining && remainingAfterConsumption > 0 && !reasonValues[req.WasteReason] {
		writeError(w, http.StatusBadRequest, "waste_reason is required when discarding a remainder")
		return
	}

	consumedAt := now
	mealID := ""
	deletedMealID := ""
	newConsumed := consumed + req.Quantity
	if provisionalMealID != "" {
		if newConsumed == 0 {
			command, deleteErr := tx.Exec(r.Context(), `
				DELETE FROM meals WHERE id = $1 AND user_id = $2`, provisionalMealID, userID)
			err = deleteErr
			if err == nil && command.RowsAffected() == 1 {
				deletedMealID = provisionalMealID
			}
		} else {
			_, err = tx.Exec(r.Context(), `
				UPDATE meal_items SET quantity_g = $1
				WHERE meal_id = $2 AND inventory_item_id = $3`,
				newConsumed, provisionalMealID, itemID)
			mealID = provisionalMealID
		}
		provisionalMealID = ""
	} else if req.Quantity > 0 {
		mealID, err = createLinkedMeal(
			r.Context(), tx, userID, foodID, itemID, name, req.Quantity, consumedAt,
		)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not reconcile consumed nutrition")
		return
	}

	wasteEventID := ""
	newWasted := wasted
	if req.DiscardRemaining && remainingAfterConsumption > 0 {
		newWasted += remainingAfterConsumption
		wasteEventID, err = insertLifecycleWasteEvent(
			r.Context(), tx, userID, itemID, foodID, remainingAfterConsumption,
			req.WasteReason, dateLabel, packageStatus,
			dateStatusAt(businessDate, expiry), now,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not record discarded pantry remainder")
			return
		}
	}
	resolved = newConsumed+newWasted >= purchased
	var provisionalMealValue any
	if provisionalMealID != "" {
		provisionalMealValue = provisionalMealID
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE inventory_items SET
			quantity_consumed = $1, quantity_wasted = $2,
			is_wasted = $3, is_resolved = $4,
			provisional_meal_id = $5, updated_at = now()
		WHERE id = $6 AND user_id = $7`,
		newConsumed, newWasted, newWasted > 0, resolved,
		provisionalMealValue, itemID, userID,
	)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "pantry consumption transaction failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not record pantry consumption")
		return
	}

	item, err := scanInventory(h.pool.QueryRow(r.Context(), inventorySelect+`
		WHERE i.user_id = $1 AND i.id = $2`, userID, itemID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pantry consumption was saved but could not be loaded")
		return
	}
	response := consumeInventoryResponse{Inventory: item, DeletedMealID: deletedMealID}
	if mealID != "" {
		meal, loadErr := loadMealResponse(r.Context(), h.pool, userID, mealID)
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "pantry consumption was saved but its meal could not be loaded")
			return
		}
		response.Meal = &meal
	}
	if wasteEventID != "" {
		event, loadErr := scanWaste(h.pool.QueryRow(r.Context(), wasteSelect+`
			WHERE w.user_id = $1 AND w.id = $2`, userID, wasteEventID))
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "pantry consumption was saved but its waste event could not be loaded")
			return
		}
		response.WasteEvent = &event
	}
	writeJSON(w, http.StatusOK, response)
}

func insertLifecycleWasteEvent(
	ctx context.Context,
	tx pgx.Tx,
	userID, inventoryID, foodID string,
	quantity float64,
	reason, dateLabel, packageStatus, dateStatus string,
	wastedAt time.Time,
) (string, error) {
	var eventID string
	err := tx.QueryRow(ctx, `
		INSERT INTO waste_events
			(user_id, inventory_item_id, food_id, quantity_g, wasted_at, reason,
			 date_label, date_status, package, spoilage, classification)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'unknown', $10)
		RETURNING id::text`,
		userID, inventoryID, foodID, quantity, wastedAt, reason, dateLabel,
		dateStatus, packageStatus, classifyWaste(reason),
	).Scan(&eventID)
	return eventID, err
}

// reconcileProvisionalMealAfterWaste keeps the scan-time intake assumption in
// step with the amount that could still be consumed. The meal remains
// provisional after a partial waste event; the first consume action finalizes
// it at the explicitly consumed amount and clears the link.
func reconcileProvisionalMealAfterWaste(ctx context.Context, tx pgx.Tx, userID, inventoryID string) error {
	var sourceType, provisionalMealID, foodID, name string
	var purchased, consumed, wasted float64
	var createdAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT i.source_type, COALESCE(i.provisional_meal_id::text, ''),
		       i.food_id::text, f.name, i.quantity_purchased::float8,
		       i.quantity_consumed::float8, i.quantity_wasted::float8, i.created_at
		FROM inventory_items i
		JOIN foods f ON f.id = i.food_id
		WHERE i.id = $1 AND i.user_id = $2
		FOR UPDATE OF i`, inventoryID, userID,
	).Scan(
		&sourceType, &provisionalMealID, &foodID, &name, &purchased,
		&consumed, &wasted, &createdAt,
	)
	if err != nil || sourceType == "ingredient" {
		return err
	}
	remainingProvisional := purchased - wasted
	if provisionalMealID == "" {
		// A full waste event deletes the provisional meal. If that event is
		// later reduced or removed before any consumption was finalized,
		// recreate the remaining scan-time assumption.
		if consumed == 0 && remainingProvisional > 0 {
			mealID, createErr := createLinkedMeal(
				ctx, tx, userID, foodID, inventoryID, name, remainingProvisional, createdAt,
			)
			if createErr != nil {
				return createErr
			}
			_, createErr = tx.Exec(ctx, `
				UPDATE inventory_items SET provisional_meal_id = $1, updated_at = now()
				WHERE id = $2 AND user_id = $3`, mealID, inventoryID, userID)
			return createErr
		}
		return nil
	}
	if remainingProvisional <= 0 {
		_, err = tx.Exec(ctx, `DELETE FROM meals WHERE id = $1 AND user_id = $2`, provisionalMealID, userID)
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE meal_items SET quantity_g = $1
		WHERE meal_id = $2 AND inventory_item_id = $3`,
		remainingProvisional, provisionalMealID, inventoryID)
	return err
}

func dateStatusAt(now time.Time, expiry *time.Time) string {
	if expiry == nil {
		return "unknown"
	}
	nowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	expiryDate := time.Date(expiry.Year(), expiry.Month(), expiry.Day(), 0, 0, 0, 0, time.UTC)
	days := int(nowDate.Sub(expiryDate).Hours() / 24)
	switch {
	case days < 0:
		return "before_date"
	case days == 0:
		return "on_date"
	case days <= 3:
		return "1_3_days_after"
	case days <= 7:
		return "4_7_days_after"
	case days <= 14:
		return "8_14_days_after"
	default:
		return "15_plus_days_after"
	}
}

type expiredInventoryItem struct {
	id                string
	foodID            string
	remaining         float64
	dateLabel         string
	packageStatus     string
	expiryDate        string
	provisionalMealID string
}

func resolveExpiredInventoryItem(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	item expiredInventoryItem,
	businessDate, wastedAt time.Time,
) (string, string, error) {
	deletedMealID := ""
	if item.provisionalMealID != "" {
		if _, err := tx.Exec(ctx, `
			DELETE FROM meals WHERE id = $1 AND user_id = $2`,
			item.provisionalMealID, userID,
		); err != nil {
			return "", "", err
		}
		deletedMealID = item.provisionalMealID
	}
	wasteEventID := ""
	if item.remaining > 0 {
		reason := "expired_best_before"
		if item.dateLabel == "use_by" {
			reason = "expired_use_by"
		}
		expiry, _ := parseOptionalDate(item.expiryDate)
		var err error
		wasteEventID, err = insertLifecycleWasteEvent(
			ctx, tx, userID, item.id, item.foodID, item.remaining, reason,
			item.dateLabel, item.packageStatus, dateStatusAt(businessDate, expiry), wastedAt,
		)
		if err != nil {
			return "", "", err
		}
	}
	_, err := tx.Exec(ctx, `
		UPDATE inventory_items SET
			quantity_wasted = quantity_wasted + $1,
			is_wasted = (quantity_wasted + $1 > 0), is_resolved = true,
			provisional_meal_id = NULL, updated_at = now()
		WHERE id = $2 AND user_id = $3 AND is_resolved = false`,
		item.remaining, item.id, userID,
	)
	return wasteEventID, deletedMealID, err
}

// reconcileExpiredInventory logically moves expired stock into waste. Rows are
// locked and marked resolved in the same transaction as the waste insert, so a
// concurrent inventory/waste list cannot create a duplicate event.
func reconcileExpiredInventory(ctx context.Context, pool *pgxpool.Pool, userID string, now time.Time) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	businessDate, err := userBusinessDate(ctx, tx, userID, now)
	if err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT i.id::text, i.food_id::text,
		       (i.quantity_purchased - i.quantity_consumed - i.quantity_wasted)::float8,
		       i.date_label::text, i.package::text,
		       to_char(COALESCE(i.use_by_date, i.best_before_date), 'YYYY-MM-DD'),
		       COALESCE(i.provisional_meal_id::text, '')
		FROM inventory_items i
		WHERE i.user_id = $1 AND i.is_resolved = false
		  AND COALESCE(i.use_by_date, i.best_before_date) < $2::date
		FOR UPDATE`, userID, businessDate)
	if err != nil {
		return err
	}
	expired := make([]expiredInventoryItem, 0)
	for rows.Next() {
		var item expiredInventoryItem
		if err := rows.Scan(
			&item.id, &item.foodID, &item.remaining, &item.dateLabel,
			&item.packageStatus, &item.expiryDate, &item.provisionalMealID,
		); err != nil {
			rows.Close()
			return err
		}
		expired = append(expired, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}

	for _, item := range expired {
		if _, _, err := resolveExpiredInventoryItem(ctx, tx, userID, item, businessDate, now); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
