package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/database"
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

func TestDBRepositoryToolCatalogBuiltinAndMCPVisibility(t *testing.T) {
	repo, userA, userB := setupRepositoryTest(t)

	if err := repo.UpsertTool(context.Background(), &tools.ToolCatalogEntry{
		Name:               "read_file",
		DisplayName:        "read_file",
		Description:        "Read a file",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
		Schema:             json.RawMessage(`{"type":"object"}`),
	}); err != nil {
		t.Fatalf("upsert builtin: %v", err)
	}

	server := &ServerConfig{
		Slug:        "jira",
		Name:        "Jira",
		Transport:   TransportStreamable,
		URL:         "https://jira.example/mcp",
		Enabled:     true,
		AutoConnect: true,
	}
	if err := repo.SaveServer(userA, server); err != nil {
		t.Fatalf("save server: %v", err)
	}
	if err := repo.UpsertTool(userA, &tools.ToolCatalogEntry{
		Name:               "mcp_jira__create_issue",
		DisplayName:        "create_issue",
		Description:        "Create Jira issue",
		Origin:             tools.ToolOriginMCPBridge,
		MCPServerID:        server.ID,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
		Schema:             json.RawMessage(`{"type":"object"}`),
	}); err != nil {
		t.Fatalf("upsert mcp tool: %v", err)
	}

	toolsA, err := repo.ListTools(userA, tools.ToolCatalogFilter{})
	if err != nil {
		t.Fatalf("list tools user A: %v", err)
	}
	if len(toolsA) != 2 {
		t.Fatalf("user A tools = %d, want 2", len(toolsA))
	}

	toolsB, err := repo.ListTools(userB, tools.ToolCatalogFilter{})
	if err != nil {
		t.Fatalf("list tools user B: %v", err)
	}
	if len(toolsB) != 1 || toolsB[0].Origin != tools.ToolOriginBuiltin {
		t.Fatalf("user B deve ver apenas builtin global, got %#v", toolsB)
	}
}

func TestDBRepositoryMarksMissingServerToolsUnavailable(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)

	server := &ServerConfig{
		Slug:        "jira",
		Name:        "Jira",
		Transport:   TransportStreamable,
		URL:         "https://jira.example/mcp",
		Enabled:     true,
		AutoConnect: true,
	}
	if err := repo.SaveServer(userA, server); err != nil {
		t.Fatalf("save server: %v", err)
	}
	for _, name := range []string{"mcp_jira__create_issue", "mcp_jira__search_issue"} {
		if err := repo.UpsertTool(userA, &tools.ToolCatalogEntry{
			Name:               name,
			DisplayName:        name,
			Origin:             tools.ToolOriginMCPBridge,
			MCPServerID:        server.ID,
			AvailabilityStatus: tools.ToolAvailabilityAvailable,
			Schema:             json.RawMessage(`{"type":"object"}`),
		}); err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
	}

	changed, err := repo.MarkServerToolsUnavailable(userA, server.ID, []string{"mcp_jira__create_issue"}, "not discovered")
	if err != nil {
		t.Fatalf("mark unavailable: %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}

	all, err := repo.ListTools(userA, tools.ToolCatalogFilter{IncludeUnavailable: true})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	statusByName := map[string]string{}
	for _, entry := range all {
		statusByName[entry.Name] = entry.AvailabilityStatus
	}
	if statusByName["mcp_jira__create_issue"] != tools.ToolAvailabilityAvailable {
		t.Fatalf("create_issue deveria continuar available, got %q", statusByName["mcp_jira__create_issue"])
	}
	if statusByName["mcp_jira__search_issue"] != tools.ToolAvailabilityUnavailable {
		t.Fatalf("search_issue deveria ficar unavailable, got %q", statusByName["mcp_jira__search_issue"])
	}
}
