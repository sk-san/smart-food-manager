package handler

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sk-san/smart-food-manager/backend/internal/middleware"
)

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantOK     bool
		wantStatus int
	}{
		{name: "valid", body: `{"name":"apple"}`, wantOK: true, wantStatus: http.StatusOK},
		{name: "malformed", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"name":"apple","extra":true}`, wantStatus: http.StatusBadRequest},
		{name: "multiple values", body: `{"name":"apple"} {"name":"pear"}`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload struct {
				Name string `json:"name"`
			}
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			res := httptest.NewRecorder()
			ok := decodeJSON(res, req, &payload)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if res.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.Code, tt.wantStatus)
			}
			if ok && payload.Name != "apple" {
				t.Errorf("name = %q", payload.Name)
			}
		})
	}
}

func TestAuthenticatedUserID(t *testing.T) {
	const secret = "persistence-test-secret"
	tests := []struct {
		name     string
		userID   string
		withAuth bool
		wantOK   bool
	}{
		{name: "missing claims"},
		{name: "non-UUID subject", userID: "user-123", withAuth: true},
		{name: "UUID subject", userID: "3f0df8a9-36d2-4d74-9e6e-9d8ce08440cd", withAuth: true, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checked := false
			inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				checked = true
				got, ok := authenticatedUserID(r)
				if ok != tt.wantOK {
					t.Errorf("ok = %v, want %v", ok, tt.wantOK)
				}
				if ok && got != tt.userID {
					t.Errorf("user ID = %q, want %q", got, tt.userID)
				}
			})
			var handler http.Handler = inner
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.withAuth {
				token, err := middleware.NewToken(secret, tt.userID, nil, time.Minute)
				if err != nil {
					t.Fatalf("NewToken: %v", err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
				handler = middleware.Authenticator(secret)(inner)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if !checked {
				t.Fatal("inner handler was not called")
			}
		})
	}
}

func TestPersistenceHelpers(t *testing.T) {
	const validID = "3f0df8a9-36d2-4d74-9e6e-9d8ce08440cd"
	if !validResourceID(validID) || validResourceID("not-a-uuid") {
		t.Error("validResourceID did not distinguish UUID and arbitrary text")
	}

	if date, err := parseOptionalDate("  "); err != nil || date != nil {
		t.Errorf("empty date = %v, %v; want nil, nil", date, err)
	}
	date, err := parseOptionalDate("2026-08-12")
	if err != nil || date == nil || date.Format("2006-01-02") != "2026-08-12" {
		t.Errorf("valid date = %v, %v", date, err)
	}
	if _, err := parseOptionalDate("12/08/2026"); err == nil {
		t.Error("invalid date was accepted")
	}

	if !finiteNonNegative(0, 1.5) || finiteNonNegative(-1) || finiteNonNegative(math.NaN()) || finiteNonNegative(math.Inf(1)) {
		t.Error("finiteNonNegative returned an unexpected result")
	}
	if !finitePositive(0.1, 5) || finitePositive(0) || finitePositive(-1) || finitePositive(math.NaN()) || finitePositive(math.Inf(-1)) {
		t.Error("finitePositive returned an unexpected result")
	}
	if !validGramQuantity(0, true) || !validGramQuantity(0.01, false) ||
		!validGramQuantity(123.45, false) || validGramQuantity(0, false) ||
		validGramQuantity(0.001, false) || validGramQuantity(maxStoredGramQuantity+0.01, false) {
		t.Error("validGramQuantity returned an unexpected result")
	}

	res := httptest.NewRecorder()
	writeNoContent(res)
	if res.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", res.Code)
	}
}

func TestValidateMeal(t *testing.T) {
	defaultTime := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	req := mealRequest{Name: "  Lunch  ", Calories: 500, Protein: 30, Carbs: 50, Fat: 15}
	got, err := validateMeal(&req, defaultTime)
	if err != nil || !got.Equal(defaultTime) || req.Name != "Lunch" {
		t.Fatalf("valid default-time meal = %v, %v, %+v", got, err, req)
	}

	req.ConsumedAt = "2026-08-12T18:30:00Z"
	got, err = validateMeal(&req, defaultTime)
	if err != nil || got.Hour() != 18 {
		t.Errorf("explicit consumed time = %v, %v", got, err)
	}

	invalid := []mealRequest{
		{Name: ""},
		{Name: strings.Repeat("x", 201)},
		{Name: "Meal", Calories: -1},
		{Name: "Meal", Calories: math.NaN()},
		{Name: "Meal", ConsumedAt: "not-a-timestamp"},
	}
	for i := range invalid {
		if _, err := validateMeal(&invalid[i], defaultTime); err == nil {
			t.Errorf("invalid meal %d was accepted: %+v", i, invalid[i])
		}
	}

	hours := map[int]string{0: "snack", 5: "breakfast", 10: "breakfast", 11: "lunch", 15: "lunch", 16: "dinner", 21: "dinner", 22: "snack"}
	for hour, want := range hours {
		if got := mealTypeAt(time.Date(2026, 8, 12, hour, 0, 0, 0, time.UTC)); got != want {
			t.Errorf("mealTypeAt(%d) = %q, want %q", hour, got, want)
		}
	}

	response := responseFromMealRequest("meal-1", defaultTime, req)
	if response.ID != "meal-1" || response.Name != req.Name || response.Calories != req.Calories {
		t.Errorf("meal response = %+v", response)
	}
}

func TestValidateInventory(t *testing.T) {
	req := inventoryRequest{
		Name: "  Spinach  ", QuantityPurchased: 500, QuantityConsumed: 100,
		PurchaseDate: "2026-08-10", BestBeforeDate: "2026-08-15",
	}
	validated, err := validateInventory(&req, 50, time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("validateInventory: %v", err)
	}
	if req.Name != "Spinach" || req.Storage != "other" || req.Package != "unopened" || req.DateLabel != "unknown" {
		t.Errorf("defaults/normalization = %+v", req)
	}
	if validated.purchaseDate == nil || validated.bestBeforeDate == nil || validated.useByDate != nil {
		t.Errorf("validated dates = %+v", validated)
	}

	invalid := []inventoryRequest{
		{Name: "", QuantityPurchased: 1},
		{Name: strings.Repeat("x", 201), QuantityPurchased: 1},
		{Name: "Item", QuantityPurchased: 0},
		{Name: "Item", QuantityPurchased: 1, QuantityConsumed: -1},
		{Name: "Item", QuantityPurchased: 1, QuantityConsumed: 2},
		{Name: "Item", QuantityPurchased: 1.001},
		{Name: "Item", QuantityPurchased: 1, Storage: "counter"},
		{Name: "Item", QuantityPurchased: 1, PurchaseDate: "tomorrow"},
		{Name: "Item", QuantityPurchased: 1, BestBeforeDate: "tomorrow"},
		{Name: "Item", QuantityPurchased: 1, UseByDate: "tomorrow"},
		{Name: "Item", QuantityPurchased: 1, BestBeforeDate: "2026-08-11"},
	}
	for i := range invalid {
		if _, err := validateInventory(&invalid[i], 0, time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)); err == nil {
			t.Errorf("invalid inventory %d was accepted: %+v", i, invalid[i])
		}
	}
}

func TestValidateWaste(t *testing.T) {
	defaultTime := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		reason             string
		wantClassification string
	}{
		{reason: "expired_use_by", wantClassification: "expiry_caused"},
		{reason: "forgot_item_existed", wantClassification: "expiry_related"},
		{reason: "overbought", wantClassification: "expiry_unrelated"},
	}
	for _, tt := range tests {
		req := wasteRequest{Quantity: 25, Reason: tt.reason, Note: " note "}
		got, err := validateWaste(&req, defaultTime)
		if err != nil || !got.Equal(defaultTime) {
			t.Fatalf("validateWaste(%s) = %v, %v", tt.reason, got, err)
		}
		if req.Classification != tt.wantClassification || req.DateLabel != "unknown" ||
			req.DateStatus != "unknown" || req.Package != "unknown" || req.Spoilage != "unknown" || req.Note != "note" {
			t.Errorf("normalized waste request = %+v", req)
		}
	}

	req := wasteRequest{Quantity: 1, Reason: "other", WastedAt: "2026-08-12T09:00:00Z"}
	got, err := validateWaste(&req, defaultTime)
	if err != nil || got.Hour() != 9 {
		t.Errorf("explicit waste time = %v, %v", got, err)
	}

	invalid := []wasteRequest{
		{Quantity: 0, Reason: "other"},
		{Quantity: 0.001, Reason: "other"},
		{Quantity: 1, Reason: "invalid"},
		{Quantity: 1, Reason: "other", DateLabel: "invalid"},
		{Quantity: 1, Reason: "other", Classification: "invalid"},
		{Quantity: 1, Reason: "other", Note: strings.Repeat("x", 2001)},
		{Quantity: 1, Reason: "other", WastedAt: "not-a-timestamp"},
	}
	for i := range invalid {
		if _, err := validateWaste(&invalid[i], defaultTime); err == nil {
			encoded, _ := json.Marshal(invalid[i])
			t.Errorf("invalid waste %d was accepted: %s", i, encoded)
		}
	}
}

func TestAutomaticExpiryWasteIsImmutable(t *testing.T) {
	if !isAutomaticExpiryReason("expired_best_before") || !isAutomaticExpiryReason("expired_use_by") {
		t.Error("automatic expiration reasons must be immutable")
	}
	if isAutomaticExpiryReason("near_expiry_but_not_used") || isAutomaticExpiryReason("other") {
		t.Error("manual waste reasons must remain editable")
	}
}

func TestHandlerConstructors(t *testing.T) {
	if NewMealsHandler(nil) == nil || NewGoalsHandler(nil) == nil ||
		NewInventoryHandler(nil) == nil || NewWasteHandler(nil) == nil {
		t.Error("handler constructor returned nil")
	}
}
