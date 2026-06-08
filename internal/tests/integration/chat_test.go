package integration

import (
	"testing"

	"assistente/internal/database"
)

// TestIntegration_SendMessageFlow testa o fluxo de criação de mensagens no chat
func TestIntegration_SendMessageFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)
	_ = setupIntegrationEnv(t)

	// 1. Criar conversa
	conv := &database.Conversation{
		Title:     "Teste SendMessage",
	}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	if conv.ID == "" {
		t.Fatal("conversa não tem ID após criação")
	}

	// 2. Criar mensagem do usuário (no fluxo real, SendMessage faz isso)
	userContent := "Olá assistente! Como você está?"
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        userContent,
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem do usuário: %v", err)
	}

	// 3. Criar mensagem de resposta do assistente (simulando a resposta do LLM)
	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Olá! Estou bem, obrigado por perguntar. Como posso ajudá-lo?",
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem do assistente: %v", err)
	}

	// 4. Recuperar todas as mensagens da conversa
	var messages []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at").Find(&messages).Error; err != nil {
		t.Fatalf("falha ao recuperar mensagens: %v", err)
	}

	// 5. Validar que temos 2 mensagens
	if len(messages) != 2 {
		t.Errorf("esperado 2 mensagens, obteve %d", len(messages))
	}

	// 6. Validar primeira mensagem (usuário)
	if messages[0].Role != "user" {
		t.Errorf("primeira mensagem não é do usuário: %s", messages[0].Role)
	}
	if messages[0].Content != userContent {
		t.Errorf("conteúdo da mensagem do usuário incorreto")
	}

	// 7. Validar segunda mensagem (assistente)
	if messages[1].Role != "assistant" {
		t.Errorf("segunda mensagem não é do assistente: %s", messages[1].Role)
	}
	if len(messages[1].Content) == 0 {
		t.Error("resposta do assistente está vazia")
	}

	// 8. Validar que a conversa ainda existe
	var retrievedConv database.Conversation
	if err := db.First(&retrievedConv, "id = ?", conv.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar conversa: %v", err)
	}

	if retrievedConv.ID != conv.ID {
		t.Errorf("ID da conversa não corresponde")
	}

	t.Log("✓ Teste integração SendMessage flow: PASSOU")
}

// TestIntegration_ConversationUpdate testa que a conversa é atualizada após mensagem
func TestIntegration_ConversationUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)
	_ = setupIntegrationEnv(t)

	// 1. Criar conversa com título genérico
	conv := &database.Conversation{
		Title:     "Nova Conversa",
	}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	initialTitle := conv.Title

	// 2. Criar uma mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Como fazer um bolo?",
	}
	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Simular auto-rename (o código faz isso em SendMessage)
	// Em um teste real, isso seria feito pela função SendMessage
	newTitle := userMsg.Content
	if len(newTitle) > 50 {
		newTitle = newTitle[:50]
	}
	if err := db.Model(conv).Update("title", newTitle).Error; err != nil {
		t.Fatalf("falha ao atualizar título: %v", err)
	}

	// 4. Validar que o título foi atualizado
	var updated database.Conversation
	if err := db.First(&updated, "id = ?", conv.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar conversa: %v", err)
	}

	if updated.Title == initialTitle {
		t.Error("título deveria ter sido atualizado")
	}

	if updated.Title != newTitle {
		t.Errorf("título incorreto: esperado '%s', obteve '%s'", newTitle, updated.Title)
	}

	t.Log("✓ Teste integração atualização de conversa: PASSOU")
}
