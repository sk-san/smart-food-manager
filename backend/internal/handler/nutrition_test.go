package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeFoodAnalyzer struct {
	textResult  string
	imageResult string
	err         error
	textCalls   int
	imageCalls  int
	system      string
	prompt      string
	mimeType    string
	image       []byte
}

func (f *fakeFoodAnalyzer) GenerateText(_ context.Context, system, prompt string) (string, error) {
	f.textCalls++
	f.system = system
	f.prompt = prompt
	return f.textResult, f.err
}

func (f *fakeFoodAnalyzer) GenerateFromImage(
	_ context.Context,
	system, prompt, mimeType string,
	image []byte,
) (string, error) {
	f.imageCalls++
	f.system = system
	f.prompt = prompt
	f.mimeType = mimeType
	f.image = append([]byte(nil), image...)
	return f.imageResult, f.err
}

func TestNutritionAnalyzeText(t *testing.T) {
	fake := &fakeFoodAnalyzer{textResult: `{"items":[{"name":"Apple","calories":95,"protein":0.5,"carbs":25,"fat":0.3,"sodium":1,"calcium":6,"iron":0.1}]}`}
	h := NewNutritionHandler(fake)
	res := performAnalyzeRequest(h, `{"type":"text","text":" one medium apple "}`)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", res.Code, res.Body.String())
	}
	var items []analyzedItem
	if err := json.Unmarshal(res.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 || items[0].Name != "Apple" || items[0].Calories != 95 {
		t.Errorf("items = %+v", items)
	}
	if items[0].ScanType != "food" || items[0].QuantityGrams != 100 ||
		items[0].Category != "other" || items[0].EstimatedExpiryDays != 3 {
		t.Errorf("backward-compatible metadata defaults = %+v", items[0])
	}
	if fake.textCalls != 1 || fake.imageCalls != 0 {
		t.Errorf("calls: text=%d image=%d", fake.textCalls, fake.imageCalls)
	}
	if !strings.Contains(fake.prompt, "one medium apple") || !strings.Contains(fake.prompt, `{"items":`) {
		t.Errorf("prompt does not contain the description and schema: %q", fake.prompt)
	}
	if fake.system != analyzeSystemPrompt {
		t.Error("system prompt was not forwarded")
	}
}

func TestNutritionAnalyzeScanMetadata(t *testing.T) {
	fake := &fakeFoodAnalyzer{textResult: `{"items":[{"name":"Protein bar","calories":220,"protein":20,"carbs":24,"fat":7,"sodium":180,"calcium":80,"iron":2,"scanType":"product","quantityGrams":55,"category":"snack","estimatedExpiryDays":120}]}`}
	res := performAnalyzeRequest(NewNutritionHandler(fake), `{"type":"text","text":"protein bar","scanType":"product"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var items []analyzedItem
	if err := json.Unmarshal(res.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ScanType != "product" || items[0].QuantityGrams != 55 ||
		items[0].Category != "snack" || items[0].EstimatedExpiryDays != 120 {
		t.Errorf("metadata = %+v", items)
	}
	if !strings.Contains(fake.prompt, "Scan intent: product") || !strings.Contains(fake.prompt, "quantityGrams") {
		t.Errorf("metadata prompt = %q", fake.prompt)
	}
}

func TestNutritionAnalyzeImage(t *testing.T) {
	image := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	fake := &fakeFoodAnalyzer{imageResult: `{"items":[]}`}
	h := NewNutritionHandler(fake)
	res := performAnalyzeRequest(h, imageAnalyzeBody("", image))

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", res.Code, res.Body.String())
	}
	if fake.imageCalls != 1 || fake.textCalls != 0 {
		t.Errorf("calls: text=%d image=%d", fake.textCalls, fake.imageCalls)
	}
	if fake.mimeType != "image/png" {
		t.Errorf("mime type = %q, want image/png", fake.mimeType)
	}
	if string(fake.image) != string(image) {
		t.Errorf("image = %v, want %v", fake.image, image)
	}
	if !strings.Contains(fake.prompt, "Analyze the food in this image") {
		t.Errorf("prompt = %q", fake.prompt)
	}
}

func TestNutritionAnalyzeSupportedImageTypes(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		image    []byte
	}{
		{name: "JPEG", mimeType: "image/jpeg", image: []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0, 'J', 'F', 'I', 'F', 0}},
		{name: "PNG", mimeType: "image/png", image: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")},
		{name: "WebP", mimeType: "image/webp", image: []byte("RIFF\x04\x00\x00\x00WEBPVP8 ")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeFoodAnalyzer{imageResult: `{"items":[]}`}
			res := performAnalyzeRequest(NewNutritionHandler(fake), imageAnalyzeBody(tt.mimeType, tt.image))

			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", res.Code, res.Body.String())
			}
			if fake.imageCalls != 1 {
				t.Fatalf("image calls = %d, want 1", fake.imageCalls)
			}
			if fake.mimeType != tt.mimeType {
				t.Errorf("mime type = %q, want %q", fake.mimeType, tt.mimeType)
			}
		})
	}
}

func TestNutritionAnalyzeValidation(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	tests := []struct {
		name       string
		handler    *NutritionHandler
		body       string
		wantStatus int
		wantError  string
	}{
		{name: "missing analyzer", handler: NewNutritionHandler(nil), body: `{"type":"text","text":"apple"}`, wantStatus: http.StatusServiceUnavailable, wantError: "analyzer not configured"},
		{name: "malformed JSON", handler: NewNutritionHandler(&fakeFoodAnalyzer{}), body: `{`, wantStatus: http.StatusBadRequest, wantError: "invalid request body"},
		{name: "blank text", handler: NewNutritionHandler(&fakeFoodAnalyzer{}), body: `{"type":"text","text":"  "}`, wantStatus: http.StatusBadRequest, wantError: "text is required"},
		{name: "invalid base64", handler: NewNutritionHandler(&fakeFoodAnalyzer{}), body: `{"type":"image","data":"%%%"}`, wantStatus: http.StatusBadRequest, wantError: "invalid image data"},
		{name: "empty image", handler: NewNutritionHandler(&fakeFoodAnalyzer{}), body: `{"type":"image","data":""}`, wantStatus: http.StatusBadRequest, wantError: "invalid image data"},
		{name: "unsupported declared MIME", handler: NewNutritionHandler(&fakeFoodAnalyzer{}), body: imageAnalyzeBody("image/gif", png), wantStatus: http.StatusUnsupportedMediaType, wantError: "unsupported image MIME type; use JPEG, PNG, or WebP"},
		{name: "unsupported content", handler: NewNutritionHandler(&fakeFoodAnalyzer{}), body: imageAnalyzeBody("", []byte("not an image")), wantStatus: http.StatusUnsupportedMediaType, wantError: "unsupported image content; use JPEG, PNG, or WebP"},
		{name: "MIME does not match content", handler: NewNutritionHandler(&fakeFoodAnalyzer{}), body: imageAnalyzeBody("image/jpeg", png), wantStatus: http.StatusUnsupportedMediaType, wantError: "image MIME type does not match its content"},
		{name: "unknown type", handler: NewNutritionHandler(&fakeFoodAnalyzer{}), body: `{"type":"audio"}`, wantStatus: http.StatusBadRequest, wantError: `type must be "text" or "image"`},
		{name: "unknown scan type", handler: NewNutritionHandler(&fakeFoodAnalyzer{}), body: `{"type":"text","text":"apple","scanType":"barcode"}`, wantStatus: http.StatusBadRequest, wantError: "scanType must be food, product, or ingredient"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := performAnalyzeRequest(tt.handler, tt.body)
			if res.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.Code, tt.wantStatus)
			}
			var payload map[string]string
			if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if payload["error"] != tt.wantError {
				t.Errorf("error = %q, want %q", payload["error"], tt.wantError)
			}
		})
	}
}

func TestNutritionAnalyzeRequestBodyLimit(t *testing.T) {
	fake := &fakeFoodAnalyzer{imageResult: `{"items":[]}`}
	body := `{"type":"image","mimeType":"image/png","data":"` + strings.Repeat("A", maxAnalyzeRequestBodyBytes) + `"}`
	res := performAnalyzeRequest(NewNutritionHandler(fake), body)

	assertAnalyzeError(t, res, http.StatusRequestEntityTooLarge, "request body exceeds 9 MiB limit")
	if fake.imageCalls != 0 {
		t.Errorf("image calls = %d, want 0", fake.imageCalls)
	}
}

func TestNutritionAnalyzeDecodedImageSizeLimit(t *testing.T) {
	t.Run("accepts image at limit", func(t *testing.T) {
		image := make([]byte, maxAnalyzeImageBytes)
		copy(image, []byte("\x89PNG\r\n\x1a\n"))
		fake := &fakeFoodAnalyzer{imageResult: `{"items":[]}`}
		res := performAnalyzeRequest(NewNutritionHandler(fake), imageAnalyzeBody("image/png", image))

		if res.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", res.Code, res.Body.String())
		}
		if fake.imageCalls != 1 {
			t.Errorf("image calls = %d, want 1", fake.imageCalls)
		}
	})

	t.Run("rejects image over limit", func(t *testing.T) {
		image := make([]byte, maxAnalyzeImageBytes+1)
		copy(image, []byte("\x89PNG\r\n\x1a\n"))
		fake := &fakeFoodAnalyzer{imageResult: `{"items":[]}`}
		res := performAnalyzeRequest(NewNutritionHandler(fake), imageAnalyzeBody("image/png", image))

		assertAnalyzeError(t, res, http.StatusRequestEntityTooLarge, "image exceeds 6 MiB limit")
		if fake.imageCalls != 0 {
			t.Errorf("image calls = %d, want 0", fake.imageCalls)
		}
	})
}

func TestNutritionAnalyzeDependencyFailures(t *testing.T) {
	tests := []struct {
		name      string
		analyzer  *fakeFoodAnalyzer
		wantError string
	}{
		{name: "provider error", analyzer: &fakeFoodAnalyzer{err: errors.New("provider unavailable")}, wantError: "analysis failed"},
		{name: "invalid provider JSON", analyzer: &fakeFoodAnalyzer{textResult: "not-json"}, wantError: "could not parse analysis result"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := performAnalyzeRequest(NewNutritionHandler(tt.analyzer), `{"type":"text","text":"apple"}`)
			if res.Code != http.StatusBadGateway {
				t.Errorf("status = %d, want %d", res.Code, http.StatusBadGateway)
			}
			var payload map[string]string
			if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if payload["error"] != tt.wantError {
				t.Errorf("error = %q, want %q", payload["error"], tt.wantError)
			}
		})
	}
}

func TestNutritionAnalyzeReturnsEmptyArrayForMissingItems(t *testing.T) {
	fake := &fakeFoodAnalyzer{textResult: `{}`}
	res := performAnalyzeRequest(NewNutritionHandler(fake), `{"type":"text","text":"apple"}`)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if strings.TrimSpace(res.Body.String()) != "[]" {
		t.Errorf("body = %q, want []", res.Body.String())
	}
}

func performAnalyzeRequest(h *NutritionHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nutrition/analyze", strings.NewReader(body))
	res := httptest.NewRecorder()
	h.Analyze(res, req)
	return res
}

func imageAnalyzeBody(mimeType string, image []byte) string {
	body, err := json.Marshal(analyzeRequest{
		Type:     "image",
		MimeType: mimeType,
		Data:     base64.StdEncoding.EncodeToString(image),
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func assertAnalyzeError(t *testing.T, res *httptest.ResponseRecorder, wantStatus int, wantError string) {
	t.Helper()
	if res.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", res.Code, wantStatus, res.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload["error"] != wantError {
		t.Errorf("error = %q, want %q", payload["error"], wantError)
	}
}
