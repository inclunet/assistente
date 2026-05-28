package app

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/controllers"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/mcp"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type appDryRunMCPTool struct{}

func (appDryRunMCPTool) Name() string                { return "mcp_jira__create_issue" }
func (appDryRunMCPTool) Description() string         { return "create issue" }
func (appDryRunMCPTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (appDryRunMCPTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: `{"created":true}`}, nil
}

func TestAppTestToolDryRun_ResolvesMCPByServerAndToolName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.MCPServer{}, &database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	prev := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(prev) })

	userID := "user-app"
	ctx := database.WithUserID(context.Background(), userID)
	if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: userID}, Username: "user-app"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&database.MCPServer{
		UUIDModel: database.UUIDModel{ID: "srv-1"},
		UserID:    userID,
		Slug:      "jira",
		Name:      "Jira",
		Transport: "stdio",
		Enabled:   true,
	}).Error; err != nil {
		t.Fatalf("seed server: %v", err)
	}

	registry := tools.NewRegistry()
	registry.MustRegister(appDryRunMCPTool{})
	mcpRepo := mcp.NewDBRepository(db)
	if err := mcpRepo.UpsertTool(ctx, &tools.ToolCatalogEntry{
		MCPServerID:        "srv-1",
		Name:               "mcp_jira__create_issue",
		DisplayName:        "create_issue",
		Origin:             tools.ToolOriginMCPBridge,
		Risk:               "network",
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}); err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}

	invRepo := toolinvocations.NewDBRepository(db)
	invSvc := toolinvocations.NewService(invRepo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	jobMgr := jobs.NewManager(jobs.ManagerConfig{
		ToolRegistry:    registry,
		ToolInvocations: invSvc,
		ContextProvider: func() context.Context { return ctx },
	})
	mcpMgr := mcp.NewManager(registry, nil, nil)
	mcpMgr.SetRepository(mcpRepo)
	app := &App{
		currentUserID: userID,
		jobsCtrl:      controllers.NewJobsController(controllers.JobsControllerConfig{JobMgr: jobMgr}),
		mcpMgr:        mcpMgr,
	}

	result, err := app.TestToolDryRun(`{"mcp_server_id":"srv-1","tool_name":"create_issue","inputs":{}}`)
	if err != nil {
		t.Fatalf("TestToolDryRun error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful dry-run, got %#v", result)
	}
	if result.ToolName != "mcp_jira__create_issue" || result.MCPServerID != "srv-1" || result.Origin != tools.ToolOriginMCPBridge {
		t.Fatalf("unexpected resolved target: %#v", result)
	}

	wantDryRun := true
	invocations, err := invRepo.List(ctx, toolinvocations.Filter{OriginType: toolinvocations.OriginToolCatalog, OriginID: "mcp_jira__create_issue", DryRun: &wantDryRun, Limit: 10})
	if err != nil {
		t.Fatalf("list invocations: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("expected one dry-run invocation, got %d", len(invocations))
	}

	entries, err := mcpRepo.ListTools(ctx, tools.ToolCatalogFilter{MCPServerID: "srv-1", IncludeUnavailable: true})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(entries) != 1 || entries[0].LastTestStatus != tools.ToolTestStatusOK {
		t.Fatalf("expected catalog last_test_status=ok, got %#v", entries)
	}

	unresolved, err := app.TestToolDryRun(`{"tool_name":"mcp_jira__create_issue","inputs":{}}`)
	if err != nil {
		t.Fatalf("TestToolDryRun unresolved MCP error: %v", err)
	}
	if unresolved == nil || !unresolved.Blocked || unresolved.Success {
		t.Fatalf("expected unresolved MCP bridge dry-run to be blocked, got %#v", unresolved)
	}

	if err := mcpRepo.UpsertTool(ctx, &tools.ToolCatalogEntry{
		MCPServerID:        "srv-1",
		Name:               "mcp_jira__delete_issue",
		DisplayName:        "delete_issue",
		Origin:             tools.ToolOriginMCPBridge,
		Risk:               "destructive",
		AvailabilityStatus: tools.ToolAvailabilityUnavailable,
		AvailabilityReason: "server disconnected",
	}); err != nil {
		t.Fatalf("seed unavailable tool catalog: %v", err)
	}

	unavailable, err := app.TestToolDryRun(`{"mcp_server_id":"srv-1","tool_name":"delete_issue","inputs":{}}`)
	if err != nil {
		t.Fatalf("TestToolDryRun unavailable error: %v", err)
	}
	if unavailable == nil || unavailable.Success || unavailable.Blocked {
		t.Fatalf("expected unavailable tool to return operational error, got %#v", unavailable)
	}
	entries, err = mcpRepo.ListTools(ctx, tools.ToolCatalogFilter{MCPServerID: "srv-1", IncludeUnavailable: true})
	if err != nil {
		t.Fatalf("list tools after unavailable test: %v", err)
	}
	var foundUnavailable bool
	for _, entry := range entries {
		if entry.Name == "mcp_jira__delete_issue" {
			foundUnavailable = true
			if entry.LastTestStatus != tools.ToolTestStatusError {
				t.Fatalf("expected unavailable catalog last_test_status=error, got %#v", entry)
			}
		}
	}
	if !foundUnavailable {
		t.Fatalf("expected unavailable tool entry in catalog, got %#v", entries)
	}

	if err := mcpRepo.UpsertTool(ctx, &tools.ToolCatalogEntry{
		MCPServerID:        "srv-1",
		Name:               "mcp_native__filesystem",
		DisplayName:        "filesystem",
		Origin:             tools.ToolOriginMCPNative,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}); err != nil {
		t.Fatalf("seed native tool catalog: %v", err)
	}
	nativeBlocked, err := app.TestToolDryRun(`{"mcp_server_id":"srv-1","tool_name":"filesystem","inputs":{}}`)
	if err != nil {
		t.Fatalf("TestToolDryRun native error: %v", err)
	}
	if nativeBlocked == nil || !nativeBlocked.Blocked || nativeBlocked.ToolCatalogID == "" {
		t.Fatalf("expected resolved native dry-run to be blocked with catalog id, got %#v", nativeBlocked)
	}
	entries, err = mcpRepo.ListTools(ctx, tools.ToolCatalogFilter{MCPServerID: "srv-1", IncludeUnavailable: true})
	if err != nil {
		t.Fatalf("list tools after native blocked test: %v", err)
	}
	var foundNativeBlocked bool
	for _, entry := range entries {
		if entry.Name == "mcp_native__filesystem" {
			foundNativeBlocked = true
			if entry.LastTestStatus != tools.ToolTestStatusBlocked {
				t.Fatalf("expected native catalog last_test_status=blocked, got %#v", entry)
			}
		}
	}
	if !foundNativeBlocked {
		t.Fatalf("expected native tool entry in catalog, got %#v", entries)
	}

	if err := mcpRepo.UpsertTool(ctx, &tools.ToolCatalogEntry{
		MCPServerID:        "srv-1",
		Name:               "mcp_native__offline",
		DisplayName:        "offline",
		Origin:             tools.ToolOriginMCPNative,
		AvailabilityStatus: tools.ToolAvailabilityUnavailable,
		AvailabilityReason: "server disconnected",
	}); err != nil {
		t.Fatalf("seed unavailable native tool catalog: %v", err)
	}
	nativeUnavailable, err := app.TestToolDryRun(`{"mcp_server_id":"srv-1","tool_name":"offline","inputs":{}}`)
	if err != nil {
		t.Fatalf("TestToolDryRun unavailable native error: %v", err)
	}
	if nativeUnavailable == nil || nativeUnavailable.Success || nativeUnavailable.Blocked {
		t.Fatalf("expected unavailable native tool to return operational error, got %#v", nativeUnavailable)
	}
	entries, err = mcpRepo.ListTools(ctx, tools.ToolCatalogFilter{MCPServerID: "srv-1", IncludeUnavailable: true})
	if err != nil {
		t.Fatalf("list tools after unavailable native test: %v", err)
	}
	var foundNativeUnavailable bool
	for _, entry := range entries {
		if entry.Name == "mcp_native__offline" {
			foundNativeUnavailable = true
			if entry.LastTestStatus != tools.ToolTestStatusError {
				t.Fatalf("expected unavailable native catalog last_test_status=error, got %#v", entry)
			}
		}
	}
	if !foundNativeUnavailable {
		t.Fatalf("expected unavailable native tool entry in catalog, got %#v", entries)
	}
}
