package mcp

import (
	"context"
	"encoding/json"
	"fmt"
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
	if err := repo.db.Create(&database.ToolCatalog{
		Name:               "mcp_leaked__tool",
		DisplayName:        "leaked",
		Origin:             tools.ToolOriginMCPBridge,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("create corrupt unscoped mcp tool: %v", err)
	}

	toolsA, err := repo.ListTools(userA, tools.ToolCatalogFilter{})
	if err != nil {
		t.Fatalf("list tools user A: %v", err)
	}
	if len(toolsA) != 2 {
		t.Fatalf("user A tools = %d, want 2: %#v", len(toolsA), toolsA)
	}
	for _, entry := range toolsA {
		if entry.Name == "mcp_leaked__tool" {
			t.Fatalf("unscoped MCP tool should not be visible: %#v", toolsA)
		}
	}

	toolsB, err := repo.ListTools(userB, tools.ToolCatalogFilter{})
	if err != nil {
		t.Fatalf("list tools user B: %v", err)
	}
	if len(toolsB) != 1 || toolsB[0].Origin != tools.ToolOriginBuiltin {
		t.Fatalf("user B deve ver apenas builtin global, got %#v", toolsB)
	}
}

func TestDBRepositoryListToolsAppliesLimit(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	for _, name := range []string{"alpha", "beta"} {
		if err := repo.UpsertTool(userA, &tools.ToolCatalogEntry{
			Name:               name,
			DisplayName:        name,
			Origin:             tools.ToolOriginBuiltin,
			AvailabilityStatus: tools.ToolAvailabilityAvailable,
			Schema:             json.RawMessage(`{"type":"object"}`),
		}); err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
	}

	entries, err := repo.ListTools(userA, tools.ToolCatalogFilter{Limit: 1})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
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

func TestDBRepositoryMarksMissingServerToolsUnavailableWithLargeSeenSet(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	server := &ServerConfig{Slug: "jira", Name: "Jira", Transport: TransportStreamable, URL: "https://jira.example/mcp", Enabled: true, AutoConnect: true}
	if err := repo.SaveServer(userA, server); err != nil {
		t.Fatalf("save server: %v", err)
	}
	userID := "user-a"
	serverID := server.ID
	rows := make([]database.ToolCatalog, 0, 1006)
	seen := make([]string, 0, 1005)
	for i := 0; i < 1005; i++ {
		name := fmt.Sprintf("mcp_jira__tool_%04d", i)
		seen = append(seen, name)
		rows = append(rows, database.ToolCatalog{
			UserID:             &userID,
			MCPServerID:        &serverID,
			Name:               name,
			DisplayName:        name,
			Origin:             tools.ToolOriginMCPBridge,
			AvailabilityStatus: tools.ToolAvailabilityAvailable,
		})
	}
	rows = append(rows, database.ToolCatalog{
		UserID:             &userID,
		MCPServerID:        &serverID,
		Name:               "mcp_jira__stale",
		DisplayName:        "stale",
		Origin:             tools.ToolOriginMCPBridge,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	})
	if err := repo.db.CreateInBatches(rows, 200).Error; err != nil {
		t.Fatalf("create catalog rows: %v", err)
	}

	changed, err := repo.MarkServerToolsUnavailable(userA, server.ID, seen, "not discovered")
	if err != nil {
		t.Fatalf("mark unavailable with large seen set: %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	var stale database.ToolCatalog
	if err := repo.db.Where("name = ?", "mcp_jira__stale").First(&stale).Error; err != nil {
		t.Fatalf("load stale row: %v", err)
	}
	if stale.AvailabilityStatus != tools.ToolAvailabilityUnavailable {
		t.Fatalf("stale status = %q", stale.AvailabilityStatus)
	}
}

func TestDBRepositoryToolUpdatesAreScopedByServerOwner(t *testing.T) {
	repo, userA, userB := setupRepositoryTest(t)
	serverA := &ServerConfig{Slug: "jira", Name: "Jira A", Transport: TransportStreamable, URL: "https://jira-a.example/mcp", Enabled: true, AutoConnect: true}
	if err := repo.SaveServer(userA, serverA); err != nil {
		t.Fatalf("save server A: %v", err)
	}
	if err := repo.UpsertTool(userA, &tools.ToolCatalogEntry{
		Name:               "mcp_jira__create_issue",
		DisplayName:        "create_issue",
		Origin:             tools.ToolOriginMCPBridge,
		MCPServerID:        serverA.ID,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
		Schema:             json.RawMessage(`{"type":"object"}`),
	}); err != nil {
		t.Fatalf("upsert user A tool: %v", err)
	}
	err := repo.UpsertTool(userB, &tools.ToolCatalogEntry{
		Name:               "mcp_jira__create_issue",
		DisplayName:        "create_issue",
		Origin:             tools.ToolOriginMCPBridge,
		MCPServerID:        serverA.ID,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
		Schema:             json.RawMessage(`{"type":"object"}`),
	})
	if err == nil {
		t.Fatal("expected cross-user MCP tool upsert to fail")
	}
	if _, err := repo.MarkServerToolsUnavailable(userB, serverA.ID, nil, "not discovered"); err == nil {
		t.Fatal("expected cross-user unavailable update to fail")
	}
}

func TestDBRepositoryUpsertToolPersistsZeroValueUpdates(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	server := &ServerConfig{Slug: "jira", Name: "Jira", Transport: TransportStreamable, URL: "https://jira.example/mcp", Enabled: true, AutoConnect: true}
	if err := repo.SaveServer(userA, server); err != nil {
		t.Fatalf("save server: %v", err)
	}
	entry := &tools.ToolCatalogEntry{
		Name:               "mcp_jira__create_issue",
		DisplayName:        "create_issue",
		Description:        "old",
		Origin:             tools.ToolOriginMCPBridge,
		MCPServerID:        server.ID,
		AvailabilityStatus: tools.ToolAvailabilityUnavailable,
		AvailabilityReason: "missing",
		LastTestStatus:     tools.ToolTestStatusError,
		LastTestError:      "boom",
		Schema:             json.RawMessage(`{"type":"object"}`),
	}
	if err := repo.UpsertTool(userA, entry); err != nil {
		t.Fatalf("upsert initial: %v", err)
	}
	var initial database.ToolCatalog
	if err := repo.db.Where("name = ?", entry.Name).First(&initial).Error; err != nil {
		t.Fatalf("load initial tool: %v", err)
	}
	if initial.LastUnavailableAt == nil {
		t.Fatal("LastUnavailableAt should be set for unavailable upsert")
	}
	entry.Description = ""
	entry.AvailabilityStatus = tools.ToolAvailabilityAvailable
	entry.AvailabilityReason = ""
	entry.LastTestStatus = tools.ToolTestStatusOK
	entry.LastTestError = ""
	if err := repo.UpsertTool(userA, entry); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	entries, err := repo.ListTools(userA, tools.ToolCatalogFilter{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var found tools.ToolCatalogEntry
	for _, got := range entries {
		if got.Name == entry.Name {
			found = got
		}
	}
	if found.Description != "" || found.AvailabilityReason != "" || found.LastTestError != "" || found.LastTestStatus != tools.ToolTestStatusOK {
		t.Fatalf("zero-value tool update not persisted: %#v", found)
	}
}

func TestDBRepositoryRecordsToolTestResults(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	if err := repo.UpsertTool(context.Background(), &tools.ToolCatalogEntry{
		Name:               "read_file",
		DisplayName:        "read_file",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
		Schema:             json.RawMessage(`{"type":"object"}`),
	}); err != nil {
		t.Fatalf("upsert builtin: %v", err)
	}
	if err := repo.RecordToolTest(context.Background(), "read_file", tools.ToolTestStatusOK, ""); err != nil {
		t.Fatalf("record builtin test: %v", err)
	}
	entries, err := repo.ListTools(userA, tools.ToolCatalogFilter{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if entries[0].LastTestStatus != tools.ToolTestStatusOK || entries[0].LastTestedAt == nil {
		t.Fatalf("unexpected builtin test metadata: %#v", entries[0])
	}

	server := &ServerConfig{Slug: "jira", Name: "Jira", Transport: TransportStreamable, URL: "https://jira.example/mcp", Enabled: true, AutoConnect: true}
	if err := repo.SaveServer(userA, server); err != nil {
		t.Fatalf("save server: %v", err)
	}
	if err := repo.UpsertTool(userA, &tools.ToolCatalogEntry{
		Name:               "mcp_jira__create_issue",
		DisplayName:        "create_issue",
		Origin:             tools.ToolOriginMCPBridge,
		MCPServerID:        server.ID,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
		Schema:             json.RawMessage(`{"type":"object"}`),
	}); err != nil {
		t.Fatalf("upsert mcp: %v", err)
	}
	if err := repo.RecordToolTest(userA, "mcp_jira__create_issue", tools.ToolTestStatusError, "boom"); err != nil {
		t.Fatalf("record mcp test: %v", err)
	}
	entries, err = repo.ListTools(userA, tools.ToolCatalogFilter{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var found tools.ToolCatalogEntry
	for _, entry := range entries {
		if entry.Name == "mcp_jira__create_issue" {
			found = entry
		}
	}
	if found.LastTestStatus != tools.ToolTestStatusError || found.LastTestError != "boom" {
		t.Fatalf("unexpected mcp test metadata: %#v", found)
	}
}

func TestDBRepositoryRequiresUserForMCPToolTestResults(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	server := &ServerConfig{Slug: "jira", Name: "Jira", Transport: TransportStreamable, URL: "https://jira.example/mcp", Enabled: true, AutoConnect: true}
	if err := repo.SaveServer(userA, server); err != nil {
		t.Fatalf("save server: %v", err)
	}
	if err := repo.UpsertTool(userA, &tools.ToolCatalogEntry{
		Name:               "mcp_jira__create_issue",
		DisplayName:        "create_issue",
		Origin:             tools.ToolOriginMCPBridge,
		MCPServerID:        server.ID,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
		Schema:             json.RawMessage(`{"type":"object"}`),
	}); err != nil {
		t.Fatalf("upsert mcp: %v", err)
	}
	err := repo.RecordToolTest(context.Background(), "mcp_jira__create_issue", tools.ToolTestStatusOK, "")
	if err != database.ErrUserScopeRequired {
		t.Fatalf("record without user = %v, want ErrUserScopeRequired", err)
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
