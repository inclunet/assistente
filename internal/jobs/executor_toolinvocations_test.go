package jobs

import (
	"context"
	"encoding/json"
	"errors"
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

type scriptedTool struct {
	results []tools.ToolResult
	calls   int
}

type contextErrorTool struct {
	result tools.ToolResult
	err    error
	calls  int
}

func (t *contextErrorTool) Name() string                { return "context_error_tool" }
func (t *contextErrorTool) Description() string         { return "context error tool" }
func (t *contextErrorTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *contextErrorTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	t.calls++
	return t.result, t.err
}

func (s *scriptedTool) Name() string                { return "scripted_tool" }
func (s *scriptedTool) Description() string         { return "scripted tool" }
func (s *scriptedTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (s *scriptedTool) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	s.calls++
	index := s.calls - 1
	if index >= len(s.results) {
		index = len(s.results) - 1
	}
	return s.results[index], nil
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

func TestJobExecutorDoesNotRetryPermanentToolFailure(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{
		Content: `{"error":{"code":"authorization_no_interlocutor","message":"sem interlocutor"}}`,
		IsError: true,
		Failure: &tools.ToolFailure{
			Code:      "authorization_no_interlocutor",
			Kind:      tools.ErrorKindAuthorization,
			Retryable: false,
		},
	}}}
	executor, repo, userCtx := invocationBackedExecutor(t, tool)
	job := &Job{
		ID:   "permanent-failure-job",
		Tool: tool.Name(),
		ErrorPolicy: ErrorPolicy{
			Strategy:   ErrorRetry,
			MaxRetries: 3,
			RetryDelay: "1ms",
		},
	}

	run := executor.Execute(userCtx, job, &TriggerContext{Type: TriggerManual})
	if tool.calls != 1 {
		t.Fatalf("tentativas = %d, esperava uma", tool.calls)
	}
	if run.Status != "failed" || run.RetryCount != 0 {
		t.Fatalf("run final incoerente: %#v", run)
	}
	for _, event := range run.RunEvents {
		if event.Type == "retry_scheduled" {
			t.Fatalf("retry permanente apareceu na timeline: %#v", run.RunEvents)
		}
	}

	invocations, err := repo.List(userCtx, toolinvocations.Filter{OriginType: toolinvocations.OriginJobRun, OriginID: run.RunID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 1 ||
		invocations[0].ErrorCode != "authorization_no_interlocutor" ||
		!invocations[0].RetryabilityKnown ||
		invocations[0].Retryable {
		t.Fatalf("invocação permanente não preservada: %#v", invocations)
	}
}

func TestJobExecutorWithoutInvocationServiceDoesNotRetryPermanentToolFailure(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{
		Content: "autorização indisponível",
		IsError: true,
		Failure: &tools.ToolFailure{
			Code:      "authorization_unavailable",
			Kind:      tools.ErrorKindAuthorization,
			Retryable: false,
		},
	}}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry:   registry,
		EventBus:       NewEventBus(),
		CircuitBreaker: NewCircuitBreaker(),
	})
	job := &Job{
		ID:   "legacy-direct-job",
		Tool: tool.Name(),
		ErrorPolicy: ErrorPolicy{
			Strategy:   ErrorRetry,
			MaxRetries: 3,
			RetryDelay: "1ms",
		},
	}

	run := executor.Execute(context.Background(), job, &TriggerContext{Type: TriggerManual})
	if tool.calls != 1 || run.RetryCount != 0 || run.Status != "failed" {
		t.Fatalf("caminho direto repetiu falha permanente: calls=%d run=%#v", tool.calls, run)
	}
}

func TestJobExecutorWithoutInvocationServiceRetriesExplicitTransientFailure(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{
		{
			Content: "serviço temporariamente indisponível",
			IsError: true,
			Failure: &tools.ToolFailure{
				Code:      "service_unavailable",
				Kind:      tools.ErrorKindUnavailable,
				Retryable: true,
			},
		},
		{Content: `{"ok":true}`},
	}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry:   registry,
		EventBus:       NewEventBus(),
		CircuitBreaker: NewCircuitBreaker(),
	})
	job := &Job{
		ID:   "legacy-transient-job",
		Tool: tool.Name(),
		ErrorPolicy: ErrorPolicy{
			Strategy:   ErrorRetry,
			MaxRetries: 1,
			RetryDelay: "1ms",
		},
	}

	run := executor.Execute(context.Background(), job, &TriggerContext{Type: TriggerManual})
	if tool.calls != 2 || run.RetryCount != 1 || run.Status != "completed" {
		t.Fatalf("caminho direto não repetiu falha transitória: calls=%d run=%#v", tool.calls, run)
	}
}

func TestJobExecutorWithoutInvocationServiceDoesNotRetryCancellation(t *testing.T) {
	tool := &contextErrorTool{err: context.Canceled}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry:   registry,
		EventBus:       NewEventBus(),
		CircuitBreaker: NewCircuitBreaker(),
	})
	job := &Job{
		ID:   "legacy-cancelled-job",
		Tool: tool.Name(),
		ErrorPolicy: ErrorPolicy{
			Strategy:   ErrorRetry,
			MaxRetries: 2,
			RetryDelay: "1ms",
		},
	}

	run := executor.Execute(context.Background(), job, &TriggerContext{Type: TriggerManual})
	if tool.calls != 1 || run.RetryCount != 0 || run.Status != "failed" {
		t.Fatalf("cancelamento foi repetido: calls=%d run=%#v", tool.calls, run)
	}
}

func TestJobExecutorWithoutInvocationServicePreservesFailureReturnedWithError(t *testing.T) {
	tool := &contextErrorTool{
		result: tools.ToolResult{
			Content: "configuração inválida",
			Failure: &tools.ToolFailure{
				Code:      "invalid_configuration",
				Kind:      tools.ErrorKindConfiguration,
				Retryable: false,
			},
		},
		err: errors.New("configuração inválida"),
	}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry:   registry,
		EventBus:       NewEventBus(),
		CircuitBreaker: NewCircuitBreaker(),
	})
	job := &Job{
		ID:   "legacy-structured-error-job",
		Tool: tool.Name(),
		ErrorPolicy: ErrorPolicy{
			Strategy:   ErrorRetry,
			MaxRetries: 2,
			RetryDelay: "1ms",
		},
	}

	run := executor.Execute(context.Background(), job, &TriggerContext{Type: TriggerManual})
	if tool.calls != 1 || run.RetryCount != 0 || run.Status != "failed" {
		t.Fatalf("falha estruturada retornada com erro foi repetida: calls=%d run=%#v", tool.calls, run)
	}
}

func TestJobExecutorRetriesTransientSecretResolutionFailure(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{Content: `{"ok":true}`}}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	secretCalls := 0
	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry: registry,
		EventBus:     NewEventBus(),
		SecretStore: secretStoreFunc(func(context.Context, string) (string, error) {
			secretCalls++
			if secretCalls == 1 {
				return "", errors.New("cofre temporariamente indisponível")
			}
			return "segredo", nil
		}),
		CircuitBreaker: NewCircuitBreaker(),
	})
	job := &Job{
		ID:     "transient-secret-job",
		Tool:   tool.Name(),
		Inputs: map[string]any{"token": `{{ secret "token" }}`},
		ErrorPolicy: ErrorPolicy{
			Strategy:   ErrorRetry,
			MaxRetries: 1,
			RetryDelay: "1ms",
		},
	}

	run := executor.Execute(context.Background(), job, &TriggerContext{Type: TriggerManual})
	if secretCalls != 2 || tool.calls != 1 || run.RetryCount != 1 || run.Status != "completed" {
		t.Fatalf("falha transitória do cofre não foi repetida corretamente: secretCalls=%d toolCalls=%d run=%#v", secretCalls, tool.calls, run)
	}
}

func TestJobExecutorDoesNotRetryInvalidOutputMapAfterToolMutation(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{Content: `{"ok":true}`}}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	executor := NewJobExecutor(ExecutorConfig{
		ToolRegistry:   registry,
		EventBus:       NewEventBus(),
		CircuitBreaker: NewCircuitBreaker(),
	})
	job := &Job{
		ID:     "invalid-output-map-job",
		Tool:   tool.Name(),
		Output: OutputConfig{Map: map[string]string{"value": "{{"}},
		ErrorPolicy: ErrorPolicy{
			Strategy:   ErrorRetry,
			MaxRetries: 2,
			RetryDelay: "1ms",
		},
	}

	run := executor.Execute(context.Background(), job, &TriggerContext{Type: TriggerManual})
	if tool.calls != 1 || run.RetryCount != 0 || run.Status != "failed" {
		t.Fatalf("mapa inválido repetiu mutação: calls=%d run=%#v", tool.calls, run)
	}
}

func TestJobExecutorRetriesUnclassifiedTransientToolFailure(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{
		{Content: "temporary outage", IsError: true},
		{Content: `{"ok":true}`},
	}}
	executor, repo, userCtx := invocationBackedExecutor(t, tool)
	job := &Job{
		ID:   "transient-failure-job",
		Tool: tool.Name(),
		ErrorPolicy: ErrorPolicy{
			Strategy:   ErrorRetry,
			MaxRetries: 1,
			RetryDelay: "1ms",
		},
	}

	run := executor.Execute(userCtx, job, &TriggerContext{Type: TriggerManual})
	if tool.calls != 2 {
		t.Fatalf("tentativas = %d, esperava duas", tool.calls)
	}
	if run.Status != "completed" || run.RetryCount != 1 {
		t.Fatalf("run final incoerente: %#v", run)
	}
	sawRetry := false
	for _, event := range run.RunEvents {
		sawRetry = sawRetry || event.Type == "retry_scheduled"
	}
	if !sawRetry {
		t.Fatalf("timeline sem retry_scheduled: %#v", run.RunEvents)
	}

	invocations, err := repo.List(userCtx, toolinvocations.Filter{OriginType: toolinvocations.OriginJobRun, OriginID: run.RunID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 2 || invocations[0].Status != toolinvocations.StatusSucceeded || invocations[1].Status != toolinvocations.StatusFailed {
		t.Fatalf("invocações transitórias inesperadas: %#v", invocations)
	}
}

func invocationBackedExecutor(t *testing.T, tool tools.Tool) (*JobExecutor, *toolinvocations.DBRepository, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, sqlErr := db.DB(); sqlErr == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&database.User{}, &database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	prev := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(prev) })
	if err := db.Create(&database.ToolCatalog{
		Name:               tool.Name(),
		DisplayName:        tool.Name(),
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}

	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	repo := toolinvocations.NewDBRepository(db)
	service := toolinvocations.NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	return NewJobExecutor(ExecutorConfig{
		ToolRegistry:    registry,
		ToolInvocations: service,
		EventBus:        NewEventBus(),
		CircuitBreaker:  NewCircuitBreaker(),
	}), repo, database.WithUserID(context.Background(), "user-jobs")
}
