package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/profiles"
	"gorm.io/gorm"
)

func TestMissingProviderChatGPTForTextOnly(t *testing.T) {
	db := managedJobTestDB(t)
	if err := db.AutoMigrate(&database.LLMProvider{}); err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(t.Context(), "reader")
	for _, tc := range []struct {
		name, chat, audio string
		want              bool
	}{
		{"chat", "work", "", true}, {"transcription", profiles.DefaultProviderSentinel, "work", false}, {"shared", "work", "work", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := profiles.DefaultProfile()
			p.Chat.LLMProvider, p.Input.LLMProviderID = tc.chat, tc.audio
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			_, err = resolveProfile(ctx, db, raw)
			var missing missingProfileProvider
			if !errors.As(err, &missing) || missing.slug != "work" || missing.chatOnly != tc.want {
				t.Fatalf("missing=%+v err=%v", missing, err)
			}
		})
	}
}

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

func TestProfileOnlyUsesExistingProvidersByRole(t *testing.T) {
	db := managedJobTestDB(t)
	if err := db.AutoMigrate(&database.LLMProvider{}, &database.PluginProfileActivation{}); err != nil {
		t.Fatal(err)
	}
	author := database.WithUserID(t.Context(), "author")
	target := database.WithUserID(t.Context(), "recipient")
	repo := database.NewProviderRepository(db)
	profile := profiles.DefaultProfile()
	roles := []struct {
		name string
		ref  *string
	}{
		{"chat", &profile.Chat.LLMProvider},
		{"assistant", &profile.Voice.Assistant.LLMProviderID},
		{"user", &profile.Voice.User.LLMProviderID},
		{"input", &profile.Input.LLMProviderID},
	}
	for _, role := range roles {
		for _, account := range []struct {
			ctx    context.Context
			prefix string
		}{{author, "author-"}, {target, "local-"}} {
			row := database.LLMProvider{ID: account.prefix + role.name, Name: role.name, Type: "openai", BaseURL: "https://example.test/v1"}
			if err := repo.SaveLLMProvider(account.ctx, &row); err != nil {
				t.Fatal(err)
			}
		}
		*role.ref = "author-" + role.name
	}
	profile.Voice.System.LLMProviderID = profiles.DefaultProviderSentinel
	store := NewInventory(db).WithProfileDefinitions(func() (profiles.PortableDefinitions, error) {
		return profiles.PortableDefinitions{"autor": func() ([]byte, error) { return json.Marshal(profile) }}, nil
	})
	_, snap, err := store.BuildPackage(author, BuilderRequest{Name: "perfil", Profiles: []string{"autor"}})
	if err != nil {
		t.Fatal(err)
	}
	allow := func(_ *gorm.DB, commit func() error) error { return commit() }
	row, err := store.InstallSource(target, "perfil", snap.Digest, "", nil, allow, func() (*pluginpackage.LocalSnapshot, error) { return snap, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetProfileActive(target, row.ID, row.Digest, "profiles/autor", true, allow); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListPluginProfiles(target)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	got, err := store.GetPluginProfile(target, items[0].Slug)
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := [5]string{got.Chat.LLMProvider, got.Voice.Assistant.LLMProviderID, got.Voice.User.LLMProviderID, got.Input.LLMProviderID, got.Voice.System.LLMProviderID}
	want := [5]string{"local-chat", "local-assistant", "local-user", "local-input", profiles.DefaultProviderSentinel}
	if gotIDs != want {
		t.Fatalf("provedores no destino: %v, esperados %v", gotIDs, want)
	}
}

func TestProfileActivationRequiresLocalShellPolicy(t *testing.T) {
	db := managedJobTestDB(t)
	if err := db.AutoMigrate(&database.AuthorizationPolicy{}, &database.PluginProfileActivation{}); err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(t.Context(), "reader")
	store := NewInventory(db)
	p := profiles.DefaultProfile()
	p.Chat.CommandAllowlist = "equipe"
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	store = store.WithProfileDefinitions(func() (profiles.PortableDefinitions, error) {
		return profiles.PortableDefinitions{"autor": func() ([]byte, error) { return raw, nil }}, nil
	})
	_, snap, err := store.BuildPackage(ctx, BuilderRequest{Name: "perfil", Profiles: []string{"autor"}})
	if err != nil {
		t.Fatal(err)
	}
	allow := func(_ *gorm.DB, fn func() error) error { return fn() }
	row, err := store.InstallSource(ctx, "perfil", snap.Digest, "", nil, allow, func() (*pluginpackage.LocalSnapshot, error) { return snap, nil })
	if err != nil {
		t.Fatal(err)
	}
	activate := func() error { return store.SetProfileActive(ctx, row.ID, row.Digest, "profiles/autor", true, allow) }
	if err := activate(); !errors.Is(err, missingProfileAllowlist("equipe")) {
		t.Fatal(err)
	}
	policy := database.AuthorizationPolicy{ID: "other-policy", UserID: "other", Domain: "shell", Slug: "equipe", Source: configdir.NewResolver("allowlists").GetHomeDir(), DefaultEffect: "deny"}
	if err := db.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	if err := activate(); !errors.Is(err, missingProfileAllowlist("equipe")) {
		t.Fatal(err)
	}
	policy.ID = "reader-policy"
	policy.UserID = "reader"
	if err := db.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	if err := activate(); err != nil {
		t.Fatal(err)
	}
}
