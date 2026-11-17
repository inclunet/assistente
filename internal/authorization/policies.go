package authorization

import (
	"context"
	"errors"
	"fmt"

	"assistente/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) Policies(ctx context.Context, domain string) ([]database.AuthorizationPolicy, error) {
	db, user, err := r.query(ctx)
	if err != nil {
		return nil, err
	}
	var rows []database.AuthorizationPolicy
	err = db.Where("user_id = ? AND domain = ?", user, domain).Order("slug, source").Find(&rows).Error
	return rows, err
}

// PolicySnapshot impede misturar default de uma revisão e regras de outra.
func (r *Repository) PolicySnapshot(ctx context.Context, id string) (database.AuthorizationPolicy, []database.AuthorizationRule, error) {
	db, user, err := r.query(ctx)
	if err != nil {
		return database.AuthorizationPolicy{}, nil, err
	}
	var policy database.AuthorizationPolicy
	var rules []database.AuthorizationRule
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND id = ?", user, id).First(&policy).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND policy_id = ? AND revoked_at IS NULL", user, id).Order("selector_key").Find(&rules).Error
	})
	return policy, rules, err
}

func (r *Repository) BindPolicy(ctx context.Context, id, profile string) error {
	db, user, err := r.query(ctx)
	if err != nil {
		return err
	}
	return database.WithSQLiteBusyRetry(ctx, "authorization.bind_policy", func() error {
		return db.Transaction(func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&database.AuthorizationPolicy{}).Where("id = ? AND user_id = ?", id, user).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return ErrNotFound
			}
			// Associação reflete a seleção efetiva do perfil sem duplicar regras.
			var existing database.AuthorizationPolicyBinding
			err := tx.Where("user_id = ? AND policy_id = ? AND scope = ? AND scope_key = ?", user, id, "profile", profile).First(&existing).Error
			if err == nil {
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&database.AuthorizationPolicyBinding{UserID: user, PolicyID: id, Scope: "profile", ScopeKey: profile}).Error
		})
	})
}

// SavePolicy substitui conjunto e metadados na mesma transação. As regras
// permanecem nas mesmas tabelas dos demais domínios, com política explícita.
func (r *Repository) SavePolicy(ctx context.Context, policy database.AuthorizationPolicy, rules []database.AuthorizationRule) error {
	db, user, err := r.query(ctx)
	if err != nil {
		return err
	}
	return database.WithSQLiteBusyRetry(ctx, "authorization.save_policy", func() error {
		return db.Transaction(func(tx *gorm.DB) error { return savePolicy(tx, user, policy, rules) })
	})
}
func savePolicy(tx *gorm.DB, user string, policy database.AuthorizationPolicy, rules []database.AuthorizationRule) error {
	if policy.Domain != "shell" || policy.Slug == "" || policy.Source == "" {
		return errors.New("política inválida")
	}
	if policy.DefaultEffect != "ask" && policy.DefaultEffect != "deny" {
		return errors.New("efeito padrão inválido")
	}
	policy.UserID = user
	var current database.AuthorizationPolicy
	err := tx.Where("user_id = ? AND domain = ? AND slug = ? AND source = ?", user, policy.Domain, policy.Slug, policy.Source).First(&current).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		policy.ID = current.ID
		policy.CreatedAt = current.CreatedAt
		policy.Revision = current.Revision + 1
	} else {
		policy.ID = ""
		policy.Revision = 1
	}
	if err := tx.Save(&policy).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ? AND policy_id = ?", user, policy.ID).Delete(&database.AuthorizationRule{}).Error; err != nil {
		return err
	}
	for i, rule := range rules {
		rule.ID = ""
		rule.UserID = user
		rule.Domain = policy.Domain
		rule.PolicyID = policy.ID
		rule.Origin = "policy"
		rule.Scope = ""
		rule.ScopeKey = ""
		rule.SelectorKey = fmt.Sprintf("%s:%08d", policy.ID, i)
		if err := validateRule(rule); err != nil {
			return err
		}
		if err := tx.Create(&rule).Error; err != nil {
			return err
		}
	}
	return nil
}
func (r *Repository) DeletePolicy(ctx context.Context, id string) error {
	db, user, err := r.query(ctx)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("user_id = ? AND id = ?", user, id).Delete(&database.AuthorizationPolicy{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := tx.Where("user_id = ? AND policy_id = ?", user, id).Delete(&database.AuthorizationPolicyBinding{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND policy_id = ?", user, id).Delete(&database.AuthorizationRule{}).Error
	})
}

// ImportPolicy usa o mesmo marcador que os demais domínios; revogar/excluir
// depois da migração não faz o arquivo reaparecer como política ativa.
func (r *Repository) ImportPolicy(ctx context.Context, source, digest string, policy database.AuthorizationPolicy, rules []database.AuthorizationRule, verify func() error) error {
	db, user, err := r.query(ctx)
	if err != nil {
		return err
	}
	if source == "" || digest == "" || verify == nil {
		return errors.New("importação de política inválida")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		marker := database.AuthorizationImport{UserID: user, Domain: "shell", Source: source, Digest: digest, Version: 1}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		var count int64
		if err := tx.Model(&database.AuthorizationPolicy{}).Where("user_id = ? AND domain = ? AND slug = ? AND source = ?", user, policy.Domain, policy.Slug, policy.Source).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := savePolicy(tx, user, policy, rules); err != nil {
				return err
			}
		}
		return verify()
	})
}
