package toolinvocations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
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
	if got.Status != StatusSucceeded || got.ToolCallID != "call-1" || got.OriginID != "turn-1" {
		t.Fatalf("unexpected invocation: %#v", got)
	}
	if len(got.Output) == 0 {
		t.Fatal("expected persisted output")
	}
}

type resultErrorTool struct{}

func (resultErrorTool) Name() string { return "result_error" }
func (resultErrorTool) Description() string {
	return "returns ToolResult.IsError without returning a Go error"
}
func (resultErrorTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (resultErrorTool) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "boom", IsError: true}, nil
}

type ctxWaitTool struct{}

func (ctxWaitTool) Name() string                { return "ctx_wait" }
func (ctxWaitTool) Description() string         { return "waits for ctx.Done and returns ctx.Err" }
func (ctxWaitTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (ctxWaitTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	<-ctx.Done()
	return tools.ToolResult{}, ctx.Err()
}

func TestBuildInvocationInputRedactsSecrets(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(echoTool{})
	svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))

	result := svc.Execute(userA, ExecuteRequest{
		Call: tools.ToolCall{
			ID:   "call-redact",
			Type: "function",
			Function: tools.FunctionCall{
				Name: "echo",
				Arguments: `{
					"password":"123",
					"authorization":"Bearer abc.def.ghi",
					"nested":{"api_key":"sk-secret"},
					"note":"hello"
				}`,
			},
		},
		Origin: Origin{Type: OriginChat, ID: "turn-redact"},
	})
	if result.Invocation.ID == "" {
		t.Fatal("expected invocation id")
	}
	inv, err := repo.Get(userA, result.Invocation.ID)
	if err != nil {
		t.Fatalf("get invocation: %v", err)
	}

	var input struct {
		ToolCall struct {
			Function struct {
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_call"`
	}
	if err := json.Unmarshal(inv.Input, &input); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	args := strings.TrimSpace(input.ToolCall.Function.Arguments)
	if args == "" {
		t.Fatal("expected redacted arguments to be persisted")
	}
	var redacted map[string]any
	if err := json.Unmarshal([]byte(args), &redacted); err != nil {
		t.Fatalf("unmarshal redacted args JSON: %v (args=%q)", err, args)
	}
	if redacted["password"] != "[redacted]" {
		t.Fatalf("expected password redacted, got=%v", redacted["password"])
	}
	if redacted["authorization"] != "[redacted]" {
		t.Fatalf("expected authorization redacted, got=%v", redacted["authorization"])
	}
	nested, _ := redacted["nested"].(map[string]any)
	if nested["api_key"] != "[redacted]" {
		t.Fatalf("expected nested api_key redacted, got=%v", nested["api_key"])
	}
	if redacted["note"] != "hello" {
		t.Fatalf("expected non-secret preserved, got=%v", redacted["note"])
	}
}

func TestBuildInvocationInputRedactsInvalidJSON(t *testing.T) {
	call := tools.ToolCall{
		ID:   "call-invalid-json",
		Type: "function",
		Function: tools.FunctionCall{
			Name:      "echo",
			Arguments: "{not-json",
		},
	}
	input := buildInvocationInput(call)
	var payload map[string]any
	if err := json.Unmarshal(input, &payload); err != nil {
		t.Fatalf("unmarshal buildInvocationInput payload: %v", err)
	}
	toolCall, _ := payload["tool_call"].(map[string]any)
	fn, _ := toolCall["function"].(map[string]any)
	args, _ := fn["arguments"].(string)
	if strings.TrimSpace(args) != `{"_redacted":true}` {
		t.Fatalf("expected invalid JSON args to be replaced with redaction marker, got=%q", args)
	}
}

func TestServicePersistsStatusesAndRetryable(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(resultErrorTool{})
	registry.MustRegister(ctxWaitTool{})

	// setupRepositoryTest semeia apenas "echo".
	for _, name := range []string{"result_error", "ctx_wait"} {
		if err := database.DB().Create(&database.ToolCatalog{
			Name:               name,
			DisplayName:        name,
			Origin:             tools.ToolOriginBuiltin,
			AvailabilityStatus: tools.ToolAvailabilityAvailable,
		}).Error; err != nil {
			t.Fatalf("seed tool catalog (%s): %v", name, err)
		}
	}

	cfg := tools.DefaultExecutorConfig()
	cfg.ToolTimeout = 5 * time.Millisecond
	exec := tools.NewExecutor(registry, cfg)
	svc := NewService(repo, exec)

	// Failed: ToolResult.IsError
	failed := svc.Execute(userA, ExecuteRequest{
		Call:   tools.ToolCall{ID: "call-failed", Type: "function", Function: tools.FunctionCall{Name: "result_error", Arguments: `{}`}},
		Origin: Origin{Type: OriginChat, ID: "turn-status"},
		DryRun: true,
	})
	if failed.Invocation.ID == "" {
		t.Fatal("expected failed invocation id")
	}
	failedInv, err := repo.Get(userA, failed.Invocation.ID)
	if err != nil {
		t.Fatalf("get failed invocation: %v", err)
	}
	if failedInv.Status != StatusFailed {
		t.Fatalf("expected status failed, got=%s", failedInv.Status)
	}
	if !failedInv.DryRun {
		t.Fatalf("expected dry-run persisted")
	}
	if failedInv.OriginType != OriginChat || failedInv.OriginID != "turn-status" {
		t.Fatalf("expected origin persisted, got type=%s id=%s", failedInv.OriginType, failedInv.OriginID)
	}

	// Cancelled
	cancelCtx, cancel := context.WithCancel(userA)
	cancel()
	cancelled := svc.Execute(cancelCtx, ExecuteRequest{
		Call:   tools.ToolCall{ID: "call-cancel", Type: "function", Function: tools.FunctionCall{Name: "ctx_wait", Arguments: `{}`}},
		Origin: Origin{Type: OriginChat, ID: "turn-cancel"},
	})
	cancelInv, err := repo.Get(userA, cancelled.Invocation.ID)
	if err != nil {
		t.Fatalf("get cancelled invocation: %v", err)
	}
	if cancelInv.Status != StatusCancelled {
		t.Fatalf("expected status cancelled, got=%s", cancelInv.Status)
	}
	if cancelInv.Retryable {
		t.Fatalf("expected cancelled not retryable")
	}

	// Timed out
	timedOut := svc.Execute(userA, ExecuteRequest{
		Call:   tools.ToolCall{ID: "call-timeout", Type: "function", Function: tools.FunctionCall{Name: "ctx_wait", Arguments: `{}`}},
		Origin: Origin{Type: OriginChat, ID: "turn-timeout"},
	})
	timeoutInv, err := repo.Get(userA, timedOut.Invocation.ID)
	if err != nil {
		t.Fatalf("get timeout invocation: %v", err)
	}
	if timeoutInv.Status != StatusTimedOut {
		t.Fatalf("expected status timed_out, got=%s", timeoutInv.Status)
	}
	if !timeoutInv.Retryable {
		t.Fatalf("expected timed_out retryable")
	}
}
