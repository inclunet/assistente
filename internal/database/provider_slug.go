package database

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"assistente/internal/slug"
	"gorm.io/gorm"
)

// O bootstrap ainda não tem conta. A adoção atribui o slug no escopo definitivo.
func prepareProviderSlug(tx *gorm.DB, provider *LLMProvider) error {
	if provider.UserID == "" {
		provider.Slug = ""
		return nil
	}
	if provider.Slug != "" {
		if len(provider.Slug) > 63 || slug.Slugify(provider.Slug, "") != provider.Slug {
			return errors.New("slug de provedor inválido")
		}
		return nil
	}
	base := slug.Slugify(provider.Name, "provedor")
	for n := 1; ; n++ {
		suffix := ""
		if n > 1 {
			suffix = "-" + strconv.Itoa(n)
		}
		candidate := strings.TrimRight(base[:min(len(base), 63-len(suffix))], "-") + suffix
		var count int64
		if err := tx.Model(&LLMProvider{}).Where("user_id = ? AND slug = ? AND id <> ?", provider.UserID, candidate, provider.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			provider.Slug = candidate
			return nil
		}
	}
}

func (p *LLMProvider) BeforeCreate(tx *gorm.DB) error {
	return prepareProviderSlug(tx.Session(&gorm.Session{NewDB: true}), p)
}

func migrateProviderSlugs(db *gorm.DB) error {
	if !db.Migrator().HasColumn(&LLMProvider{}, "Slug") {
		return errMigrationDeferred
	}
	return db.Transaction(func(tx *gorm.DB) error { return fillProviderSlugs(tx, "") })
}

func fillProviderSlugs(tx *gorm.DB, owner string) error {
	var providers []LLMProvider
	query := tx.Select("id", "name", "user_id").Where("user_id <> '' AND (slug IS NULL OR slug = '')")
	if owner != "" {
		query = query.Where("user_id = ?", owner)
	}
	if err := query.Order("created_at, id").Find(&providers).Error; err != nil {
		return err
	}
	for i := range providers {
		p := &providers[i]
		if err := prepareProviderSlug(tx, p); err != nil {
			return err
		}
		if err := tx.Model(&LLMProvider{}).Where("id = ?", p.ID).UpdateColumn("slug", p.Slug).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetBySlug resolve a referência portável somente na conta autenticada.
func (r *ProviderRepository) GetBySlug(ctx context.Context, value string) (*LLMProvider, error) {
	owner, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var provider LLMProvider
	if err := r.db.WithContext(ctx).Where("user_id = ? AND slug = ?", owner, value).Take(&provider).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}
