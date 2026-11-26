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
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginProfileActivation{}, &database.Job{}, &database.PluginProviderBinding{}, &database.LLMProvider{}, &database.PluginMCPBinding{}, &database.MCPServer{}, &database.MCPServerLog{}, &database.ToolCatalog{}); err != nil {
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
	allow := func(_ *gorm.DB, commit func() error) error { return commit() }
	denied := errors.New("expired session")
	rejectCommit := func(tx *gorm.DB, commit func() error) error {
		if tx == nil {
			return commit()
		}
		return denied
	}
	key := "mcp/sample"
	if err := store.RegisterMCP(alice, row.ID, row.Digest, key, rejectCommit); !errors.Is(err, denied) {
		t.Fatalf("register after expiry: %v", err)
	}
	if rows, err := repo.ListServers(alice); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	if err := store.RegisterMCP(bob, row.ID, row.Digest, key, allow); err == nil {
		t.Fatal("foreign registration")
	}
	register := func() string {
		t.Helper()
		if err := store.RegisterMCP(alice, row.ID, row.Digest, key, allow); err != nil {
			t.Fatal(err)
		}
		components, err := store.InstalledComponents(alice, row.ID, row.Digest)
		if err != nil || len(components) != 1 || !components[0].Active {
			t.Fatal(components, err)
		}
		return components[0].Slug
	}
	if err := store.RegisterMCP(alice, row.ID, row.Digest, key, allow); !errors.Is(err, ErrMCPSyncPending) {
		t.Fatal(err)
	}
	stored, err := store.InstalledComponents(alice, row.ID, row.Digest)
	if err != nil || len(stored) != 1 || !stored[0].Active || len(runtime.List()) != 0 {
		t.Fatal(stored, err)
	}
	failSync = false
	if err := store.SyncMCP(alice, allow); err != nil {
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
	if err := store.RemoveMCP(bob, row.ID, row.Digest, key, false, allow); err == nil {
		t.Fatal("foreign removal")
	}
	if err := store.RemoveMCP(alice, row.ID, row.Digest, key, false, rejectCommit); !errors.Is(err, denied) {
		t.Fatalf("remove after expiry: %v", err)
	}
	if _, err := repo.GetServer(alice, slug); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteServer(alice, slug); err != nil {
		t.Fatal(err)
	}
	components, err := store.InstalledComponents(alice, row.ID, row.Digest)
	if err != nil || components[0].Active {
		t.Fatal(components, err)
	}
	slug = register()
	if err := db.Model(&database.MCPServer{}).Where("slug=?", slug).Updates(map[string]any{"name": "Edited", "url": "https://example.org/edited"}).Error; err != nil {
		t.Fatal(err)
	}
	write("README.md", "upgrade")
	upgrade, err := pluginpackage.InspectLocal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := store.BoundMCPServers(alice, row.ID, snap.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound) != 1 || !bound[0].Edited || bound[0].Name != "Edited" {
		t.Fatalf("edited registration: %+v", bound)
	}
	components, err = store.InstalledComponents(alice, row.ID, snap.Digest)
	if err != nil || len(components) != 1 || components[0].EditedServer != "Edited" {
		t.Fatal(components, err)
	}
	if err := store.ChangeInstallation(alice, row.ID, row.Digest, "disable", allow); !errors.Is(err, ErrComponentEdited) {
		t.Fatal(err)
	}
	if _, err := store.InstallReviewed(alice, pkg, upgrade.Digest, snap.Digest, bound, allow); !errors.Is(err, ErrComponentEdited) {
		t.Fatal(err)
	}
	if err := store.RemoveMCP(alice, row.ID, row.Digest, key, true, allow); err != nil {
		t.Fatal(err)
	}
	independent := runtime.List()
	if len(independent) != 1 || independent[0].Slug != slug || independent[0].URL != "https://example.org/edited" || independent[0].Name != "Edited" {
		t.Fatalf("runtime after preserving independent server: %+v", independent)
	}

	managed := register()
	if managed == slug {
		t.Fatal("replaced independent server")
	}
	bound, err = store.BoundMCPServers(alice, row.ID, snap.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InstallReviewed(alice, pkg, upgrade.Digest, snap.Digest, bound, allow); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetServer(alice, managed); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal(err)
	}
	if saved, err := repo.GetServer(alice, slug); err != nil || saved.Name != "Edited" {
		t.Fatal(saved, err)
	}
}

func TestRegisterMCPRefusesPreviewOnlyEndpoints(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginProviderBinding{}, &database.LLMProvider{}, &database.PluginMCPBinding{}, &database.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	pkg := t.TempDir()
	if err := os.WriteFile(filepath.Join(pkg, "plugin.json"), []byte(`{"$schema":"`+pluginpackage.ManifestSchema+`","name":"preview"}`), 0600); err != nil {
		t.Fatal(err)
	}
	const config = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"query":{"type":"sse","url":"https://example.org/mcp?q=test"},"headers":{"type":"streamable-http","url":"https://example.org/mcp","headers":{"X-Example":"test"}}}}`
	if err := os.WriteFile(filepath.Join(pkg, "mcp.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := pluginpackage.InspectLocal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), "owner")
	inventory := NewInventory(db)
	row, err := inventory.InstallLocal(ctx, snap.Origin, snap.Digest, "")
	if err != nil {
		t.Fatal(err)
	}
	allow := func(_ *gorm.DB, commit func() error) error { return commit() }
	for _, key := range []string{"mcp/query", "mcp/headers"} {
		if err := inventory.RegisterMCP(ctx, row.ID, row.Digest, key, allow); !errors.Is(err, ErrComponentUnavailable) {
			t.Errorf("register %s: %v", key, err)
		}
	}
	for _, model := range []any{&database.PluginProviderBinding{}, &database.LLMProvider{}, &database.PluginMCPBinding{}, &database.MCPServer{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Errorf("%T: %d, %v", model, count, err)
		}
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
