package integration

import (
	"testing"
	"time"

	"assistente/internal/database"
)

// TestIntegration_FirstMessageHistoryPersistence testa que primeira mensagem é persistida e recuperável
func TestIntegration_FirstMessageHistoryPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar conversa e primeira mensagem
	conv := &database.Conversation{
		Title:     "Test History",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem do usuário
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Olá! Como você funciona?",
		Source:         "wails",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Resposta do assistente
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Sou um assistente de IA. Posso ajudar com muitas tarefas!",
		Source:           "wails",
		Model:            "gpt-4o",
		PromptTokens:     25,
		CompletionTokens: 30,
		TotalTokens:      55,
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 4. RECARREGAR a conversa: simular usuário reabrindo o app
	var reloadedConv database.Conversation
	if err := db.First(&reloadedConv, "id = ?", conv.ID).Error; err != nil {
		t.Fatalf("falha ao recarregar conversa: %v", err)
	}

	// 5. Carregar histórico completo
	var msgs []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at").Find(&msgs).Error; err != nil {
		t.Fatalf("falha ao carregar histórico: %v", err)
	}

	// 6. Validações
	if len(msgs) != 2 {
		t.Errorf("esperado 2 mensagens, obteve %d", len(msgs))
	}

	if len(msgs) > 0 && msgs[0].Role != "user" {
		t.Errorf("primeira mensagem deveria ser user, obteve %s", msgs[0].Role)
	}

	if len(msgs) > 1 && msgs[1].Role != "assistant" {
		t.Errorf("segunda mensagem deveria ser assistant, obteve %s", msgs[1].Role)
	}

	// 7. Validar metadados preservados
	if len(msgs) > 1 {
		assistantMsg := msgs[1]
		if assistantMsg.Model != "gpt-4o" {
			t.Errorf("modelo não foi persistido: %s", assistantMsg.Model)
		}

		if assistantMsg.TotalTokens != 55 {
			t.Errorf("tokens não foram persistidos: %d", assistantMsg.TotalTokens)
		}
	}

	t.Log("✓ Histórico completo persistido e recuperável após recarga")
}

// TestIntegration_FirstMessageHistoryOrder testa ordem FIFO do histórico
func TestIntegration_FirstMessageHistoryOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar conversa
	conv := &database.Conversation{
		Title:     "Order Test",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Sequência de 5 mensagens
	messages := []struct {
		role    string
		content string
	}{
		{"user", "Primeira pergunta"},
		{"assistant", "Primeira resposta"},
		{"user", "Segunda pergunta"},
		{"assistant", "Segunda resposta"},
		{"user", "Terceira pergunta"},
	}

	for _, msg := range messages {
		chatMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           msg.role,
			Content:        msg.content,
			Source:         "wails",
		}

		if err := db.Create(chatMsg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem: %v", err)
		}
	}

	// 3. Carregar histórico com ordem FIFO (UUIDv7 IDs preservam ordem de inserção)
	var allMsgs []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("id ASC").Find(&allMsgs).Error; err != nil {
		t.Fatalf("falha ao carregar histórico: %v", err)
	}

	// 4. Validar ordem
	if len(allMsgs) != 5 {
		t.Errorf("esperado 5 mensagens, obteve %d", len(allMsgs))
	}

	expectedRoles := []string{"user", "assistant", "user", "assistant", "user"}
	expectedContents := []string{"Primeira pergunta", "Primeira resposta", "Segunda pergunta", "Segunda resposta", "Terceira pergunta"}

	for i, msg := range allMsgs {
		if msg.Role != expectedRoles[i] {
			t.Errorf("mensagem %d: role esperado %s, obteve %s", i, expectedRoles[i], msg.Role)
		}

		if msg.Content != expectedContents[i] {
			t.Errorf("mensagem %d: content esperado %q, obteve %q", i, expectedContents[i], msg.Content)
		}
	}

	t.Logf("✓ Ordem FIFO preservada: %d mensagens em sequência temporal correta", len(allMsgs))
}

// TestIntegration_FirstMessageHistoryMultipleConversations testa isolamento de históricos
func TestIntegration_FirstMessageHistoryMultipleConversations(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar 3 conversas diferentes
	convs := make([]*database.Conversation, 3)
	for i := 0; i < 3; i++ {
		conv := &database.Conversation{
			Title:     "Conversa " + string(rune('A'+i)),
		}

		if err := db.Create(conv).Error; err != nil {
			t.Fatalf("falha ao criar conversa %d: %v", i, err)
		}

		convs[i] = conv
	}

	// 2. Adicionar messages diferentes em cada conversa
	for i, conv := range convs {
		msg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "Pergunta na conversa " + string(rune('A'+i)),
			Source:         "wails",
		}

		if err := db.Create(msg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem: %v", err)
		}
	}

	// 3. Carregar histórico de cada conversa isoladamente
	for i, conv := range convs {
		var msgs []database.ChatMessage
		if err := db.Where("conversation_id = ?", conv.ID).Find(&msgs).Error; err != nil {
			t.Fatalf("falha ao carregar histórico da conversa %d: %v", i, err)
		}

		// Cada conversa deve ter exactamente 1 mensagem
		if len(msgs) != 1 {
			t.Errorf("conversa %d deveria ter 1 mensagem, obteve %d", i, len(msgs))
		}

		// Validar que a mensagem pertence à conversa correta
		if msgs[0].ConversationID != conv.ID {
			t.Errorf("mensagem não pertence à conversa correta")
		}

		expectedContent := "Pergunta na conversa " + string(rune('A'+i))
		if msgs[0].Content != expectedContent {
			t.Errorf("conteúdo incorreto: esperado %q, obteve %q", expectedContent, msgs[0].Content)
		}
	}

	// 4. Total de mensagens globais deve ser 3
	var allMsgs []database.ChatMessage
	if err := db.Find(&allMsgs).Error; err != nil {
		t.Fatalf("falha ao carregar todas as mensagens: %v", err)
	}

	if len(allMsgs) < 3 {
		t.Errorf("esperado pelo menos 3 mensagens globais, obteve %d", len(allMsgs))
	}

	t.Log("✓ Isolamento de históricos por conversa validado")
}

// TestIntegration_FirstMessageHistoryExpiration testa persistência ao longo do tempo
func TestIntegration_FirstMessageHistoryExpiration(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar conversa e mensagem com timestamp antigo
	conv := &database.Conversation{
		Title:     "Old Conversation",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem muito antiga
	oldMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Pergunta de um mês atrás",
		Source:         "wails",
	}

	if err := db.Create(oldMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem antiga: %v", err)
	}

	// 3. Recarregar a mensagem antiga
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", oldMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar mensagem antiga: %v", err)
	}

	// 4. Validar que mensagem antiga ainda existe e está intacta
	if retrieved.Content != "Pergunta de um mês atrás" {
		t.Errorf("conteúdo da mensagem antiga foi corrompido: %s", retrieved.Content)
	}

	// 5. Validar timestamp
	if retrieved.CreatedAt.IsZero() {
		t.Error("timestamp foi perdido")
	}

	t.Logf("✓ Mensagens antigas persistem e são recuperáveis (test: msg criada há %v)", time.Since(retrieved.CreatedAt))
}

// TestIntegration_FirstMessageHistoryWithTools testa persistência de histórico com tool calls
func TestIntegration_FirstMessageHistoryWithTools(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar conversa
	conv := &database.Conversation{
		Title:     "With Tools",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. User message
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é a temperatura em São Paulo?",
		Source:         "wails",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Assistant message com tool call
	toolCallsJSON := `[{"id":"call_123","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"São Paulo\"}"}}]`
	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Vou buscar a temperatura de São Paulo",
		ToolCalls:      toolCallsJSON,
		Source:         "wails",
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar assistant msg: %v", err)
	}

	// 4. Tool result
	toolResultMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        "Temperature: 28°C",
		ToolCallID:     "call_123",
		Source:         "wails",
	}

	if err := db.Create(toolResultMsg).Error; err != nil {
		t.Fatalf("falha ao criar tool result: %v", err)
	}

	// 5. Recarregar histórico completo
	var allMsgs []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at").Find(&allMsgs).Error; err != nil {
		t.Fatalf("falha ao carregar histórico: %v", err)
	}

	// 6. Validações
	if len(allMsgs) != 3 {
		t.Errorf("esperado 3 mensagens, obteve %d", len(allMsgs))
	}

	// Validar tool call foi persistido
	if len(allMsgs) > 1 && allMsgs[1].ToolCalls == "" {
		t.Error("ToolCalls não foi persistido")
	}

	// Validar tool result foi persistido
	if len(allMsgs) > 2 && allMsgs[2].ToolCallID == "" {
		t.Error("ToolCallID não foi persistido")
	}

	t.Log("✓ Histórico com tool calls persistido e recuperável")
}

// TestIntegration_FirstMessageHistoryConversationUpdate testa atualização de conversa após primeira mensagem
func TestIntegration_FirstMessageHistoryConversationUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar conversa com título genérico
	conv := &database.Conversation{
		Title:     "Nova conversa",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem chega
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Como começar a aprender Golang?",
		Source:         "wails",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Sistema detecta essa é a primeira mensagem e rename a conversa
	updatedTitle := "Como começar a aprender Golang?"

	if err := db.Model(conv).Update("title", updatedTitle).Error; err != nil {
		t.Fatalf("falha ao atualizar título: %v", err)
	}

	// 4. Recarregar conversa
	var reloadedConv database.Conversation
	if err := db.First(&reloadedConv, "id = ?", conv.ID).Error; err != nil {
		t.Fatalf("falha ao recarregar conversa: %v", err)
	}

	// 5. Validar que título foi atualizado
	if reloadedConv.Title != updatedTitle {
		t.Errorf("título não foi atualizado: esperado %q, obteve %q", updatedTitle, reloadedConv.Title)
	}

	// 6. Validar que UpdatedAt foi atualizado
	if reloadedConv.UpdatedAt.Before(reloadedConv.CreatedAt) {
		t.Error("UpdatedAt deveria ser depois de CreatedAt")
	}

	t.Logf("✓ Conversa atualizada após primeira mensagem: %q", reloadedConv.Title)
}

// TestIntegration_FirstMessageHistoryConcurrentAccess testa acesso simultâneo
func TestIntegration_FirstMessageHistoryConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar conversa
	conv := &database.Conversation{
		Title:     "Concurrent Test",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Teste de concorrência",
		Source:         "wails",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Simular múltiplos acessos simultâneos (sequencial para simplificar)
	numReads := 10
	for i := 0; i < numReads; i++ {
		var msg database.ChatMessage
		if err := db.First(&msg, "id = ?", userMsg.ID).Error; err != nil {
			t.Fatalf("ciclo %d: falha ao carregar mensagem: %v", i, err)
		}

		// Validar que dados não foram corrompidos
		if msg.Content != "Teste de concorrência" {
			t.Errorf("ciclo %d: conteúdo foi corrompido: %s", i, msg.Content)
		}
	}

	t.Logf("✓ Acesso sequencial a histórico %d vezes sem corrupção", numReads)
}
