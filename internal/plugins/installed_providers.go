package plugins

import (
	"context"
	"errors"

	"strings"
	"time"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
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
	return i.registerProvider(ctx, id, digest, key, "", admit)
}

// ReuseProvider binds a reviewed existing registration without acquiring ownership.
func (i *Inventory) ReuseProvider(ctx context.Context, id, digest, key, revision string, admit commandsecurity.Admission) error {
	if revision == "" {
		return ErrInstallationChanged
	}
	return i.registerProvider(ctx, id, digest, key, revision, admit)
}

func (i *Inventory) registerProvider(ctx context.Context, id, digest, key, revision string, admit commandsecurity.Admission) error {
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
			if revision != "" {
				token, err := providerReviewToken(local)
				if err != nil {
					return err
				}
				if token != revision || !providers.MatchesPackageDefinition(local, snap.Files[entry]) {
					return ErrInstallationChanged
				}
				var binding database.PluginProviderBinding
				err = tx.Where("installation_id=? AND component_key=?", id, key).Take(&binding).Error
				if err == nil {
					if !binding.ConsumerOnly || binding.ProviderID != local.ID {
						return ErrInstallationChanged
					}
					return tx.Model(&binding).Update("digest", digest).Error
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				return tx.Create(&database.PluginProviderBinding{InstallationID: id, ComponentKey: key, Digest: digest, ProviderID: local.ID, ConsumerOnly: true}).Error
			}
			var receipt database.PluginProviderBinding
			if err := tx.Where("installation_id=? AND component_key=? AND provider_id=?", id, key, local.ID).Take(&receipt).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrProviderSlugTaken
				}
				return err
			}
			if receipt.ConsumerOnly {
				return ErrInstallationChanged
			}
			providerID = local.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if revision != "" {
			return ErrInstallationChanged
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
	if revision != "" {
		return nil
	}
	if err := i.refreshProvider(ctx, providerID, admit); err != nil {
		return errors.Join(ErrProviderSyncPending, err)
	}
	return nil
}

// DescribeExistingProviders compares the portable definitions without modifying local registrations.
func (i *Inventory) DescribeExistingProviders(ctx context.Context, components []Component, snapshot *pluginpackage.LocalSnapshot, installation string) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
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
	var rows []struct {
		database.LLMProvider
		ComponentKey  string
		ConsumerOnly  bool
		BindingDigest string
	}
	err = i.db.WithContext(ctx).Model(&database.LLMProvider{}).
		Select("llm_providers.*, coalesce(b.component_key, '') as component_key, coalesce(b.consumer_only,0) as consumer_only, coalesce(b.digest,'') as binding_digest").
		Joins("LEFT JOIN plugin_provider_bindings b ON b.provider_id=llm_providers.id AND b.installation_id=?", installation).
		Where("llm_providers.user_id=? AND llm_providers.slug IN ?", owner, slugs).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, row := range rows {
		c := &components[indices[row.Slug]]
		if row.ComponentKey == c.Key && !row.ConsumerOnly {
			c.Active = true
		} else if providers.MatchesPackageDefinition(&row.LLMProvider, snapshot.Files[c.Entry]) {
			if row.ComponentKey == c.Key && row.ConsumerOnly && row.BindingDigest == snapshot.Digest {
				c.Reason = "providerInUse"
			} else {
				c.Reason = "providerAlreadyAvailable"
				c.ProviderRevision, err = providerReviewToken(&row.LLMProvider)
				if err != nil {
					return err
				}
			}
		} else {
			c.Reason = "providerSlugTaken"
		}
	}
	return nil
}

func (i *Inventory) WithProviderRemoval(remove func(func() ([]string, error)) error) *Inventory {
	copy := *i
	copy.removeProviders = remove
	return &copy
}
