// Package authorization implementa a persistência e os contratos comuns de
// autorização. Adaptadores mantêm a validação e o matching específicos.
package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = errors.New("autorização não encontrada")

type Repository struct{ db func() *gorm.DB }

func NewRepository(db func() *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) query(ctx context.Context) (*gorm.DB, string, error) {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	if r == nil || r.db == nil {
		return nil, "", errors.New("banco de autorizações indisponível")
	}
	db := r.db()
	if db == nil {
		return nil, "", errors.New("banco de autorizações indisponível")
	}
	return db.WithContext(ctx), user, nil
}

func validateRule(rule database.AuthorizationRule) error {
	switch rule.Domain {
	case "network", "filesystem", "shell", "acp":
	default:
		return fmt.Errorf("domínio inválido: %q", rule.Domain)
	}
	switch rule.Scope {
	case "":
		if rule.PolicyID == "" || rule.ScopeKey != "" {
			return errors.New("regra sem escopo requer política")
		}
	case "global":
		if rule.ScopeKey != "" {
			return errors.New("escopo global não aceita chave")
		}
	case "profile", "workspace":
		if strings.TrimSpace(rule.ScopeKey) == "" {
			return errors.New("escopo requer identidade")
		}
	default:
		return errors.New("escopo temporário não pode ser persistido")
	}
	switch rule.Effect {
	case "allow", "deny", "ask":
	default:
		return errors.New("efeito inválido")
	}
	switch rule.Origin {
	case "policy", "consent", "migration":
	default:
		return errors.New("origem inválida")
	}
	if rule.SelectorKey == "" || rule.SelectorVersion != 1 || !json.Valid([]byte(rule.Selector)) {
		return errors.New("seletor de autorização inválido")
	}
	return nil
}

// Upsert mantém a identidade da regra, inclusive quando ela é reautorizada.
func (r *Repository) Upsert(ctx context.Context, rule database.AuthorizationRule) error {
	db, user, err := r.query(ctx)
	if err != nil {
		return err
	}
	if err := validateRule(rule); err != nil {
		return err
	}
	rule.UserID = user
	rule.ID = ""
	rule.RevokedAt = nil
	return upsert(db, &rule)
}

func upsert(db *gorm.DB, rule *database.AuthorizationRule) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "domain"}, {Name: "scope"}, {Name: "scope_key"}, {Name: "selector_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"selector", "selector_version", "effect", "operation", "origin", "policy_id", "granted_by", "updated_at", "revoked_at", "expires_at"}),
	}).Create(rule).Error
}

// List devolve somente regras ativas do usuário/contexto solicitado. Erros
// retornam ao chamador; lista vazia não é substituto para falha de leitura.
func (r *Repository) List(ctx context.Context, domain, scope, key string) ([]database.AuthorizationRule, error) {
	return r.ListScopes(ctx, domain, map[string]string{scope: key})
}

// ListScopes lê allows e denies dos contextos aplicáveis em um único snapshot.
// Uma revogação posterior ao SELECT não desfaz uma operação já autorizada.
func (r *Repository) ListScopes(ctx context.Context, domain string, scopes map[string]string) ([]database.AuthorizationRule, error) {
	db, user, err := r.query(ctx)
	if err != nil {
		return nil, err
	}
	return listScopes(db, user, domain, scopes, "")
}

func listScopes(db *gorm.DB, user, domain string, scopes map[string]string, selectorKey string) ([]database.AuthorizationRule, error) {
	var rules []database.AuthorizationRule
	conditions := db.Where("1 = 0")
	for _, scope := range []string{"profile", "workspace", "global"} {
		if key, ok := scopes[scope]; ok {
			conditions = conditions.Or("scope = ? AND scope_key = ?", scope, key)
		}
	}
	if selectorKey != "" {
		db = db.Where("selector_key = ?", selectorKey)
	}
	err := db.Where("user_id = ? AND domain = ? AND revoked_at IS NULL", user, domain).Where(conditions).
		Where("expires_at IS NULL OR expires_at > ?", time.Now().UTC()).Order("CASE scope WHEN 'profile' THEN 0 WHEN 'workspace' THEN 1 ELSE 2 END, id").Find(&rules).Error
	return rules, err
}

func (r *Repository) Revoke(ctx context.Context, domain, scope, key, selector string) error {
	db, user, err := r.query(ctx)
	if err != nil {
		return err
	}
	result := db.Model(&database.AuthorizationRule{}).Where("user_id = ? AND domain = ? AND scope = ? AND scope_key = ? AND selector_key = ? AND revoked_at IS NULL", user, domain, scope, key, selector).
		Updates(map[string]any{"revoked_at": time.Now().UTC(), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) Imported(ctx context.Context, domain, source string) (bool, error) {
	db, user, err := r.query(ctx)
	if err != nil {
		return false, err
	}
	var count int64
	err = db.Model(&database.AuthorizationImport{}).Where("user_id = ? AND domain = ? AND source = ?", user, domain, source).Count(&count).Error
	return count > 0, err
}

func (r *Repository) ScopeKeys(ctx context.Context, domain, scope string) ([]string, error) {
	db, user, err := r.query(ctx)
	if err != nil {
		return nil, err
	}
	var keys []string
	err = db.Model(&database.AuthorizationRule{}).Where("user_id = ? AND domain = ? AND scope = ? AND revoked_at IS NULL", user, domain, scope).Distinct("scope_key").Order("scope_key").Pluck("scope_key", &keys).Error
	return keys, err
}

// Import grava o conjunto validado e o marcador atomicamente. Uma segunda
// importação nunca altera regras que o usuário já revogou/editou no banco.
func (r *Repository) Import(ctx context.Context, domain, source, digest string, rules []database.AuthorizationRule, verifySource func() error) error {
	db, user, err := r.query(ctx)
	if err != nil {
		return err
	}
	if source == "" || digest == "" {
		return errors.New("origem de importação inválida")
	}
	if verifySource == nil {
		return errors.New("importação requer verificação da origem")
	}
	for _, rule := range rules {
		if err := validateRule(rule); err != nil {
			return err
		}
		if rule.Domain != domain {
			return errors.New("domínio incompatível com a importação")
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		marker := database.AuthorizationImport{UserID: user, Domain: domain, Source: source, Digest: digest, Version: 1}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		for _, rule := range rules {
			rule.ID = ""
			rule.UserID = user
			rule.Origin = "migration"
			// Uma origem legada não sobrescreve regra já existente no banco.
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule).Error; err != nil {
				return err
			}
		}
		return verifySource()
	})
}
