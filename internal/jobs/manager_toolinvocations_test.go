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

func TestManagerTestToolContext_RecordsDryRunToolCatalogInvocations(t *testing.T) {
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
		ToolRegistry: registry,
		ToolInvocations: invSvc,
		ContextProvider: func() context.Context { return userCtx },
	})

	// Success path.
	okRes, err := mgr.TestToolContext(userCtx, "tool_ok", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("TestToolContext ok err: %v", err)
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

	// Error path.
	errRes, err := mgr.TestToolContext(userCtx, "tool_err", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("TestToolContext err err: %v", err)
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
