package database

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestProviderRevisionGuardsVersionDirectUpdatesWithoutDoubleIncrement(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.AutoMigrate(&LLMProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLLMModelCapabilities(db); err != nil {
		t.Fatal(err)
	}
	if err := migrateLLMProviderRevisionGuards(db); err != nil {
		t.Fatal(err)
	}
	if err := migrateLLMProviderRevisionGuards(db); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "owner")
	provider := &LLMProvider{ID: "versioned", UserID: "owner", Name: "Original", Type: "openai", BaseURL: "https://api.example.com/v1", APIFormat: "openai", CredentialPattern: "original.example.com"}
	repo := NewProviderRepository(db)
	if err := repo.SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	assertRevision := func(compatibility int) {
		t.Helper()
		var row LLMProvider
		if err := db.First(&row, "id = ?", provider.ID).Error; err != nil {
			t.Fatal(err)
		}
		if row.CompatibilityRevision != compatibility {
			t.Fatalf("revisão de compatibilidade = %d; esperada %d", row.CompatibilityRevision, compatibility)
		}
	}
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Update("credential_pattern", "replacement.example.com").Error; err != nil {
		t.Fatal(err)
	}
	assertRevision(1)
	if err := db.First(provider, "id = ?", provider.ID).Error; err != nil {
		t.Fatal(err)
	}
	provider.CredentialPattern = "repository-replacement.example.com"
	if err := repo.SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	assertRevision(1)
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Updates(map[string]any{"base_url": "https://new.example.com/v1", "api_format": "openai_responses", "auth_mode": "required"}).Error; err != nil {
		t.Fatal(err)
	}
	assertRevision(2)
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Update("default_model", "new-model").Error; err != nil {
		t.Fatal(err)
	}
	assertRevision(2)
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Update("default_model", "new-model").Error; err != nil {
		t.Fatal(err)
	}
	assertRevision(2)
	if err := db.First(provider, "id = ?", provider.ID).Error; err != nil {
		t.Fatal(err)
	}
	provider.BaseURL = "https://third.example.com/v1"
	if err := repo.SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	assertRevision(3)
	rollback := errors.New("rollback")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&LLMProvider{}).Where("id = ?", provider.ID).Update("credential_pattern", "rollback.example.com").Error; err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("rollback: %v", err)
	}
	assertRevision(3)
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).UpdateColumn("compatibility_revision", 1).Error; err == nil {
		t.Fatal("permitiu regressão de compatibility_revision")
	}
	assertRevision(3)
}

func TestProviderCompatibilityRevisionUsesEffectiveDefaults(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.AutoMigrate(&LLMProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := migrateLLMProviderRevisionGuards(db); err != nil {
		t.Fatal(err)
	}
	provider := &LLMProvider{
		ID: "effective-defaults", UserID: "owner", Type: "openai",
		BaseURL: "https://api.openai.com/v1",
	}
	if err := NewProviderRepository(db).SaveLLMProvider(WithUserID(context.Background(), "owner"), provider); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Updates(map[string]any{
		"api_format": "openai_responses", "reasoning_content_mode": "disabled",
	}).Error; err != nil {
		t.Fatal(err)
	}
	var stored LLMProvider
	if err := db.First(&stored, "id = ?", provider.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CompatibilityRevision != 1 {
		t.Fatalf("representações explícitas dos padrões avançaram compatibilidade: %d", stored.CompatibilityRevision)
	}
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Update("api_format", "openai").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&stored, "id = ?", provider.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CompatibilityRevision != 2 {
		t.Fatalf("mudança de formato efetivo não avançou compatibilidade: %d", stored.CompatibilityRevision)
	}
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Update("reasoning_content_mode", "replay_with_tools").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&stored, "id = ?", provider.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CompatibilityRevision != 3 {
		t.Fatalf("mudança de modo de reasoning efetivo não avançou compatibilidade: %d", stored.CompatibilityRevision)
	}
}
