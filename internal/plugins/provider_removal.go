package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"

	"assistente/internal/database"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"gorm.io/gorm"
)

func providerReviewToken(row *database.LLMProvider) (string, error) {
	data, err := json.Marshal(row)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

type ProviderRemovalItem struct {
	Version   string   `json:"version"`
	Preserve  bool     `json:"preserve"`
	Consumers []string `json:"consumers"`
}

// ProviderRemovalPreview retorna os slugs exibidos e a versão de cada cadastro local.
func (i *Inventory) ProviderRemovalPreview(ctx context.Context, id, digest string) (map[string]ProviderRemovalItem, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var row database.PluginInstallation
	if err := i.db.WithContext(ctx).Select("id").Where("id=? AND digest=? AND user_id=?", id, digest, owner).Take(&row).Error; err != nil {
		return nil, err
	}
	var bindingCount int64
	if err := i.db.WithContext(ctx).Model(&database.PluginProviderBinding{}).Where("installation_id=?", id).Count(&bindingCount).Error; err != nil {
		return nil, err
	}
	if bindingCount == 0 {
		return map[string]ProviderRemovalItem{}, nil
	}
	var versions map[string]ProviderRemovalItem
	err = i.withProviderConsumers(func(refs map[string][]string, complete bool) error {
		var err error
		_, versions, err = i.boundProviderVersions(i.db.WithContext(ctx), owner, id, refs, complete)
		return err
	})
	return versions, err
}

func (i *Inventory) boundProviderVersions(tx *gorm.DB, owner, id string, refs map[string][]string, complete bool) ([]database.LLMProvider, map[string]ProviderRemovalItem, error) {
	var rows []database.LLMProvider
	err := tx.Model(&database.LLMProvider{}).Joins("JOIN plugin_provider_bindings b ON b.provider_id=llm_providers.id").Where("b.installation_id=? AND llm_providers.user_id=?", id, owner).Find(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	versions := make(map[string]ProviderRemovalItem, len(rows))
	if len(rows) == 0 {
		return rows, versions, nil
	}
	var active []database.PluginProfileActivation
	if err := tx.Joins("JOIN plugin_installations p ON p.id=plugin_profile_activations.installation_id AND p.digest=plugin_profile_activations.digest").Where("p.user_id=? AND p.id<>? AND p.disabled=?", owner, id, false).Find(&active).Error; err != nil {
		return nil, nil, err
	}
	for _, activation := range active {
		p, err := readPackageProfile(activation.Definition)
		if err != nil {
			complete = false
			continue
		}
		indexProviderReferences(refs, p, p.Name+" ("+activation.InstallationID+"/"+activation.ComponentKey+")")
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		token, err := providerReviewToken(&row)
		if err != nil {
			return nil, nil, err
		}
		ids = append(ids, row.ID)
		consumers := append(slices.Clone(refs[row.ID]), refs[row.Slug]...)
		versions[row.Slug] = ProviderRemovalItem{Version: token, Preserve: !complete || row.IsDefault, Consumers: consumers}
	}
	var bindings []struct {
		Slug, InstallationID, Name, Origin string
		ConsumerOnly                       bool
	}
	err = tx.Table("plugin_provider_bindings b").Select("s.slug, p.id AS installation_id, p.name, p.origin, b.consumer_only").Joins("JOIN plugin_installations p ON p.id=b.installation_id").Joins("JOIN llm_providers s ON s.id=b.provider_id AND s.user_id=p.user_id").Where("p.user_id=? AND s.id IN ?", owner, ids).Scan(&bindings).Error
	if err != nil {
		return nil, nil, err
	}
	for _, binding := range bindings {
		item := versions[binding.Slug]
		if binding.InstallationID == id {
			item.Preserve = item.Preserve || binding.ConsumerOnly
		} else {
			item.Consumers = append(item.Consumers, fmt.Sprintf("%s (%s)", binding.Name, binding.Origin))
		}
		versions[binding.Slug] = item
	}
	for slug, item := range versions {
		sort.Strings(item.Consumers)
		item.Consumers = slices.Compact(item.Consumers)
		item.Preserve = item.Preserve || len(item.Consumers) > 0
		versions[slug] = item
	}
	return rows, versions, nil
}

func indexProviderReferences(refs map[string][]string, p *profiles.Profile, name string) {
	_ = p.MapProviderReferences(func(ref string) (string, error) { refs[ref] = append(refs[ref], name); return ref, nil })
}

func (i *Inventory) WithProviderConsumers(snapshot func(func(map[string][]string, bool) error) error) *Inventory {
	copy := *i
	copy.providerConsumers = snapshot
	return &copy
}

func (i *Inventory) withProviderConsumers(use func(map[string][]string, bool) error) error {
	if i.providerConsumers == nil {
		return use(map[string][]string{}, false)
	}
	return i.providerConsumers(use)
}

func (i *Inventory) applyProviderRemoval(tx *gorm.DB, ctx context.Context, owner, id, choice string, reviewed map[string]ProviderRemovalItem, refs map[string][]string, complete bool) ([]string, error) {
	rows, current, err := i.boundProviderVersions(tx, owner, id, refs, complete)
	if err != nil {
		return nil, err
	}
	if !maps.EqualFunc(current, reviewed, func(a, b ProviderRemovalItem) bool {
		return a.Version == b.Version && a.Preserve == b.Preserve && slices.Equal(a.Consumers, b.Consumers)
	}) || (len(rows) > 0 && choice != "detach" && choice != "remove") {
		return nil, ErrInstallationChanged
	}
	var removed []string
	if choice == "remove" {
		for _, row := range rows {
			if current[row.Slug].Preserve {
				continue
			}
			if err := providers.DeleteNativeProviderTx(ctx, tx, row.ID); err != nil {
				return nil, err
			}
			removed = append(removed, row.ID)
		}
	}
	return removed, tx.Where("installation_id=?", id).Delete(&database.PluginProviderBinding{}).Error
}
