package wailsapi

import (
	"assistente/internal/plugincatalog"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/plugins"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func TestPluginPreviewUsesSelectedDirectoryAndSession(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"$schema":"`+pluginpackage.ManifestSchema+`","name":"local"}`), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "plugins.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginProfileActivation{}, &database.Job{}, &database.PluginMCPBinding{}, &database.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	inventory := plugins.NewInventory(db)
	api := NewPlugins()
	active := true
	missing := errors.New("stale")
	ctx := database.WithUserID(context.Background(), "owner")
	session := stubSession{ctx: ctx}
	AttachPlugins(api, session, inventory, func() (string, error) { return dir, nil }, func(context.Context) (commandsecurity.Admission, error) {
		return func(handoff func() error) error {
			if !active {
				return missing
			}
			return handoff()
		}, nil
	})
	preview, err := api.PreviewLocal()
	if err != nil || preview.Name != "local" || len(preview.Files) != 1 {
		t.Fatal(preview, err)
	}
	if _, err := api.InstallReviewed("foreign-token"); err == nil {
		t.Fatal("accepted foreign token")
	}
	if err := api.DiscardPreview(preview.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := api.InstallReviewed(preview.Token); err == nil {
		t.Fatal("installed discarded selection")
	}
	preview, err = api.PreviewLocal()
	if err != nil {
		t.Fatal(err)
	}
	active = false
	if _, err := api.InstallReviewed(preview.Token); !errors.Is(err, missing) {
		t.Fatal(err)
	}
	active = true
	rows, err := api.ListInstalled()
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	p, err := api.PreviewLocal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.InstallReviewed(p.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := api.InstallReviewed(p.Token); err == nil {
		t.Fatal("replayed selection")
	}
	rows, err = api.ListInstalled()
	if err != nil || len(rows) != 1 || rows[0].Digest != p.Digest {
		t.Fatal(rows, err)
	}
}

func TestLocalCatalogSurvivesFailedRefreshAndRemoval(t *testing.T) {
	directory := t.TempDir()
	write := func(name, data string) {
		t.Helper()
		target := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(".claude-plugin/marketplace.json", `{"name":"demo","owner":{"name":"publisher"},"plugins":[{"name":"sample","source":"./plugins/sample"}]}`)
	write("plugins/sample/plugin.json", `{"$schema":"`+pluginpackage.ManifestSchema+`","name":"sample"}`)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "repositories.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.PluginRepository{}, &database.PluginInstallation{}, &database.PluginRevision{}); err != nil {
		t.Fatal(err)
	}
	inventory := plugins.NewInventory(db)
	user := database.WithUserID(context.Background(), "alice")
	api := NewPlugins()
	admission := commandsecurity.Admission(func(fn func() error) error { return fn() })
	AttachPlugins(api, stubSession{ctx: user}, inventory, func() (string, error) { return directory, nil }, func(context.Context) (commandsecurity.Admission, error) { return admission, nil })
	if err := api.AddLocalRepository(); err != nil {
		t.Fatal(err)
	}
	repos, err := api.ListRepositories()
	if err != nil || len(repos) != 1 {
		t.Fatal(repos, err)
	}
	repo := repos[0]
	if foreign, err := inventory.Repositories(database.WithUserID(context.Background(), "bob")); err != nil || len(foreign) != 0 {
		t.Fatal(foreign, err)
	}
	preview, err := api.PreviewRepositoryEntry(repo.ID, repo.Revision, string(plugincatalog.Claude), "sample")
	if err != nil {
		t.Fatal(err)
	}
	row, err := api.InstallReviewed(preview.Token)
	if err != nil || row.DiscoveryRepositoryID != repo.ID {
		t.Fatal(row, err)
	}
	write(".claude-plugin/marketplace.json", "invalid")
	if err := api.RefreshRepository(repo.ID, repo.Revision); err == nil {
		t.Fatal("replaced cache with invalid index")
	}
	repos, err = api.ListRepositories()
	if err != nil || len(repos) != 1 || repos[0].Revision != repo.Revision {
		t.Fatal(repos, err)
	}
	// Simulate removal after the anchored read and before the publication's writer lock.
	_, err = inventory.InstallSource(user, preview.Origin, preview.Digest, preview.Digest, admission, func() (*pluginpackage.LocalSnapshot, error) {
		snap, err := inventory.InspectRepositoryEntry(user, repo.ID, repo.Revision, plugincatalog.Claude, "sample")
		if err != nil {
			return nil, err
		}
		if err := api.RemoveRepository(repo.ID, repo.Revision); err != nil {
			t.Fatal(err)
		}
		return snap, nil
	})
	if err == nil {
		t.Fatal("publication ignored removed repository")
	}
	if err := api.RefreshRepository(repo.ID, repo.Revision); err == nil {
		t.Fatal("recreated removed repository")
	}
	rows, err := api.ListInstalled()
	if err != nil || len(rows) != 1 || rows[0].ID != row.ID {
		t.Fatal(rows, err)
	}
}
