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
			_, err = resolveProfile(ctx, db, raw, nil, "", "")
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
	preview := DiscoverComponents(snap)
	if err := store.DescribeProfileDependencies(user, preview, snap, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.PluginProviderBinding{}).Where("installation_id=?", row.ID).UpdateColumn("consumer_only", true).Error; err != nil {
		t.Fatal(err)
	}
	update := *snap
	update.Digest = "nova-revisao"
	pendingUpdate := DiscoverComponents(&update)
	if err := store.DescribeProfileDependencies(user, pendingUpdate, &update, row.ID); err != nil {
		t.Fatal(err)
	}
	for _, c := range pendingUpdate {
		if c.Kind == "profile" && c.PendingProvider != p.Slug {
			t.Fatalf("prévia reutilizou o vínculo anterior: %+v", c)
		}
	}
	var pendingPreview bool
	for _, c := range preview {
		if c.Kind == "profile" {
			pendingPreview = c.PendingProvider == p.Slug
		}
	}
	if !pendingPreview {
		t.Fatalf("a prévia de outra instalação deve pedir conferência de uso: %+v", preview)
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
	if err := db.Model(&database.PluginRevision{}).Where("installation_id=? AND digest=?", row.ID, row.Digest).UpdateColumn("provider_requirements", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TRIGGER refuse_index BEFORE UPDATE OF provider_requirements ON plugin_revisions BEGIN SELECT RAISE(ABORT,'index unavailable'); END").Error; err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetPluginProfile(user, items[0].Slug); err != nil || got.Chat.LLMProvider != dest.ID {
		t.Fatal(got, err)
	}
	if err := db.Exec("DROP TRIGGER refuse_index").Error; err != nil {
		t.Fatal(err)
	}
	dest.BaseURL = "https://diferente.example/v1"
	if err := repo.SaveLLMProvider(user, dest); err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []bool{false, true} {
		if legacy {
			if err := db.Model(&database.PluginRevision{}).Where("installation_id=? AND digest=?", row.ID, row.Digest).UpdateColumn("provider_requirements", nil).Error; err != nil {
				t.Fatal(err)
			}
		}
		var pending unavailableProfileProvider
		if err := activate(); !errors.As(err, &pending) {
			t.Fatalf("ativação com configuração divergente: %v", err)
		}
		if _, err := store.GetPluginProfile(user, items[0].Slug); !errors.As(err, &pending) {
			t.Fatalf("perfil ativo após edição do provedor: %v", err)
		}
		if listed, err := store.ListPluginProfiles(user); err != nil || len(listed) != 0 {
			t.Fatal(listed, err)
		}
		details, err := store.InstalledComponents(user, row.ID, row.Digest)
		if err != nil {
			t.Fatal(err)
		}
		var seen bool
		for _, c := range details {
			if c.Kind == "profile" {
				seen = c.Active && c.PendingProvider == p.Slug && !c.CanCreateChatGPT
			}
		}
		if !seen {
			t.Fatal(details)
		}
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
