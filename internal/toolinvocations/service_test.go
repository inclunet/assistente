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

func TestOutputForPersistence_CapsLargeOutputAndDropsLargeMetadata(t *testing.T) {
	max := 256
	svc := &Service{persistMaxResultSize: max}

	result := tools.ToolResult{
		Content: strings.Repeat("x", 4096),
		Metadata: map[string]any{
			"huge": strings.Repeat("m", 4096),
		},
	}

	out := svc.outputForPersistence(result)
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
	if len(out) > max {
		t.Fatalf("persisted output size = %d, want <= %d", len(out), max)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("persisted output is not valid JSON: %v (out=%q)", err, string(out))
	}
	if _, ok := payload["content"].(string); !ok {
		t.Fatalf("expected content string, got=%T", payload["content"])
	}
	if _, ok := payload["is_error"].(bool); !ok {
		t.Fatalf("expected is_error bool, got=%T", payload["is_error"])
	}
	// Metadata deve ter sido dropada para caber no limite.
	if _, ok := payload["metadata"]; ok {
		t.Fatalf("expected metadata to be dropped, got=%v", payload["metadata"])
	}
}

func TestOutputForPersistence_DropsNonSerializableMetadataAndStillCapsSize(t *testing.T) {
	max := 256
	svc := &Service{persistMaxResultSize: max}

	result := tools.ToolResult{
		Content:  strings.Repeat("y", 4096),
		IsError:  true,
		Metadata: map[string]any{"bad": func() {}},
	}

	out := svc.outputForPersistence(result)
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
	if len(out) > max {
		t.Fatalf("persisted output size = %d, want <= %d", len(out), max)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("persisted output is not valid JSON: %v (out=%q)", err, string(out))
	}
	if payload["is_error"] != true {
		t.Fatalf("expected is_error=true, got=%v", payload["is_error"])
	}
	if _, ok := payload["metadata"]; ok {
		t.Fatalf("expected metadata to be dropped when non-serializable")
	}
}

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

// captureInvocationTool registra o ID da invocação corrente visto via ctx
// (AEP-0068): permite verificar que o Service carimba WithCurrentInvocationID.
type captureInvocationTool struct {
	seen *string
}

func (captureInvocationTool) Name() string        { return "echo" }
func (captureInvocationTool) Description() string { return "echo" }
func (captureInvocationTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t captureInvocationTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	if t.seen != nil {
		*t.seen = CurrentInvocationID(ctx)
	}
	return tools.ToolResult{Content: "ok"}, nil
}

func TestServiceInheritsParentInvocationFromContext(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	var seenCurrent string
	registry := tools.NewRegistry()
	registry.MustRegister(captureInvocationTool{seen: &seenCurrent})
	svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))

	// Sem ParentInvocationID explícito no request, mas carimbado no ctx:
	// deve ser herdado e persistido na invocação.
	ctx := WithParentInvocationID(userA, "parent-inv-1")
	result := svc.Execute(ctx, ExecuteRequest{
		Call:   tools.ToolCall{ID: "call-1", Type: "function", Function: tools.FunctionCall{Name: "echo", Arguments: `{}`}},
		Origin: Origin{Type: OriginChat, ID: "turn-1"},
	})
	if result.Invocation.ID == "" {
		t.Fatal("expected invocation id")
	}
	got, err := repo.Get(userA, result.Invocation.ID)
	if err != nil {
		t.Fatalf("get invocation: %v", err)
	}
	if got.ParentInvocationID != "parent-inv-1" {
		t.Fatalf("ParentInvocationID herdado do ctx esperado parent-inv-1, veio %q", got.ParentInvocationID)
	}
	// A tool deve enxergar o ID da própria invocação no ctx.
	if seenCurrent != result.Invocation.ID {
		t.Fatalf("CurrentInvocationID esperado %q, veio %q", result.Invocation.ID, seenCurrent)
	}
}

func TestServiceExplicitParentInvocationWins(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(echoTool{})
	svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))

	ctx := WithParentInvocationID(userA, "from-ctx")
	result := svc.Execute(ctx, ExecuteRequest{
		Call:               tools.ToolCall{ID: "call-1", Type: "function", Function: tools.FunctionCall{Name: "echo", Arguments: `{}`}},
		Origin:             Origin{Type: OriginChat, ID: "turn-1"},
		ParentInvocationID: "explicit",
	})
	got, err := repo.Get(userA, result.Invocation.ID)
	if err != nil {
		t.Fatalf("get invocation: %v", err)
	}
	if got.ParentInvocationID != "explicit" {
		t.Fatalf("ParentInvocationID explícito deve vencer; veio %q", got.ParentInvocationID)
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

type ctxBlockTool struct {
	started chan<- struct{}
}

func (t ctxBlockTool) Name() string                { return "ctx_block" }
func (t ctxBlockTool) Description() string         { return "signals start then waits for ctx.Done" }
func (t ctxBlockTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t ctxBlockTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	if t.started != nil {
		select {
		case t.started <- struct{}{}:
		default:
		}
	}
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

func TestBuildInvocationDisplayMetadataRedactsArguments(t *testing.T) {
	svc := &Service{persistMaxInputSize: tools.DefaultMaxResultSize}
	metadata := svc.buildInvocationDisplayMetadata(tools.ToolCall{
		ID:   "call-display-redact",
		Type: "function",
		Function: tools.FunctionCall{
			Name: "echo",
			Arguments: `{
				"password":"123",
				"authorization":"Bearer abc.def.ghi",
				"note":"hello"
			}`,
		},
	}, 1, 12, false)
	raw := string(metadata)
	if strings.Contains(raw, "123") || strings.Contains(raw, "Bearer abc.def.ghi") {
		t.Fatalf("display metadata leaked sensitive arguments: %s", raw)
	}
	var payload struct {
		Display struct {
			Arguments string `json:"arguments"`
		} `json:"display"`
	}
	if err := json.Unmarshal(metadata, &payload); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(payload.Display.Arguments), &args); err != nil {
		t.Fatalf("unmarshal redacted display args: %v", err)
	}
	if args["password"] != "[redacted]" || args["authorization"] != "[redacted]" || args["note"] != "hello" {
		t.Fatalf("unexpected redacted display args: %#v", args)
	}
}

func TestBuildInvocationDisplayMetadataTruncatesArguments(t *testing.T) {
	max := 256
	svc := &Service{persistMaxInputSize: max}
	metadata := svc.buildInvocationDisplayMetadata(tools.ToolCall{
		ID:   "call-display-truncate",
		Type: "function",
		Function: tools.FunctionCall{
			Name:      "echo",
			Arguments: `{"value":"` + strings.Repeat("a", 4096) + `"}`,
		},
	}, 2, 34, false)
	if len(metadata) > max {
		t.Fatalf("metadata size = %d, want <= %d", len(metadata), max)
	}
	var payload struct {
		Display struct {
			ArgumentsTruncated        bool `json:"_arguments_truncated"`
			ArgumentsOriginalSizeByte int  `json:"_arguments_original_size_bytes"`
		} `json:"display"`
	}
	if err := json.Unmarshal(metadata, &payload); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if !payload.Display.ArgumentsTruncated || payload.Display.ArgumentsOriginalSizeByte == 0 {
		t.Fatalf("expected display arguments truncation metadata, got %#v", payload.Display)
	}
	var raw map[string]map[string]any
	if err := json.Unmarshal(metadata, &raw); err != nil {
		t.Fatalf("unmarshal raw metadata: %v", err)
	}
	if _, ok := raw["display"]["arguments"]; ok {
		t.Fatalf("expected truncated display arguments to be omitted, got %s", string(metadata))
	}
	if strings.Contains(string(metadata), "TRUNCADO") {
		t.Fatalf("metadata should not contain textual truncation suffix: %s", string(metadata))
	}
}

func TestBuildInvocationDisplayMetadataUsesCompactFallbackWithinLimit(t *testing.T) {
	max := 64
	metadata := buildInvocationDisplayMetadata(tools.ToolCall{
		ID:   "call-display-compact",
		Type: "function",
		Function: tools.FunctionCall{
			Name:      "mcp_" + strings.Repeat("server", 20) + "__" + strings.Repeat("tool", 20),
			Arguments: `{"value":"` + strings.Repeat("a", 1024) + `"}`,
		},
	}, strings.Repeat("a", 1024), 1, 12, false, max)
	if len(metadata) > max {
		t.Fatalf("metadata size = %d, want <= %d: %s", len(metadata), max, string(metadata))
	}
	if !json.Valid(metadata) {
		t.Fatalf("metadata should remain valid json: %s", string(metadata))
	}
}

func TestBuildInvocationDisplayPayloadDetectsOnlyOfficialMCPBridgeNames(t *testing.T) {
	tests := []struct {
		name            string
		wantOrigin      string
		wantServerLabel string
		wantName        string
	}{
		{
			name:            "mcp_github__search_code",
			wantOrigin:      "mcp_bridge",
			wantServerLabel: "github",
			wantName:        "search_code",
		},
		{
			name:            "builtin__with_separator",
			wantOrigin:      "builtin",
			wantServerLabel: "",
			wantName:        "builtin__with_separator",
		},
		{
			name:            "mcp___missing_server",
			wantOrigin:      "builtin",
			wantServerLabel: "",
			wantName:        "mcp___missing_server",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := buildInvocationDisplayPayload(tools.ToolCall{
				Type: "function",
				Function: tools.FunctionCall{
					Name:      tt.name,
					Arguments: "{}",
				},
			}, "{}", 1, 12, false, false, 0)
			display, _ := payload["display"].(map[string]any)
			if display["origin"] != tt.wantOrigin || display["server_label"] != tt.wantServerLabel || display["name"] != tt.wantName {
				t.Fatalf("unexpected display payload: %#v", display)
			}
		})
	}
}

func TestServicePersistsStatusesAndRetryable(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(resultErrorTool{})
	registry.MustRegister(ctxWaitTool{})
	started := make(chan struct{}, 1)
	registry.MustRegister(ctxBlockTool{started: started})

	// setupRepositoryTest semeia apenas "echo".
	for _, name := range []string{"result_error", "ctx_wait", "ctx_block"} {
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

	// Cancelled (cancela DURANTE execução)
	cancelCtx, cancel := context.WithCancel(userA)
	go func() {
		<-started
		cancel()
	}()
	cancelled := svc.Execute(cancelCtx, ExecuteRequest{
		Call:   tools.ToolCall{ID: "call-cancel", Type: "function", Function: tools.FunctionCall{Name: "ctx_block", Arguments: `{}`}},
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

func TestRecordTreatsNonNoneErrorKindAsFailed(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(echoTool{})
	svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))

	inv, err := svc.Record(userA, RecordRequest{
		Call: tools.ToolCall{
			ID:   "call-rec",
			Type: "function",
			Function: tools.FunctionCall{
				Name:      "echo",
				Arguments: `{"value":"ok"}`,
			},
		},
		Origin:       Origin{Type: OriginChat, ID: "turn-rec"},
		Result:       tools.ToolResult{Content: ""},
		ErrorKind:    tools.ErrorKindNotFound,
		ErrorMessage: "",
		Retryable:    false,
		DurationMs:   1,
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if inv.Status != StatusFailed {
		t.Fatalf("expected status failed, got=%s", inv.Status)
	}
}

func TestServiceTruncatesPersistedInputWhenTooLarge(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	registry.MustRegister(echoTool{})
	svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))

	// Cria um argumento grande o suficiente para estourar o limite padrão (100KB).
	big := strings.Repeat("a", tools.DefaultMaxResultSize*2)
	args := `{"value":"` + big + `"}`

	result := svc.Execute(userA, ExecuteRequest{
		Call: tools.ToolCall{
			ID:   "call-big-input",
			Type: "function",
			Function: tools.FunctionCall{
				Name:      "echo",
				Arguments: args,
			},
		},
		Origin: Origin{Type: OriginChat, ID: "turn-big"},
	})
	if result.Invocation.ID == "" {
		t.Fatal("expected invocation id")
	}
	inv, err := repo.Get(userA, result.Invocation.ID)
	if err != nil {
		t.Fatalf("get invocation: %v", err)
	}
	if len(inv.Input) == 0 {
		t.Fatal("expected input persisted")
	}
	if len(inv.Input) > tools.DefaultMaxResultSize {
		t.Fatalf("expected input capped, got=%d bytes", len(inv.Input))
	}
	var payload map[string]any
	if err := json.Unmarshal(inv.Input, &payload); err != nil {
		t.Fatalf("unmarshal input payload: %v", err)
	}
	trunc, _ := payload["_input_truncated"].(bool)
	if !trunc {
		t.Fatalf("expected _input_truncated=true, got=%v", payload["_input_truncated"])
	}
}
