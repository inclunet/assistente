package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// mockTool implements Tool for testing.
type mockTool struct {
	name    string
	exec    func(ctx context.Context, args json.RawMessage) (ToolResult, error)
	panics  bool
	panicV  any
}

func (m *mockTool) Name() string                { return m.name }
func (m *mockTool) Description() string          { return "mock tool" }
func (m *mockTool) Parameters() json.RawMessage  { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if m.panics {
		panic(m.panicV)
	}
	return m.exec(ctx, args)
}

func newRegistry(tools ...Tool) *Registry {
	r := NewRegistry()
	for _, t := range tools {
		r.MustRegister(t)
	}
	return r
}

func TestExecuteSingle_Success(t *testing.T) {
	tool := &mockTool{
		name: "ok_tool",
		exec: func(_ context.Context, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "hello"}, nil
		},
	}
	e := NewExecutor(newRegistry(tool), DefaultExecutorConfig())
	res := e.ExecuteOne(context.Background(), ToolCall{
		ID:       "c1",
		Function: FunctionCall{Name: "ok_tool", Arguments: `{}`},
	})

	if res.Result.IsError {
		t.Fatalf("expected success, got error: %s", res.Result.Content)
	}
	if res.ErrorKind != "" {
		t.Fatalf("expected no ErrorKind, got %q", res.ErrorKind)
	}
	if res.DurationMs < 0 {
		t.Fatal("DurationMs should be >= 0")
	}
	if res.Result.Content != "hello" {
		t.Fatalf("expected 'hello', got %q", res.Result.Content)
	}
}

func TestExecuteSingle_NotFound(t *testing.T) {
	e := NewExecutor(NewRegistry(), DefaultExecutorConfig())
	res := e.ExecuteOne(context.Background(), ToolCall{
		ID:       "c1",
		Function: FunctionCall{Name: "missing", Arguments: `{}`},
	})

	if !res.Result.IsError {
		t.Fatal("expected error for missing tool")
	}
	if res.ErrorKind != ErrorKindNotFound {
		t.Fatalf("expected ErrorKind=%q, got %q", ErrorKindNotFound, res.ErrorKind)
	}
	if res.Retryable {
		t.Fatal("not_found should not be retryable")
	}
}

func TestExecuteSingle_InvalidArgs(t *testing.T) {
	tool := &mockTool{
		name: "t",
		exec: func(_ context.Context, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "x"}, nil
		},
	}
	e := NewExecutor(newRegistry(tool), DefaultExecutorConfig())
	res := e.ExecuteOne(context.Background(), ToolCall{
		ID:       "c1",
		Function: FunctionCall{Name: "t", Arguments: `{bad json`},
	})

	if !res.Result.IsError {
		t.Fatal("expected error for invalid args")
	}
	if res.ErrorKind != ErrorKindInvalidArgs {
		t.Fatalf("expected ErrorKind=%q, got %q", ErrorKindInvalidArgs, res.ErrorKind)
	}
	if res.Retryable {
		t.Fatal("invalid_args should not be retryable")
	}
}

func TestExecuteSingle_Panic(t *testing.T) {
	tool := &mockTool{
		name:   "panic_tool",
		panics: true,
		panicV: "boom",
	}
	e := NewExecutor(newRegistry(tool), DefaultExecutorConfig())
	res := e.ExecuteOne(context.Background(), ToolCall{
		ID:       "c1",
		Function: FunctionCall{Name: "panic_tool", Arguments: `{}`},
	})

	if !res.Result.IsError {
		t.Fatal("expected error for panic")
	}
	if res.ErrorKind != ErrorKindPanic {
		t.Fatalf("expected ErrorKind=%q, got %q", ErrorKindPanic, res.ErrorKind)
	}
	if res.Retryable {
		t.Fatal("panic should not be retryable")
	}
}

func TestExecuteSingle_Timeout(t *testing.T) {
	tool := &mockTool{
		name: "slow_tool",
		exec: func(ctx context.Context, _ json.RawMessage) (ToolResult, error) {
			select {
			case <-ctx.Done():
				return ToolResult{}, ctx.Err()
			case <-time.After(5 * time.Second):
				return ToolResult{Content: "done"}, nil
			}
		},
	}
	cfg := DefaultExecutorConfig()
	cfg.ToolTimeout = 50 * time.Millisecond
	e := NewExecutor(newRegistry(tool), cfg)

	res := e.ExecuteOne(context.Background(), ToolCall{
		ID:       "c1",
		Function: FunctionCall{Name: "slow_tool", Arguments: `{}`},
	})

	if !res.Result.IsError {
		t.Fatal("expected timeout error")
	}
	if res.ErrorKind != ErrorKindTimeout {
		t.Fatalf("expected ErrorKind=%q, got %q", ErrorKindTimeout, res.ErrorKind)
	}
	if !res.Retryable {
		t.Fatal("timeout should be retryable")
	}
	if res.DurationMs <= 0 {
		t.Fatalf("DurationMs should be > 0, got %d", res.DurationMs)
	}
}

func TestExecuteSingle_ExecError(t *testing.T) {
	tool := &mockTool{
		name: "err_tool",
		exec: func(_ context.Context, _ json.RawMessage) (ToolResult, error) {
			return ToolResult{}, context.DeadlineExceeded
		},
	}
	e := NewExecutor(newRegistry(tool), DefaultExecutorConfig())
	res := e.ExecuteOne(context.Background(), ToolCall{
		ID:       "c1",
		Function: FunctionCall{Name: "err_tool", Arguments: `{}`},
	})

	if !res.Result.IsError {
		t.Fatal("expected error")
	}
	if res.ErrorKind != ErrorKindUnknown {
		t.Fatalf("expected ErrorKind=%q, got %q", ErrorKindUnknown, res.ErrorKind)
	}
}

func TestExecuteAll_Parallel(t *testing.T) {
	tool := &mockTool{
		name: "delay",
		exec: func(_ context.Context, args json.RawMessage) (ToolResult, error) {
			time.Sleep(10 * time.Millisecond)
			return ToolResult{Content: "ok"}, nil
		},
	}
	e := NewExecutor(newRegistry(tool), DefaultExecutorConfig())
	calls := make([]ToolCall, 5)
	for i := range calls {
		calls[i] = ToolCall{
			ID:       "c" + string(rune('0'+i)),
			Function: FunctionCall{Name: "delay", Arguments: `{}`},
		}
	}

	start := time.Now()
	results := e.ExecuteAll(context.Background(), calls)
	elapsed := time.Since(start)

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Result.IsError {
			t.Fatalf("unexpected error: %s", r.Result.Content)
		}
	}
	// Se paralelo, deveria levar ~10ms, não ~50ms
	if elapsed > 200*time.Millisecond {
		t.Fatalf("execution too slow for parallel: %v", elapsed)
	}
}

func TestTruncateUTF8_Safe(t *testing.T) {
	// "café" em UTF-8: c(1) a(1) f(1) é(2) = 5 bytes
	s := "café"
	if len(s) != 5 {
		t.Fatalf("expected 5 bytes, got %d", len(s))
	}

	// Truncar no meio do 'é' (byte 4) deve recuar para byte 3
	result := truncateUTF8(s, 4)
	if !utf8.ValidString(result) {
		t.Fatalf("result is not valid UTF-8: %q", result)
	}
	if result != "caf" {
		t.Fatalf("expected 'caf', got %q", result)
	}

	// Truncar exatamente em 5 retorna a string inteira
	result = truncateUTF8(s, 5)
	if result != "café" {
		t.Fatalf("expected 'café', got %q", result)
	}

	// Truncar em 3 é safe (limite de rune)
	result = truncateUTF8(s, 3)
	if result != "caf" {
		t.Fatalf("expected 'caf', got %q", result)
	}
}

func TestTruncateUTF8_LargeResult(t *testing.T) {
	tool := &mockTool{
		name: "big",
		exec: func(_ context.Context, _ json.RawMessage) (ToolResult, error) {
			// Gera 200KB de conteúdo com caracteres multibyte
			content := strings.Repeat("日本語テスト ", 20000) // ~120KB+ de conteúdo multibyte
			return ToolResult{Content: content}, nil
		},
	}
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 1024 // 1KB para teste rápido
	e := NewExecutor(newRegistry(tool), cfg)

	res := e.ExecuteOne(context.Background(), ToolCall{
		ID:       "c1",
		Function: FunctionCall{Name: "big", Arguments: `{}`},
	})

	if res.Result.IsError {
		t.Fatalf("unexpected error: %s", res.Result.Content)
	}
	if !utf8.ValidString(res.Result.Content) {
		t.Fatal("truncated result is not valid UTF-8")
	}
	if res.Result.Metadata["truncated"] != true {
		t.Fatal("expected metadata.truncated=true")
	}
	if !strings.Contains(res.Result.Content, "[TRUNCADO:") {
		t.Fatal("expected truncation notice in content")
	}
}

func TestExecuteContextCancellation(t *testing.T) {
	tool := &mockTool{
		name: "waiting",
		exec: func(ctx context.Context, _ json.RawMessage) (ToolResult, error) {
			<-ctx.Done()
			return ToolResult{}, ctx.Err()
		},
	}
	e := NewExecutor(newRegistry(tool), DefaultExecutorConfig())

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	res := e.ExecuteOne(ctx, ToolCall{
		ID:       "c1",
		Function: FunctionCall{Name: "waiting", Arguments: `{}`},
	})

	if !res.Result.IsError {
		t.Error("expected IsError=true after cancellation")
	}
}
