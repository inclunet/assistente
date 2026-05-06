package database

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

type userIDContextKey struct{}

var ErrUserScopeRequired = errors.New("usuário autenticado obrigatório")

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, strings.TrimSpace(userID))
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	userID, ok := ctx.Value(userIDContextKey{}).(string)
	userID = strings.TrimSpace(userID)
	return userID, ok && userID != ""
}

func RequireUserID(ctx context.Context) (string, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return "", ErrUserScopeRequired
	}
	return userID, nil
}

func ScopeByUser(ctx context.Context, query *gorm.DB, column string) *gorm.DB {
	if query == nil {
		return query
	}
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return query
	}
	if strings.TrimSpace(column) == "" {
		column = "user_id"
	}
	return query.Where(column+" = ?", userID)
}
