package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/database"
	"assistente/internal/toolcatalog"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRepositoryTest(t *testing.T) (*DBRepository, context.Context, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.MCPServer{}, &database.MCPServerLog{}, &database.ToolCatalog{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previous)
	})
	return NewDBRepository(db), database.WithUserID(context.Background(), "user-a"), database.WithUserID(context.Background(), "user-b")
}

func TestDBRepositoryServersAreScopedByUser(t *testing.T) {
	repo, userA, userB := setupRepositoryTest(t)

	cfgA := &ServerConfig{
		Slug:        "github",
		Name:        "GitHub A",
		Transport:   TransportStreamable,
		URL:         "https://mcp.github.example/a",
		Enabled:     true,
		AutoConnect: true,
	}
	if err := repo.SaveServer(userA, cfgA); err != nil {
		t.Fatalf("save user A: %v", err)
	}

	cfgB := &ServerConfig{
		Slug:        "github",
		Name:        "GitHub B",
		Transport:   TransportStreamable,
		URL:         "https://mcp.github.example/b",
		Enabled:     true,
		AutoConnect: true,
	}
	if err := repo.SaveServer(userB, cfgB); err != nil {
		t.Fatalf("save user B: %v", err)
	}

	gotA, err := repo.GetServer(userA, "github")
	if err != nil {
		t.Fatalf("get user A: %v", err)
	}
	gotB, err := repo.GetServer(userB, "github")
	if err != nil {
		t.Fatalf("get user B: %v", err)
	}
	if gotA.ID == gotB.ID {
		t.Fatalf("servidores de usuários diferentes não deveriam compartilhar ID")
	}
	if gotA.URL != cfgA.URL || gotB.URL != cfgB.URL {
		t.Fatalf("escopo por usuário falhou: got A=%q B=%q", gotA.URL, gotB.URL)
	}
}

func TestDBRepositoryRequiresUserForServers(t *testing.T) {
	repo, _, _ := setupRepositoryTest(t)
	err := repo.SaveServer(context.Background(), &ServerConfig{Slug: "github", Name: "GitHub"})
	if err != database.ErrUserScopeRequired {
		t.Fatalf("SaveServer sem usuário: got %v, want ErrUserScopeRequired", err)
	}
}

func TestDBRepositoryRejectsReservedNativeServerSlug(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	err := repo.SaveServer(userA, &ServerConfig{Slug: "native", Name: "Native", Transport: TransportStdio})
	if err == nil {
		t.Fatal("expected reserved native slug to be rejected")
	}
}

func TestDBRepositoryDeleteServerPreservesToolCatalogRowsForJobs(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	// O catálogo de tools tem dono dedicado (internal/toolcatalog); o teste o usa
	// para criar/reanexar entradas, enquanto o MCP repo cuida do servidor.
	catalog := toolcatalog.NewService(toolcatalog.NewDBRepository(repo.db))
	if err := repo.db.AutoMigrate(&database.Job{}); err != nil {
		t.Fatalf("migrate jobs: %v", err)
	}
	if err := repo.SaveServer(userA, &ServerConfig{Slug: "jira", Name: "Jira", Transport: TransportStdio}); err != nil {
		t.Fatalf("save server: %v", err)
	}
	server, err := repo.GetServer(userA, "jira")
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	tool := database.ToolCatalog{
		UserID:             &server.UserID,
		MCPServerID:        &server.ID,
		Name:               "mcp_jira__create_issue",
		DisplayName:        "Create Issue",
		Origin:             tools.ToolOriginMCPBridge,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}
	if err := repo.db.Create(&tool).Error; err != nil {
		t.Fatalf("seed tool: %v", err)
	}
	if err := repo.db.Create(&database.Job{
		UserID:        server.UserID,
		Slug:          "sync-jira",
		Name:          "Sync Jira",
		Enabled:       true,
		ToolCatalogID: tool.ID,
		ToolName:      tool.Name,
	}).Error; err != nil {
		t.Fatalf("seed job: %v", err)
	}

	if err := repo.DeleteServer(userA, "jira"); err != nil {
		t.Fatalf("delete server: %v", err)
	}
	var got database.ToolCatalog
	if err := repo.db.First(&got, "id = ?", tool.ID).Error; err != nil {
		t.Fatalf("tool catalog row should remain: %v", err)
	}
	if got.MCPServerID != nil || got.AvailabilityStatus != tools.ToolAvailabilityUnavailable {
		t.Fatalf("tool catalog row not detached/unavailable: %#v", got)
	}
	var job database.Job
	if err := repo.db.First(&job, "slug = ?", "sync-jira").Error; err != nil {
		t.Fatalf("job should keep catalog reference: %v", err)
	}
	if job.ToolCatalogID != tool.ID || job.ToolName != tool.Name {
		t.Fatalf("job lost tool identity: %#v", job)
	}

	if err := repo.SaveServer(userA, &ServerConfig{Slug: "jira", Name: "Jira Recreated", Transport: TransportStdio}); err != nil {
		t.Fatalf("recreate server: %v", err)
	}
	recreated, err := repo.GetServer(userA, "jira")
	if err != nil {
		t.Fatalf("get recreated server: %v", err)
	}
	reattached := tools.ToolCatalogEntry{
		UserID:             recreated.UserID,
		MCPServerID:        recreated.ID,
		Name:               tool.Name,
		DisplayName:        "Create Issue",
		Origin:             tools.ToolOriginMCPBridge,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
		Schema:             json.RawMessage(`{"type":"object"}`),
	}
	if err := catalog.UpsertTool(userA, &reattached); err != nil {
		t.Fatalf("upsert reattached tool: %v", err)
	}
	if reattached.ID != tool.ID {
		t.Fatalf("recreated MCP tool should reattach detached row: got %s, want %s", reattached.ID, tool.ID)
	}
	if err := repo.db.First(&got, "id = ?", tool.ID).Error; err != nil {
		t.Fatalf("reload reattached tool: %v", err)
	}
	if got.MCPServerID == nil || *got.MCPServerID != recreated.ID || got.AvailabilityStatus != tools.ToolAvailabilityAvailable {
		t.Fatalf("tool catalog row was not reattached to recreated server: %#v", got)
	}
}

func TestDBRepositorySaveServerPersistsZeroValueUpdates(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	cfg := &ServerConfig{
		Slug:               "jira",
		Name:               "Jira",
		Description:        "old description",
		Transport:          TransportStreamable,
		URL:                "https://jira.example/mcp",
		OAuth2CallbackPort: 7777,
		Enabled:            true,
		AutoConnect:        true,
	}
	if err := repo.SaveServer(userA, cfg); err != nil {
		t.Fatalf("save initial: %v", err)
	}
	cfg.Description = ""
	cfg.URL = ""
	cfg.OAuth2CallbackPort = 0
	cfg.Enabled = false
	cfg.AutoConnect = false
	if err := repo.SaveServer(userA, cfg); err != nil {
		t.Fatalf("save update: %v", err)
	}
	got, err := repo.GetServer(userA, "jira")
	if err != nil {
		t.Fatalf("get updated: %v", err)
	}
	if got.Description != "" || got.URL != "" || got.OAuth2CallbackPort != 0 || got.Enabled || got.AutoConnect {
		t.Fatalf("zero-value update not persisted: %#v", got)
	}
}

func TestDBRepositoryDuplicateServerRejectsExistingSlug(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	source := &ServerConfig{
		Slug:      "github",
		Name:      "GitHub",
		Transport: TransportStreamable,
		URL:       "https://github.example/mcp",
	}
	if err := repo.SaveServer(userA, source); err != nil {
		t.Fatalf("save source: %v", err)
	}
	existing := &ServerConfig{
		Slug:      "github-copy",
		Name:      "Existing Copy",
		Transport: TransportStreamable,
		URL:       "https://existing.example/mcp",
	}
	if err := repo.SaveServer(userA, existing); err != nil {
		t.Fatalf("save existing target: %v", err)
	}

	if _, err := repo.DuplicateServer(userA, "github", "github-copy"); err == nil {
		t.Fatal("expected duplicate into existing slug to fail")
	}
	got, err := repo.GetServer(userA, "github-copy")
	if err != nil {
		t.Fatalf("get existing target: %v", err)
	}
	if got.Name != "Existing Copy" || got.URL != "https://existing.example/mcp" {
		t.Fatalf("existing server was overwritten: %#v", got)
	}
}

func TestDBRepositoryLogEventValidatesServerOwnership(t *testing.T) {
	repo, userA, userB := setupRepositoryTest(t)
	server := &ServerConfig{Slug: "jira", Name: "Jira", Transport: TransportStreamable, URL: "https://jira.example/mcp", Enabled: true, AutoConnect: true}
	if err := repo.SaveServer(userA, server); err != nil {
		t.Fatalf("save server: %v", err)
	}
	err := repo.LogEvent(userB, &MCPServerLog{
		ServerID: server.ID,
		Type:     "connected",
		Message:  "cross-user write",
	})
	if err == nil {
		t.Fatal("expected cross-user log write to fail")
	}
}
