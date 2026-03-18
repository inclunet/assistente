package integration

import (
	"testing"
	"time"

	"assistente/internal/database"
)

// TestIntegration_ConversationStorage testa armazenamento de conversas
func TestIntegration_ConversationStorage(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)
	_ = setupIntegrationEnv(t)

	// 1. Criar conversa
	conv := &database.Conversation{
		Title:     "Conversa de Teste",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	if conv.ID == 0 {
		t.Error("conversa sem ID após criação")
	}

	// 2. Recuperar conversa
	var retrieved database.Conversation
	if err := db.First(&retrieved, "id = ?", conv.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar conversa: %v", err)
	}

	if retrieved.ID != conv.ID {
		t.Errorf("ID incorreto: esperado %d, obteve %d", conv.ID, retrieved.ID)
	}

	if retrieved.Title != "Conversa de Teste" {
		t.Errorf("título incorreto: %s", retrieved.Title)
	}

	t.Log("✓ Teste integração armazenamento de conversas: PASSOU")
}

// TestIntegration_MessagePersistence testa armazenamento de mensagens
func TestIntegration_MessagePersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)
	_ = setupIntegrationEnv(t)

	// 1. Criar conversa
	conv := &database.Conversation{
		Title:     "Conversa com Mensagens",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Criar mensagens
	messages := []database.ChatMessage{
		{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "Olá!",
			CreatedAt:      time.Now(),
		},
		{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        "Oi! Como vai?",
			CreatedAt:      time.Now().Add(100 * time.Millisecond),
		},
	}

	for _, msg := range messages {
		if err := db.Create(&msg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem: %v", err)
		}
	}

	// 3. Recuperar mensagens
	var retrieved []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at").Find(&retrieved).Error; err != nil {
		t.Fatalf("falha ao recuperar mensagens: %v", err)
	}

	if len(retrieved) != len(messages) {
		t.Errorf("número de mensagens: esperado %d, obteve %d", len(messages), len(retrieved))
	}

	for i, msg := range retrieved {
		if msg.Role != messages[i].Role {
			t.Errorf("mensagem %d: role incorreto", i)
		}
		if msg.Content != messages[i].Content {
			t.Errorf("mensagem %d: conteúdo incorreto", i)
		}
	}

	t.Log("✓ Teste integração persistência de mensagens: PASSOU")
}

// TestIntegration_MultipleConversations testa múltiplas conversas isoladas
func TestIntegration_MultipleConversations(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)
	_ = setupIntegrationEnv(t)

	// 1. Criar 3 conversas
	conversas := make([]*database.Conversation, 3)
	for i := 0; i < 3; i++ {
		conv := &database.Conversation{
			Title:     "Conversa " + string(rune('1'+i)),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := db.Create(conv).Error; err != nil {
			t.Fatalf("falha ao criar conversa: %v", err)
		}
		conversas[i] = conv
	}

	// 2. Adicionar mensagens diferentes para cada conversa
	for convIdx, conv := range conversas {
		for msgIdx := 0; msgIdx < 2; msgIdx++ {
			msg := &database.ChatMessage{
				ConversationID: conv.ID,
				Role:           "user",
				Content:        "Mensagem " + string(rune('a'+msgIdx)) + " da conversa " + string(rune('1'+convIdx)),
				CreatedAt:      time.Now().Add(time.Duration(msgIdx) * time.Second),
			}
			if err := db.Create(msg).Error; err != nil {
				t.Fatalf("falha ao criar mensagem: %v", err)
			}
		}
	}

	// 3. Validar isolamento e contagem
	var totalConvs int64
	if err := db.Model(&database.Conversation{}).Count(&totalConvs).Error; err != nil {
		t.Fatalf("falha ao contar conversas: %v", err)
	}

	if totalConvs != 3 {
		t.Errorf("total de conversas: esperado 3, obteve %d", totalConvs)
	}

	var totalMsgs int64
	if err := db.Model(&database.ChatMessage{}).Count(&totalMsgs).Error; err != nil {
		t.Fatalf("falha ao contar mensagens: %v", err)
	}

	if totalMsgs != 6 {
		t.Errorf("total de mensagens: esperado 6, obteve %d", totalMsgs)
	}

	// 4. Validar que cada conversa possui suas mensagens
	for _, conv := range conversas {
		var count int64
		if err := db.Model(&database.ChatMessage{}).Where("conversation_id = ?", conv.ID).Count(&count).Error; err != nil {
			t.Fatalf("falha ao contar mensagens da conversa: %v", err)
		}

		if count != 2 {
			t.Errorf("conversa %d: esperado 2 mensagens, obteve %d", conv.ID, count)
		}
	}

	t.Log("✓ Teste integração múltiplas conversas: PASSOU")
}
