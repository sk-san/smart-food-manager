package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/example/food-app/backend/internal/middleware"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type AuthHandler struct {
	db     DB // Connection to Postgres
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

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	var dbPasswordHash string
	var roles []string

	// 1. Look up the user by email
	query := "SELECT password_hash, roles FROM users WHERE email = $1"
	err := h.db.QueryRow(r.Context(), query, req.Email).Scan(&dbPasswordHash, &roles)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// 2. Safely compare the typed password with the encrypted database hash
	err = bcrypt.CompareHashAndPassword([]byte(dbPasswordHash), []byte(req.Password))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// 3. Success! Generate the real JWT token
	token, err := middleware.NewToken(h.secret, req.Email, roles, h.ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: token})
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
