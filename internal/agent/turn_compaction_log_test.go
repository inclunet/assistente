package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/tools"
)

func TestToolContextOutcome(t *testing.T) {
	for _, tc := range []struct {
		name, outcome string
		window, keep  int
		cancel        bool
	}{
		{"zero recuperado", "prepared_full", 120, 1, false},
		{"resultado reduzido", "prepared_reduced", 300, 10, false},
		{"mínimo protegido", "context_exhausted", 120, 10, false},
		{"cancelamento", "cancelled", 120, 1, true},
		{"arredondamento de duas ferramentas", "context_exhausted", 120, 10, false},
		{"resumo indisponível", "context_exhausted", 120, 1, false},
		{"sem pressão", "", 2000, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "diagnostico")
			if err != nil {
				t.Fatal(err)
			}
			old := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(f, nil)))
			t.Cleanup(func() { slog.SetDefault(old); _ = f.Close() })
			params := llm.ChatParams{ContextWindow: tc.window}
			ctx, cancel := context.WithCancel(buildAgenticInvocationContext(t.Context(), params, "conversa-1", "turno-1"))
			defer cancel()
			secret := "conteudo-privado"
			r := &agenticLoopRunner{params: params, messages: markCompactionTarget([]llm.Message{
				{Role: "user", Content: secret}, {Role: "assistant", Content: strings.Repeat("x", 450)}, {Role: "user", Content: "pedido"}, {Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-1"}}},
			}), compaction: &TurnCompaction{MinKeep: tc.keep, Summarize: func(context.Context, string, string, string) (string, error) {
				if tc.name == "resumo indisponível" {
					return "", nil
				}
				if tc.cancel {
					cancel()
					return "", ctx.Err()
				}
				return "resumo", nil
			}}}
			content := secret
			if tc.outcome == "prepared_reduced" {
				content = strings.Repeat(secret, 100)
			}
			result := tools.ToolResult{Content: content, RawExact: true}
			original := tools.ContentForModel(result)
			contents := []string{original}
			batch := []tools.ToolExecutionResult{{Result: result}}
			if tc.name == "arredondamento de duas ferramentas" {
				r.messages = []llm.Message{{Role: "user", Content: "a"}}
				r.params.ContextWindow = 16 // 14 tokens seguros - 5 do pedido - 8 de overhead = 1 disponível.
				batch = []tools.ToolExecutionResult{{Result: tools.ToolResult{Content: "a"}}, {Result: tools.ToolResult{Content: "b"}}}
				contents = []string{"a", "b"}
			}
			ready := r.prepareToolContext(ctx, 7, batch, contents)
			raw, err := os.ReadFile(f.Name())
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), secret) {
				t.Fatal("conteúdo exposto no diagnóstico")
			}
			count := 0
			for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				if line == "" {
					continue
				}
				var entry map[string]any
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					t.Fatal(err)
				}
				if entry["component"] != "agent.context" {
					continue
				}
				if entry["msg"] != "tool_context_outcome" {
					t.Fatalf("registro duplicado de contexto: %v", entry)
				}
				count++
				if entry["outcome"] != tc.outcome || entry["conversation_id"] != "conversa-1" || entry["turn_id"] != "turno-1" || entry["iteration"] != float64(7) {
					t.Fatalf("diagnóstico incorreto: %v", entry)
				}
				if tc.name == "resumo indisponível" && entry["summary_failures"] != float64(1) {
					t.Fatalf("falha não identificada: %v", entry)
				}
				if tc.name == "arredondamento de duas ferramentas" && (entry["available_tokens_before"] != float64(1) || entry["tool_result_tokens_estimate"] != float64(2)) {
					t.Fatalf("pressão incorreta: %v", entry)
				}
				if _, ok := entry["compaction_ms"].(float64); !ok {
					t.Fatal("sem duração")
				}
				if tc.outcome == "prepared_full" && (entry["available_tokens_before"] != float64(0) || entry["available_tokens_after"].(float64) <= 0 || entry["summaries_applied"] != float64(1) || contents[0] != original) {
					t.Fatalf("recuperação incorreta: %v", entry)
				}
				if tc.outcome == "prepared_reduced" && contents[0] == original {
					t.Fatal("resultado deveria ter mudado")
				}
			}
			wantCount := 1
			if tc.outcome == "" {
				wantCount = 0
			}
			if count != wantCount || ready != (tc.outcome != "context_exhausted" && tc.outcome != "cancelled") {
				t.Fatalf("registros=%d pronto=%v", count, ready)
			}
		})
	}
}
