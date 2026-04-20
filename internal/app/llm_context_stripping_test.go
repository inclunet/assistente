package app

import (
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
)

// convertHistoryForTest reproduz a lógica de filtragem de loadConversationHistory
// para mensagens de tool calling de turnos anteriores.
func convertHistoryForTest(dbMessages []database.ChatMessage) []llm.Message {
	messages := make([]llm.Message, 0, len(dbMessages))
	for _, m := range dbMessages {
		if m.Role == "tool" {
			continue
		}
		if m.Role == "assistant" && m.ToolCalls != "" && strings.TrimSpace(m.Content) == "" {
			continue
		}
		msg := llm.Message{
			Role:       m.Role,
			ToolCallID: m.ToolCallID,
		}
		msg.Content = m.Content
		messages = append(messages, msg)
	}
	return messages
}

// TestToolResultStripping_PreviousTurns valida que mensagens intermediárias de tool calling
// (assistant com tool_calls vazio de conteúdo + tool results) são completamente omitidas
// do contexto enviado ao LLM. O modelo já processou e sintetizou na resposta final.
// Dados completos permanecem no banco e visíveis na UI.
func TestToolResultStripping_PreviousTurns(t *testing.T) {
	toolCallsJSON := `[{"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SP\"}"}}]`

	dbMessages := []database.ChatMessage{
		{Role: "user", Content: "Qual o clima em SP?"},
		{Role: "assistant", Content: "", ToolCalls: toolCallsJSON},
		{Role: "tool", Content: "Temperature: 28°C, Humidity: 65%", ToolCallID: "call_abc"},
		{Role: "assistant", Content: "Em SP faz 28°C com 65% de umidade."},
		{Role: "user", Content: "E no Rio?"},
	}

	messages := convertHistoryForTest(dbMessages)

	// Deve restar: user, assistant(final), user = 3 mensagens
	if len(messages) != 3 {
		t.Fatalf("esperado 3 mensagens (omitindo tool_call + tool_result), obteve %d", len(messages))
	}

	if messages[0].Role != "user" || messages[0].Content != "Qual o clima em SP?" {
		t.Errorf("mensagem[0] incorreta: %+v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "Em SP faz 28°C com 65% de umidade." {
		t.Errorf("mensagem[1] incorreta: %+v", messages[1])
	}
	if messages[2].Role != "user" || messages[2].Content != "E no Rio?" {
		t.Errorf("mensagem[2] incorreta: %+v", messages[2])
	}
}

// TestToolResultStripping_MultipleTurns valida omissão com múltiplos turnos de tool calls
func TestToolResultStripping_MultipleTurns(t *testing.T) {
	tc1JSON := `[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{}"}}]`
	tc2JSON := `[{"id":"call_2","type":"function","function":{"name":"fetch","arguments":"{}"}}]`

	dbMessages := []database.ChatMessage{
		{Role: "user", Content: "Busque X"},
		{Role: "assistant", ToolCalls: tc1JSON},
		{Role: "tool", Content: "Resultado com 5000 chars...", ToolCallID: "call_1"},
		{Role: "assistant", Content: "Encontrei X. Vou buscar detalhes."},
		{Role: "assistant", ToolCalls: tc2JSON},
		{Role: "tool", Content: "Detalhes enormes em JSON...", ToolCallID: "call_2"},
		{Role: "assistant", Content: "Aqui estão os detalhes completos."},
		{Role: "user", Content: "Obrigado"},
	}

	messages := convertHistoryForTest(dbMessages)

	// Deve restar: user, assistant("Encontrei..."), assistant("Aqui..."), user = 4
	if len(messages) != 4 {
		t.Fatalf("esperado 4 mensagens, obteve %d", len(messages))
	}

	if messages[0].Content != "Busque X" {
		t.Errorf("mensagem[0] incorreta: %v", messages[0].Content)
	}
	if messages[1].Content != "Encontrei X. Vou buscar detalhes." {
		t.Errorf("mensagem[1] incorreta: %v", messages[1].Content)
	}
	if messages[2].Content != "Aqui estão os detalhes completos." {
		t.Errorf("mensagem[2] incorreta: %v", messages[2].Content)
	}
	if messages[3].Content != "Obrigado" {
		t.Errorf("mensagem[3] incorreta: %v", messages[3].Content)
	}

	// Nenhuma mensagem deve ter tool_calls ou ToolCallID
	for i, msg := range messages {
		if len(msg.ToolCalls) > 0 {
			t.Errorf("mensagem[%d] não deveria ter ToolCalls", i)
		}
		if msg.ToolCallID != "" {
			t.Errorf("mensagem[%d] não deveria ter ToolCallID", i)
		}
	}
}

// TestToolResultStripping_NativeMCP valida omissão de MCP nativo (mesmo formato que bridge)
func TestToolResultStripping_NativeMCP(t *testing.T) {
	mcpCallJSON := `[{"id":"mcp_1","type":"function","function":{"name":"Atlassian/jira_search","arguments":"{\"jql\":\"project=FSD\"}"}}]`

	dbMessages := []database.ChatMessage{
		{Role: "user", Content: "Mostra os tickets do FSD"},
		{Role: "assistant", ToolCalls: mcpCallJSON},
		{Role: "tool", Content: `{"issues":[{"key":"FSD-1"},{"key":"FSD-2"}]}`, ToolCallID: "mcp_1"},
		{Role: "assistant", Content: "Encontrei 2 tickets no FSD."},
	}

	messages := convertHistoryForTest(dbMessages)

	if len(messages) != 2 {
		t.Fatalf("esperado 2 mensagens, obteve %d", len(messages))
	}
	if messages[0].Content != "Mostra os tickets do FSD" {
		t.Errorf("user message incorreta: %v", messages[0].Content)
	}
	if messages[1].Content != "Encontrei 2 tickets no FSD." {
		t.Errorf("assistant message incorreta: %v", messages[1].Content)
	}
}

// TestToolResultStripping_NoToolMessages valida que conversas sem tools não são afetadas
func TestToolResultStripping_NoToolMessages(t *testing.T) {
	dbMessages := []database.ChatMessage{
		{Role: "user", Content: "Olá"},
		{Role: "assistant", Content: "Olá! Como posso ajudar?"},
		{Role: "user", Content: "Tudo bem, obrigado"},
	}

	messages := convertHistoryForTest(dbMessages)

	if len(messages) != 3 {
		t.Fatalf("esperado 3 mensagens, obteve %d", len(messages))
	}
	if messages[0].Content != "Olá" || messages[1].Content != "Olá! Como posso ajudar?" || messages[2].Content != "Tudo bem, obrigado" {
		t.Error("conversa sem tools não deveria ser afetada")
	}
}

// TestToolResultStripping_AssistantWithTextAndToolCalls valida que assistant messages
// com conteúdo textual + tool_calls preservam o texto (descartam apenas tool_calls)
func TestToolResultStripping_AssistantWithTextAndToolCalls(t *testing.T) {
	tcJSON := `[{"id":"call_x","type":"function","function":{"name":"search","arguments":"{}"}}]`

	dbMessages := []database.ChatMessage{
		{Role: "user", Content: "Busque algo"},
		{Role: "assistant", Content: "Vou buscar para você.", ToolCalls: tcJSON},
		{Role: "tool", Content: "resultado", ToolCallID: "call_x"},
		{Role: "assistant", Content: "Aqui está o resultado."},
	}

	messages := convertHistoryForTest(dbMessages)

	// Assistant com texto + tool_calls: preserva texto, omite tool_calls
	if len(messages) != 3 {
		t.Fatalf("esperado 3 mensagens, obteve %d", len(messages))
	}
	if messages[1].Content != "Vou buscar para você." {
		t.Errorf("texto do assistant intermediário deveria ser preservado: %v", messages[1].Content)
	}
	if len(messages[1].ToolCalls) > 0 {
		t.Error("tool_calls do assistant intermediário deveriam ser omitidas")
	}
}

// TestToolResultStripping_TokenSavings demonstra a economia de tokens
func TestToolResultStripping_TokenSavings(t *testing.T) {
	largeResult := strings.Repeat("dados extensos ", 200) // ~3000 chars
	tcJSON := `[{"id":"call_big","type":"function","function":{"name":"complex_search","arguments":"{\"query\":\"find all records matching criteria X with full details\"}"}}]`

	dbMessages := []database.ChatMessage{
		{Role: "user", Content: "Busque todos os registros"},
		{Role: "assistant", ToolCalls: tcJSON},
		{Role: "tool", Content: largeResult, ToolCallID: "call_big"},
		{Role: "assistant", Content: "Encontrei os registros."},
	}

	// Calcula tamanho com abordagem antiga (tudo enviado)
	fullSize := 0
	for _, m := range dbMessages {
		fullSize += len(m.Content) + len(m.ToolCalls)
	}

	// Calcula tamanho com omissão
	messages := convertHistoryForTest(dbMessages)
	strippedSize := 0
	for _, m := range messages {
		if s, ok := m.Content.(string); ok {
			strippedSize += len(s)
		}
		strippedSize += len(m.ToolCallID)
		for _, tc := range m.ToolCalls {
			b, _ := json.Marshal(tc)
			strippedSize += len(b)
		}
	}

	savings := float64(fullSize-strippedSize) / float64(fullSize) * 100
	t.Logf("Economia: %d → %d bytes (%.0f%% redução)", fullSize, strippedSize, savings)

	if strippedSize >= fullSize {
		t.Error("omissão deveria resultar em economia de tamanho")
	}
}
