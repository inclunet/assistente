package database

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestMigration36VersionsDirectProviderUpdatesWithoutDoubleIncrement(t *testing.T) {
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
	if err := repo.CreateLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	assertRevision := func(compatibility, configuration int) {
		t.Helper()
		var row LLMProvider
		if err := db.First(&row, "id = ?", provider.ID).Error; err != nil {
			t.Fatal(err)
		}
		if row.CompatibilityRevision != compatibility || row.ConfigRevision != configuration {
			t.Fatalf("revisões = %d/%d; esperadas %d/%d", row.CompatibilityRevision, row.ConfigRevision, compatibility, configuration)
		}
	}
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Updates(map[string]any{"credential_pattern": "replacement.example.com", "base_url": "https://new.example.com/v1", "api_format": "openai_responses", "auth_mode": "required"}).Error; err != nil {
		t.Fatal(err)
	}
	assertRevision(2, 2)
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Update("default_model", "new-model").Error; err != nil {
		t.Fatal(err)
	}
	assertRevision(2, 3)
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Update("default_model", "new-model").Error; err != nil {
		t.Fatal(err)
	}
	assertRevision(2, 3)
	if err := db.First(provider, "id = ?", provider.ID).Error; err != nil {
		t.Fatal(err)
	}
	provider.BaseURL = "https://third.example.com/v1"
	if err := repo.SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	assertRevision(3, 4)
	rollback := errors.New("rollback")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&LLMProvider{}).Where("id = ?", provider.ID).Update("credential_pattern", "rollback.example.com").Error; err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("rollback: %v", err)
	}
	assertRevision(3, 4)
	for _, column := range []string{"compatibility_revision", "config_revision"} {
		if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).UpdateColumn(column, 1).Error; err == nil {
			t.Fatalf("permitiu regressão de %s", column)
		}
	}
	assertRevision(3, 4)
}
