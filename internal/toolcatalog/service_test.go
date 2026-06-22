package toolcatalog

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type catalogTestTool struct {
	name string
	meta tools.CatalogMetadata
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

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1):
// cada builtin é a fonte autoritativa dos próprios metadados.
func (t catalogTestTool) CatalogMetadata() tools.CatalogMetadata { return t.meta }

func TestServiceSyncBuiltinsCatalogsGlobalToolsOnly(t *testing.T) {
	repo, userA, userB := setupCatalogTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(catalogTestTool{name: "read_file"})
	registry.MustRegisterOptIn(catalogTestTool{name: "job"})
	registry.MustRegisterDiscoverableOptIn(catalogTestTool{name: "job_pipeline"})
	registry.MustRegister(catalogTestTool{name: "mcp_jira__create_issue"})

	svc := NewService(repo)
	if err := svc.SyncBuiltins(context.Background(), registry); err != nil {
		t.Fatalf("SyncBuiltins: %v", err)
	}

	for _, ctx := range []context.Context{userA, userB} {
		entries, err := repo.ListTools(ctx, tools.ToolCatalogFilter{})
		if err != nil {
			t.Fatalf("ListTools: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("entries = %d, want 2: %#v", len(entries), entries)
		}
		got := map[string]string{}
		for _, entry := range entries {
			got[entry.Name] = entry.Origin
		}
		if got["job_pipeline"] != tools.ToolOriginBuiltin || got["read_file"] != tools.ToolOriginBuiltin {
			t.Fatalf("unexpected builtin entries: %#v", entries)
		}
		if _, ok := got["job"]; ok {
			t.Fatalf("hidden opt-in tool should not be cataloged: %#v", entries)
		}
	}
}

func TestServiceCatalogToolQueriesPersistedCatalog(t *testing.T) {
	repo, userA, _ := setupCatalogTest(t)
	readonlyMeta := tools.CatalogMetadata{Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"}
	registry := tools.NewRegistry()
	registry.MustRegister(catalogTestTool{name: "read_file", meta: readonlyMeta})
	registry.MustRegister(catalogTestTool{name: "grep_search", meta: readonlyMeta})

	svc := NewService(repo)
	if err := svc.SyncBuiltins(context.Background(), registry); err != nil {
		t.Fatalf("SyncBuiltins: %v", err)
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

	// A tool de catálogo (em internal/tools) consome este serviço via a interface
	// CatalogToolStore (ListTools), confirmando o desacoplamento do MCP.
	result, err := tools.NewCatalogTool(svc).Execute(userA, json.RawMessage(`{"package":"coding_readonly","limit":10}`))
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

func TestServiceSyncMCPServerToolsServerScopedAvailability(t *testing.T) {
	repo, userA, _ := setupCatalogTest(t)
	serverID := seedServer(t, repo, "user-a", "jira")
	svc := NewService(repo)

	initial := []MCPToolDescriptor{
		{Name: "create_issue", FullName: "mcp_jira__create_issue", Description: "Create", Schema: json.RawMessage(`{"type":"object"}`)},
		{Name: "search_issue", FullName: "mcp_jira__search_issue", Description: "Search", Schema: json.RawMessage(`{"type":"object"}`)},
	}
	if err := svc.SyncMCPServerTools(userA, "jira", serverID, "user-a", initial); err != nil {
		t.Fatalf("sync initial: %v", err)
	}
	if err := svc.SyncMCPServerTools(userA, "jira", serverID, "user-a", initial[:1]); err != nil {
		t.Fatalf("sync second: %v", err)
	}

	entries, err := repo.ListTools(userA, tools.ToolCatalogFilter{IncludeUnavailable: true})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	statusByName := map[string]string{}
	for _, entry := range entries {
		if entry.MCPServerID != serverID {
			t.Fatalf("MCPServerID = %q, want %q", entry.MCPServerID, serverID)
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

func TestServiceSyncMCPServerToolsRequiresUserScope(t *testing.T) {
	repo, userA, _ := setupCatalogTest(t)
	serverID := seedServer(t, repo, "user-a", "jira")
	svc := NewService(repo)

	err := svc.SyncMCPServerTools(context.Background(), "jira", serverID, "user-a", []MCPToolDescriptor{
		{Name: "create_issue", FullName: "mcp_jira__create_issue"},
	})
	if err != database.ErrUserScopeRequired {
		t.Fatalf("sync without user = %v, want ErrUserScopeRequired", err)
	}
	_ = userA
}
