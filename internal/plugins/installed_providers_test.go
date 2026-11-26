package plugins

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"gorm.io/gorm"
)

func TestRegisterProviderFromReviewedPackage(t *testing.T) {
	db := managedJobTestDB(t)
	if err := db.AutoMigrate(&database.LLMProvider{}, &database.PluginProviderBinding{}); err != nil {
		t.Fatal(err)
	}
	source := database.WithUserID(t.Context(), "author")
	target := database.WithUserID(t.Context(), "recipient")
	repo := database.NewProviderRepository(db)
	original := database.LLMProvider{ID: "author-private-id", Name: "Equipe", BaseURL: "https://example.test/v1", Type: "openai", CredentialPattern: "author-private-credential"}
	if err := repo.SaveLLMProvider(source, &original); err != nil {
		t.Fatal(err)
	}
	var synced []string
	unavailable := errors.New("registro indisponível")
	var refreshErr error = unavailable
	inventory := NewInventory(db).WithProviderRefresh(func(_ context.Context, id string, _ commandsecurity.Admission) error {
		synced = append(synced, id)
		return refreshErr
	})
	_, snapshot, err := inventory.BuildPackage(source, BuilderRequest{Name: "equipe", Providers: []string{original.Slug}})
	if err != nil {
		t.Fatal(err)
	}
	allow := func(_ *gorm.DB, fn func() error) error { return fn() }
	install := func(ctx context.Context) string {
		t.Helper()
		item, err := inventory.InstallSource(ctx, "equipe", snapshot.Digest, "", nil, allow, func() (*pluginpackage.LocalSnapshot, error) { return snapshot, nil })
		if err != nil {
			t.Fatal(err)
		}
		return item.ID
	}
	id := install(target)
	key := "providers/" + original.Slug
	if err := inventory.RegisterProvider(source, install(source), snapshot.Digest, key, allow); !errors.Is(err, ErrProviderSlugTaken) {
		t.Fatalf("provedor preexistente: %v", err)
	}
	if err := inventory.RegisterProvider(target, id, snapshot.Digest, key, func(_ *gorm.DB, _ func() error) error { return unavailable }); !errors.Is(err, unavailable) {
		t.Fatalf("admissão recusada: %v", err)
	}
	if _, err := repo.GetBySlug(target, original.Slug); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("publicação sem admissão: %v", err)
	}
	if err := inventory.RegisterProvider(target, id, snapshot.Digest, key, allow); !errors.Is(err, ErrProviderSyncPending) {
		t.Fatal(err)
	}
	local, err := repo.GetBySlug(target, original.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if local.ID == original.ID || local.CredentialPattern == original.CredentialPattern || local.IsDefault || !local.RequiresExplicitDefault {
		t.Fatalf("cadastro herdou o estado da origem")
	}
	refreshErr = nil
	if err := inventory.RegisterProvider(target, id, snapshot.Digest, key, allow); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 2 || synced[0] != local.ID || synced[1] != local.ID {
		t.Fatalf("sincronizações: %v", synced)
	}
	components, err := inventory.InstalledComponents(target, id, snapshot.Digest)
	if err != nil || len(components) != 1 || !components[0].Active || components[0].Slug != original.Slug {
		t.Fatalf("componentes após o cadastro: %+v, %v", components, err)
	}
	if err := repo.DeleteLLMProvider(target, local.ID); err != nil {
		t.Fatal(err)
	}
	if err := inventory.RegisterProvider(target, id, snapshot.Digest, key, allow); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 3 || synced[2] == local.ID {
		t.Fatalf("recadastro após exclusão: %v", synced)
	}
}
