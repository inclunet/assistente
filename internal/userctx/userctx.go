package userctx

import (
	"context"
	"strings"
)

type userIDContextKey struct{}

// WithUserID returns a context carrying the authenticated user identifier.
func WithUserID(ctx context.Context, userID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, userIDContextKey{}, strings.TrimSpace(userID))
}

// UserIDFromContext extracts the authenticated user identifier from ctx.
func UserIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	userID, ok := ctx.Value(userIDContextKey{}).(string)
	userID = strings.TrimSpace(userID)
	return userID, ok && userID != ""
}

// WithSessionID records the session captured together with the authenticated user.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sessionIDContextKey{}, strings.TrimSpace(sessionID))
}

type sessionIDContextKey struct{}

// SessionIDFromContext returns the original session, never the current UI session.
func SessionIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	sessionID, ok := ctx.Value(sessionIDContextKey{}).(string)
	return sessionID, ok && sessionID != ""
}
