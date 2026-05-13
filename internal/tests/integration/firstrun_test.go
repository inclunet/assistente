package integration

import (
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
)

// TestIntegration_FirstUserMessage testa o fluxo da primeira mensagem do usuário
// Cobre: criação de conversa, primeira mensagem armazenada corretamente, metadados
func TestIntegration_FirstUserMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Criar primeira conversa
	conv := &database.Conversation{
		Title:     "Nova Conversa",
		Channel:   "", // Local (Wails)
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	if conv.ID == "" {
		t.Fatal("conversa não recebeu ID")
	}

	// 2. Usuário envia primeira mensagem
	firstMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é a capital da França?",
		Source:         "wails", // Origem: aplicação local
	}

	if err := db.Create(firstMsg).Error; err != nil {
		t.Fatalf("falha ao criar primeira mensagem: %v", err)
	}

	if firstMsg.ID == "" {
		t.Fatal("mensagem não recebeu ID")
	}

	// 3. Recuperar mensagem e validar
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", firstMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar primeira mensagem: %v", err)
	}

	if retrieved.ConversationID != conv.ID {
		t.Errorf("conversationID incorreto: esperado %s, obteve %s", conv.ID, retrieved.ConversationID)
	}

	if retrieved.Role != "user" {
		t.Errorf("role incorreto: esperado 'user', obteve '%s'", retrieved.Role)
	}

	if retrieved.Content != "Qual é a capital da França?" {
		t.Errorf("conteúdo incorreto: %s", retrieved.Content)
	}

	if retrieved.Source != "wails" {
		t.Errorf("source incorreto: esperado 'wails', obteve '%s'", retrieved.Source)
	}

	t.Log("✓ Primeira mensagem do usuário criada e persistida corretamente")
}

// TestIntegration_FirstAssistantResponse testa a resposta do assistente à primeira mensagem
func TestIntegration_FirstAssistantResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar conversa e primeira mensagem
	conv := &database.Conversation{
		Title:     "Teste Resposta",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é a capital da França?",
		TurnID:         nil, // Será preenchido quando criado
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem do usuário: %v", err)
	}

	// 2. Assistente responde
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "A capital da França é Paris.",
		Model:            "gpt-4o",    // Modelo que foi usado
		PromptTokens:     45,          // Tokens de entrada
		CompletionTokens: 12,          // Tokens de saída
		TotalTokens:      57,          // Total
		TurnID:           &userMsg.ID, // Agrupa com a mensagem do usuário
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta do assistente: %v", err)
	}

	// 3. Validar resposta
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.Role != "assistant" {
		t.Errorf("role incorreto: esperado 'assistant', obteve '%s'", retrieved.Role)
	}

	if retrieved.Model != "gpt-4o" {
		t.Errorf("modelo incorreto: esperado 'gpt-4o', obteve '%s'", retrieved.Model)
	}

	if retrieved.TotalTokens != 57 {
		t.Errorf("tokens incorretos: esperado 57, obteve %d", retrieved.TotalTokens)
	}

	if retrieved.TurnID == nil || *retrieved.TurnID != userMsg.ID {
		t.Error("TurnID não foi preenchido corretamente")
	}

	t.Log("✓ Resposta do assistente criada com metadados corretos")
}

// TestIntegration_FirstMessageAutoRenameConversation testa renameamento automático da conversa
func TestIntegration_FirstMessageAutoRenameConversation(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Criar conversa com título genérico
	conv := &database.Conversation{
		Title:     "Nova Conversa",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	originalTitle := conv.Title

	// 2. Primeira mensagem com conteúdo descritivo
	msgContent := "Como faço para aprender programação em Go de forma eficaz?"
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        msgContent,
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Simular auto-rename (lógica que acontece em SendMessage)
	// Se título é genérico e primeira mensagem tem conteúdo, renomear
	if conv.Title == "Nova Conversa" && msgContent != "" {
		newTitle := msgContent
		if len(newTitle) > 50 {
			newTitle = newTitle[:50]
		}

		if err := db.Model(conv).Update("title", newTitle).Error; err != nil {
			t.Fatalf("falha ao renomear conversa: %v", err)
		}

		// 4. Validar que o título foi atualizado
		var updated database.Conversation
		if err := db.First(&updated, "id = ?", conv.ID).Error; err != nil {
			t.Fatalf("falha ao recuperar conversa atualizada: %v", err)
		}

		if updated.Title == originalTitle {
			t.Error("título deveria ter sido alterado")
		}

		if updated.Title != newTitle {
			t.Errorf("título incorreto: esperado '%s', obteve '%s'", newTitle, updated.Title)
		}

		t.Logf("✓ Auto-rename realizado: '%s' → '%s'", originalTitle, updated.Title)
	}

	t.Log("✓ Teste auto-rename da conversa: PASSOU")
}

// TestIntegration_FirstMessageConversationHistory testa carregamento do histórico
func TestIntegration_FirstMessageConversationHistory(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Criar conversa
	conv := &database.Conversation{
		Title:     "Histórico",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira ocorrência: histórico vazio
	var messages []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at ASC").Find(&messages).Error; err != nil {
		t.Fatalf("falha ao carregar histórico: %v", err)
	}

	if len(messages) != 0 {
		t.Error("histórico deveria estar vazio para conversa nova")
	}

	// 3. Adicionar primeira mensagem
	msg1 := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é a cor do meu chapéu?",
	}

	if err := db.Create(msg1).Error; err != nil {
		t.Fatalf("falha ao criar primeira mensagem: %v", err)
	}

	// 4. Adicionar resposta
	msg2 := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Não tenho informação sobre a cor do seu chapéu.",
		TurnID:         &msg1.ID,
	}

	if err := db.Create(msg2).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 5. Recarregar histórico
	messages = []database.ChatMessage{}
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at ASC").Find(&messages).Error; err != nil {
		t.Fatalf("falha ao recarregar histórico: %v", err)
	}

	// 6. Validar histórico
	if len(messages) != 2 {
		t.Errorf("esperado 2 mensagens, obteve %d", len(messages))
	}

	// 7. Validar ordem (FIFO)
	if messages[0].Role != "user" {
		t.Error("primeira mensagem deveria ser do usuário")
	}

	if messages[1].Role != "assistant" {
		t.Error("segunda mensagem deveria ser do assistente")
	}

	// 8. Validar conteúdo
	if messages[0].Content != "Qual é a cor do meu chapéu?" {
		t.Errorf("conteúdo da primeira mensagem incorreto: %s", messages[0].Content)
	}

	if messages[1].Content != "Não tenho informação sobre a cor do seu chapéu." {
		t.Errorf("conteúdo da segunda mensagem incorreto: %s", messages[1].Content)
	}

	t.Log("✓ Histórico carregado e mantém ordem FIFO")
}

// TestIntegration_FirstMessageWithAudio testa primeira mensagem contendo áudio
func TestIntegration_FirstMessageWithAudio(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Criar conversa
	conv := &database.Conversation{
		Title:     "Mensagem com Áudio",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem com áudio (usuário gravou um áudio)
	audioBase64 := "//NExAAAAAANIAAAAAExBTUUzLjk4LjJVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVU="

	msg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Olá, aqui está minha pergunta em áudio",
		Audio:          audioBase64,
		AudioMimeType:  "audio/mpeg",
		Source:         "wails",
	}

	if err := db.Create(msg).Error; err != nil {
		t.Fatalf("falha ao armazenar mensagem com áudio: %v", err)
	}

	// 3. Recuperar e validar
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", msg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar mensagem: %v", err)
	}

	if retrieved.Audio != audioBase64 {
		t.Error("áudio não foi persistido corretamente")
	}

	if retrieved.AudioMimeType != "audio/mpeg" {
		t.Errorf("MIME type incorreto: esperado 'audio/mpeg', obteve '%s'", retrieved.AudioMimeType)
	}

	if retrieved.Content != "Olá, aqui está minha pergunta em áudio" {
		t.Errorf("conteúdo incorreto: %s", retrieved.Content)
	}

	t.Log("✓ Primeira mensagem com áudio persistida corretamente")
}

// TestIntegration_FirstMessageProviderMetadata testa rastreamento de modelo e tokens
func TestIntegration_FirstMessageProviderMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Criar conversa
	conv := &database.Conversation{
		Title:     "Provider Metadata",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Setup de providers
	registry := llm.NewProviderRegistry()

	provider1 := &llm.ProviderConfig{
		ID:      "openai-gpt4",
		Name:    "OpenAI GPT-4",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4o",
	}

	if err := registry.Register(provider1); err != nil {
		t.Fatalf("falha ao registrar provider: %v", err)
	}

	// 3. Primeira mensagem do usuário
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é a população do Brasil?",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem do usuário: %v", err)
	}

	// 4. Resposta com rastreamento de modelo e tokens
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "A população do Brasil é aproximadamente 215 milhões de pessoas.",
		Model:            "gpt-4o", // Qual modelo foi usado
		PromptTokens:     35,       // Tokens consumidos (contexto + prompt)
		CompletionTokens: 18,       // Tokens gerados (resposta)
		TotalTokens:      53,       // Total para billing/tracking
		TurnID:           &userMsg.ID,
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 5. Validar que metadados foram persistidos
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.Model != "gpt-4o" {
		t.Errorf("modelo incorreto: esperado 'gpt-4o', obteve '%s'", retrieved.Model)
	}

	if retrieved.PromptTokens != 35 {
		t.Errorf("prompt tokens incorreto: esperado 35, obteve %d", retrieved.PromptTokens)
	}

	if retrieved.CompletionTokens != 18 {
		t.Errorf("completion tokens incorreto: esperado 18, obteve %d", retrieved.CompletionTokens)
	}

	if retrieved.TotalTokens != 53 {
		t.Errorf("total tokens incorreto: esperado 53, obteve %d", retrieved.TotalTokens)
	}

	t.Logf("✓ Metadados rastreados: modelo=%s, tokens=%d+%d=%d",
		retrieved.Model, retrieved.PromptTokens, retrieved.CompletionTokens, retrieved.TotalTokens)
}

// TestIntegration_FirstMessageWithTurnTracking testa rastreamento de turno (TurnID)
func TestIntegration_FirstMessageWithTurnTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Criar conversa
	conv := &database.Conversation{
		Title:     "Turn Tracking",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem do usuário (inicia um turno)
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é a capital de Itália?",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Resposta do assistente (mesmo turno)
	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "A capital da Itália é Roma.",
		TurnID:         &userMsg.ID, // Agrupa com a mensagem inicadora
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 4. Buscar todas as mensagens do turno (via TurnID)
	var turnMessages []database.ChatMessage
	if err := db.Where("conversation_id = ? AND turn_id = ?", conv.ID, userMsg.ID).Find(&turnMessages).Error; err != nil {
		t.Fatalf("falha ao buscar mensagens do turno: %v", err)
	}

	if len(turnMessages) != 1 {
		t.Errorf("esperado 1 mensagem com TurnID=%s, obteve %d", userMsg.ID, len(turnMessages))
	}

	if turnMessages[0].ID != assistantMsg.ID {
		t.Errorf("mensagem do turno incorreta: esperado %s, obteve %s", assistantMsg.ID, turnMessages[0].ID)
	}

	// 5. Validar estrutura de turno
	if assistantMsg.TurnID == nil || *assistantMsg.TurnID != userMsg.ID {
		t.Error("TurnID deveria apontar para a mensagem do usuário que iniciou o turno")
	}

	t.Log("✓ Rastreamento de turno (TurnID) funcionando corretamente")
}
