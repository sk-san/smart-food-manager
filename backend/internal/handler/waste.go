package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WasteHandler struct {
	pool *pgxpool.Pool
}

func NewWasteHandler(pool *pgxpool.Pool) *WasteHandler {
	return &WasteHandler{pool: pool}
}

type wasteRequest struct {
	InventoryItemID string  `json:"inventory_item_id"`
	FoodID          string  `json:"food_id"`
	Quantity        float64 `json:"quantity_g"`
	WastedAt        string  `json:"wasted_at"`
	Reason          string  `json:"reason"`
	DateLabel       string  `json:"date_label"`
	DateStatus      string  `json:"date_status"`
	Package         string  `json:"package"`
	Spoilage        string  `json:"spoilage"`
	Classification  string  `json:"classification"`
	Note            string  `json:"note"`
}

type wasteResponse struct {
	ID                  string    `json:"id"`
	InventoryItemID     string    `json:"inventory_item_id,omitempty"`
	FoodID              string    `json:"food_id"`
	FoodName            string    `json:"food_name"`
	Category            string    `json:"category"`
	Quantity            float64   `json:"quantity_g"`
	WastedAt            time.Time `json:"wasted_at"`
	Reason              string    `json:"reason"`
	DateLabel           string    `json:"date_label"`
	DateStatus          string    `json:"date_status"`
	Package             string    `json:"package"`
	Spoilage            string    `json:"spoilage"`
	Classification      string    `json:"classification"`
	Note                string    `json:"note,omitempty"`
	ImpactKGCO2e        float64   `json:"impact_kg_co2e"`
	VirtualWaterL       float64   `json:"virtual_water_l"`
	TreeEquivalents     float64   `json:"tree_equivalents"`
	ImpactFactorVersion string    `json:"impact_factor_version"`
	CreatedAt           time.Time `json:"created_at"`
}

const wasteSelect = `
		SELECT w.id::text, COALESCE(w.inventory_item_id::text, ''), w.food_id::text,
		       f.name, COALESCE(f.category, ''), w.quantity_g::float8,
		       w.wasted_at, w.reason::text,
	       w.date_label::text, w.date_status::text, w.package::text,
	       w.spoilage::text, COALESCE(w.classification::text, ''),
	       COALESCE(w.note, ''), w.created_at
	FROM waste_events w
	JOIN foods f ON f.id = w.food_id`

func scanWaste(row pgx.Row) (wasteResponse, error) {
	var event wasteResponse
	err := row.Scan(&event.ID, &event.InventoryItemID, &event.FoodID, &event.FoodName, &event.Category,
		&event.Quantity, &event.WastedAt, &event.Reason, &event.DateLabel,
		&event.DateStatus, &event.Package, &event.Spoilage, &event.Classification,
		&event.Note, &event.CreatedAt)
	if err == nil {
		impact := estimateWasteImpact(event.Category, event.FoodName, event.Quantity)
		event.ImpactKGCO2e = impact.KGCO2e
		event.VirtualWaterL = impact.VirtualWaterL
		event.TreeEquivalents = impact.TreeEquivalents
		event.ImpactFactorVersion = environmentalFactorVersion
	}
	return event, err
}

func (h *WasteHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := reconcileExpiredInventory(r.Context(), h.pool, userID, time.Now()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not reconcile expired pantry items")
		return
	}
	rows, err := h.pool.Query(r.Context(), wasteSelect+`
		WHERE w.user_id = $1 ORDER BY w.wasted_at DESC`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load waste events")
		return
	}
	defer rows.Close()
	events := make([]wasteResponse, 0)
	for rows.Next() {
		event, err := scanWaste(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not read waste events")
			return
		}
		events = append(events, event)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "could not read waste events")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *WasteHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	eventID := chi.URLParam(r, "eventID")
	if !validResourceID(eventID) {
		writeError(w, http.StatusBadRequest, "invalid waste event id")
		return
	}
	event, err := scanWaste(h.pool.QueryRow(r.Context(), wasteSelect+`
		WHERE w.user_id = $1 AND w.id = $2`, userID, eventID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "waste event not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load waste event")
		return
	}
	writeJSON(w, http.StatusOK, event)
}

var (
	reasonValues = map[string]bool{
		"expired_best_before": true, "expired_use_by": true,
		"near_expiry_but_not_used": true, "spoiled_visible": true,
		"smelled_or_tasted_bad": true, "forgot_item_existed": true,
		"overbought": true, "cooked_too_much": true,
		"leftover_not_eaten": true, "unsure_if_safe": true,
		"storage_failure": true, "preference_changed": true, "other": true,
	}
	dateStatusValues = map[string]bool{
		"before_date": true, "on_date": true, "1_3_days_after": true,
		"4_7_days_after": true, "8_14_days_after": true,
		"15_plus_days_after": true, "unknown": true,
	}
	spoilageValues = map[string]bool{
		"none": true, "visual_mold": true, "discoloration": true,
		"smell": true, "texture_change": true, "taste": true, "unknown": true,
	}
	classificationValues = map[string]bool{
		"expiry_caused": true, "expiry_related": true, "expiry_unrelated": true,
	}
)

func classifyWaste(reason string) string {
	switch reason {
	case "expired_best_before", "expired_use_by":
		return "expiry_caused"
	case "near_expiry_but_not_used", "forgot_item_existed":
		return "expiry_related"
	default:
		return "expiry_unrelated"
	}
}

func isAutomaticExpiryReason(reason string) bool {
	return reason == "expired_best_before" || reason == "expired_use_by"
}

var errWasteEventChanged = errors.New("waste event changed during mutation")

type lockedWasteMutation struct {
	inventoryID string
	foodID      string
	reason      string
	expiryDate  string
	quantity    float64
	remaining   float64
}

// lockWasteMutation establishes the global lifecycle lock order:
// inventory_items first, then waste_events. The initial event read is
// intentionally unlocked; after obtaining the inventory lock the event is
// locked and re-read so a concurrent inventory delete cannot redirect the
// quantity adjustment.
func lockWasteMutation(
	ctx context.Context,
	tx pgx.Tx,
	userID, eventID string,
) (lockedWasteMutation, error) {
	var initialInventoryID string
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(inventory_item_id::text, '')
		FROM waste_events
		WHERE id = $1 AND user_id = $2`, eventID, userID).Scan(&initialInventoryID)
	if err != nil {
		return lockedWasteMutation{}, err
	}

	inventoryLocked := false
	remaining := 0.0
	var locked lockedWasteMutation
	if initialInventoryID != "" {
		err = tx.QueryRow(ctx, `
			SELECT (quantity_purchased - quantity_consumed - quantity_wasted)::float8,
			       COALESCE(to_char(COALESCE(use_by_date, best_before_date), 'YYYY-MM-DD'), '')
			FROM inventory_items
			WHERE id = $1 AND user_id = $2
			FOR UPDATE`, initialInventoryID, userID).Scan(&remaining, &locked.expiryDate)
		switch {
		case err == nil:
			inventoryLocked = true
		case errors.Is(err, pgx.ErrNoRows):
			// A concurrent inventory delete may have committed after the
			// initial read. Its FK action clears the event link; verify that
			// state under the event lock below and then treat it as standalone.
			err = nil
		default:
			return lockedWasteMutation{}, err
		}
	}

	err = tx.QueryRow(ctx, `
		SELECT COALESCE(inventory_item_id::text, ''), food_id::text,
		       quantity_g::float8, reason::text
		FROM waste_events
		WHERE id = $1 AND user_id = $2
		FOR UPDATE`, eventID, userID).Scan(
		&locked.inventoryID, &locked.foodID, &locked.quantity, &locked.reason,
	)
	if err != nil {
		return lockedWasteMutation{}, err
	}
	if locked.inventoryID != "" {
		if !inventoryLocked || locked.inventoryID != initialInventoryID {
			return lockedWasteMutation{}, errWasteEventChanged
		}
	} else if inventoryLocked {
		return lockedWasteMutation{}, errWasteEventChanged
	}
	locked.remaining = remaining
	return locked, nil
}

func wouldReduceExpiredWaste(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	locked lockedWasteMutation,
	newQuantity float64,
	at time.Time,
) (bool, error) {
	if locked.inventoryID == "" || newQuantity >= locked.quantity || locked.expiryDate == "" {
		return false, nil
	}
	expiry, err := parseOptionalDate(locked.expiryDate)
	if err != nil {
		return false, err
	}
	businessDate, err := userBusinessDate(ctx, tx, userID, at)
	if err != nil {
		return false, err
	}
	return isExpiredOn(expiry, businessDate), nil
}

func validateWaste(req *wasteRequest, defaultTime time.Time) (time.Time, error) {
	if !validGramQuantity(req.Quantity, false) {
		return time.Time{}, errors.New("quantity_g must be positive grams with at most two decimal places")
	}
	if !reasonValues[req.Reason] {
		return time.Time{}, errors.New("invalid discard reason")
	}
	if req.DateLabel == "" {
		req.DateLabel = "unknown"
	}
	if req.DateStatus == "" {
		req.DateStatus = "unknown"
	}
	if req.Package == "" {
		req.Package = "unknown"
	}
	if req.Spoilage == "" {
		req.Spoilage = "unknown"
	}
	if req.Classification == "" {
		req.Classification = classifyWaste(req.Reason)
	}
	if !dateLabelValues[req.DateLabel] || !dateStatusValues[req.DateStatus] ||
		!packageValues[req.Package] || !spoilageValues[req.Spoilage] ||
		!classificationValues[req.Classification] {
		return time.Time{}, errors.New("invalid waste event classification value")
	}
	req.Note = strings.TrimSpace(req.Note)
	if len(req.Note) > 2000 {
		return time.Time{}, errors.New("note must be at most 2000 characters")
	}
	if strings.TrimSpace(req.WastedAt) == "" {
		return defaultTime, nil
	}
	wastedAt, err := time.Parse(time.RFC3339, req.WastedAt)
	if err != nil {
		return time.Time{}, errors.New("wasted_at must be an RFC3339 timestamp")
	}
	return wastedAt, nil
}

func (h *WasteHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req wasteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	wastedAt, err := validateWaste(&req, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.InventoryItemID == "" && req.FoodID == "" {
		writeError(w, http.StatusBadRequest, "inventory_item_id or food_id is required")
		return
	}
	if req.InventoryItemID != "" && !validResourceID(req.InventoryItemID) ||
		req.FoodID != "" && !validResourceID(req.FoodID) {
		writeError(w, http.StatusBadRequest, "invalid inventory or food id")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create waste event")
		return
	}
	defer tx.Rollback(r.Context())
	if req.InventoryItemID != "" {
		var foodID, itemDateLabel, itemPackage string
		var remaining float64
		err = tx.QueryRow(r.Context(), `
			SELECT food_id::text,
			       (quantity_purchased - quantity_consumed - quantity_wasted)::float8,
			       date_label::text, package::text
			FROM inventory_items
			WHERE id = $1 AND user_id = $2
			FOR UPDATE`, req.InventoryItemID, userID).Scan(
			&foodID, &remaining, &itemDateLabel, &itemPackage)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "pantry item not found")
			return
		}
		if err == nil && req.Quantity > remaining {
			writeError(w, http.StatusConflict, errInvalidQuantity.Error())
			return
		}
		if req.FoodID != "" && req.FoodID != foodID {
			writeError(w, http.StatusBadRequest, "food_id does not match the pantry item")
			return
		}
		req.FoodID = foodID
		if req.DateLabel == "unknown" {
			req.DateLabel = itemDateLabel
		}
		if req.Package == "unknown" {
			req.Package = itemPackage
		}
	} else {
		var exists bool
		err = tx.QueryRow(r.Context(), `
			SELECT EXISTS (
				SELECT 1 FROM foods
				WHERE id = $1 AND (is_global OR created_by = $2)
			)`, req.FoodID, userID).Scan(&exists)
		if err == nil && !exists {
			writeError(w, http.StatusNotFound, "food not found")
			return
		}
	}
	var eventID string
	if err == nil {
		err = tx.QueryRow(r.Context(), `
			INSERT INTO waste_events
				(user_id, inventory_item_id, food_id, quantity_g, wasted_at, reason,
				 date_label, date_status, package, spoilage, classification, note)
			VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULLIF($12, ''))
			RETURNING id::text`, userID, req.InventoryItemID, req.FoodID, req.Quantity,
			wastedAt, req.Reason, req.DateLabel, req.DateStatus, req.Package,
			req.Spoilage, req.Classification, req.Note).Scan(&eventID)
	}
	if err == nil && req.InventoryItemID != "" {
		_, err = tx.Exec(r.Context(), `
			UPDATE inventory_items SET
				quantity_wasted = quantity_wasted + $1,
				is_wasted = true,
				is_resolved = (quantity_consumed + quantity_wasted + $1 >= quantity_purchased),
				updated_at = now()
			WHERE id = $2 AND user_id = $3`, req.Quantity, req.InventoryItemID, userID)
	}
	if err == nil && req.InventoryItemID != "" {
		err = reconcileProvisionalMealAfterWaste(r.Context(), tx, userID, req.InventoryItemID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create waste event")
		return
	}
	event, err := scanWaste(h.pool.QueryRow(r.Context(), wasteSelect+`
		WHERE w.user_id = $1 AND w.id = $2`, userID, eventID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "waste event was saved but could not be loaded")
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (h *WasteHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	eventID := chi.URLParam(r, "eventID")
	if !validResourceID(eventID) {
		writeError(w, http.StatusBadRequest, "invalid waste event id")
		return
	}
	var req wasteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	wastedAt, err := validateWaste(&req, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update waste event")
		return
	}
	defer tx.Rollback(r.Context())
	locked, err := lockWasteMutation(r.Context(), tx, userID, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "waste event not found")
		return
	}
	if errors.Is(err, errWasteEventChanged) {
		writeError(w, http.StatusConflict, "waste event changed; retry the request")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update waste event")
		return
	}
	if isAutomaticExpiryReason(locked.reason) {
		writeError(w, http.StatusConflict, "automatic expiry waste events cannot be changed")
		return
	}
	reopensExpired, err := wouldReduceExpiredWaste(
		r.Context(), tx, userID, locked, req.Quantity, time.Now(),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not validate pantry expiration")
		return
	}
	if reopensExpired {
		writeError(w, http.StatusConflict, "waste from an expired pantry item cannot be reduced")
		return
	}
	if req.InventoryItemID != "" && req.InventoryItemID != locked.inventoryID ||
		req.FoodID != "" && req.FoodID != locked.foodID {
		writeError(w, http.StatusBadRequest, "an event cannot be moved to another pantry item or food")
		return
	}
	if locked.inventoryID != "" && req.Quantity-locked.quantity > locked.remaining {
		writeError(w, http.StatusConflict, errInvalidQuantity.Error())
		return
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `
			UPDATE waste_events SET quantity_g = $1, wasted_at = $2, reason = $3,
				date_label = $4, date_status = $5, package = $6, spoilage = $7,
				classification = $8, note = NULLIF($9, '')
			WHERE id = $10 AND user_id = $11`, req.Quantity, wastedAt, req.Reason,
			req.DateLabel, req.DateStatus, req.Package, req.Spoilage,
			req.Classification, req.Note, eventID, userID)
	}
	if err == nil && locked.inventoryID != "" {
		delta := req.Quantity - locked.quantity
		_, err = tx.Exec(r.Context(), `
			UPDATE inventory_items SET
				quantity_wasted = quantity_wasted + $1,
				is_wasted = (quantity_wasted + $1 > 0),
				is_resolved = (quantity_consumed + quantity_wasted + $1 >= quantity_purchased),
				updated_at = now()
			WHERE id = $2 AND user_id = $3`, delta, locked.inventoryID, userID)
	}
	if err == nil && locked.inventoryID != "" {
		err = reconcileProvisionalMealAfterWaste(r.Context(), tx, userID, locked.inventoryID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update waste event")
		return
	}
	event, err := scanWaste(h.pool.QueryRow(r.Context(), wasteSelect+`
		WHERE w.user_id = $1 AND w.id = $2`, userID, eventID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "waste event was saved but could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (h *WasteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	eventID := chi.URLParam(r, "eventID")
	if !validResourceID(eventID) {
		writeError(w, http.StatusBadRequest, "invalid waste event id")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete waste event")
		return
	}
	defer tx.Rollback(r.Context())
	locked, err := lockWasteMutation(r.Context(), tx, userID, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "waste event not found")
		return
	}
	if errors.Is(err, errWasteEventChanged) {
		writeError(w, http.StatusConflict, "waste event changed; retry the request")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete waste event")
		return
	}
	if isAutomaticExpiryReason(locked.reason) {
		writeError(w, http.StatusConflict, "automatic expiry waste events cannot be changed")
		return
	}
	reopensExpired, err := wouldReduceExpiredWaste(
		r.Context(), tx, userID, locked, 0, time.Now(),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not validate pantry expiration")
		return
	}
	if reopensExpired {
		writeError(w, http.StatusConflict, "waste from an expired pantry item cannot be reduced")
		return
	}
	_, err = tx.Exec(r.Context(), `
		DELETE FROM waste_events WHERE id = $1 AND user_id = $2`, eventID, userID)
	if err == nil && locked.inventoryID != "" {
		_, err = tx.Exec(r.Context(), `
			UPDATE inventory_items SET
				quantity_wasted = quantity_wasted - $1,
				is_wasted = (quantity_wasted - $1 > 0),
				is_resolved = (quantity_consumed + quantity_wasted - $1 >= quantity_purchased),
				updated_at = now()
			WHERE id = $2 AND user_id = $3`, locked.quantity, locked.inventoryID, userID)
	}
	if err == nil && locked.inventoryID != "" {
		err = reconcileProvisionalMealAfterWaste(r.Context(), tx, userID, locked.inventoryID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete waste event")
		return
	}
	writeNoContent(w)
}
