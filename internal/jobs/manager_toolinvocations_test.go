package jobs

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type testToolOK struct{}

func (testToolOK) Name() string                { return "tool_ok" }
func (testToolOK) Description() string         { return "ok" }
func (testToolOK) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (testToolOK) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

type testToolErr struct{}

func (testToolErr) Name() string                { return "tool_err" }
func (testToolErr) Description() string         { return "err" }
func (testToolErr) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (testToolErr) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{}, context.Canceled
}

type testToolWrite struct{}

func (testToolWrite) Name() string                { return "write_file" }
func (testToolWrite) Description() string         { return "write" }
func (testToolWrite) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (testToolWrite) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: `{"wrote":true}`}, nil
}

func TestManagerTestToolDryRunContext_RecordsDryRunToolCatalogInvocations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	prev := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(prev) })

	// Seed tool catalog entries so toolinvocations can resolve catalog IDs.
	if err := db.Create(&database.ToolCatalog{Name: "tool_ok", DisplayName: "tool_ok", Origin: tools.ToolOriginBuiltin, AvailabilityStatus: tools.ToolAvailabilityAvailable}).Error; err != nil {
		t.Fatalf("seed tool catalog tool_ok: %v", err)
	}
	if err := db.Create(&database.ToolCatalog{Name: "tool_err", DisplayName: "tool_err", Origin: tools.ToolOriginBuiltin, AvailabilityStatus: tools.ToolAvailabilityAvailable}).Error; err != nil {
		t.Fatalf("seed tool catalog tool_err: %v", err)
	}

	userCtx := database.WithUserID(context.Background(), "user-jobs")

	registry := tools.NewRegistry()
	registry.MustRegister(testToolOK{})
	registry.MustRegister(testToolErr{})

	repo := toolinvocations.NewDBRepository(db)
	exec := tools.NewExecutor(registry, tools.DefaultExecutorConfig())
	invSvc := toolinvocations.NewService(repo, exec)

	mgr := NewManager(ManagerConfig{
		ToolRegistry:    registry,
		ToolInvocations: invSvc,
		ContextProvider: func() context.Context { return userCtx },
	})

	// Success path.
	okRes, err := mgr.TestToolDryRunContext(userCtx, TestToolRequest{ToolName: "tool_ok", Inputs: map[string]any{}})
	if err != nil {
		t.Fatalf("TestToolDryRunContext ok err: %v", err)
	}
	if okRes == nil || !okRes.Success {
		t.Fatalf("expected success result, got %#v", okRes)
	}
	wantDryRun := true
	invocations, err := repo.List(userCtx, toolinvocations.Filter{OriginType: toolinvocations.OriginToolCatalog, OriginID: "tool_ok", DryRun: &wantDryRun, Limit: 10})
	if err != nil {
		t.Fatalf("list invocations ok: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("expected 1 invocation for tool_ok, got %d", len(invocations))
	}
	if invocations[0].OriginType != toolinvocations.OriginToolCatalog || invocations[0].OriginID != "tool_ok" {
		t.Fatalf("unexpected origin: %#v", invocations[0])
	}
	if !invocations[0].DryRun {
		t.Fatalf("expected dry_run=true, got %#v", invocations[0])
	}
	var okTool database.ToolCatalog
	if err := db.Where("name = ?", "tool_ok").First(&okTool).Error; err != nil {
		t.Fatalf("load tool_ok catalog: %v", err)
	}
	if invocations[0].ToolCatalogID != okTool.ID {
		t.Fatalf("expected explicit/auto catalog id %q, got %q", okTool.ID, invocations[0].ToolCatalogID)
	}

	// Error path.
	errRes, err := mgr.TestToolDryRunContext(userCtx, TestToolRequest{ToolName: "tool_err", Inputs: map[string]any{}})
	if err != nil {
		t.Fatalf("TestToolDryRunContext err err: %v", err)
	}
	if errRes == nil || errRes.Success {
		t.Fatalf("expected failure result, got %#v", errRes)
	}
	invocations, err = repo.List(userCtx, toolinvocations.Filter{OriginType: toolinvocations.OriginToolCatalog, OriginID: "tool_err", DryRun: &wantDryRun, Limit: 10})
	if err != nil {
		t.Fatalf("list invocations err: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("expected 1 invocation for tool_err, got %d", len(invocations))
	}
	if invocations[0].Status != toolinvocations.StatusFailed {
		t.Fatalf("expected status=failed, got %#v", invocations[0])
	}
}

func TestManagerTestToolDryRunContext_BlocksUnsafeAndNative(t *testing.T) {
	userCtx := database.WithUserID(context.Background(), "user-jobs")
	registry := tools.NewRegistry()
	registry.MustRegister(testToolWrite{})

	mgr := NewManager(ManagerConfig{
		ToolRegistry:    registry,
		ContextProvider: func() context.Context { return userCtx },
	})

	blocked, err := mgr.TestToolDryRunContext(userCtx, TestToolRequest{ToolName: "write_file"})
	if err != nil {
		t.Fatalf("unsafe dry-run err: %v", err)
	}
	if blocked == nil || !blocked.Blocked || blocked.Success {
		t.Fatalf("expected unsafe tool to be blocked, got %#v", blocked)
	}

	allowed, err := mgr.TestToolDryRunContext(userCtx, TestToolRequest{ToolName: "write_file", AllowUnsafe: true})
	if err != nil {
		t.Fatalf("allow unsafe dry-run err: %v", err)
	}
	if allowed == nil || !allowed.Success {
		t.Fatalf("expected allow_unsafe execution to succeed, got %#v", allowed)
	}

	native, err := mgr.TestToolDryRunContext(userCtx, TestToolRequest{
		ToolName:    "mcp_native__create_issue",
		Origin:      tools.ToolOriginMCPNative,
		MCPServerID: "server-1",
	})
	if err != nil {
		t.Fatalf("native dry-run err: %v", err)
	}
	if native == nil || !native.Blocked || native.Success {
		t.Fatalf("expected native MCP dry-run to be blocked, got %#v", native)
	}
}
