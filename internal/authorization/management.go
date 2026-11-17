package authorization

import (
	"context"
	"time"

	"assistente/internal/database"
)

// ManagementRules inclui revogadas para a pessoa reconhecer o resultado da
// revogação. Regras de políticas nomeadas são geridas com o próprio documento.
func (r *Repository) ManagementRules(ctx context.Context) ([]database.AuthorizationRule, error) {
	db, user, err := r.query(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]database.AuthorizationRule, 0)
	err = db.Where("user_id = ? AND (policy_id = '' OR policy_id IS NULL)", user).Order("domain, scope, scope_key, id").Find(&rows).Error
	return rows, err
}

func (r *Repository) RevokeID(ctx context.Context, id string) error {
	db, user, err := r.query(ctx)
	if err != nil {
		return err
	}
	result := db.Model(&database.AuthorizationRule{}).Where("user_id = ? AND id = ? AND revoked_at IS NULL AND (policy_id = '' OR policy_id IS NULL)", user, id).Updates(map[string]any{"revoked_at": time.Now().UTC(), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
