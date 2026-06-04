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

type simpleTool struct{}

func (simpleTool) Name() string                { return "job_tool" }
func (simpleTool) Description() string         { return "job tool" }
func (simpleTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (simpleTool) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

func TestJobExecutor_RecordsToolInvocationForRun(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// :memory: SQLite é por conexão; uma única conexão garante que o schema
	// persista mesmo sob acesso concorrente (ver setupJobsRepositoryTest).
	if sqlDB, sErr := db.DB(); sErr == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&database.User{}, &database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	prev := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(prev) })

	// Seed tool catalog entry for the job tool.
	if err := db.Create(&database.ToolCatalog{
		Name:               "job_tool",
		DisplayName:        "job_tool",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}

	userCtx := database.WithUserID(context.Background(), "user-jobs")

	toolRegistry := tools.NewRegistry()
	toolRegistry.MustRegister(simpleTool{})

	repo := toolinvocations.NewDBRepository(db)
	exec := tools.NewExecutor(toolRegistry, tools.DefaultExecutorConfig())
	invSvc := toolinvocations.NewService(repo, exec)

	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry:    toolRegistry,
		ToolInvocations: invSvc,
		CircuitBreaker:  NewCircuitBreaker(),
	})

	job := &Job{ID: "job-1", Tool: "job_tool"}
	rl := executor.Execute(userCtx, job, &TriggerContext{Type: TriggerManual})
	if rl == nil {
		t.Fatal("expected run log")
	}

	invocations, err := repo.List(userCtx, toolinvocations.Filter{OriginType: toolinvocations.OriginJobRun, OriginID: rl.RunID, Limit: 10})
	if err != nil {
		t.Fatalf("list invocations: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("expected 1 invocation for run, got %d", len(invocations))
	}
	if invocations[0].OriginID != rl.RunID {
		t.Fatalf("expected origin_id=%s, got=%s", rl.RunID, invocations[0].OriginID)
	}
	if invocations[0].OriginType != toolinvocations.OriginJobRun {
		t.Fatalf("expected origin_type=%s, got=%s", toolinvocations.OriginJobRun, invocations[0].OriginType)
	}

	// Consistência (issue #127): a invocação de um job referencia o armazenamento
	// comum pelo tool_catalog (mesmo contrato de tools builtin/MCP) e termina com
	// status normalizado. Isso comprova que jobs não dependem de um log isolado
	// para representar a chamada de tool: a tool é resolvida no catálogo comum.
	if invocations[0].ToolCatalogID == "" {
		t.Fatalf("expected invocation linked to tool_catalog (builtin tool), got empty tool_catalog_id")
	}
	if invocations[0].Status != toolinvocations.StatusSucceeded {
		t.Fatalf("expected status=%s, got=%s", toolinvocations.StatusSucceeded, invocations[0].Status)
	}
	if invocations[0].DryRun {
		t.Fatalf("expected dry_run=false for real job run, got true")
	}
}
