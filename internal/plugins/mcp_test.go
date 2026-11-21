package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/database"
	"assistente/internal/mcp"
	"assistente/internal/pluginpackage"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func TestPackagedMCPLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "servers.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginProfileActivation{}, &database.Job{}, &database.PluginMCPBinding{}, &database.MCPServer{}, &database.MCPServerLog{}, &database.ToolCatalog{}); err != nil {
		t.Fatal(err)
	}
	pkg := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(pkg, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("plugin.json", `{"$schema":"`+pluginpackage.ManifestSchema+`","name":"example"}`)
	write("mcp.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"sample":{"type":"streamable-http","url":"https://example.org/mcp"}}}`)
	snap, err := pluginpackage.InspectLocal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	alice := database.WithUserID(context.Background(), "alice")
	bob := database.WithUserID(context.Background(), "bob")
	repo := mcp.NewDBRepository(db)
	runtime := mcp.NewManager(nil, nil, nil)
	runtime.SetRepository(repo)
	runtime.SetAuthContextProvider(func() context.Context { return alice })
	t.Cleanup(runtime.CloseAll)
	failSync := true
	store := NewInventory(db).WithMCPSync(func(ctx context.Context) error {
		if failSync {
			return errors.New("simulated reload failure")
		}
		return runtime.ReloadPackagedConfigs(ctx)
	})
	row, err := store.InstallLocal(alice, pkg, snap.Digest, "")
	if err != nil {
		t.Fatal(err)
	}
	key := "mcp/sample"
	if err := store.RegisterMCP(bob, row.ID, row.Digest, key); err == nil {
		t.Fatal("foreign registration")
	}
	register := func() string {
		t.Helper()
		if err := store.RegisterMCP(alice, row.ID, row.Digest, key); err != nil {
			t.Fatal(err)
		}
		components, err := store.InstalledComponents(alice, row.ID, row.Digest)
		if err != nil || len(components) != 1 || !components[0].Active {
			t.Fatal(components, err)
		}
		return components[0].Slug
	}
	if err := store.RegisterMCP(alice, row.ID, row.Digest, key); !errors.Is(err, ErrMCPSyncPending) {
		t.Fatal(err)
	}
	stored, err := store.InstalledComponents(alice, row.ID, row.Digest)
	if err != nil || len(stored) != 1 || !stored[0].Active || len(runtime.List()) != 0 {
		t.Fatal(stored, err)
	}
	failSync = false
	if err := store.SyncMCP(alice); err != nil {
		t.Fatal(err)
	}
	if servers := runtime.List(); len(servers) != 1 || servers[0].Slug != stored[0].Slug || servers[0].Status != mcp.StatusDisconnected {
		t.Fatal(servers)
	}
	slug := register()
	if register() != slug {
		t.Fatal("duplicate registration")
	}
	cfg, err := repo.GetServer(alice, slug)
	if err != nil || cfg.URL != "https://example.org/mcp" || cfg.AutoConnect || !cfg.Enabled {
		t.Fatal(cfg, err)
	}
	if err := store.RemoveMCP(bob, row.ID, row.Digest, key, false); err == nil {
		t.Fatal("foreign removal")
	}
	if err := repo.DeleteServer(alice, slug); err != nil {
		t.Fatal(err)
	}
	components, err := store.InstalledComponents(alice, row.ID, row.Digest)
	if err != nil || components[0].Active {
		t.Fatal(components, err)
	}
	slug = register()
	if err := db.Model(&database.MCPServer{}).Where("slug=?", slug).Update("name", "Edited").Error; err != nil {
		t.Fatal(err)
	}
	write("README.md", "upgrade")
	upgrade, err := pluginpackage.InspectLocal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InstallLocal(alice, pkg, upgrade.Digest, snap.Digest); !errors.Is(err, ErrComponentEdited) {
		t.Fatal(err)
	}
	if err := store.RemoveMCP(alice, row.ID, row.Digest, key, true); err != nil {
		t.Fatal(err)
	}
	managed := register()
	if managed == slug {
		t.Fatal("replaced independent server")
	}
	if _, err := store.InstallLocal(alice, pkg, upgrade.Digest, snap.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetServer(alice, managed); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal(err)
	}
	if saved, err := repo.GetServer(alice, slug); err != nil || saved.Name != "Edited" {
		t.Fatal(saved, err)
	}
}

func TestPackagedMCPRejectsUnmappedEnvelope(t *testing.T) {
	for _, raw := range []string{
		`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","McpServers":{}}`,
		`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{},"other":true}`,
	} {
		components := DiscoverComponents(&pluginpackage.LocalSnapshot{Files: map[string][]byte{"mcp.json": []byte(raw)}})
		if len(components) != 1 || components[0].Kind != "invalidMcp" {
			t.Fatal(components)
		}
	}
}
