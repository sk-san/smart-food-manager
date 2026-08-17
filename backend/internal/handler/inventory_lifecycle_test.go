package handler

import (
	"math"
	"testing"
	"time"
)

func TestValidateInventoryScanAndNormalizeNutrition(t *testing.T) {
	defaultTime := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	req := inventoryScanRequest{
		SourceType: " Product ", Name: " Protein bar ", Category: " snack ",
		Quantity: 50, ExpiryDate: "2026-09-01",
		Nutrients: nutritionAmounts{Calories: 200, Protein: 10, Sodium: 150},
	}
	validated, err := validateInventoryScan(&req, defaultTime)
	if err != nil {
		t.Fatalf("validateInventoryScan: %v", err)
	}
	if req.SourceType != "product" || req.Name != "Protein bar" || req.Category != "snack" {
		t.Errorf("normalized request = %+v", req)
	}
	if req.Storage != "other" || req.Package != "unopened" || req.DateLabel != "best_before" {
		t.Errorf("defaults = %+v", req)
	}
	if validated.expiry == nil || validated.expiry.Format("2006-01-02") != "2026-09-01" || !validated.consumedAt.Equal(defaultTime) {
		t.Errorf("validated = %+v", validated)
	}

	per100g := nutritionScale(req.Nutrients, 100/req.Quantity)
	if per100g.Calories != 400 || per100g.Protein != 20 || per100g.Sodium != 300 {
		t.Errorf("per 100g = %+v", per100g)
	}
}

func TestValidateInventoryScanRejectsInvalidValues(t *testing.T) {
	valid := inventoryScanRequest{
		SourceType: "ingredient", Name: "Spinach", Quantity: 100,
	}
	tests := []struct {
		name   string
		mutate func(*inventoryScanRequest)
	}{
		{name: "source", mutate: func(req *inventoryScanRequest) { req.SourceType = "barcode" }},
		{name: "name", mutate: func(req *inventoryScanRequest) { req.Name = "" }},
		{name: "quantity", mutate: func(req *inventoryScanRequest) { req.Quantity = 0 }},
		{name: "quantity precision", mutate: func(req *inventoryScanRequest) { req.Quantity = 0.001 }},
		{name: "nutrient", mutate: func(req *inventoryScanRequest) { req.Nutrients.Calories = -1 }},
		{name: "nan nutrient", mutate: func(req *inventoryScanRequest) { req.Nutrients.Protein = math.NaN() }},
		{name: "date", mutate: func(req *inventoryScanRequest) { req.ExpiryDate = "tomorrow" }},
		{name: "consumed at", mutate: func(req *inventoryScanRequest) { req.ConsumedAt = "lunchtime" }},
		{name: "storage", mutate: func(req *inventoryScanRequest) { req.Storage = "counter" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			tt.mutate(&req)
			if _, err := validateInventoryScan(&req, time.Now()); err == nil {
				t.Errorf("invalid request accepted: %+v", req)
			}
		})
	}
}

func TestValidateConsumeInventory(t *testing.T) {
	valid := []consumeInventoryRequest{
		{Quantity: 25},
		{DiscardRemaining: true, WasteReason: "leftover_not_eaten"},
		{Quantity: 10, DiscardRemaining: true, WasteReason: "other"},
	}
	for _, req := range valid {
		if err := validateConsumeInventory(req); err != nil {
			t.Errorf("valid request %+v: %v", req, err)
		}
	}
	invalid := []consumeInventoryRequest{
		{},
		{Quantity: -1},
		{Quantity: math.NaN()},
		{Quantity: 0.001},
		{DiscardRemaining: true, WasteReason: "not-a-reason"},
	}
	for _, req := range invalid {
		if err := validateConsumeInventory(req); err == nil {
			t.Errorf("invalid request accepted: %+v", req)
		}
	}
}

func TestDateStatusAt(t *testing.T) {
	now := time.Date(2026, 8, 16, 23, 30, 0, 0, time.FixedZone("test", 2*60*60))
	tests := []struct {
		expiry string
		want   string
	}{
		{expiry: "", want: "unknown"},
		{expiry: "2026-08-17", want: "before_date"},
		{expiry: "2026-08-16", want: "on_date"},
		{expiry: "2026-08-15", want: "1_3_days_after"},
		{expiry: "2026-08-10", want: "4_7_days_after"},
		{expiry: "2026-08-05", want: "8_14_days_after"},
		{expiry: "2026-07-01", want: "15_plus_days_after"},
	}
	for _, tt := range tests {
		expiry, err := parseOptionalDate(tt.expiry)
		if err != nil {
			t.Fatal(err)
		}
		if got := dateStatusAt(now, expiry); got != tt.want {
			t.Errorf("dateStatusAt(%s) = %q, want %q", tt.expiry, got, tt.want)
		}
	}
}

func TestIsExpiredOn(t *testing.T) {
	today, err := time.Parse("2006-01-02", "2026-08-16")
	if err != nil {
		t.Fatal(err)
	}
	yesterday, _ := time.Parse("2006-01-02", "2026-08-15")
	tomorrow, _ := time.Parse("2006-01-02", "2026-08-17")
	if !isExpiredOn(&yesterday, today) {
		t.Error("yesterday should be expired")
	}
	if isExpiredOn(&today, today) || isExpiredOn(&tomorrow, today) || isExpiredOn(nil, today) {
		t.Error("today, future, and missing expiry must remain available")
	}
}
