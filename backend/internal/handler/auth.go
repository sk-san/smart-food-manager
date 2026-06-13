package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/example/food-app/backend/internal/logging"
	"github.com/example/food-app/backend/internal/middleware"
	"github.com/example/food-app/backend/internal/telemetry"
)

type AuthHandler struct {
	secret string
	ttl    time.Duration
}

func NewAuthHandler(secret string, ttl time.Duration) *AuthHandler {
	return &AuthHandler{secret: secret, ttl: ttl}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// Login is a DEMO stub. It issues a token for any non-empty email so the
// scaffold runs end to end. Replace with a real lookup against the users table
// plus password-hash verification before shipping.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		logLoginAttempt(ctx, false, "invalid_request", time.Since(start))
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	// TODO: verify credentials against the database.
	roles := []string{"user"}
	token, err := middleware.NewToken(h.secret, req.Email, roles, h.ttl)
	if err != nil {
		logLoginAttempt(ctx, false, "internal_error", time.Since(start))
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	// The demo subject is an email address: hash it so no PII reaches the logs.
	logging.SetUserID(ctx, logging.HashIdentifier(req.Email))
	logLoginAttempt(ctx, true, "", time.Since(start))
	writeJSON(w, http.StatusOK, loginResponse{Token: token})
}

// logLoginAttempt emits the blueprint auth events and login metric.
// Failure logs never reveal whether the account exists or which part of
// the credentials was wrong.
func logLoginAttempt(ctx context.Context, ok bool, reason string, d time.Duration) {
	e := logging.Event{
		Severity: logging.LevelInfo,
		Message:  "User login completed",
		Name:     logging.EventAuthLoginCompleted,
		Category: logging.CategoryAuth,
		Action:   "login",
		Outcome:  logging.OutcomeSuccess,
		Duration: d,
	}
	if !ok {
		e.Severity = logging.LevelWarning
		e.Message = "User login failed"
		e.Name = logging.EventAuthLoginFailed
		e.Outcome = logging.OutcomeFailure
		e.Attrs = append(e.Attrs, slog.String("auth.reason", reason))
	}
	logging.Emit(ctx, e)
	telemetry.AuthLoginAttemptsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("event.outcome", e.Outcome),
	))
}

// Me returns the authenticated caller's claims.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": claims.UserID,
		"roles":   claims.Roles,
	})
}
