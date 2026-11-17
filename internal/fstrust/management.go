package fstrust

import (
	"assistente/internal/database"
	"context"
)

func (m *Manager) SessionRules(ctx context.Context) ([]database.AuthorizationRule, error) {
	return m.core.SessionRules(ctx)
}
func (m *Manager) RevokeSession(ctx context.Context, id string) error {
	return m.core.RevokeSession(ctx, id)
}
