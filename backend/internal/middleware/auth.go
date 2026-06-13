package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/example/food-app/backend/internal/logging"
)

type contextKey string

const claimsContextKey contextKey = "claims"

// Claims is the JWT payload carried on authenticated requests.
type Claims struct {
	UserID string   `json:"uid"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

// NewToken issues a signed HS256 token for a user with the given roles.
func NewToken(secret, userID string, roles []string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// Authenticator returns middleware that validates a Bearer token and stores
// the resulting claims in the request context. Requests without a valid token
// are rejected with 401.
func Authenticator(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerToken(r)
			if raw == "" {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			claims, err := parseClaims(raw, secret)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			// Publish a pseudonymous user ID for request logs. The demo
			// token subject is an email address, so hash it; switch to the
			// raw internal ID once real user accounts exist.
			logging.SetUserID(ctx, logging.HashIdentifier(claims.UserID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuthenticator validates a Bearer token when present but lets
// unauthenticated requests through. Used by endpoints that serve both,
// such as the frontend telemetry sink, where a verified token binds
// events to the user without making authentication mandatory.
func OptionalAuthenticator(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if raw := bearerToken(r); raw != "" {
				if claims, err := parseClaims(raw, secret); err == nil {
					ctx := context.WithValue(r.Context(), claimsContextKey, claims)
					logging.SetUserID(ctx, logging.HashIdentifier(claims.UserID))
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseClaims(raw, secret string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// ClaimsFromContext retrieves the authenticated claims, if present.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsContextKey).(*Claims)
	return c, ok
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
