package toolinvocations

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/tools"
)

type echoTool struct{}

func (echoTool) Name() string { return "echo" }
func (echoTool) Description() string {
	return "echo"
}
func (echoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}}}`)
}
func (echoTool) Execute(_ context.Context, args json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: string(args)}, nil
}

func TestServiceExecutesAndPersistsInvocation(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(echoTool{})
	svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))

	result := svc.Execute(userA, ExecuteRequest{
		Call: tools.ToolCall{
			ID:   "call-1",
			Type: "function",
			Function: tools.FunctionCall{
				Name:      "echo",
				Arguments: `{"value":"ok"}`,
			},
		},
		Origin: Origin{Type: OriginChat, ID: "turn-1"},
	})
	if result.Execution.Result.IsError {
		t.Fatalf("execution returned error: %s", result.Execution.Result.Content)
	}
	if result.Invocation.ID == "" {
		t.Fatal("expected invocation id")
	}
	got, err := repo.Get(userA, result.Invocation.ID)
	if err != nil {
		t.Fatalf("get invocation: %v", err)
	}
	if got.Status != StatusCompleted || got.ToolCallID != "call-1" || got.OriginID != "turn-1" {
		t.Fatalf("unexpected invocation: %#v", got)
	}
	if got.Output == nil || len(got.Output) == 0 {
		t.Fatal("expected persisted output")
	}
}
