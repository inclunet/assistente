package portability

import (
	"context"
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

func importProvider(ctx context.Context, provider ProviderExport) (bool, error) {
	normalized, err := validateProviderExport(provider)
	if err != nil {
		return false, err
	}
	if existing, err := findExistingProviderByID(ctx, normalized.ID); err != nil {
		return false, err
	} else if existing != nil {
		return overwriteProvider(ctx, normalized)
	}

	err = database.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return persistProvider(ctx, tx, normalized, nil)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func overwriteProvider(ctx context.Context, provider ProviderExport) (bool, error) {
	normalized, err := validateProviderExport(provider)
	if err != nil {
		return false, err
	}

	existing, err := findExistingProviderByID(ctx, normalized.ID)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return importProvider(ctx, normalized)
	}

	err = database.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return persistProvider(ctx, tx, normalized, existing)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func persistProvider(ctx context.Context, tx *gorm.DB, provider ProviderExport, existing *database.LLMProvider) error {
	createdAt := provider.CreatedAt
	if createdAt.IsZero() {
		if existing != nil && !existing.CreatedAt.IsZero() {
			createdAt = existing.CreatedAt
		} else {
			createdAt = time.Now().UTC()
		}
	}
	updatedAt := createdAt
	if existing != nil && provider.CreatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	if provider.IsDefault {
		if err := database.ScopeByUser(ctx, tx.Model(&database.LLMProvider{}), "user_id").
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
			UpdatedAt:         updatedAt,
		}
		if userID, ok := database.UserIDFromContext(ctx); ok {
			model.UserID = userID
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
	existing.UpdatedAt = updatedAt
	return tx.Save(existing).Error
}

func validateProviderExport(provider ProviderExport) (ProviderExport, error) {
	normalized := provider
	normalized.ID = strings.TrimSpace(provider.ID)
	normalized.Name = strings.TrimSpace(provider.Name)
	normalized.Type = strings.TrimSpace(provider.Type)
	normalized.APIFormat = strings.TrimSpace(provider.APIFormat)
	normalized.BaseURL = strings.TrimSpace(provider.BaseURL)
	normalized.Model = strings.TrimSpace(provider.Model)
	normalized.DefaultModel = strings.TrimSpace(provider.DefaultModel)
	normalized.CredentialPattern = strings.TrimSpace(provider.CredentialPattern)

	if normalized.ID == "" {
		return ProviderExport{}, fmt.Errorf("provider sem id não pode ser importado")
	}
	if normalized.Name == "" {
		return ProviderExport{}, fmt.Errorf("provider %q sem name não pode ser importado", normalized.ID)
	}
	if normalized.Type == "" {
		return ProviderExport{}, fmt.Errorf("provider %q sem type não pode ser importado", normalized.ID)
	}
	if normalized.BaseURL == "" {
		return ProviderExport{}, fmt.Errorf("provider %q sem baseUrl não pode ser importado", normalized.ID)
	}
	return normalized, nil
}

func findExistingProviderByID(ctx context.Context, providerID string) (*database.LLMProvider, error) {
	var provider database.LLMProvider
	err := database.ScopeByUser(ctx, database.DB(), "user_id").Where("id = ?", strings.TrimSpace(providerID)).First(&provider).Error
	if err == nil {
		return &provider, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("erro ao localizar provider %q: %w", providerID, err)
}
