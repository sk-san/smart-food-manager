package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
}

// analyzedItem matches the frontend AnalyzedFoodItem. Macros are grams;
// sodium/calcium/iron are milligrams; calories are kcal.
type analyzedItem struct {
	Name     string  `json:"name"`
	Calories float64 `json:"calories"`
	Protein  float64 `json:"protein"`
	Carbs    float64 `json:"carbs"`
	Fat      float64 `json:"fat"`
	Sodium   float64 `json:"sodium"`
	Calcium  float64 `json:"calcium"`
	Iron     float64 `json:"iron"`
}

const analyzeSystemPrompt = `You are a nutrition estimator for a food logging app.
Given a text description or a photo of a meal, identify each distinct food item
and estimate its nutrition for the portion actually shown or described.
Be realistic; if unsure, give your best estimate. Respond strictly as JSON.`

// analyzeSchemaPrompt is appended to both modes so the model returns the exact
// shape the frontend expects.
const analyzeSchemaPrompt = `Return JSON of this exact shape:
{"items": [
  {"name": string, "calories": number, "protein": number, "carbs": number,
   "fat": number, "sodium": number, "calcium": number, "iron": number}
]}
Units: calories in kcal; protein, carbs and fat in grams; sodium, calcium and
iron in milligrams. Use numbers (not strings) and omit no fields.`

// Analyze accepts a text or image payload from the modal, runs it through the
// Gemini model, and returns the estimated food items.
func (h *NutritionHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	if h.analyzer == nil {
		writeError(w, http.StatusServiceUnavailable, "analyzer not configured")
		return
	}

	var req analyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var raw string
	var err error
	switch req.Type {
	case "text":
		if strings.TrimSpace(req.Text) == "" {
			writeError(w, http.StatusBadRequest, "text is required")
			return
		}
		prompt := "Food description:\n" + req.Text + "\n\n" + analyzeSchemaPrompt
		raw, err = h.analyzer.GenerateText(r.Context(), analyzeSystemPrompt, prompt)

	case "image":
		image, decErr := base64.StdEncoding.DecodeString(req.Data)
		if decErr != nil || len(image) == 0 {
			writeError(w, http.StatusBadRequest, "invalid image data")
			return
		}
		mime := req.MimeType
		if mime == "" {
			mime = http.DetectContentType(image)
		}
		if !strings.HasPrefix(mime, "image/") {
			writeError(w, http.StatusUnsupportedMediaType, "data is not an image")
			return
		}
		prompt := "Analyze the food in this image.\n\n" + analyzeSchemaPrompt
		raw, err = h.analyzer.GenerateFromImage(r.Context(), analyzeSystemPrompt, prompt, mime, image)

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
	writeJSON(w, http.StatusOK, parsed.Items)
}
