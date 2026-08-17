package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testJWTSecret = "unit-test-secret"

func TestAuthenticator(t *testing.T) {
	token, err := NewToken(testJWTSecret, "user-123", []string{"user", "admin"}, time.Minute)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		claims, ok := ClaimsFromContext(r.Context())
		if !ok {
			t.Fatal("authenticated request has no claims")
		}
		if claims.UserID != "user-123" {
			t.Errorf("UserID = %q, want user-123", claims.UserID)
		}
		if len(claims.Roles) != 2 || claims.Roles[1] != "admin" {
			t.Errorf("Roles = %v, want [user admin]", claims.Roles)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	Authenticator(testJWTSecret)(next).ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusNoContent)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
}

func TestAuthenticatorRejectsInvalidCredentials(t *testing.T) {
	expired, err := NewToken(testJWTSecret, "user-123", nil, -time.Minute)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	wrongSecret, err := NewToken("different-secret", "user-123", nil, time.Minute)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}

	tests := []struct {
		name       string
		authHeader string
		wantBody   string
	}{
		{name: "missing header", wantBody: "missing bearer token"},
		{name: "malformed header", authHeader: "Basic abc", wantBody: "missing bearer token"},
		{name: "empty bearer value", authHeader: "Bearer   ", wantBody: "missing bearer token"},
		{name: "garbage token", authHeader: "Bearer not-a-jwt", wantBody: "invalid token"},
		{name: "wrong signature", authHeader: "Bearer " + wrongSecret, wantBody: "invalid token"},
		{name: "expired token", authHeader: "Bearer " + expired, wantBody: "invalid token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			res := httptest.NewRecorder()

			Authenticator(testJWTSecret)(next).ServeHTTP(res, req)

			if res.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", res.Code, http.StatusUnauthorized)
			}
			if !strings.Contains(res.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", res.Body.String(), tt.wantBody)
			}
			if called {
				t.Error("next handler was called")
			}
		})
	}
}

func TestOptionalAuthenticator(t *testing.T) {
	validToken, err := NewToken(testJWTSecret, "optional-user", []string{"user"}, time.Minute)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}

	tests := []struct {
		name       string
		authHeader string
		wantClaims bool
	}{
		{name: "anonymous request"},
		{name: "invalid token", authHeader: "Bearer invalid"},
		{name: "valid token", authHeader: "Bearer " + validToken, wantClaims: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				claims, ok := ClaimsFromContext(r.Context())
				if ok != tt.wantClaims {
					t.Errorf("claims present = %v, want %v", ok, tt.wantClaims)
				}
				if ok && claims.UserID != "optional-user" {
					t.Errorf("UserID = %q, want optional-user", claims.UserID)
				}
				w.WriteHeader(http.StatusAccepted)
			})
			req := httptest.NewRequest(http.MethodPost, "/optional", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			res := httptest.NewRecorder()

			OptionalAuthenticator(testJWTSecret)(next).ServeHTTP(res, req)

			if res.Code != http.StatusAccepted {
				t.Errorf("status = %d, want %d", res.Code, http.StatusAccepted)
			}
			if !nextCalled {
				t.Error("next handler was not called")
			}
		})
	}
}

func TestRequireRole(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{name: "unauthenticated", wantStatus: http.StatusUnauthorized},
		{name: "missing role", authHeader: tokenForRoles(t, "user"), wantStatus: http.StatusForbidden},
		{name: "matching role", authHeader: tokenForRoles(t, "user", "admin"), wantStatus: http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			handler := RequireRole("admin")(next)
			if tt.authHeader != "" {
				handler = Authenticator(testJWTSecret)(handler)
			}

			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			if res.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.Code, tt.wantStatus)
			}
		})
	}
}

func tokenForRoles(t *testing.T, roles ...string) string {
	t.Helper()
	token, err := NewToken(testJWTSecret, "user-123", roles, time.Minute)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	return "Bearer " + token
}
