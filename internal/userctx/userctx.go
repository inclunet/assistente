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
