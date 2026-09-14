package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

func TestNewJobExecutorRequiresInvocationLedger(t *testing.T) {
	registry := tools.NewRegistry()
	if _, err := NewJobExecutor(ExecutorConfig{ToolRegistry: registry}); err == nil {
		t.Fatal("esperava erro de montagem sem ledger")
	}
}

func TestNewJobExecutorRequiresToolRegistry(t *testing.T) {
	service := toolinvocations.NewService(
		newJobExecutorTestLedger(),
		tools.NewExecutor(tools.NewRegistry(), tools.DefaultExecutorConfig()),
	)
	if _, err := NewJobExecutor(ExecutorConfig{ToolInvocations: service}); err == nil {
		t.Fatal("esperava erro de montagem sem registry")
	}
}

func TestJobExecutorRuntimeDefenseDoesNotExecuteWithoutLedger(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{Content: `{"ok":true}`}}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	executor := &JobExecutor{toolRegistry: registry}

	result := executor.executeTool(
		context.Background(),
		&Job{ID: "invalid-wiring", Tool: tool.Name()},
		nil,
		json.RawMessage(`{}`),
		json.RawMessage(`{}`),
	)
	if tool.calls != 0 {
		t.Fatalf("tool executada sem ledger: %d chamadas", tool.calls)
	}
	if !result.Result.IsError || result.ErrorCode != "tool_invocation_ledger_unavailable" {
		t.Fatalf("falha fechada inesperada: %#v", result)
	}
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

	executor := mustNewJobExecutor(t, ExecutorConfig{
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

func TestJobExecutorRetriesTransientSecretResolutionFailure(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{Content: `{"ok":true}`}}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	secretCalls := 0
	executor := mustNewJobExecutor(t, ExecutorConfig{
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

func TestJobExecutorPersistsTemplateSecretsRedactedInLedger(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{Content: `{"ok":true}`}}}
	executor, repo, userCtx := invocationBackedExecutor(t, tool)
	executor.secretStore = secretStoreFunc(func(context.Context, string) (string, error) {
		return "segredo-super-sensivel", nil
	})
	run := executor.Execute(userCtx, &Job{
		ID:   "secret-ledger-job",
		Tool: tool.Name(),
		Inputs: map[string]any{
			"query": `{{ secret "token" }}`,
		},
	}, &TriggerContext{Type: TriggerManual})
	if run.Status != "completed" {
		t.Fatalf("run falhou: %#v", run)
	}
	invocations, err := repo.List(userCtx, toolinvocations.Filter{
		OriginType: toolinvocations.OriginJobRun,
		OriginID:   run.RunID,
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 1 {
		t.Fatalf("invocações = %d, esperava uma", len(invocations))
	}
	persisted := string(invocations[0].Input) + string(invocations[0].Metadata)
	if strings.Contains(persisted, "segredo-super-sensivel") {
		t.Fatalf("segredo persistido no ledger: %s", persisted)
	}
	if !strings.Contains(persisted, redactedValue) {
		t.Fatalf("redação ausente do ledger: %s", persisted)
	}
}

func TestJobExecutorPersistsMockDryRunOnlyInLedger(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{Content: `{"unexpected":true}`}}}
	executor, repo, userCtx := invocationBackedExecutor(t, tool)
	run := executor.Execute(userCtx, &Job{
		ID:     "mock-dry-run",
		Tool:   tool.Name(),
		Inputs: map[string]any{"query": "configured"},
		DryRun: DryRunConfig{
			Enabled:    true,
			MockOutput: map[string]any{"mocked": true},
		},
	}, &TriggerContext{Type: TriggerManual})
	if run.Status != "completed" || tool.calls != 0 {
		t.Fatalf("mock dry-run executou tool ou falhou: calls=%d run=%#v", tool.calls, run)
	}
	invocations, err := repo.List(userCtx, toolinvocations.Filter{
		OriginType: toolinvocations.OriginJobRun,
		OriginID:   run.RunID,
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 1 || !invocations[0].DryRun ||
		!strings.Contains(toolinvocations.ExtractToolInvocationResult(string(invocations[0].Output)).Content, `"mocked":true`) {
		t.Fatalf("mock não foi representado no ledger: %#v", invocations)
	}
}

func TestJobExecutorFailsRunWhenLedgerCompletionFails(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{Content: `{"ok":true}`}}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	ledger := newJobExecutorTestLedger()
	ledger.completeErr = errors.New("complete unavailable")
	service := toolinvocations.NewService(
		ledger,
		tools.NewExecutor(registry, tools.DefaultExecutorConfig()),
	)
	executor := mustNewJobExecutor(t, ExecutorConfig{
		ToolRegistry:    registry,
		ToolInvocations: service,
		EventBus:        NewEventBus(),
		CircuitBreaker:  NewCircuitBreaker(),
	})

	run := executor.Execute(context.Background(), &Job{
		ID:   "ledger-complete-failure",
		Tool: tool.Name(),
		ErrorPolicy: ErrorPolicy{
			Strategy:   ErrorRetry,
			MaxRetries: 2,
			RetryDelay: "1ms",
		},
	}, &TriggerContext{Type: TriggerManual})

	if run.Status != "failed" || run.RetryCount != 0 {
		t.Fatalf("falha de auditoria não fechou o run: %#v", run)
	}
	if tool.calls != 1 {
		t.Fatalf("tool repetida após falha de persistência: %d chamadas", tool.calls)
	}
}

func TestJobExecutorDoesNotRetryInvalidOutputMapAfterToolMutation(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{Content: `{"ok":true}`}}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	executor := mustNewJobExecutor(t, ExecutorConfig{
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

func TestJobExecutorDoesNotRetryInvalidInputTemplate(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{Content: `{"ok":true}`}}}
	registry := tools.NewRegistry()
	registry.MustRegister(tool)
	executor := mustNewJobExecutor(t, ExecutorConfig{
		ToolRegistry:   registry,
		EventBus:       NewEventBus(),
		CircuitBreaker: NewCircuitBreaker(),
	})
	job := &Job{
		ID:     "invalid-input-template-job",
		Tool:   tool.Name(),
		Inputs: map[string]any{"value": "{{"},
		ErrorPolicy: ErrorPolicy{
			Strategy:   ErrorRetry,
			MaxRetries: 2,
			RetryDelay: "1ms",
		},
	}

	run := executor.Execute(context.Background(), job, &TriggerContext{Type: TriggerManual})
	if tool.calls != 0 || run.RetryCount != 0 || run.Status != "failed" {
		t.Fatalf("template inválido foi repetido: calls=%d run=%#v", tool.calls, run)
	}
	for _, event := range run.RunEvents {
		if event.Type == "retry_scheduled" {
			t.Fatalf("template inválido apareceu como retry: %#v", run.RunEvents)
		}
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
	return mustNewJobExecutor(t, ExecutorConfig{
		ToolRegistry:    registry,
		ToolInvocations: service,
		EventBus:        NewEventBus(),
		CircuitBreaker:  NewCircuitBreaker(),
	}), repo, database.WithUserID(context.Background(), "user-jobs")
}
