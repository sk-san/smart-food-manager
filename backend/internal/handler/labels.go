package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sk-san/smart-food-manager/backend/internal/logging"
	"github.com/sk-san/smart-food-manager/backend/internal/middleware"
)

// maxLabelImageBytes caps the size of an uploaded label image.
const maxLabelImageBytes = 10 << 20 // 10 MiB

// LabelExtractor turns a label image into structured text. *gemini.Client
// satisfies this; the handler depends on the interface so it stays testable.
type LabelExtractor interface {
	GenerateFromImage(ctx context.Context, system, prompt, mimeType string, image []byte) (string, error)
}

type LabelHandler struct {
	pool      *pgxpool.Pool
	extractor LabelExtractor
}

func NewLabelHandler(pool *pgxpool.Pool, extractor LabelExtractor) *LabelHandler {
	return &LabelHandler{pool: pool, extractor: extractor}
}

// nutrientRef is a row of the nutrient master used to constrain extraction.
type nutrientRef struct {
	id   int
	code string
	unit string
}

// extractedNutrient is one nutrient amount returned by the model, already
// normalized to per-100g.
type extractedNutrient struct {
	Code          string  `json:"code"`
	AmountPer100g float64 `json:"amount_per_100g"`
}

// extraction is the structured result the model returns for a label.
type extraction struct {
	Name      string              `json:"name"`
	FoodType  string              `json:"food_type"`
	Category  string              `json:"category"`
	Nutrients []extractedNutrient `json:"nutrients"`
}

type extractResponse struct {
	FoodID         string   `json:"food_id"`
	Name           string   `json:"name"`
	FoodType       string   `json:"food_type"`
	SavedNutrients int      `json:"saved_nutrients"`
	SkippedCodes   []string `json:"skipped_codes"`
}

const labelSystemPrompt = `You extract nutrition facts from a product label image.
Read the label and return nutrient amounts normalized to PER 100 g (or per 100 ml).
If the label only lists values per serving, use the serving size to convert to per 100 g.
Only use nutrient codes from the allowed list; ignore anything not in the list.
Respond strictly as JSON, no prose.`

// ExtractAndSave accepts a multipart image upload ("image"), extracts the
// label's nutrients via the vision model, and persists a food plus its
// per-100g nutrient content.
func (h *LabelHandler) ExtractAndSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()

	if h.extractor == nil {
		writeError(w, http.StatusServiceUnavailable, "extractor not configured")
		return
	}

	emitFileEvent(ctx, logging.EventFileUploadStarted, logging.OutcomeStarted, 0, nil)

	// Guard against oversized bodies before buffering them.
	r.Body = http.MaxBytesReader(w, r.Body, maxLabelImageBytes+1024)

	file, header, err := r.FormFile("image")
	if err != nil {
		emitFileEvent(ctx, logging.EventFileUploadFailed, logging.OutcomeFailure, time.Since(start), err)
		writeError(w, http.StatusBadRequest, "image file is required (multipart field 'image')")
		return
	}
	defer file.Close()

	image, err := io.ReadAll(io.LimitReader(file, maxLabelImageBytes+1))
	if err != nil {
		emitFileEvent(ctx, logging.EventFileUploadFailed, logging.OutcomeFailure, time.Since(start), err)
		writeError(w, http.StatusBadRequest, "could not read uploaded image")
		return
	}
	if len(image) == 0 {
		emitFileEvent(ctx, logging.EventFileUploadFailed, logging.OutcomeFailure, time.Since(start), fmt.Errorf("empty image"))
		writeError(w, http.StatusBadRequest, "uploaded image is empty")
		return
	}
	if len(image) > maxLabelImageBytes {
		emitFileEvent(ctx, logging.EventFileUploadFailed, logging.OutcomeFailure, time.Since(start), fmt.Errorf("image too large"))
		writeError(w, http.StatusRequestEntityTooLarge, "image exceeds size limit")
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(image)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		emitFileEvent(ctx, logging.EventFileUploadFailed, logging.OutcomeFailure, time.Since(start), fmt.Errorf("unsupported type %q", mimeType))
		writeError(w, http.StatusUnsupportedMediaType, "uploaded file is not an image")
		return
	}

	// Constrain the model to the nutrient codes we can actually store.
	refs, byCode, err := h.loadNutrients(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load nutrient master")
		return
	}

	raw, err := h.extractor.GenerateFromImage(ctx, labelSystemPrompt, buildLabelPrompt(refs), mimeType, image)
	if err != nil {
		emitFileEvent(ctx, logging.EventFileUploadFailed, logging.OutcomeFailure, time.Since(start), err)
		writeError(w, http.StatusBadGateway, "label extraction failed")
		return
	}

	var ext extraction
	if err := json.Unmarshal([]byte(raw), &ext); err != nil {
		writeError(w, http.StatusBadGateway, "could not parse extraction result")
		return
	}
	if strings.TrimSpace(ext.Name) == "" {
		writeError(w, http.StatusUnprocessableEntity, "no product name found on the label")
		return
	}

	foodID, saved, skipped, err := h.persist(ctx, ext, byCode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save food")
		return
	}

	emitFileEvent(ctx, logging.EventFileUploadCompleted, logging.OutcomeSuccess, time.Since(start), nil)
	writeJSON(w, http.StatusCreated, extractResponse{
		FoodID:         foodID,
		Name:           ext.Name,
		FoodType:       normalizeFoodType(ext.FoodType),
		SavedNutrients: saved,
		SkippedCodes:   skipped,
	})
}

// loadNutrients returns the active nutrient master both as an ordered slice
// (for the prompt) and a code->id map (for persistence).
func (h *LabelHandler) loadNutrients(ctx context.Context) ([]nutrientRef, map[string]int, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT id, code, unit FROM nutrients WHERE is_active = true ORDER BY sort_order, code`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	refs := make([]nutrientRef, 0)
	byCode := make(map[string]int)
	for rows.Next() {
		var n nutrientRef
		if err := rows.Scan(&n.id, &n.code, &n.unit); err != nil {
			return nil, nil, err
		}
		refs = append(refs, n)
		byCode[n.code] = n.id
	}
	return refs, byCode, rows.Err()
}

// persist writes the food and its matched per-100g nutrients in one
// transaction. Nutrients whose code is unknown or whose amount is negative are
// collected into skipped and reported back to the caller.
func (h *LabelHandler) persist(ctx context.Context, ext extraction, byCode map[string]int) (string, int, []string, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return "", 0, nil, err
	}
	defer tx.Rollback(ctx)

	var createdBy any
	if claims, ok := middleware.ClaimsFromContext(ctx); ok && uuid.Validate(claims.UserID) == nil {
		createdBy = claims.UserID
	}
	var foodID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO foods (name, food_type, category, is_global, created_by)
		 VALUES ($1, $2, $3, false, $4)
		 RETURNING id`,
		ext.Name, normalizeFoodType(ext.FoodType), nullString(ext.Category), createdBy,
	).Scan(&foodID); err != nil {
		return "", 0, nil, err
	}

	saved := 0
	skipped := make([]string, 0)
	for _, n := range ext.Nutrients {
		id, ok := byCode[n.Code]
		if !ok || n.AmountPer100g < 0 {
			skipped = append(skipped, n.Code)
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO food_nutrients (food_id, nutrient_id, amount_per_100g)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (food_id, nutrient_id)
			 DO UPDATE SET amount_per_100g = EXCLUDED.amount_per_100g`,
			foodID, id, n.AmountPer100g,
		); err != nil {
			return "", 0, nil, err
		}
		saved++
	}

	if err := tx.Commit(ctx); err != nil {
		return "", 0, nil, err
	}
	return foodID, saved, skipped, nil
}

// buildLabelPrompt lists the allowed nutrient codes (with units) and the
// expected JSON shape.
func buildLabelPrompt(refs []nutrientRef) string {
	var b strings.Builder
	b.WriteString("Allowed nutrient codes (code: unit):\n")
	for _, n := range refs {
		fmt.Fprintf(&b, "- %s: %s\n", n.code, n.unit)
	}
	b.WriteString(`
Return JSON of this exact shape:
{
  "name": "<product name>",
  "food_type": "raw_material" | "prepared_food",
  "category": "<short category, optional>",
  "nutrients": [ { "code": "<allowed code>", "amount_per_100g": <number> } ]
}`)
	return b.String()
}

// normalizeFoodType clamps the model's value to the food_type enum.
func normalizeFoodType(t string) string {
	if t == "raw_material" {
		return "raw_material"
	}
	return "prepared_food"
}

// nullString maps an empty string to a SQL NULL.
func nullString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// emitFileEvent emits a structured file-upload lifecycle event.
func emitFileEvent(ctx context.Context, name, outcome string, d time.Duration, err error) {
	e := logging.Event{
		Severity: logging.LevelInfo,
		Message:  "Label image upload " + outcome,
		Name:     name,
		Category: logging.CategoryFile,
		Action:   "label_extract",
		Outcome:  outcome,
		Duration: d,
	}
	if err != nil {
		e.Severity = logging.LevelError
		e.Attrs = append(e.Attrs, slog.String("error.message", logging.Truncate(err.Error(), 256)))
	}
	logging.Emit(ctx, e)
}
