package acptrust

import (
	"assistente/internal/database"
	"context"
)

func (m *Store) SessionRules(ctx context.Context) ([]database.AuthorizationRule, error) {
	return m.core.SessionRules(ctx)
}
func (m *Store) RevokeSession(ctx context.Context, id string) error {
	return m.core.RevokeSession(ctx, id)
}
