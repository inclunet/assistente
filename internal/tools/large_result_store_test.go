package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestProtectModelResultUsesAnnotationsAndCanResume(t *testing.T) {
	original := strings.Repeat("abcç", 20_000)
	protected, ok := ProtectModelResult(ToolResult{Content: original}, 1024)
	if !ok || len(protected.Content) > 1024 {
		t.Fatalf("proteção falhou: ok=%v bytes=%d", ok, len(protected.Content))
	}
	window := protected.Annotations.OutputWindow
	if !window.HasMore || window.ResultID == "" || window.NextOffset != len(protected.Content) {
		t.Fatalf("anotação inválida: %+v", window)
	}
	if strings.Contains(strings.ToUpper(protected.Content), "TRUNCAD") {
		t.Fatal("aviso textual contaminou conteúdo")
	}
	raw, _ := json.Marshal(map[string]any{"result_id": window.ResultID, "offset": window.NextOffset, "limit": 777})
	next, err := NewReadToolResult().Execute(context.Background(), raw)
	if err != nil || next.IsError {
		t.Fatalf("retomada falhou: err=%v result=%+v", err, next)
	}
	if protected.Content+next.Content != original[:len(protected.Content)+len(next.Content)] {
		t.Fatal("retomada não preservou bytes exatos")
	}
}

func TestProtectExternalModelResultDelimitsPreviewWithoutChangingStoredJSON(t *testing.T) {
	original := `{"items":[` + strings.Repeat(`{"value":"abcdef"},`, 1000) + `null]}`
	protected, ok := ProtectExternalModelResult(ToolResult{Content: original}, 512)
	if !ok || !strings.HasPrefix(protected.Content, "--- INÍCIO DA PRÉVIA MCP") {
		t.Fatalf("prévia MCP não delimitada: %q", protected.Content)
	}
	window := protected.Annotations.OutputWindow
	stored, found := loadModelResult(window.ResultID)
	if !found || stored != original || !json.Valid([]byte(stored)) {
		t.Fatal("JSON MCP integral não foi preservado")
	}
}

func TestExecutorRejectsRawExactInsteadOfTruncating(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "raw_test",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: strings.Repeat("x", 2048), RawExact: true}, nil
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 128
	got := NewExecutor(registry, cfg).ExecuteOne(context.Background(), ToolCall{
		ID: "call-raw", Function: FunctionCall{Name: "raw_test", Arguments: `{}`},
	})
	if got.ErrorCode != "raw_result_too_large" || !got.Result.IsError ||
		strings.Contains(got.Result.Content, strings.Repeat("x", 20)) {
		t.Fatalf("raw foi cortado silenciosamente: %+v", got)
	}
}

func TestExecutorMakesLargeMCPJSONRecoverable(t *testing.T) {
	original := `{"items":[` + strings.Repeat(`{"id":1},`, 2000) + `null]}`
	registry := NewRegistry()
	registry.MustRegister(&mockTool{
		name: "mcp_server__large",
		exec: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: original}, nil
		},
	})
	cfg := DefaultExecutorConfig()
	cfg.MaxResultSize = 1024
	got := NewExecutor(registry, cfg).ExecuteOne(context.Background(), ToolCall{
		ID: "call-mcp", Function: FunctionCall{Name: "mcp_server__large", Arguments: `{}`},
	})
	if got.Result.IsError || got.Result.Annotations == nil || got.Result.Annotations.OutputWindow == nil {
		t.Fatalf("MCP grande não ficou recuperável: %+v", got)
	}
	if !strings.Contains(got.Result.Content, "PRÉVIA MCP") {
		t.Fatalf("prévia MCP não delimitada: %q", got.Result.Content)
	}
	stored, ok := loadModelResult(got.Result.Annotations.OutputWindow.ResultID)
	if !ok || stored != original || !json.Valid([]byte(stored)) {
		t.Fatal("executor corrompeu JSON MCP preservado")
	}
}
