package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type catalogTestTool struct {
	name string
}

func (t catalogTestTool) Name() string { return t.name }
func (t catalogTestTool) Description() string {
	return "test tool"
}
func (t catalogTestTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t catalogTestTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "ok"}, nil
}

func TestSyncBuiltinToolsCatalogsGlobalToolsOnly(t *testing.T) {
	repo, userA, userB := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(catalogTestTool{name: "read_file"})
	registry.MustRegisterOptIn(catalogTestTool{name: "job"})
	registry.MustRegister(catalogTestTool{name: "mcp_jira__create_issue"})

	m := NewManager(registry, nil, nil)
	m.SetRepository(repo)
	if err := m.SyncBuiltinTools(context.Background()); err != nil {
		t.Fatalf("SyncBuiltinTools: %v", err)
	}

	for _, ctx := range []context.Context{userA, userB} {
		entries, err := repo.ListTools(ctx, tools.ToolCatalogFilter{})
		if err != nil {
			t.Fatalf("ListTools: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("entries = %d, want 1: %#v", len(entries), entries)
		}
		if entries[0].Name != "read_file" || entries[0].Origin != tools.ToolOriginBuiltin {
			t.Fatalf("unexpected builtin entry: %#v", entries[0])
		}
	}
}

func TestCatalogToolQueriesPersistedRepository(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(catalogTestTool{name: "read_file"})
	registry.MustRegister(catalogTestTool{name: "grep_search"})

	m := NewManager(registry, nil, nil)
	m.SetRepository(repo)
	if err := m.SyncBuiltinTools(context.Background()); err != nil {
		t.Fatalf("SyncBuiltinTools: %v", err)
	}
	if err := repo.UpsertTool(context.Background(), &tools.ToolCatalogEntry{
		Name:               "old_search",
		DisplayName:        "old_search",
		Description:        "Unavailable search",
		Origin:             tools.ToolOriginBuiltin,
		Category:           "filesystem",
		Class:              "read_context",
		Package:            "coding_readonly",
		Risk:               "read",
		Schema:             json.RawMessage(`{"type":"object"}`),
		AvailabilityStatus: tools.ToolAvailabilityUnavailable,
		AvailabilityReason: "removed",
	}); err != nil {
		t.Fatalf("upsert unavailable builtin: %v", err)
	}

	result, err := tools.NewCatalogTool(repo).Execute(userA, json.RawMessage(`{"package":"coding_readonly","limit":10}`))
	if err != nil {
		t.Fatalf("catalog execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("catalog returned error: %s", result.Content)
	}
	var payload struct {
		SelectedTools []string `json:"selected_tools"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode catalog response: %v", err)
	}
	got := map[string]bool{}
	for _, name := range payload.SelectedTools {
		got[name] = true
	}
	if !got["read_file"] || !got["grep_search"] {
		t.Fatalf("catalog did not return synced builtin tools: %#v", payload.SelectedTools)
	}
	if got["old_search"] {
		t.Fatalf("catalog returned unavailable tool without include_unavailable: %#v", payload.SelectedTools)
	}
}

func TestSyncMCPToolsCatalogsServerScopedAvailability(t *testing.T) {
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
		t.Fatalf("SaveServer: %v", err)
	}

	m := NewManager(tools.NewRegistry(), nil, nil)
	m.SetRepository(repo)
	m.servers["jira"] = &ServerStatus{
		ID:     server.ID,
		Slug:   "jira",
		Config: *server,
		Status: StatusConnected,
	}

	initial := []MCPToolInfo{
		{Name: "create_issue", FullName: "mcp_jira__create_issue", Description: "Create", Schema: json.RawMessage(`{"type":"object"}`), ServerSlug: "jira"},
		{Name: "search_issue", FullName: "mcp_jira__search_issue", Description: "Search", Schema: json.RawMessage(`{"type":"object"}`), ServerSlug: "jira"},
	}
	if err := m.syncMCPTools(userA, "jira", initial); err != nil {
		t.Fatalf("sync initial: %v", err)
	}
	if err := m.syncMCPTools(userA, "jira", initial[:1]); err != nil {
		t.Fatalf("sync second: %v", err)
	}

	entries, err := repo.ListTools(userA, tools.ToolCatalogFilter{IncludeUnavailable: true})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	statusByName := map[string]string{}
	for _, entry := range entries {
		if entry.MCPServerID != server.ID {
			t.Fatalf("MCPServerID = %q, want %q", entry.MCPServerID, server.ID)
		}
		statusByName[entry.Name] = entry.AvailabilityStatus
	}
	if statusByName["mcp_jira__create_issue"] != tools.ToolAvailabilityAvailable {
		t.Fatalf("create_issue status = %q", statusByName["mcp_jira__create_issue"])
	}
	if statusByName["mcp_jira__search_issue"] != tools.ToolAvailabilityUnavailable {
		t.Fatalf("search_issue status = %q", statusByName["mcp_jira__search_issue"])
	}
}

func TestSyncMCPToolsRequiresUserScope(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	server := &ServerConfig{Slug: "jira", Name: "Jira", Transport: TransportStreamable, URL: "https://jira.example/mcp", Enabled: true, AutoConnect: true}
	if err := repo.SaveServer(userA, server); err != nil {
		t.Fatalf("SaveServer: %v", err)
	}

	m := NewManager(tools.NewRegistry(), nil, nil)
	m.SetRepository(repo)
	m.servers["jira"] = &ServerStatus{ID: server.ID, Slug: "jira", Config: *server}

	err := m.syncMCPTools(context.Background(), "jira", []MCPToolInfo{{Name: "create_issue", FullName: "mcp_jira__create_issue"}})
	if err != database.ErrUserScopeRequired {
		t.Fatalf("sync without user = %v, want ErrUserScopeRequired", err)
	}
}
