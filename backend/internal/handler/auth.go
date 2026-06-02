package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/example/food-app/backend/internal/middleware"
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
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	// TODO: verify credentials against the database.
	roles := []string{"user"}
	token, err := middleware.NewToken(h.secret, req.Email, roles, h.ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: token})
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
