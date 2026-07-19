package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/crypto/bcrypt"

	"github.com/example/food-app/backend/internal/logging"
	"github.com/example/food-app/backend/internal/middleware"
	"github.com/example/food-app/backend/internal/telemetry"
)

// DB is the subset of *pgxpool.Pool the auth handler needs. Depending on the
// interface (rather than the concrete pool) keeps this handler unit-testable
// with a fake.
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

// Login verifies the caller's email and password against the users table
// (bcrypt-hashed) and, on success, issues a JWT carrying the user's id and
// roles.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		logLoginAttempt(ctx, false, "invalid_request", time.Since(start))
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	// Hash the email for logs before we know whether the account exists, so
	// every branch below (found or not) logs against the same identifier.
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
		// Same response whether the account doesn't exist or the row can't be
		// read, so the caller can't use timing/response differences to probe
		// which accounts are registered.
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
