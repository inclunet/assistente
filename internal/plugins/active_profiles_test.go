package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/profiles"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func TestInstalledProfileAccountAndRevision(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "profiles.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginProfileActivation{}, &database.PluginSkillActivation{}, &database.Job{}, &database.PluginMCPBinding{}, &database.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	store := NewInventory(db)
	manager := profiles.NewManager()
	manager.SetPluginSource(store)
	owner := database.WithUserID(context.Background(), "alice")
	other := database.WithUserID(context.Background(), "bob")
	root := t.TempDir()
	dir := filepath.Join(root, nativeExtension, "profiles")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "plugin.json"), `{"$schema":"`+pluginpackage.ManifestSchema+`","name":"portable","extensions":{"br.com.inclunet.assistente":{"version":1,"profiles":["writer"]}}}`)
	install := func(previous string) database.PluginInstallation {
		t.Helper()
		snap, err := pluginpackage.InspectLocal(root)
		if err != nil {
			t.Fatal(err)
		}
		r, err := store.InstallLocal(owner, root, snap.Digest, previous)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	write(filepath.Join(dir, "writer.json"), `{"name":"Writer","maxTokens":123}`)
	row := install("")
	admit := func(_ *gorm.DB, action func() error) error { return action() }
	set := func(ctx context.Context, active bool) error {
		return store.SetProfileActive(ctx, row.ID, row.Digest, "profiles/writer", active, admit)
	}
	if err := set(other, true); err == nil {
		t.Fatal("foreign activation")
	}
	denied := errors.New("session ended")
	if err := store.SetProfileActive(owner, row.ID, row.Digest, "profiles/writer", true, func(*gorm.DB, func() error) error { return denied }); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	list := func(ctx context.Context) []profiles.ProfileInfo {
		t.Helper()
		r, err := store.ListPluginProfiles(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if len(list(owner)) != 0 {
		t.Fatal("activated before approval")
	}
	for range 2 {
		if err := set(owner, true); err != nil {
			t.Fatal(err)
		}
	}
	active := list(owner)
	if len(active) != 1 || len(list(other)) != 0 {
		t.Fatal(active)
	}
	slug := active[0].Slug
	p, err := manager.GetForContext(owner, slug)
	if err != nil || p.Name != "Writer" || *p.Chat.MaxTokens != 123 {
		t.Fatal(p, err)
	}
	if _, err := manager.GetForContext(other, slug); err == nil {
		t.Fatal("another account resolved profile")
	}
	write(filepath.Join(dir, "writer.json"), `{"name":"Writer","maxTokens":256}`)
	row = install(row.Digest)
	if _, err := manager.GetForContext(owner, slug); err == nil {
		t.Fatal("previous profile still available")
	}
	if err := set(owner, true); err != nil {
		t.Fatal(err)
	}
	if a := list(owner); len(a) != 1 || a[0].Slug == slug {
		t.Fatal("new grant reused revoked profile", a)
	}
	if err := store.ChangeInstallation(owner, row.ID, row.Digest, "disable", admit); err != nil {
		t.Fatal(err)
	}
	if err := store.ChangeInstallation(owner, row.ID, row.Digest, "enable", admit); err != nil {
		t.Fatal(err)
	}
	if len(list(owner)) != 0 {
		t.Fatal("revived profile after re-enable")
	}
	if err := set(owner, true); err != nil {
		t.Fatal(err)
	}
	if err := store.ChangeInstallation(owner, row.ID, row.Digest, "uninstall", admit); err != nil {
		t.Fatal(err)
	}
	if len(list(owner)) != 0 {
		t.Fatal("survived uninstall")
	}
}
