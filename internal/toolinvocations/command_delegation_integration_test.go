package toolinvocations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/database"
	"assistente/internal/tools"
)

type commandDelegationTool struct {
	calls  *int
	result tools.ToolResult
}

func (t commandDelegationTool) Name() string { return "echo" }

func (t commandDelegationTool) Description() string { return "tool de delegação" }

func (t commandDelegationTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t commandDelegationTool) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	if t.calls != nil {
		*t.calls++
	}
	return t.result, nil
}

func TestCommandDelegationPersistsOriginCorrelationAndRedactsSensitivePaths(t *testing.T) {
	repo, userCtx, _ := setupRepositoryTest(t)
	var calls int
	registry := tools.NewRegistry()
	registry.MustRegister(commandDelegationTool{
		calls: &calls,
		result: tools.ToolResult{
			Content:  `{"visible":"ok","secret":"output-secret"}`,
			Metadata: map[string]any{"secret": "metadata-secret"},
		},
	})
	service := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	catalogID, err := repo.ResolveToolCatalogID(userCtx, "echo")
	if err != nil {
		t.Fatal(err)
	}
	commandID := "command-invocation-delegation-1"
	result := service.Execute(userCtx, ExecuteRequest{
		Call: tools.ToolCall{
			ID:   "command-tool-call-1",
			Type: "function",
			Function: tools.FunctionCall{
				Name:      "echo",
				Arguments: `{"path":"/tmp/work","token":"input-secret","nested":{"password":"nested-secret"}}`,
			},
		},
		SensitivePaths: commandcatalog.SensitivePaths{
			Input:  []string{"/token", "/nested/password"},
			Output: []string{"/secret"},
		},
		Origin:                        toolinvocationOrigin(commandID),
		ToolCatalogID:                 catalogID,
		RequireCanonicalToolCatalogID: true,
	})
	if !result.Persisted || result.Execution.Result.IsError {
		t.Fatalf("delegação não persistiu/executou: %+v", result)
	}
	if calls != 1 {
		t.Fatalf("tool executada %d vezes, esperado 1", calls)
	}

	row, err := repo.Get(userCtx, result.Invocation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.OriginType != OriginCommandInvocation || row.OriginID != commandID || row.ToolCatalogID != catalogID {
		t.Fatalf("correlação/origem canônica ausente: %+v", row)
	}
	for _, persisted := range []string{string(row.Input), string(row.Output)} {
		for _, secret := range []string{"input-secret", "nested-secret", "output-secret", "metadata-secret"} {
			if strings.Contains(persisted, secret) {
				t.Fatalf("segredo persistido em %q: %s", persisted, secret)
			}
		}
	}
	if !strings.Contains(string(row.Input), "/tmp/work") || !strings.Contains(string(row.Output), "visible") {
		t.Fatalf("dados não sensíveis foram removidos: input=%s output=%s", row.Input, row.Output)
	}
}

func TestCommandDelegationRejectsForeignCatalogBeforeToolExecution(t *testing.T) {
	repo, userCtx, _ := setupRepositoryTest(t)
	foreignOwner := "user-b"
	foreignCatalog := &database.ToolCatalog{
		UserID:             &foreignOwner,
		Name:               "foreign-command-tool",
		DisplayName:        "foreign-command-tool",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}
	if err := database.DB().Create(foreignCatalog).Error; err != nil {
		t.Fatal(err)
	}
	var calls int
	registry := tools.NewRegistry()
	registry.MustRegister(foreignCatalogTool{calls: &calls})
	service := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	result := service.Execute(userCtx, ExecuteRequest{
		Call: tools.ToolCall{
			ID:   "foreign-command-call",
			Type: "function",
			Function: tools.FunctionCall{
				Name:      foreignCatalog.Name,
				Arguments: `{}`,
			},
		},
		Origin:                        toolinvocationOrigin("foreign-command-invocation"),
		ToolCatalogID:                 foreignCatalog.ID,
		RequireCanonicalToolCatalogID: true,
	})
	if calls != 0 {
		t.Fatalf("catálogo foreign alcançou Tool.Execute: %d", calls)
	}
	if result.Persisted || !result.Execution.Result.IsError {
		t.Fatalf("catálogo foreign não foi rejeitado antes da execução: %+v", result)
	}
	var count int64
	if err := database.DB().Model(&database.ToolInvocation{}).
		Where("user_id = ? AND tool_call_id = ?", "user-a", "foreign-command-call").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejeição foreign criou invocação: %d", count)
	}
}

func toolinvocationOrigin(id string) Origin {
	return Origin{Type: OriginCommandInvocation, ID: id}
}

type foreignCatalogTool struct{ calls *int }

func (t foreignCatalogTool) Name() string { return "foreign-command-tool" }

func (t foreignCatalogTool) Description() string { return "tool foreign de teste" }

func (t foreignCatalogTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t foreignCatalogTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	*t.calls++
	return tools.ToolResult{Content: "executed"}, nil
}
