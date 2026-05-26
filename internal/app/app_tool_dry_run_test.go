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
}
