package plugins

import (
	"context"
	"errors"
	"strings"
	"time"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/providers"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrProviderSlugTaken = errors.New("PLUGIN_PROVIDER_SLUG_TAKEN")
var ErrProviderSyncPending = errors.New("PLUGIN_PROVIDER_SYNC_PENDING")

func (i *Inventory) WithProviderRefresh(refresh func(context.Context, string, commandsecurity.Admission) error) *Inventory {
	copy := *i
	copy.refreshProvider = refresh
	return &copy
}

// RegisterProvider só registra o recurso da revisão conferida na conta atual.
func (i *Inventory) RegisterProvider(ctx context.Context, id, digest, key string, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if admit == nil || i.refreshProvider == nil {
		return ErrComponentUnavailable
	}
	snap, err := i.installedSnapshot(ctx, id, digest)
	if err != nil {
		return err
	}
	var entry string
	for _, c := range DiscoverComponents(snap) {
		if c.Key == key && c.Kind == "provider" && c.Reason == "" {
			entry = c.Entry
			break
		}
	}
	if entry == "" {
		return ErrComponentUnavailable
	}
	p, err := providers.ReadPackageDefinition(snap.Files[entry], strings.TrimPrefix(key, "providers/"))
	if err != nil {
		return err
	}
	publish, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var providerID string
	err = database.WithSQLiteAdmittedCommit(publish, i.db, "plugins.provider.register", func(tx *gorm.DB) error {
		var installed database.PluginInstallation
		if err := tx.Where("id=? AND user_id=? AND digest=? AND disabled=?", id, owner, digest, false).Take(&installed).Error; err != nil {
			return err
		}
		repo := database.NewProviderRepository(tx)
		local, err := repo.GetBySlug(ctx, p.Slug)
		if err == nil {
			var receipt database.PluginProviderBinding
			if err := tx.Where("installation_id=? AND component_key=? AND digest=? AND provider_id=?", id, key, digest, local.ID).Take(&receipt).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrProviderSlugTaken
				}
				return err
			}
			providerID = local.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Where("installation_id=? AND component_key=? AND NOT EXISTS (SELECT 1 FROM llm_providers WHERE id=plugin_provider_bindings.provider_id)", id, key).Delete(&database.PluginProviderBinding{}).Error; err != nil {
			return err
		}
		fresh, err := uuid.NewV7()
		if err != nil {
			return err
		}
		providerID = fresh.String()
		if err := providers.InsertPackageDefinition(ctx, tx, *p, providerID); err != nil {
			return err
		}
		return tx.Create(&database.PluginProviderBinding{InstallationID: id, ComponentKey: key, Digest: digest, ProviderID: providerID}).Error
	}, admit)
	if err != nil {
		return err
	}
	if err := i.refreshProvider(ctx, providerID, admit); err != nil {
		return errors.Join(ErrProviderSyncPending, err)
	}
	return nil
}

func (i *Inventory) markExistingProviders(ctx context.Context, components []Component, owner, installation, digest string) error {
	indices := make(map[string]int)
	var slugs []string
	for n, c := range components {
		if c.Kind == "provider" && c.Reason == "" {
			indices[c.Slug] = n
			slugs = append(slugs, c.Slug)
		}
	}
	if len(slugs) == 0 {
		return nil
	}
	var rows []struct{ Slug, ComponentKey string }
	err := i.db.WithContext(ctx).Model(&database.LLMProvider{}).
		Select("llm_providers.slug, coalesce(b.component_key, '') as component_key").
		Joins("LEFT JOIN plugin_provider_bindings b ON b.provider_id=llm_providers.id AND b.installation_id=? AND b.digest=?", installation, digest).
		Where("llm_providers.user_id=? AND llm_providers.slug IN ?", owner, slugs).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, row := range rows {
		c := &components[indices[row.Slug]]
		if row.ComponentKey == c.Key {
			c.Active = true
		} else {
			c.Reason = "providerSlugTaken"
		}
	}
	return nil
}
