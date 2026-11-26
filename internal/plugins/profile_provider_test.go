package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/profiles"
	"gorm.io/gorm"
)

func TestNativeProfileUsesDestinationProviderBySlug(t *testing.T) {
	db := managedJobTestDB(t)
	if err := db.AutoMigrate(&database.LLMProvider{}, &database.PluginProviderBinding{}, &database.PluginProfileActivation{}); err != nil {
		t.Fatal(err)
	}
	author := database.WithUserID(t.Context(), "author")
	user := database.WithUserID(t.Context(), "user")
	p := &database.LLMProvider{ID: "upstream-private-id", Name: "Equipe", Type: "openai", BaseURL: "https://equipe.example/v1"}
	repo := database.NewProviderRepository(db)
	if err := repo.SaveLLMProvider(author, p); err != nil {
		t.Fatal(err)
	}
	profile := profiles.DefaultProfile()
	profile.Chat.LLMProvider = p.ID
	profile.Chat.Model = "modelo-do-plugin"
	profile.Voice.Assistant.LLMProviderID = p.ID
	profile.Voice.User.LLMProviderID = p.ID
	profile.Voice.System.LLMProviderID = p.ID
	profile.Input.LLMProviderID = p.ID
	store := NewInventory(db).WithProfileDefinitions(func() (profiles.PortableDefinitions, error) {
		return profiles.PortableDefinitions{"autor": func() ([]byte, error) { return json.Marshal(profile) }}, nil
	}).WithProviderRefresh(func(context.Context, string, commandsecurity.Admission) error { return nil })
	_, snap, err := store.BuildPackage(author, BuilderRequest{Name: "conjunto", Providers: []string{p.Slug}, Profiles: []string{"autor"}})
	if err != nil {
		t.Fatal(err)
	}
	allow := func(_ *gorm.DB, fn func() error) error { return fn() }
	row, err := store.InstallSource(user, "conjunto", snap.Digest, "", nil, allow, func() (*pluginpackage.LocalSnapshot, error) { return snap, nil })
	if err != nil {
		t.Fatal(err)
	}
	activate := func() error { return store.SetProfileActive(user, row.ID, row.Digest, "profiles/autor", true, allow) }
	if err := activate(); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("dependência ausente: %v", err)
	}
	if err := store.RegisterProvider(user, row.ID, row.Digest, "providers/"+p.Slug, allow); err != nil {
		t.Fatal(err)
	}
	if err := activate(); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListPluginProfiles(user)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	got, err := store.GetPluginProfile(user, items[0].Slug)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := repo.GetBySlug(user, p.Slug)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{got.Chat.LLMProvider, got.Voice.Assistant.LLMProviderID, got.Voice.User.LLMProviderID, got.Voice.System.LLMProviderID, got.Input.LLMProviderID} {
		if id != dest.ID || id == p.ID {
			t.Fatalf("referência de provedor não resolvida no destino: %q", id)
		}
	}
	if got.Chat.Model != profile.Chat.Model {
		t.Fatal("modelo escolhido para o chat foi perdido")
	}
	if err := repo.DeleteLLMProvider(user, dest.ID); err != nil {
		t.Fatal(err)
	}
	components, err := store.InstalledComponents(user, row.ID, row.Digest)
	if err != nil {
		t.Fatal(err)
	}
	var missing string
	for _, c := range components {
		if c.Kind == "profile" {
			missing = c.MissingProvider
		}
	}
	if missing != p.Slug {
		t.Fatalf("dependência ausente: %q", missing)
	}
	if _, err := store.GetPluginProfile(user, items[0].Slug); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal(err)
	}
}
