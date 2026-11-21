package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/database"
	"assistente/internal/terminal"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/tools/shell"

	"assistente/internal/sqlite"
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
	results  []tools.ToolResult
	calls    int
	identity tools.MCPToolIdentity
}

func (s *scriptedTool) MCPIdentity() tools.MCPToolIdentity { return s.identity }

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

type shellDispatchProbe struct {
	shell.SessionManager
	command    string
	calls      int
	startupErr error
	dispatches int
	entry      *terminal.HistoryEntry
}

func (p *shellDispatchProbe) RunEphemeral(_ context.Context, _, command string, _ time.Duration, _ string) (*terminal.HistoryEntry, error) {
	p.command = command
	p.calls++
	if p.calls == 1 && p.startupErr != nil {
		return nil, errors.Join(terminal.ErrSessionStart, p.startupErr)
	}
	p.dispatches++
	if p.entry != nil {
		return p.entry, nil
	}
	return nil, errors.New("resultado do comando indisponível após o despacho")
}

func TestJobRetriesOnlyPolicyApprovedShellStartFailure(t *testing.T) {
	for _, byPolicy := range []bool{false, true} {
		t.Run(fmt.Sprintf("politica=%t", byPolicy), func(t *testing.T) {
			probe := &shellDispatchProbe{startupErr: terminal.ErrSessionCapacity, entry: &terminal.HistoryEntry{Output: "ok"}}
			al := &allowlist.Allowlist{DefaultAction: "confirm"}
			if byPolicy {
				al.AutoApprove = []string{"echo"}
			}
			prompts, policies := 0, 0
			confirm := func(context.Context, string, string) (bool, error) {
				prompts++
				return true, nil
			}
			policy := func(context.Context) (*allowlist.Allowlist, error) {
				policies++
				return al, nil
			}
			tool := shell.NewRunCommand(probe, confirm, policy, t.TempDir())
			executor, repo, ctx := invocationBackedExecutor(t, tool)
			job := &Job{ID: "shell-start", Tool: tool.Name(), Inputs: map[string]any{"command": "echo ok"}, ErrorPolicy: ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 2, RetryDelay: "1ms"}}
			run := executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual})
			record, err := repo.List(ctx, toolinvocations.Filter{OriginID: run.RunID})
			if err != nil {
				t.Fatal(err)
			}
			if byPolicy {
				if run.Status != RunStatusCompleted || run.RetryCount != 1 || probe.calls != 2 || probe.dispatches != 1 || policies != 2 || prompts != 0 || len(record) != 2 {
					t.Fatalf("run=%+v tentativas=%d despachos=%d políticas=%d diálogos=%d invocações=%+v", run, probe.calls, probe.dispatches, policies, prompts, record)
				}
			} else if run.Status != RunStatusFailed || run.RetryCount != 0 || probe.calls != 1 || probe.dispatches != 0 || policies != 1 || prompts != 1 || len(record) != 1 {
				t.Fatalf("run=%+v tentativas=%d despachos=%d políticas=%d diálogos=%d invocações=%+v", run, probe.calls, probe.dispatches, policies, prompts, record)
			}
			failures := 0
			for _, item := range record {
				if item.ErrorCode == shell.FailureSessionStart {
					failures++
					if !item.RetryabilityKnown || item.Retryable != byPolicy {
						t.Fatalf("falha de criação=%+v", item)
					}
				}
			}
			if failures != 1 {
				t.Fatalf("invocações=%+v", record)
			}
		})
	}
}

func TestJobDoesNotReplayAuthorizedShellCommand(t *testing.T) {
	for _, tc := range []struct {
		name        string
		autoApprove []string
		wantPrompts int
	}{
		{"aprovação pontual", nil, 1},
		{"liberação pela política", []string{"echo"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompts := 0
			terminal := new(shellDispatchProbe)
			tool := shell.NewRunCommand(terminal, func(context.Context, string, string) (bool, error) {
				prompts++
				return true, nil
			}, func(context.Context) (*allowlist.Allowlist, error) {
				return &allowlist.Allowlist{AutoApprove: tc.autoApprove, DefaultAction: "confirm"}, nil
			}, t.TempDir())
			executor, repo, ctx := invocationBackedExecutor(t, tool)
			job := &Job{
				ID: "shell-retry", Tool: tool.Name(), Inputs: map[string]any{"command": "echo ok"},
				ErrorPolicy: ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 2, RetryDelay: "1ms"},
			}
			run := executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual})
			if run.Status != "failed" || run.RetryCount != 0 || terminal.calls != 1 || prompts != tc.wantPrompts {
				t.Fatalf("status=%s retries=%d despachos=%d diálogos=%d", run.Status, run.RetryCount, terminal.calls, prompts)
			}
			record, err := repo.List(ctx, toolinvocations.Filter{OriginID: run.RunID})
			if err != nil || len(record) != 1 || !record[0].RetryabilityKnown || record[0].Retryable {
				t.Fatalf("invocações=%+v erro=%v", record, err)
			}
		})
	}
}

func TestInvalidShellDirectoryKeepsHostPathOutOfTechnicalHistory(t *testing.T) {
	const hostOnly = "local-project-private-name"
	root := filepath.Join(t.TempDir(), hostOnly)
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	const requested = "broken-directory"
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, requested)); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink indisponível: %v", err)
		}
		t.Fatal(err)
	}
	probe := &shellDispatchProbe{}
	tool := shell.NewRunCommand(probe, nil, nil, root)
	executor, repo, ctx := invocationBackedExecutor(t, tool)
	job := &Job{ID: "invalid-shell-directory", Tool: tool.Name(), Inputs: map[string]any{"command": "example", "working_directory": requested}}
	run := executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual})
	if run.Status != RunStatusFailed || probe.calls != 0 {
		t.Fatalf("status=%s despachos=%d", run.Status, probe.calls)
	}
	records, err := repo.List(ctx, toolinvocations.Filter{OriginID: run.RunID})
	if err != nil || len(records) != 1 {
		t.Fatalf("registros=%d erro=%v", len(records), err)
	}
	record := records[0]
	for _, message := range []string{run.Error, record.ErrorMessage, string(record.Output)} {
		if !strings.Contains(message, "working_directory inválido") || strings.Contains(message, hostOnly) || strings.Contains(message, requested) {
			t.Fatalf("diagnóstico persistido=%q", message)
		}
	}
}

func TestJobDoesNotReplayShellAfterOutputSecretFailure(t *testing.T) {
	probe := &shellDispatchProbe{entry: &terminal.HistoryEntry{Output: "ok"}}
	prompts := 0
	tool := shell.NewRunCommand(probe, func(context.Context, string, string) (bool, error) {
		prompts++
		return true, nil
	}, nil, t.TempDir())
	executor, repo, ctx := invocationBackedExecutor(t, tool)
	job := &Job{
		ID: "shell-output-secret", Tool: tool.Name(), Inputs: map[string]any{"command": "echo ok"},
		Output:      OutputConfig{Map: map[string]string{"value": `{{ secret "output" }}`}},
		ErrorPolicy: ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 2, RetryDelay: "1ms"},
	}
	run := executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual})
	if run.Status != RunStatusFailed || run.RetryCount != 0 || probe.calls != 1 || prompts != 1 ||
		!strings.Contains(run.Error, "resolve output map") || !strings.Contains(run.Error, "no secret store configured") {
		t.Fatalf("despachos=%d confirmações=%d run=%+v", probe.calls, prompts, run)
	}
	record, err := repo.List(ctx, toolinvocations.Filter{OriginID: run.RunID})
	if err != nil || len(record) != 1 || record[0].Status != toolinvocations.StatusSucceeded {
		t.Fatalf("invocações=%+v erro=%v", record, err)
	}
}

func TestJobDoesNotReplayOtherToolAfterOutputSecretFailure(t *testing.T) {
	tool := &scriptedTool{results: []tools.ToolResult{{Content: "efeito concluído"}}}
	executor, repo, ctx := invocationBackedExecutor(t, tool)
	job := &Job{
		ID: "output-secret", Tool: tool.Name(),
		Output:      OutputConfig{Map: map[string]string{"value": `{{ secret "output" }}`}},
		ErrorPolicy: ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 2, RetryDelay: "1ms"},
	}
	run := executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual})
	if run.Status != RunStatusFailed || run.RetryCount != 0 || tool.calls != 1 ||
		!strings.Contains(run.Error, "resolve output map") || !strings.Contains(run.Error, "no secret store configured") {
		t.Fatalf("chamadas=%d run=%+v", tool.calls, run)
	}
	invocations, err := repo.List(ctx, toolinvocations.Filter{OriginID: run.RunID})
	if err != nil || len(invocations) != 1 || invocations[0].Status != toolinvocations.StatusSucceeded {
		t.Fatalf("invocações=%+v erro=%v", invocations, err)
	}
}

func TestToolOutputAcrossJobDryRunAndHistory(t *testing.T) {
	tool := &scriptedTool{}
	executor, repo, ctx := invocationBackedExecutor(t, tool)
	manager := mustNewManager(t, ManagerConfig{ToolRegistry: executor.toolRegistry, ToolInvocations: executor.toolInvocations})
	for _, tc := range []struct {
		name, content string
		want          map[string]any
	}{
		{"objeto", `{"a":2,"_meta_source":"payload"}`, map[string]any{"a": float64(2)}},
		{"array", `[1,null,{"x":false}]`, map[string]any{"content": []any{float64(1), nil, map[string]any{"x": false}}}},
		{"null", " null\n", map[string]any{"content": " null\n"}},
		{"número", " 12.0 ", map[string]any{"content": " 12.0 "}},
		{"booleano", "false", map[string]any{"content": "false"}},
		{"string", `"texto"`, map[string]any{"content": `"texto"`}},
		{"texto", "texto comum", map[string]any{"content": "texto comum"}},
		{"vazio", "", map[string]any{}},
		{"branco", " \t\n", map[string]any{"content": " \t\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool.results = []tools.ToolResult{{Content: tc.content, Metadata: map[string]any{"source": "tool"}}}
			want := maps.Clone(tc.want)
			want["_meta_source"] = "tool"
			run := executor.Execute(ctx, &Job{ID: "output-shape", Tool: tool.Name()}, &TriggerContext{Type: TriggerManual})
			if run.Status != RunStatusCompleted || !reflect.DeepEqual(run.Output, want) {
				t.Fatalf("execução: status=%s saída=%#v erro=%s", run.Status, run.Output, run.Error)
			}
			dry, err := manager.TestToolDryRunContext(ctx, TestToolRequest{ToolName: tool.Name()})
			if err != nil || dry == nil || !dry.Success || !reflect.DeepEqual(dry.Output, want) {
				t.Fatalf("dry run=%+v erro=%v", dry, err)
			}
			invocations, err := repo.List(ctx, toolinvocations.Filter{OriginID: run.RunID})
			if err != nil || len(invocations) != 1 {
				t.Fatalf("invocações=%+v erro=%v", invocations, err)
			}
			wantHistory := want
			if tc.name == "branco" {
				wantHistory = map[string]any{"_meta_source": "tool"}
			}
			if got := invocationOutput(string(invocations[0].Output)); !reflect.DeepEqual(got, wantHistory) {
				t.Fatalf("histórico=%#v esperado=%#v", got, wantHistory)
			}
		})
	}
}

func TestJobShellNullOutputPreservesMetadata(t *testing.T) {
	probe := &shellDispatchProbe{entry: &terminal.HistoryEntry{Output: "null"}}
	tool := shell.NewRunCommand(probe, func(context.Context, string, string) (bool, error) {
		return true, nil
	}, nil, t.TempDir())
	executor, _, ctx := invocationBackedExecutor(t, tool)
	job := &Job{ID: "shell-null", Tool: tool.Name(), Inputs: map[string]any{"command": "echo null"}}
	run := executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual})
	if run.Status != RunStatusCompleted || run.Output["content"] != "null" || run.Output["_meta_command"] != "echo null" || probe.calls != 1 {
		t.Fatalf("despachos=%d run=%+v", probe.calls, run)
	}
}

func TestJobShellRedactsSecretCommandMetadata(t *testing.T) {
	const template = `example {{ secret "token" }}`
	const token = "valor-sintetico-3417"
	for _, tc := range []struct {
		name, secretKey, expectedCommand string
		inputs                           map[string]any
	}{
		{"canonical", "command", "example " + token, map[string]any{"command": template}},
		{"capitalized", "Command", "example " + token, map[string]any{"Command": template}},
		{"uppercase", "COMMAND", "example " + token, map[string]any{"COMMAND": template}},
		// O marshal ordena as chaves, e o shell usa a última variante. Em public_last,
		// o comando público é executado, mas a presença de outra variante sensível
		// torna conservadora a projeção de command nos metadados.
		{"public_last", "COMMAND", "example public", map[string]any{"COMMAND": template, "command": "example public"}},
		{"secret_last", "Command", "example " + token, map[string]any{"COMMAND": "example public", "Command": template}},
	} {
		for _, route := range []string{"job", "dry-run"} {
			t.Run(tc.name+"/"+route, func(t *testing.T) {
				checkSecretCommandMetadata(t, route, tc.inputs, tc.secretKey, tc.expectedCommand, token)
			})
		}
	}
}

func checkSecretCommandMetadata(t *testing.T, route string, inputs map[string]any, secretKey, expectedCommand, token string) {
	t.Helper()
	probe := &shellDispatchProbe{entry: &terminal.HistoryEntry{Output: "ok"}}
	tool := shell.NewRunCommand(probe, func(context.Context, string, string) (bool, error) { return true, nil }, nil, t.TempDir())
	executor, repo, ctx := invocationBackedExecutor(t, tool)
	executor.secretStore = secretStoreFunc(func(context.Context, string) (string, error) { return token, nil })
	var output map[string]any
	if route == "job" {
		run := executor.Execute(ctx, &Job{ID: "secret-shell", Tool: tool.Name(), Inputs: inputs}, &TriggerContext{Type: TriggerManual})
		if run.Status != RunStatusCompleted || run.ResolvedInputs[secretKey] != redactedValue {
			t.Fatal("execução ou redação dos argumentos falhou")
		}
		output = run.Output
	} else {
		manager := mustNewManager(t, ManagerConfig{ToolRegistry: executor.toolRegistry, ToolInvocations: executor.toolInvocations, SecretStore: executor.secretStore})
		result, err := manager.TestToolDryRunContext(ctx, TestToolRequest{ToolName: tool.Name(), Inputs: inputs, AllowUnsafe: true})
		if err != nil || result == nil || !result.Success {
			t.Fatal("dry-run falhou")
		}
		output = result.Output
	}
	if probe.calls != 1 || probe.command != expectedCommand {
		t.Fatal("comando resolvido não chegou à execução")
	}
	records, err := repo.List(ctx, toolinvocations.Filter{Limit: 10})
	if err != nil || len(records) != 1 {
		t.Fatalf("invocações=%d, erro=%v", len(records), err)
	}
	record := records[0]
	snapshot, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(snapshot), token) {
		t.Fatal("token encontrado no ledger")
	}
	for _, result := range []map[string]any{output, invocationOutput(string(record.Output))} {
		if result["_meta_command"] != redactedValue || result["_meta_exitCode"] == nil || result["content"] != "ok" {
			t.Fatal("projeção de comando ou metadados incorretos")
		}
	}
	public := executor.Execute(ctx, &Job{ID: "public-shell", Tool: tool.Name(), Inputs: map[string]any{"command": "example public"}}, &TriggerContext{Type: TriggerManual})
	if public.Status != RunStatusCompleted || public.Output["_meta_command"] != "example public" {
		t.Fatal("redação interferiu na próxima invocação")
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
	for _, mcp := range []bool{false, true} {
		t.Run(map[bool]string{false: "builtin", true: "mcp_alias"}[mcp], func(t *testing.T) {
			tool := &scriptedTool{results: []tools.ToolResult{{Content: `{"unexpected":true}`}}}
			if mcp {
				tool.identity = tools.MCPToolIdentity{ServerSlug: "remote", ToolName: "search"}
			}
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
			var payload struct {
				Display struct {
					Name   string `json:"name"`
					Origin string `json:"origin"`
					Server string `json:"server_label"`
				} `json:"display"`
			}
			if err := json.Unmarshal(invocations[0].Metadata, &payload); err != nil {
				t.Fatal(err)
			}
			wantName, wantOrigin, wantServer := tool.Name(), "mcp_native", ""
			if mcp {
				wantName, wantOrigin, wantServer = "search", "mcp_native", "remote"
			}
			if payload.Display.Name != wantName || payload.Display.Origin != wantOrigin || payload.Display.Server != wantServer {
				t.Fatalf("mock dry-run perdeu apresentação: %s", invocations[0].Metadata)
			}
		})
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
	if err := db.AutoMigrate(&database.User{}, &database.MCPServer{}, &database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	prev := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(prev) })
	entry := database.ToolCatalog{
		Name:               tool.Name(),
		DisplayName:        tool.Name(),
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}
	if identity, ok := tools.MCPIdentityOf(tool); ok {
		server := database.MCPServer{UserID: "user-jobs", Slug: identity.ServerSlug, Name: identity.ServerSlug, Enabled: true}
		if err := db.Create(&server).Error; err != nil {
			t.Fatal(err)
		}
		entry.UserID, entry.MCPServerID = &server.UserID, &server.ID
		entry.Origin, entry.DisplayName = tools.ToolOriginMCPBridge, identity.ToolName
	}
	if err := db.Create(&entry).Error; err != nil {
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

func TestJobRetryComLotacaoRealDoTerminal(t *testing.T) {
	for _, limit := range []int{0, 1} {
		t.Run(fmt.Sprintf("limite=%d", limit), func(t *testing.T) {
			cfg := terminal.DefaultManagerConfig()
			cfg.MaxSessions = limit
			dir := t.TempDir()
			manager := terminal.NewManager(cfg, nil)
			t.Cleanup(manager.CloseAll)
			var occupied *terminal.Session
			if limit > 0 {
				var err error
				occupied, err = manager.Create("ocupado", dir)
				if err != nil {
					t.Fatal(err)
				}
			}
			reads := 0
			policy := func(context.Context) (*allowlist.Allowlist, error) {
				reads++
				if reads == 2 && occupied != nil {
					if err := manager.Close(occupied.ID()); err != nil {
						t.Error(err)
					}
				}
				return &allowlist.Allowlist{AutoApprove: []string{"echo"}, DefaultAction: "confirm"}, nil
			}
			tool := shell.NewRunCommand(manager, nil, policy, dir)
			executor, repo, ctx := invocationBackedExecutor(t, tool)
			job := &Job{ID: "shell-capacity", Tool: tool.Name(), Inputs: map[string]any{"command": "echo ok", "persistent": true}, ErrorPolicy: ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 2, RetryDelay: "1ms"}}
			run := executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual})
			records, err := repo.List(ctx, toolinvocations.Filter{OriginID: run.RunID})
			if err != nil {
				t.Fatal(err)
			}
			if limit == 0 {
				if run.Status != RunStatusFailed || run.RetryCount != 0 || reads != 1 || len(records) != 1 || records[0].Retryable {
					t.Fatalf("run=%+v leituras=%d registros=%+v", run, reads, records)
				}
			} else if run.Status != RunStatusCompleted || run.RetryCount != 1 || reads != 2 || len(records) != 2 {
				t.Fatalf("run=%+v leituras=%d registros=%+v", run, reads, records)
			}
		})
	}
}
