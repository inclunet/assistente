package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"

	"assistente/internal/database"
	"assistente/internal/providers"
	"gorm.io/gorm"
)

// ProviderRemovalPreview retorna os slugs exibidos e a versão de cada cadastro local.
func (i *Inventory) ProviderRemovalPreview(ctx context.Context, id, digest string) (map[string]string, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var row database.PluginInstallation
	if err := i.db.WithContext(ctx).Select("id").Where("id=? AND digest=? AND user_id=?", id, digest, owner).Take(&row).Error; err != nil {
		return nil, err
	}
	_, versions, err := boundProviderVersions(i.db.WithContext(ctx), owner, id)
	return versions, err
}

func boundProviderVersions(tx *gorm.DB, owner, id string) ([]database.LLMProvider, map[string]string, error) {
	var rows []database.LLMProvider
	err := tx.Model(&database.LLMProvider{}).Joins("JOIN plugin_provider_bindings b ON b.provider_id=llm_providers.id").Where("b.installation_id=? AND llm_providers.user_id=?", id, owner).Find(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	versions := make(map[string]string, len(rows))
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			return nil, nil, err
		}
		versions[row.Slug] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return rows, versions, nil
}

func applyProviderRemoval(tx *gorm.DB, ctx context.Context, owner, id, choice string, reviewed map[string]string) ([]string, error) {
	rows, current, err := boundProviderVersions(tx, owner, id)
	if err != nil {
		return nil, err
	}
	if !maps.Equal(current, reviewed) || (len(rows) > 0 && choice != "detach" && choice != "remove") {
		return nil, ErrInstallationChanged
	}
	var removed []string
	if choice == "remove" {
		for _, row := range rows {
			if err := providers.DeleteNativeProviderTx(ctx, tx, row.ID); err != nil {
				return nil, err
			}
			removed = append(removed, row.ID)
		}
	}
	return removed, tx.Where("installation_id=?", id).Delete(&database.PluginProviderBinding{}).Error
}
