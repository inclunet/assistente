package database

import "testing"

func TestRestoredProviderDoesNotRecoverDeletedPluginBinding(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.AutoMigrate(&LLMProvider{}, &PluginProviderBinding{}); err != nil {
		t.Fatal(err)
	}
	if err := installProviderBindingCleanup(db); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(t.Context(), "owner")
	repo := NewProviderRepository(db)
	provider := LLMProvider{ID: "restored-id", Name: "Equipe", Type: "openai", BaseURL: "https://equipe.example/v1"}
	if err := repo.SaveLLMProvider(ctx, &provider); err != nil {
		t.Fatal(err)
	}
	binding := PluginProviderBinding{InstallationID: "installed", ComponentKey: "providers/equipe", Digest: "revision", ProviderID: provider.ID}
	if err := db.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteLLMProvider(ctx, provider.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveLLMProvider(ctx, &provider); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&PluginProviderBinding{}).Where("provider_id=?", provider.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal(count, err)
	}
}
