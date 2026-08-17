package middleware

import (
	"net/http"
	"regexp"

	"github.com/sk-san/smart-food-manager/backend/internal/logging"
)

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RequestContext seeds the request context for structured logging: it
// captures the browser session ID supplied by the frontend (X-Session-Id)
// and installs the holder through which auth middleware and handlers
// publish the user ID to the request logger.
func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := logging.WithUserHolder(r.Context())
		if sid := r.Header.Get("X-Session-Id"); sessionIDPattern.MatchString(sid) {
			ctx = logging.WithSessionID(ctx, sid)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
