package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/crypto/bcrypt"

	"github.com/sk-san/smart-food-manager/backend/internal/logging"
	"github.com/sk-san/smart-food-manager/backend/internal/middleware"
	"github.com/sk-san/smart-food-manager/backend/internal/telemetry"
)

type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type AuthHandler struct {
	db     DB
	secret string
	ttl    time.Duration
}

func NewAuthHandler(db DB, secret string, ttl time.Duration) *AuthHandler {
	return &AuthHandler{db: db, secret: secret, ttl: ttl}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// validateLoginInput validates and sanitizes input data.
// Uses a pointer receiver so that strings.TrimSpace modifies the original struct in-place.
func validateLoginInput(req *loginRequest) error {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Email == "" || req.Password == "" {
		return errors.New("email and password are required")
	}

	// Email length and RFC format check (ensures pure email format without display names)
	if len(req.Email) > 254 {
		return errors.New("invalid input format")
	}
	addr, err := mail.ParseAddress(req.Email)
	if err != nil || addr.Address != req.Email {
		return errors.New("invalid input format")
	}

	// Password length check (8+ chars for security, <= 72 bytes for bcrypt DoS prevention)
	if len(req.Password) < 8 || len(req.Password) > 72 {
		return errors.New("invalid input format")
	}

	return nil
}

// Login verifies the caller's email and password against the users table.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logLoginAttempt(ctx, false, "invalid_request_body", time.Since(start))
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateLoginInput(&req); err != nil {
		logLoginAttempt(ctx, false, "invalid_validation", time.Since(start))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	logging.SetUserID(ctx, logging.HashIdentifier(req.Email))

	var userID, passwordHash string
	var roles []string
	row := h.db.QueryRow(ctx, `
		SELECT u.id, u.password_hash,
		       COALESCE(array_agg(r.name) FILTER (WHERE r.name IS NOT NULL), '{}')
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r       ON r.id = ur.role_id
		WHERE u.email = $1 AND u.is_active
		GROUP BY u.id, u.password_hash`, req.Email)
	if err := row.Scan(&userID, &passwordHash, &roles); err != nil {
		logLoginAttempt(ctx, false, "invalid_credentials", time.Since(start))
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		logLoginAttempt(ctx, false, "invalid_credentials", time.Since(start))
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := middleware.NewToken(h.secret, userID, roles, h.ttl)
	if err != nil {
		logLoginAttempt(ctx, false, "internal_error", time.Since(start))
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	logLoginAttempt(ctx, true, "", time.Since(start))
	writeJSON(w, http.StatusOK, loginResponse{Token: token})
}

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
