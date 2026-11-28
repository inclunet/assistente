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

func TestMigrationPreservesProviderSourceAndAllowsConsumers(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.Exec("CREATE TABLE plugin_provider_bindings (installation_id TEXT, component_key TEXT, digest TEXT NOT NULL, provider_id TEXT NOT NULL, PRIMARY KEY (installation_id,component_key))").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX idx_plugin_provider_bindings_provider_id ON plugin_provider_bindings(provider_id)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO plugin_provider_bindings VALUES ('source','model','v1','provider')").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&PluginProviderBinding{}); err != nil {
		t.Fatal(err)
	}
	for _, m := range schemaMigrations {
		if m.Version == 50 {
			if err := m.Run(db); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, id := range []string{"one", "two"} {
		row := PluginProviderBinding{InstallationID: id, ComponentKey: "model", Digest: "v1", ProviderID: "provider", ConsumerOnly: true}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	source := PluginProviderBinding{InstallationID: "other", ComponentKey: "model", Digest: "v1", ProviderID: "provider"}
	if err := db.Create(&source).Error; err == nil {
		t.Fatal("dois fornecedores para o mesmo cadastro")
	}
}
