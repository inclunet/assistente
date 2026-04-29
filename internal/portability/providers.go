package portability

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
)

func exportProvider(provider *database.LLMProvider) ProviderExport {
	return ProviderExport{
		ID:                provider.ID,
		Name:              provider.Name,
		Type:              provider.Type,
		APIFormat:         provider.APIFormat,
		BaseURL:           provider.BaseURL,
		Model:             provider.Model,
		DefaultModel:      provider.DefaultModel,
		IsDefault:         provider.IsDefault,
		Timeout:           provider.Timeout,
		CredentialPattern: provider.CredentialPattern,
		CreatedAt:         provider.CreatedAt,
	}
}

func importProvider(provider ProviderExport) (bool, error) {
	if strings.TrimSpace(provider.ID) == "" {
		return false, fmt.Errorf("provider sem id não pode ser importado")
	}
	if existing, err := findExistingProviderByID(provider.ID); err != nil {
		return false, err
	} else if existing != nil {
		return overwriteProvider(provider)
	}

	err := database.DB().Transaction(func(tx *gorm.DB) error {
		return persistProvider(tx, provider, nil)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func overwriteProvider(provider ProviderExport) (bool, error) {
	existing, err := findExistingProviderByID(provider.ID)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return importProvider(provider)
	}

	err = database.DB().Transaction(func(tx *gorm.DB) error {
		return persistProvider(tx, provider, existing)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func persistProvider(tx *gorm.DB, provider ProviderExport, existing *database.LLMProvider) error {
	createdAt := provider.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	if provider.IsDefault {
		if err := tx.Model(&database.LLMProvider{}).
			Where("is_default = ? AND id <> ?", true, strings.TrimSpace(provider.ID)).
			Update("is_default", false).Error; err != nil {
			return err
		}
	}

	if existing == nil {
		model := database.LLMProvider{
			ID:                strings.TrimSpace(provider.ID),
			Name:              provider.Name,
			Type:              provider.Type,
			APIFormat:         provider.APIFormat,
			BaseURL:           provider.BaseURL,
			Model:             provider.Model,
			DefaultModel:      provider.DefaultModel,
			IsDefault:         provider.IsDefault,
			Timeout:           provider.Timeout,
			CredentialPattern: provider.CredentialPattern,
			CreatedAt:         createdAt,
			UpdatedAt:         createdAt,
		}
		return tx.Create(&model).Error
	}

	existing.Name = provider.Name
	existing.Type = provider.Type
	existing.APIFormat = provider.APIFormat
	existing.BaseURL = provider.BaseURL
	existing.Model = provider.Model
	existing.DefaultModel = provider.DefaultModel
	existing.IsDefault = provider.IsDefault
	existing.Timeout = provider.Timeout
	existing.CredentialPattern = provider.CredentialPattern
	existing.CreatedAt = createdAt
	existing.UpdatedAt = createdAt
	return tx.Save(existing).Error
}

func findExistingProviderByID(providerID string) (*database.LLMProvider, error) {
	var provider database.LLMProvider
	err := database.DB().Where("id = ?", strings.TrimSpace(providerID)).First(&provider).Error
	if err == nil {
		return &provider, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("erro ao localizar provider %q: %w", providerID, err)
}
