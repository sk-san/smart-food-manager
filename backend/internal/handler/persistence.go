package handler

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sk-san/smart-food-manager/backend/internal/middleware"
)

var errInvalidQuantity = errors.New("quantity exceeds the inventory remaining")

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON value")
		return false
	}
	return true
}

func authenticatedUserID(r *http.Request) (string, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || uuid.Validate(claims.UserID) != nil {
		return "", false
	}
	return claims.UserID, true
}

func validResourceID(raw string) bool {
	return uuid.Validate(raw) == nil
}

func parseOptionalDate(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, errors.New("dates must use YYYY-MM-DD")
	}
	return &t, nil
}

func finiteNonNegative(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return false
		}
	}
	return true
}

func finitePositive(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			return false
		}
	}
	return true
}

const maxStoredGramQuantity = 99999999.99

// validGramQuantity matches the NUMERIC(10,2) columns used for inventory,
// meal-item, and waste quantities. Rejecting values the database would round
// to zero keeps client errors from surfacing as constraint failures.
func validGramQuantity(value float64, allowZero bool) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > maxStoredGramQuantity {
		return false
	}
	if !allowZero && value == 0 {
		return false
	}
	return math.Abs(value*100-math.Round(value*100)) < 1e-7
}

func writeNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
