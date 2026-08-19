package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
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

// currentUser is the account identity behind GET and PATCH /api/v1/me. The id
// and roles come from the verified token; the address and name are read from
// the users row so an edit made in another tab is visible on the next load.
type currentUser struct {
	UserID      string   `json:"user_id"`
	Roles       []string `json:"roles"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
}

// displayNameExpr resolves the name to show for an account. Rows created before
// migration 0004, and any inserted without a name since, fall back to the local
// part of the address so the account page never renders a blank identity.
const displayNameExpr = `COALESCE(NULLIF(btrim(display_name), ''), split_part(email::text, '@', 1))`

const displayNameMaxRunes = 60

// validateDisplayName trims the submitted name and rejects what the account
// page could not render as a single line of text.
func validateDisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("display name is required")
	}
	if utf8.RuneCountInString(name) > displayNameMaxRunes {
		return "", fmt.Errorf("display name must be %d characters or fewer", displayNameMaxRunes)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", errors.New("display name must not contain control characters")
		}
	}
	return name, nil
}

// accountFromClaims rejects a token whose subject could never match a users
// row, so a malformed id fails as unauthorized rather than as a query error.
func accountFromClaims(r *http.Request) (currentUser, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || uuid.Validate(claims.UserID) != nil {
		return currentUser{}, false
	}
	return currentUser{UserID: claims.UserID, Roles: claims.Roles}, true
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := accountFromClaims(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	err := h.db.QueryRow(r.Context(), `
		SELECT email, `+displayNameExpr+`
		FROM users
		WHERE id = $1 AND is_active`, user.UserID).Scan(&user.Email, &user.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		// The token is well-formed but its account is gone or deactivated.
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load account")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

type updateAccountRequest struct {
	DisplayName string `json:"display_name"`
}

// UpdateMe saves the display name the user typed on the account page. It is the
// only account field the API lets a caller change.
func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	user, ok := accountFromClaims(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req updateAccountRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name, err := validateDisplayName(req.DisplayName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	err = h.db.QueryRow(r.Context(), `
		UPDATE users
		SET display_name = $2, updated_at = now()
		WHERE id = $1 AND is_active
		RETURNING email, `+displayNameExpr, user.UserID, name).Scan(&user.Email, &user.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save display name")
		return
	}
	writeJSON(w, http.StatusOK, user)
}
