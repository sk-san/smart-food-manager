package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

// FoodAnalyzer estimates nutrition from a text description or food image.
// *gemini.Client satisfies this; the handler depends on the interface so it
// stays unit-testable with a fake.
type FoodAnalyzer interface {
	GenerateText(ctx context.Context, system, prompt string) (string, error)
	GenerateFromImage(ctx context.Context, system, prompt, mimeType string, image []byte) (string, error)
}

type NutritionHandler struct {
	analyzer FoodAnalyzer
}

func NewNutritionHandler(analyzer FoodAnalyzer) *NutritionHandler {
	return &NutritionHandler{analyzer: analyzer}
}

// analyzeRequest mirrors the body sent by the frontend AddEntryModal:
// either {type:"text", text:"..."} or
// {type:"image", mimeType:"image/jpeg", data:"<base64>"}.
type analyzeRequest struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
	ScanType string `json:"scanType"`
}

// analyzedItem matches the frontend AnalyzedFoodItem. Macros are grams;
// sodium/calcium/iron are milligrams; calories are kcal.
type analyzedItem struct {
	Name                string  `json:"name"`
	Calories            float64 `json:"calories"`
	Protein             float64 `json:"protein"`
	Carbs               float64 `json:"carbs"`
	Fat                 float64 `json:"fat"`
	Sodium              float64 `json:"sodium"`
	Calcium             float64 `json:"calcium"`
	Iron                float64 `json:"iron"`
	ScanType            string  `json:"scanType"`
	QuantityGrams       float64 `json:"quantityGrams"`
	Category            string  `json:"category"`
	EstimatedExpiryDays int     `json:"estimatedExpiryDays"`
}

const (
	// Base64 adds roughly one third to the decoded size. Nine MiB leaves room
	// for a six MiB image plus its JSON envelope while still putting a hard
	// bound on memory used by the decoder.
	maxAnalyzeRequestBodyBytes = 9 << 20
	maxAnalyzeImageBytes       = 6 << 20
)

var supportedAnalyzeImageTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

const analyzeSystemPrompt = `You are a nutrition estimator for a food logging app.
Given a text description or a photo, identify each distinct food item and
estimate its nutrition for the portion actually shown or described. The scan
intent is food (ready to eat), product (packaged), or ingredient (raw material).
Be realistic; if unsure, give your best estimate. Respond strictly as JSON.`

// analyzeSchemaPrompt is appended to both modes so the model returns the exact
// shape the frontend expects.
const analyzeSchemaPrompt = `Return JSON of this exact shape:
{"items": [
  {"name": string, "calories": number, "protein": number, "carbs": number,
   "fat": number, "sodium": number, "calcium": number, "iron": number,
   "scanType": "food" | "product" | "ingredient", "quantityGrams": number,
   "category": string, "estimatedExpiryDays": integer}
]}
Units: calories in kcal; protein, carbs and fat in grams; sodium, calcium and
iron in milligrams. Nutrition is for quantityGrams, not per 100 g. Use numbers
(not strings) and omit no fields.`

// Analyze accepts a text or image payload from the modal, runs it through the
// Gemini model, and returns the estimated food items.
func (h *NutritionHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	if h.analyzer == nil {
		writeError(w, http.StatusServiceUnavailable, "analyzer not configured")
		return
	}

	var req analyzeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAnalyzeRequestBodyBytes))
	if err := decoder.Decode(&req); err != nil {
		writeAnalyzeDecodeError(w, err)
		return
	}
	// Consume the rest of the body. This both rejects a second JSON value and
	// ensures trailing data cannot bypass MaxBytesReader after the first value.
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeAnalyzeDecodeError(w, err)
		return
	}
	scanType := strings.ToLower(strings.TrimSpace(req.ScanType))
	if scanType == "" {
		scanType = "food"
	}
	if !sourceTypeValues[scanType] {
		writeError(w, http.StatusBadRequest, "scanType must be food, product, or ingredient")
		return
	}
	scanContext := "Scan intent: " + scanType + ".\n"

	var raw string
	var err error
	switch req.Type {
	case "text":
		if strings.TrimSpace(req.Text) == "" {
			writeError(w, http.StatusBadRequest, "text is required")
			return
		}
		prompt := scanContext + "Food description:\n" + req.Text + "\n\n" + analyzeSchemaPrompt
		raw, err = h.analyzer.GenerateText(r.Context(), analyzeSystemPrompt, prompt)

	case "image":
		image, decErr := base64.StdEncoding.DecodeString(req.Data)
		if decErr != nil || len(image) == 0 {
			writeError(w, http.StatusBadRequest, "invalid image data")
			return
		}
		if len(image) > maxAnalyzeImageBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "image exceeds 6 MiB limit")
			return
		}
		imageType, validationErr := validateAnalyzeImage(req.MimeType, image)
		if validationErr != "" {
			writeError(w, http.StatusUnsupportedMediaType, validationErr)
			return
		}
		prompt := scanContext + "Analyze the food in this image.\n\n" + analyzeSchemaPrompt
		raw, err = h.analyzer.GenerateFromImage(r.Context(), analyzeSystemPrompt, prompt, imageType, image)

	default:
		writeError(w, http.StatusBadRequest, `type must be "text" or "image"`)
		return
	}

	if err != nil {
		// Detail is captured in the dependency logs/metrics; keep the client
		// response generic.
		writeError(w, http.StatusBadGateway, "analysis failed")
		return
	}

	var parsed struct {
		Items []analyzedItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		writeError(w, http.StatusBadGateway, "could not parse analysis result")
		return
	}

	// The frontend expects a bare array; never return a nil slice.
	if parsed.Items == nil {
		parsed.Items = []analyzedItem{}
	}
	for i := range parsed.Items {
		parsed.Items[i].ScanType = scanType
		if !finitePositive(parsed.Items[i].QuantityGrams) {
			parsed.Items[i].QuantityGrams = 100
		}
		parsed.Items[i].Category = strings.TrimSpace(parsed.Items[i].Category)
		if parsed.Items[i].Category == "" {
			parsed.Items[i].Category = "other"
		}
		if parsed.Items[i].EstimatedExpiryDays <= 0 {
			parsed.Items[i].EstimatedExpiryDays = defaultEstimatedExpiryDays(scanType)
		}
	}
	writeJSON(w, http.StatusOK, parsed.Items)
}

func defaultEstimatedExpiryDays(scanType string) int {
	switch scanType {
	case "product":
		return 30
	case "ingredient":
		return 7
	default:
		return 3
	}
}

func writeAnalyzeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(w, http.StatusRequestEntityTooLarge, "request body exceeds 9 MiB limit")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid request body")
}

// validateAnalyzeImage treats the bytes as authoritative and only accepts the
// formats the client normalizes to. A supplied MIME type must also be valid,
// supported, and agree with the detected content.
func validateAnalyzeImage(declaredType string, image []byte) (string, string) {
	detectedType := http.DetectContentType(image)
	if _, ok := supportedAnalyzeImageTypes[detectedType]; !ok {
		return "", "unsupported image content; use JPEG, PNG, or WebP"
	}

	if strings.TrimSpace(declaredType) == "" {
		return detectedType, ""
	}

	parsedType, _, err := mime.ParseMediaType(strings.TrimSpace(declaredType))
	if err != nil {
		return "", "unsupported image MIME type; use JPEG, PNG, or WebP"
	}
	parsedType = strings.ToLower(parsedType)
	if _, ok := supportedAnalyzeImageTypes[parsedType]; !ok {
		return "", "unsupported image MIME type; use JPEG, PNG, or WebP"
	}
	if parsedType != detectedType {
		return "", "image MIME type does not match its content"
	}
	return detectedType, ""
}
