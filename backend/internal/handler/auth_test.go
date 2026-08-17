package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/sk-san/smart-food-manager/backend/internal/middleware"
)

type fakeAuthDB struct {
	row       pgx.Row
	queryRuns int
	query     string
	args      []any
}

func (db *fakeAuthDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	db.queryRuns++
	db.query = query
	db.args = append([]any(nil), args...)
	return db.row
}

type fakeRow struct {
	scan func(dest ...any) error
}

func (r fakeRow) Scan(dest ...any) error {
	return r.scan(dest...)
}

func TestValidateLoginInput(t *testing.T) {
	tests := []struct {
		name      string
		req       loginRequest
		wantEmail string
		wantError string
	}{
		{name: "normalizes valid email", req: loginRequest{Email: "  USER@Example.COM ", Password: "password123"}, wantEmail: "user@example.com"},
		{name: "missing email", req: loginRequest{Password: "password123"}, wantError: "email and password are required"},
		{name: "missing password", req: loginRequest{Email: "user@example.com"}, wantError: "email and password are required"},
		{name: "display name rejected", req: loginRequest{Email: "User <user@example.com>", Password: "password123"}, wantError: "invalid input format"},
		{name: "invalid address", req: loginRequest{Email: "not-an-email", Password: "password123"}, wantError: "invalid input format"},
		{name: "email too long", req: loginRequest{Email: strings.Repeat("a", 245) + "@example.com", Password: "password123"}, wantError: "invalid input format"},
		{name: "password too short", req: loginRequest{Email: "user@example.com", Password: "short"}, wantError: "invalid input format"},
		{name: "password over bcrypt limit", req: loginRequest{Email: "user@example.com", Password: strings.Repeat("x", 73)}, wantError: "invalid input format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req
			err := validateLoginInput(&req)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("validateLoginInput: %v", err)
				}
				if req.Email != tt.wantEmail {
					t.Errorf("email = %q, want %q", req.Email, tt.wantEmail)
				}
				return
			}
			if err == nil || err.Error() != tt.wantError {
				t.Errorf("error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestLoginIssuesTokenForVerifiedUser(t *testing.T) {
	const password = "correct-horse"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	db := &fakeAuthDB{row: fakeRow{scan: func(dest ...any) error {
		*(dest[0].(*string)) = "user-123"
		*(dest[1].(*string)) = string(hash)
		*(dest[2].(*[]string)) = []string{"user"}
		return nil
	}}}
	h := NewAuthHandler(db, "login-test-secret", time.Minute)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":" USER@Example.COM ","password":"correct-horse"}`))
	res := httptest.NewRecorder()

	h.Login(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", res.Code, res.Body.String())
	}
	if db.queryRuns != 1 {
		t.Fatalf("query runs = %d, want 1", db.queryRuns)
	}
	if len(db.args) != 1 || db.args[0] != "user@example.com" {
		t.Errorf("query args = %v, want normalized email", db.args)
	}
	if !strings.Contains(db.query, "WHERE u.email = $1 AND u.is_active") {
		t.Errorf("query does not constrain email and active status: %s", db.query)
	}

	var response loginResponse
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if response.Token == "" {
		t.Fatal("login returned an empty token")
	}

	claimsChecked := false
	protected := middleware.Authenticator("login-test-secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			t.Fatal("token did not produce claims")
		}
		claimsChecked = true
		if claims.UserID != "user-123" || len(claims.Roles) != 1 || claims.Roles[0] != "user" {
			t.Errorf("claims = %+v", claims)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	protectedReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	protectedReq.Header.Set("Authorization", "Bearer "+response.Token)
	protectedRes := httptest.NewRecorder()
	protected.ServeHTTP(protectedRes, protectedReq)
	if protectedRes.Code != http.StatusNoContent || !claimsChecked {
		t.Errorf("token verification status = %d, claimsChecked = %v", protectedRes.Code, claimsChecked)
	}
}

func TestLoginRejectsInvalidRequestsBeforeQuerying(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{`},
		{name: "missing credentials", body: `{}`},
		{name: "invalid email", body: `{"email":"invalid","password":"password123"}`},
		{name: "short password", body: `{"email":"user@example.com","password":"short"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeAuthDB{}
			h := NewAuthHandler(db, "secret", time.Minute)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(tt.body))
			res := httptest.NewRecorder()

			h.Login(res, req)

			if res.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", res.Code)
			}
			if db.queryRuns != 0 {
				t.Errorf("query runs = %d, want 0", db.queryRuns)
			}
		})
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	validHash, err := bcrypt.GenerateFromPassword([]byte("different-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	tests := []struct {
		name string
		row  pgx.Row
	}{
		{name: "user not found", row: fakeRow{scan: func(...any) error { return pgx.ErrNoRows }}},
		{name: "wrong password", row: fakeRow{scan: func(dest ...any) error {
			*(dest[0].(*string)) = "user-123"
			*(dest[1].(*string)) = string(validHash)
			*(dest[2].(*[]string)) = []string{"user"}
			return nil
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeAuthDB{row: tt.row}
			h := NewAuthHandler(db, "secret", time.Minute)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
				strings.NewReader(`{"email":"user@example.com","password":"password123"}`))
			res := httptest.NewRecorder()

			h.Login(res, req)

			if res.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", res.Code)
			}
			var payload map[string]string
			if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if payload["error"] != "invalid email or password" {
				t.Errorf("error = %q", payload["error"])
			}
		})
	}
}

func TestMe(t *testing.T) {
	h := NewAuthHandler(nil, "me-test-secret", time.Minute)

	t.Run("rejects missing claims", func(t *testing.T) {
		res := httptest.NewRecorder()
		h.Me(res, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
		if res.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", res.Code)
		}
	})

	t.Run("returns authenticated claims", func(t *testing.T) {
		token, err := middleware.NewToken("me-test-secret", "user-456", []string{"user"}, time.Minute)
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		middleware.Authenticator("me-test-secret")(http.HandlerFunc(h.Me)).ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.Code)
		}
		var payload struct {
			UserID string   `json:"user_id"`
			Roles  []string `json:"roles"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if payload.UserID != "user-456" || len(payload.Roles) != 1 || payload.Roles[0] != "user" {
			t.Errorf("payload = %+v", payload)
		}
	})
}
