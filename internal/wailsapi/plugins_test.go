package wailsapi

import (
	"archive/zip"
	"assistente/internal/core/ports"
	"assistente/internal/plugincatalog"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/plugins"
	"assistente/internal/sqlite"
	"assistente/internal/userctx"
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
	if err := db.AutoMigrate(&database.PluginRepository{}, &database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.Job{}, &database.PluginProfileActivation{}, &database.PluginProviderBinding{}, &database.LLMProvider{}, &database.PluginMCPBinding{}, &database.MCPServer{}, &database.MCPServerLog{}, &database.ToolCatalog{}); err != nil {
		t.Fatal(err)
	}
	var syncErr error
	inventory := plugins.NewInventory(db).WithMCPSync(func(context.Context) error { return syncErr })
	api := NewPlugins()
	active := true
	missing := errors.New("stale")
	ctx := database.WithUserID(context.Background(), "owner")
	session := stubSession{ctx: userctx.WithSessionID(ctx, "session")}
	var notifications int
	AttachPlugins(api, session, inventory, func() (string, error) { return dir, nil }, func(context.Context) (commandsecurity.Admission, error) {
		return func(_ *gorm.DB, handoff func() error) error {
			if !active {
				return missing
			}
			return handoff()
		}, nil
	}, func() { notifications++ })
	t.Run("ZIP alterado requer outra prévia", func(t *testing.T) {
		filename := filepath.Join(dir, "review.zip")
		writeRevision := func(version string) {
			t.Helper()
			var data bytes.Buffer
			w := zip.NewWriter(&data)
			f, err := w.Create("plugin.json")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.Write([]byte(`{"$schema":"` + pluginpackage.ManifestSchema + `","name":"test","version":"` + version + `"}`)); err != nil {
				t.Fatal(err)
			}
			if err = w.Close(); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filename, data.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
		AttachPluginDialogs(api, func(ports.OpenFileOptions) (string, error) { return filename, nil }, nil)
		writeRevision("1")
		p, err := api.PreviewLocalZIP()
		if err != nil {
			t.Fatal(err)
		}
		writeRevision("2")
		if _, err = api.InstallReviewed(p.Token); !errors.Is(err, plugins.ErrPackageChanged) {
			t.Fatalf("expected changed revision, got %v", err)
		}
		if err = os.Remove(filename); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cadastra loja ZIP com a seleção já revisada", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "market.zip")
		writeZIP := func(version string) {
			t.Helper()
			var buf bytes.Buffer
			w := zip.NewWriter(&buf)
			for name, contents := range map[string]string{
				".claude-plugin/marketplace.json": `{"name":"store","owner":{"name":"author"},"plugins":[{"name":"demo","source":"./demo"}]}`,
				"demo/plugin.json":                `{"$schema":"` + pluginpackage.ManifestSchema + `","name":"demo","version":"` + version + `"}`,
			} {
				member, err := w.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = member.Write([]byte(contents)); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, buf.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
		calls := 0
		AttachPluginDialogs(api, func(ports.OpenFileOptions) (string, error) { calls++; return file, nil }, nil)
		var malformed bytes.Buffer
		z := zip.NewWriter(&malformed)
		entry, err := z.Create(".cursor-plugin/marketplace.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte("{")); err != nil {
			t.Fatal(err)
		}
		if err = z.Close(); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, malformed.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = api.PreviewLocalZIP(); err == nil || errors.Is(err, pluginpackage.ErrMissingManifest) {
			t.Fatal("lost catalog error", err)
		}
		calls = 0
		writeZIP("1")
		p, err := api.PreviewLocalZIP()
		if err != nil || !p.Repository {
			t.Fatal(p, err)
		}
		if _, err = api.InstallReviewed(p.Token); err == nil {
			t.Fatal("catalog treated as plugin")
		}
		writeZIP("2")
		if err = api.AddReviewedRepository(p.Token); !errors.Is(err, plugins.ErrPackageChanged) {
			t.Fatal(err)
		}
		p, err = api.PreviewLocalZIP()
		if err != nil {
			t.Fatal(err)
		}
		if err = api.AddReviewedRepository(p.Token); err != nil || calls != 2 {
			t.Fatal(calls, err)
		}
		rows, err := api.ListRepositories()
		if err != nil || len(rows) != 1 {
			t.Fatal(rows, err)
		}
		row := rows[0]
		check, err := api.PreviewRepositoryEntry(row.ID, row.Revision, "claude-code", "demo")
		if err != nil || check.Version != "2" {
			t.Fatal(check, err)
		}
		writeZIP("3")
		if _, err = api.InstallReviewed(check.Token); !errors.Is(err, plugins.ErrPackageChanged) {
			t.Fatal(err)
		}
		if err = api.RefreshRepository(row.ID, row.Revision); err != nil {
			t.Fatal(err)
		}
		if err = api.AddReviewedRepository(p.Token); err == nil {
			t.Fatal("accepted consumed preview")
		}
		rows, err = api.ListRepositories()
		if err != nil || len(rows) != 1 || rows[0].Revision == row.Revision {
			t.Fatal(rows, err)
		}
		if err = api.RemoveRepository(row.ID, rows[0].Revision); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("não substitui índice selecionado por outro formato", func(t *testing.T) {
		store := t.TempDir()
		body := []byte(`{"name":"store","owner":{"name":"author"},"plugins":[]}`)
		for _, dir := range []string{".claude-plugin", ".cursor-plugin"} {
			if err := os.Mkdir(filepath.Join(store, dir), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(store, dir, "marketplace.json"), body, 0600); err != nil {
				t.Fatal(err)
			}
		}
		selected := filepath.Join(store, ".claude-plugin", "marketplace.json")
		AttachPluginDialogs(api, func(ports.OpenFileOptions) (string, error) {
			if err := os.Remove(selected); err != nil {
				t.Fatal(err)
			}
			return selected, nil
		}, nil)
		if saved, err := api.AddIndexRepository(); err == nil || saved {
			t.Fatal(saved, err)
		}
		rows, err := api.ListRepositories()
		if err != nil || len(rows) != 0 {
			t.Fatal(rows, err)
		}
	})

	syncErr = errors.New("runtime unavailable")
	for _, wrongSession := range []string{"", "previous"} {
		if err := api.SyncMCP(wrongSession); !errors.Is(err, database.ErrUserScopeRequired) {
			t.Fatalf("unexpected sync in another session: %v", err)
		}
	}
	if err := api.SyncMCP("session"); !errors.Is(err, syncErr) {
		t.Fatal(err)
	}
	syncErr = nil
	var pickerCalls atomic.Int32
	api.choose = func() (string, error) {
		pickerCalls.Add(1)
		return dir, nil
	}
	// Hold the old admission until the next page has received its own preview.
	var first atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	capture := api.capture
	api.capture = func(ctx context.Context) (commandsecurity.Admission, error) {
		if !first.Swap(true) {
			close(entered)
			<-release
		}
		return capture(ctx)
	}
	old := make(chan error, 1)
	go func() { _, err := api.PreviewLocal(); old <- err }()
	<-entered
	current, err := api.PreviewLocal()
	close(release)
	if oldErr := <-old; !errors.Is(oldErr, errPreviewSuperseded) || err != nil || pickerCalls.Load() != 1 {
		t.Fatalf("old error %v, current error %v, picker calls %d", oldErr, err, pickerCalls.Load())
	}
	api.mu.Lock()
	currentSelection := api.selection
	api.mu.Unlock()
	if currentSelection == nil || currentSelection.preview.Token != current.Token {
		t.Fatal("late preview replaced the current selection")
	}
	// A picker that was already open when superseded must not inspect its result.
	pickerEntered, pickerRelease := make(chan struct{}), make(chan struct{})
	first.Store(false)
	api.capture = capture
	api.choose = func() (string, error) {
		if !first.Swap(true) {
			close(pickerEntered)
			<-pickerRelease
			return filepath.Join(dir, "removed"), nil
		}
		return dir, nil
	}
	go func() { _, err := api.PreviewLocal(); old <- err }()
	<-pickerEntered
	_, err = api.PreviewLocal()
	close(pickerRelease)
	if oldErr := <-old; !errors.Is(oldErr, errPreviewSuperseded) || err != nil {
		t.Fatalf("superseded picker: %v, current: %v", oldErr, err)
	}
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
	if notifications != 0 {
		t.Fatalf("unexpected catalog event before any activation: %d", notifications)
	}
	skillDir := filepath.Join(dir, "skills", "note")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: note\ndescription: Instruções\n---\nTexto"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"sample":{"type":"sse","url":"https://example.org/mcp"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	updated, err := api.PreviewLocal()
	if err != nil {
		t.Fatal(err)
	}
	installed, err := api.InstallReviewed(updated.Token)
	if err != nil || notifications != 1 {
		t.Fatal(err, notifications)
	}
	if err := api.SetSkillActive(installed.ID, p.Digest, "skills/note", true); err == nil || notifications != 1 {
		t.Fatal(err, notifications)
	}
	for index, enable := range []bool{true, false} {
		if err := api.SetSkillActive(installed.ID, installed.Digest, "skills/note", enable); err != nil || notifications != index+2 {
			t.Fatal(err, notifications)
		}
	}
	if err := api.RegisterMCP(installed.ID, installed.Digest, "mcp/sample"); err != nil || notifications != 3 {
		t.Fatal(err, notifications)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.MCPServer{}).Where("name = ?", "sample").Update("name", "Renamed").Error; err != nil {
		t.Fatal(err)
	}
	edited, err := api.PreviewLocal()
	if err != nil || len(edited.EditedMCPServers) != 1 || edited.EditedMCPServers[0] != "sample (Renamed)" {
		t.Fatal(edited, err)
	}
	if err := db.Model(&database.MCPServer{}).Where("name = ?", "Renamed").Update("name", "sample").Error; err != nil {
		t.Fatal(err)
	}
	revised, err := api.PreviewLocal()
	if err != nil {
		t.Fatal(err)
	}
	if len(revised.AffectedMCPServers) != 1 || revised.AffectedMCPServers[0] != "sample" {
		t.Fatalf("missing impact: %+v", revised)
	}
	if err := api.RemoveMCP(installed.ID, installed.Digest, "mcp/sample", true); err != nil {
		t.Fatal(err)
	}
	if err := api.RegisterMCP(installed.ID, installed.Digest, "mcp/sample"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.InstallReviewed(revised.Token); !errors.Is(err, plugins.ErrInstallationChanged) {
		t.Fatalf("changed identity: %v", err)
	}
	revised, err = api.PreviewLocal()
	if err != nil {
		t.Fatal(err)
	}
	syncErr = errors.New("runtime offline")
	if _, err := api.InstallReviewed(revised.Token); !errors.Is(err, plugins.ErrMCPSyncPending) || notifications != 4 {
		t.Fatal(err, notifications)
	}

	syncErr = nil
	if err := api.ChangeInstallation(installed.ID, p.Digest, "disable"); err == nil || notifications != 4 {
		t.Fatal(err, notifications)
	}
	if err := api.ChangeInstallation(installed.ID, revised.Digest, "disable"); err != nil || notifications != 5 {
		t.Fatal(err, notifications)
	}
	if err := api.ChangeInstallation(installed.ID, revised.Digest, "enable"); err != nil || notifications != 5 {
		t.Fatal(err, notifications)
	}
	if err := api.RegisterMCP(installed.ID, revised.Digest, "mcp/sample"); err != nil {
		t.Fatal(err)
	}
	syncErr = errors.New("runtime offline")
	if err := api.ChangeInstallation(installed.ID, revised.Digest, "uninstall"); !errors.Is(err, plugins.ErrMCPSyncPending) || notifications != 6 {
		t.Fatal(err, notifications)
	}
}

func TestSkillCatalogNotifiedWhenOnlyJobSyncFails(t *testing.T) {
	for _, action := range []string{"update", "disable", "uninstall"} {
		t.Run(action, func(t *testing.T) {
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
			if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginProfileActivation{}, &database.PluginProviderBinding{}, &database.LLMProvider{}, &database.PluginMCPBinding{}, &database.MCPServer{}, &database.Job{}); err != nil {
				t.Fatal(err)
			}
			inventory := plugins.NewInventory(db)
			ctx := database.WithUserID(t.Context(), "owner")
			api := NewPlugins()
			notifications := 0
			AttachPlugins(api, stubSession{ctx: userctx.WithSessionID(ctx, "session")}, inventory, func() (string, error) { return dir, nil }, func(context.Context) (commandsecurity.Admission, error) {
				return func(_ *gorm.DB, commit func() error) error { return commit() }, nil
			}, func() { notifications++ })
			preview, err := api.PreviewLocal()
			if err != nil {
				t.Fatal(err)
			}
			row, err := api.InstallReviewed(preview.Token)
			if err != nil || notifications != 0 {
				t.Fatal(err, notifications)
			}
			job := database.Job{UserID: "owner", Slug: "managed", PluginInstallationID: row.ID, PluginDigest: row.Digest}
			if err := db.Create(&job).Error; err != nil {
				t.Fatal(err)
			}
			if action == "update" {
				if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("new version"), 0600); err != nil {
					t.Fatal(err)
				}
				preview, err = api.PreviewLocal()
				if err != nil {
					t.Fatal(err)
				}
				_, err = api.InstallReviewed(preview.Token)
			} else {
				err = api.ChangeInstallation(row.ID, row.Digest, action)
			}
			if !errors.Is(err, plugins.ErrJobSyncPending) || errors.Is(err, plugins.ErrMCPSyncPending) {
				t.Fatalf("expected only job sync failure after %s, got %v", action, err)
			}
			if notifications != 1 {
				t.Fatalf("expected one catalog notification after commit, got %d", notifications)
			}
		})
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
	admission := commandsecurity.Admission(func(_ *gorm.DB, fn func() error) error { return fn() })
	AttachPlugins(api, stubSession{ctx: user}, inventory, func() (string, error) { return directory, nil }, func(context.Context) (commandsecurity.Admission, error) { return admission, nil }, nil)
	if added, err := api.AddLocalRepository(); err != nil || !added {
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
	_, err = inventory.InstallSource(user, preview.Origin, preview.Digest, preview.Digest, nil, admission, func() (*pluginpackage.LocalSnapshot, error) {
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
