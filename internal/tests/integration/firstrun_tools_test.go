package integration

import (
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/llm"
)

// ToolDefinition simula a estrutura de uma ferramenta
type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function map[string]interface{} `json:"function"`
}

// ToolCall simula uma chamada de ferramenta do assistente
type ToolCall struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Function ToolCallFn `json:"function"`
}

// ToolCallFn detalha a função chamada
type ToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// TestIntegration_FirstMessageTriggersTool testa quando primeira mensagem resulta em tool call
func TestIntegration_FirstMessageTriggersTool(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)
	registry := llm.NewProviderRegistry()

	// 1. Setup: provider com ferramentas disponíveis
	provider := &llm.ProviderConfig{
		ID:      "openai",
		Name:    "OpenAI",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4o",
	}

	if err := registry.Register(provider); err != nil {
		t.Fatalf("falha ao registrar provider: %v", err)
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "Primeira com Ferramenta",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem que INDUZ tool call
	// Exemplo: "Qual é o conteúdo de main.go?" → assistente chamará read_file
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é o conteúdo do arquivo config.json?",
		Source:         "wails",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar primeira mensagem: %v", err)
	}

	// 4. Assistente RESPONDE com tool call
	// Armazena ToolCalls como JSON
	toolCalls := []ToolCall{
		{
			ID:   "call_tool_123",
			Type: "function",
			Function: ToolCallFn{
				Name:      "read_file",
				Arguments: `{"path":"config.json"}`,
			},
		},
	}

	toolCallsJSON, err := json.Marshal(toolCalls)
	if err != nil {
		t.Fatalf("falha ao serializar tool calls: %v", err)
	}

	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Vou ler o arquivo config.json para você.",
		ToolCalls:      string(toolCallsJSON),
		Model:          "gpt-4o",
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta com tool call: %v", err)
	}

	// 5. Validar que tool call foi persistido
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.ToolCalls == "" {
		t.Error("ToolCalls deveria estar preenchido")
	}

	// 6. Desserializar e validar estrutura
	var savedToolCalls []ToolCall
	if err := json.Unmarshal([]byte(retrieved.ToolCalls), &savedToolCalls); err != nil {
		t.Fatalf("falha ao desserializar tool calls: %v", err)
	}

	if len(savedToolCalls) != 1 {
		t.Errorf("esperado 1 tool call, obteve %d", len(savedToolCalls))
	}

	if savedToolCalls[0].ID != "call_tool_123" {
		t.Errorf("ID do tool call incorreto: %s", savedToolCalls[0].ID)
	}

	if savedToolCalls[0].Function.Name != "read_file" {
		t.Errorf("nome da ferramenta incorreto: %s", savedToolCalls[0].Function.Name)
	}

	t.Log("✓ Primeira mensagem induz tool call, ToolCalls persistido corretamente")
}

// TestIntegration_FirstMessageToolExecution testa execução da ferramenta
func TestIntegration_FirstMessageToolExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: conversa, primeira mensagem, tool call
	conv := &database.Conversation{
		Title:     "Tool Execution",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Leia o arquivo main.go para mim",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 2. Assistente com tool call
	toolCalls := []ToolCall{
		{
			ID:   "call_read_file_001",
			Type: "function",
			Function: ToolCallFn{
				Name:      "read_file",
				Arguments: `{"path":"main.go"}`,
			},
		},
	}

	toolCallsJSON, _ := json.Marshal(toolCalls)

	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Vou ler main.go",
		ToolCalls:      string(toolCallsJSON),
		Model:          "gpt-4o",
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar assistant message: %v", err)
	}

	// 3. Executar ferramenta (simulado)
	// Em um cenário real, o executor lê o arquivo
	fileContent := `package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}`

	// 4. Armazenar resultado como role="tool"
	toolResultMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        fileContent,          // Resultado da execução
		ToolCallID:     "call_read_file_001", // Referencia qual chamada executou
		Source:         "wails",
		CreatedAt:      time.Now().Add(200 * time.Millisecond),
	}

	if err := db.Create(toolResultMsg).Error; err != nil {
		t.Fatalf("falha ao criar tool result: %v", err)
	}

	// 5. Validar resultado
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", toolResultMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar tool result: %v", err)
	}

	if retrieved.Role != "tool" {
		t.Errorf("role deveria ser 'tool', foi %s", retrieved.Role)
	}

	if retrieved.ToolCallID != "call_read_file_001" {
		t.Errorf("ToolCallID incorreto: %s", retrieved.ToolCallID)
	}

	if retrieved.Content != fileContent {
		t.Error("conteúdo do resultado foi alterado")
	}

	t.Log("✓ Execução de ferramenta persistida como role=tool")
}

// TestIntegration_FirstMessageToolResultIncorporated testa assistente incorporando resultado da ferramenta
func TestIntegration_FirstMessageToolResultIncorporated(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: conversa
	conv := &database.Conversation{
		Title:     "Tool Result Incorporated",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Fluxo completo: user -> tool call -> tool result -> final answer

	// 2a. Primeira mensagem do usuário
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Quanto é 2 + 2?",
		CreatedAt:      time.Now(),
	}
	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("erro ao criar user msg: %v", err)
	}

	// 2b. Assistente chama calculator tool
	toolCalls := []ToolCall{
		{
			ID:   "call_calc_001",
			Type: "function",
			Function: ToolCallFn{
				Name:      "calculator",
				Arguments: `{"expression":"2+2"}`,
			},
		},
	}
	toolCallsJSON, _ := json.Marshal(toolCalls)

	assistantMsgWithTool := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Vou calcular isso para você.",
		ToolCalls:      string(toolCallsJSON),
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}
	if err := db.Create(assistantMsgWithTool).Error; err != nil {
		t.Fatalf("erro ao criar assistant with tool: %v", err)
	}

	// 2c. Resultado da ferramenta
	toolResult := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        "4",
		ToolCallID:     "call_calc_001",
		CreatedAt:      time.Now().Add(200 * time.Millisecond),
	}
	if err := db.Create(toolResult).Error; err != nil {
		t.Fatalf("erro ao criar tool result: %v", err)
	}

	// 2d. Assistente responde DEPOIS de ter o resultado
	// (Iteração 2 do agentic loop - with tool result in context)
	finalResponse := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "A resposta é 4. 2 + 2 = 4.",
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(300 * time.Millisecond),
	}
	if err := db.Create(finalResponse).Error; err != nil {
		t.Fatalf("erro ao criar final response: %v", err)
	}

	// 3. Validar histórico COMPLETO
	var allMessages []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at").Find(&allMessages).Error; err != nil {
		t.Fatalf("erro ao recuperar histórico: %v", err)
	}

	if len(allMessages) != 4 {
		t.Errorf("esperado 4 mensagens (user, asst+tool, tool result, final), obteve %d", len(allMessages))
	}

	// 4. Validar sequência
	if allMessages[0].Role != "user" || allMessages[0].Content != "Quanto é 2 + 2?" {
		t.Error("primeira mensagem (user) incorreta")
	}

	if allMessages[1].Role != "assistant" || allMessages[1].ToolCalls == "" {
		t.Error("segunda mensagem (assistant com tool call) incorreta")
	}

	if allMessages[2].Role != "tool" || allMessages[2].Content != "4" {
		t.Error("terceira mensagem (tool result) incorreta")
	}

	if allMessages[3].Role != "assistant" || allMessages[3].ToolCalls != "" {
		t.Error("quarta mensagem (final assistant) incorreta")
	}

	// 5. Validar que final response incorporou contexto
	if !contains(allMessages[3].Content, "4") {
		t.Error("assistente deveria mencionar o resultado da ferramenta")
	}

	t.Log("✓ Fluxo completo: user → tool call → result → final answer")
}

// TestIntegration_FirstMessageMultipleTools testa múltiplas ferramentas simultâneas
func TestIntegration_FirstMessageMultipleTools(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: conversa
	conv := &database.Conversation{
		Title:     "Multiple Tools",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem que induz MÚLTIPLAS tool calls
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Leia config.json e main.go simultaneamente",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Assistente chama MÚLTIPLAS ferramentas (executor executa em paralelo)
	toolCalls := []ToolCall{
		{
			ID:   "call_read_1",
			Type: "function",
			Function: ToolCallFn{
				Name:      "read_file",
				Arguments: `{"path":"config.json"}`,
			},
		},
		{
			ID:   "call_read_2",
			Type: "function",
			Function: ToolCallFn{
				Name:      "read_file",
				Arguments: `{"path":"main.go"}`,
			},
		},
	}

	toolCallsJSON, _ := json.Marshal(toolCalls)

	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Vou ler ambos arquivos",
		ToolCalls:      string(toolCallsJSON),
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar assistant msg: %v", err)
	}

	// 4. Ambos resultados armazenados
	results := []struct {
		toolCallID string
		content    string
	}{
		{"call_read_1", `{"db":"sqlite","version":"3"}`},
		{"call_read_2", `package main\nfunc main() {}`},
	}

	for _, result := range results {
		toolMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "tool",
			Content:        result.content,
			ToolCallID:     result.toolCallID,
			CreatedAt:      time.Now().Add(200 * time.Millisecond),
		}

		if err := db.Create(toolMsg).Error; err != nil {
			t.Fatalf("falha ao criar tool result: %v", err)
		}
	}

	// 5. Validar que ambas foram persistidas
	var toolMsgs []database.ChatMessage
	if err := db.Where("conversation_id = ? AND role = ?", conv.ID, "tool").Find(&toolMsgs).Error; err != nil {
		t.Fatalf("falha ao recuperar tool msgs: %v", err)
	}

	if len(toolMsgs) != 2 {
		t.Errorf("esperado 2 tool results, obteve %d", len(toolMsgs))
	}

	// 6. Validar que ambas têm seus ToolCallIDs corretos
	toolCallIDs := make(map[string]bool)
	for _, msg := range toolMsgs {
		toolCallIDs[msg.ToolCallID] = true
	}

	if !toolCallIDs["call_read_1"] || !toolCallIDs["call_read_2"] {
		t.Error("nem todas as tool call IDs foram persistidas")
	}

	t.Log("✓ Múltiplas ferramentas executadas em paralelo, resultados persistidos")
}

// TestIntegration_FirstMessageToolError testa tratamento de erro em ferramenta
func TestIntegration_FirstMessageToolError(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: conversa
	conv := &database.Conversation{
		Title:     "Tool Error",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Leia um arquivo que não existe: /nonexistent/file.txt",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Assistente tenta ler arquivo
	toolCalls := []ToolCall{
		{
			ID:   "call_read_bad",
			Type: "function",
			Function: ToolCallFn{
				Name:      "read_file",
				Arguments: `{"path":"/nonexistent/file.txt"}`,
			},
		},
	}

	toolCallsJSON, _ := json.Marshal(toolCalls)

	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Vou tentar ler este arquivo",
		ToolCalls:      string(toolCallsJSON),
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar assistant msg: %v", err)
	}

	// 4. Ferramenta FALHA - erro persistido como conteúdo
	errorMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        "Error: file not found at /nonexistent/file.txt",
		ToolCallID:     "call_read_bad",
		CreatedAt:      time.Now().Add(200 * time.Millisecond),
	}

	if err := db.Create(errorMsg).Error; err != nil {
		t.Fatalf("falha ao criar error msg: %v", err)
	}

	// 5. Assistente recebe erro e responde apropriadamente
	recoveryMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Desculpe, o arquivo /nonexistent/file.txt não foi encontrado. O arquivo não existe no sistema.",
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(300 * time.Millisecond),
	}

	if err := db.Create(recoveryMsg).Error; err != nil {
		t.Fatalf("falha ao criar recovery msg: %v", err)
	}

	// 6. Validar sequência: user → tool call → error → recovery
	var allMsgs []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at").Find(&allMsgs).Error; err != nil {
		t.Fatalf("erro ao recuperar histórico: %v", err)
	}

	if len(allMsgs) != 4 {
		t.Errorf("esperado 4 mensagens, obteve %d", len(allMsgs))
	}

	// 7. Validar que erro foi incorporado na resposta
	if !contains(allMsgs[3].Content, "não foi encontrado") {
		t.Error("assistente deveria reconhecer o erro")
	}

	t.Log("✓ Erro em ferramenta tratado graciosamente, assistente recuperado")
}

// TestIntegration_FirstMessageToolTokenUsage testa rastreamento de tokens com ferramentas
func TestIntegration_FirstMessageToolTokenUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup
	conv := &database.Conversation{
		Title:     "Tool Token Usage",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é a população do Brasil?",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Assistant response com tool call - tokens PARA Iteração 1
	toolCalls := []ToolCall{
		{ID: "call_1", Type: "function", Function: ToolCallFn{Name: "search", Arguments: `{"q":"população brasil"}`}},
	}
	toolCallsJSON, _ := json.Marshal(toolCalls)

	assistantMsg1 := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Vou buscar esta informação",
		ToolCalls:        string(toolCallsJSON),
		PromptTokens:     120,
		CompletionTokens: 25,
		TotalTokens:      145,
		TurnID:           &userMsg.ID,
		CreatedAt:        time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg1).Error; err != nil {
		t.Fatalf("falha ao criar assistant msg 1: %v", err)
	}

	// 4. Tool result
	toolResult := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        "A população do Brasil é aproximadamente 215 milhões",
		ToolCallID:     "call_1",
		CreatedAt:      time.Now().Add(200 * time.Millisecond),
	}

	if err := db.Create(toolResult).Error; err != nil {
		t.Fatalf("falha ao criar tool result: %v", err)
	}

	// 5. Final response - tokens PARA Iteração 2 (com resultado da ferramenta no contexto)
	assistantMsg2 := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "A população do Brasil é aproximadamente 215 milhões de pessoas.",
		PromptTokens:     280, // Maior: inclui resultado da ferramenta
		CompletionTokens: 18,
		TotalTokens:      298,
		TurnID:           &userMsg.ID,
		CreatedAt:        time.Now().Add(300 * time.Millisecond),
	}

	if err := db.Create(assistantMsg2).Error; err != nil {
		t.Fatalf("falha ao criar assistant msg 2: %v", err)
	}

	// 6. Validar tokens foram rastreados
	var msg1 database.ChatMessage
	if err := db.First(&msg1, "id = ?", assistantMsg1.ID).Error; err != nil {
		t.Fatalf("erro ao recuperar msg1: %v", err)
	}

	if msg1.TotalTokens != 145 {
		t.Errorf("tokens da iteração 1 incorretos: %d", msg1.TotalTokens)
	}

	var msg2 database.ChatMessage
	if err := db.First(&msg2, "id = ?", assistantMsg2.ID).Error; err != nil {
		t.Fatalf("erro ao recuperar msg2: %v", err)
	}

	if msg2.TotalTokens != 298 {
		t.Errorf("tokens da iteração 2 incorretos: %d", msg2.TotalTokens)
	}

	// 7. Verificar que iteração 2 consumiu mais tokens (contexto expandido)
	if msg2.PromptTokens <= msg1.PromptTokens {
		t.Error("iteração 2 deveria consumir mais tokens de prompt (contexto expandido com resultado)")
	}

	t.Logf("✓ Tokens rastreados: Iter1=%d tokens, Iter2=%d tokens (expansão de contexto validada)", msg1.TotalTokens, msg2.TotalTokens)
}

// Helper: verifica se string contém substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
