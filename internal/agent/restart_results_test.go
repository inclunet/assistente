package agent

import (
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
)

func TestRestartEvidencePreservesMediaAndPartial(t *testing.T) {
	content := "arquivo salvo"
	msgs := []llm.Message{{Role: "user", MessageID: "turn", Content: []interface{}{map[string]interface{}{"type": "image_url"}}}, {Role: "assistant", Content: "prefixo"}}
	results := []database.RestartToolResult{{Name: "write_file", Status: "succeeded", Output: `{"content":"arquivo salvo"}`}}
	if err := AddRestartResults(msgs, "turn", results, llm.ChatParams{}, false); err != nil {
		t.Fatal(err)
	}
	parts := msgs[0].Content.([]interface{})
	if len(parts) != 2 || parts[0].(map[string]interface{})["type"] != "image_url" || !strings.Contains(parts[1].(map[string]interface{})["text"].(string), content) || msgs[1].Content != "prefixo" {
		t.Fatalf("mensagens: %v", msgs)
	}
	if err := AddRestartResults(msgs, "turn", results, llm.ChatParams{ContextWindow: 1}, false); err == nil {
		t.Fatal("enviaria a retomada sem espaço para o registro")
	}
}

func TestRestartKeepsSmallConfirmationWhenNewerOutputCannotFit(t *testing.T) {
	large := strings.Repeat("resultado-extenso", 2500)
	rows := []database.RestartToolResult{
		{ID: "recent", Status: "succeeded", Output: `{"content":"` + large + `"}`},
		{ID: "previous", Status: "succeeded", Output: `{"content":"ação confirmada"}`},
	}
	msgs := []llm.Message{{Role: "user", MessageID: "turn", Content: "pergunta"}}
	if err := AddRestartResults(msgs, "turn", rows, llm.ChatParams{ContextWindow: 1024}, false); err != nil {
		t.Fatal(err)
	}
	text := msgs[0].GetContentAsString()
	if strings.Contains(text, large) || !strings.Contains(text, "ação confirmada") || !strings.Contains(text, "recent") {
		t.Fatal("não preservou a confirmação que cabia e a referência da saída maior")
	}
}
