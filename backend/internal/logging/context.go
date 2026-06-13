package logging

import "context"

type ctxKey int

const (
	sessionIDKey ctxKey = iota
	userHolderKey
)

// WithSessionID returns a context carrying the browser session identifier
// supplied by the frontend (X-Session-Id header).
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey, id)
}

// SessionIDFrom returns the session identifier, if any.
func SessionIDFrom(ctx context.Context) string {
	s, _ := ctx.Value(sessionIDKey).(string)
	return s
}

// userHolder lets inner middleware and handlers publish the authenticated
// user ID to the request logger wrapped around them: the holder is installed
// before routing and filled in once authentication resolves. Request-scoped
// and not goroutine-safe.
type userHolder struct{ id string }

// WithUserHolder installs an empty user holder; SetUserID fills it later.
func WithUserHolder(ctx context.Context) context.Context {
	return context.WithValue(ctx, userHolderKey, &userHolder{})
}

// SetUserID records the loggable user identifier for the current request.
// Pass an internal user ID, or a HashIdentifier value when only PII (such
// as an email address) is available. No-op when no holder is installed.
func SetUserID(ctx context.Context, id string) {
	if h, ok := ctx.Value(userHolderKey).(*userHolder); ok {
		h.id = id
	}
}

// UserIDFrom returns the user identifier recorded for this request.
func UserIDFrom(ctx context.Context) string {
	if h, ok := ctx.Value(userHolderKey).(*userHolder); ok {
		return h.id
	}
	return ""
}
