package integration

import (
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/database"
)

// TestIntegration_FirstMessageViaTelegram testa primeira mensagem chegando via Telegram
func TestIntegration_FirstMessageViaTelegram(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: mensagem chega via Telegram
	channel := "telegram"

	// 2. Primeira mensagem chega via Telegram
	userMsg := &database.ChatMessage{
		Role:      "user",
		Content:   "Olá! Qual é a capital da França?",
		Source:    channel,
		CreatedAt: time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem Telegram: %v", err)
	}

	// 3. Validar que Source foi preservado
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", userMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar mensagem: %v", err)
	}

	if retrieved.Source != channel {
		t.Errorf("Source não foi preservado: esperado %s, obteve %s", channel, retrieved.Source)
	}

	// 4. Assistente responde
	assistantMsg := &database.ChatMessage{
		Role:      "assistant",
		Content:   "A capital da França é Paris.",
		Source:    channel,
		CreatedAt: time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 5. Validar que resposta volta para o mesmo Source
	var assistantRetrieved database.ChatMessage
	if err := db.First(&assistantRetrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if assistantRetrieved.Source != channel {
		t.Errorf("resposta deveria ter Source=%s, obteve %s", channel, assistantRetrieved.Source)
	}

	t.Logf("✓ Primeira mensagem via Telegram (Source=%s) funcionou corretamente", channel)
}

// TestIntegration_FirstMessageViaSignal testa primeira mensagem chegando via Signal
func TestIntegration_FirstMessageViaSignal(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: mensagem chega via Signal
	channel := "signal"

	// 2. Primeira mensagem chega via Signal
	userMsg := &database.ChatMessage{
		Role:      "user",
		Content:   "Qual é a receita de brigadeiro?",
		Source:    channel,
		CreatedAt: time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem Signal: %v", err)
	}

	// 3. Validar que Source foi preservado
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", userMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar mensagem: %v", err)
	}

	if retrieved.Source != channel {
		t.Errorf("Source não foi preservado: esperado %s, obteve %s", channel, retrieved.Source)
	}

	// 4. Assistente responde com receita
	recipeMD := `# Brigadeiro

## Ingredientes
- 1 lata de leite condensado
- 4 colheres de chocolate em pó
- 1 colher de manteiga

## Modo de Fazer
1. Misture tudo em uma panela
2. Mexa sempre
3. Retire quando não grudar mais
4. Recheie de chocolate granulado`

	assistantMsg := &database.ChatMessage{
		Role:      "assistant",
		Content:   recipeMD,
		Source:    channel,
		CreatedAt: time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 5. Validar que resposta volta para o mesmo Source e contém markdown
	var assistantRetrieved database.ChatMessage
	if err := db.First(&assistantRetrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if assistantRetrieved.Source != channel {
		t.Errorf("resposta deveria ter Source=%s, obteve %s", channel, assistantRetrieved.Source)
	}

	if !contains(assistantRetrieved.Content, "Ingredientes") {
		t.Error("resposta não contém o conteúdo esperado")
	}

	t.Logf("✓ Primeira mensagem via Signal (Source=%s) funcionou corretamente", channel)
}

// TestIntegration_FirstMessageMultipleChannels testa mensagens de múltiplos canais
func TestIntegration_FirstMessageMultipleChannels(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: múltiplos canais
	channels := []struct {
		name    string
		content string
	}{
		{"telegram", "Primeira via Telegram"},
		{"signal", "Primeira via Signal"},
		{"slack", "Primeira via Slack"},
	}

	// 2. Criar mensagem em cada canal
	for _, ch := range channels {
		userMsg := &database.ChatMessage{
			Role:      "user",
			Content:   ch.content,
			Source:    ch.name,
			CreatedAt: time.Now(),
		}

		if err := db.Create(userMsg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem para %s: %v", ch.name, err)
		}
	}

	// 3. Validar que todas as mensagens foram criadas
	var allMsgs []database.ChatMessage
	if err := db.Find(&allMsgs).Error; err != nil {
		t.Fatalf("falha ao recuperar histórico: %v", err)
	}

	if len(allMsgs) < 3 {
		t.Errorf("esperado pelo menos 3 mensagens de canais, obteve %d", len(allMsgs))
	}

	// 4. Validar que cada canal tem seu Source
	channelCount := make(map[string]int)
	for _, msg := range allMsgs {
		if msg.Source != "" {
			channelCount[msg.Source]++
		}
	}

	if len(channelCount) < 3 {
		t.Errorf("esperado 3 canais diferentes, obteve %d: %v", len(channelCount), channelCount)
	}

	t.Logf("✓ Múltiplos canais funcionam isolados: %d mensagens em %d canais", len(allMsgs), len(channelCount))
}

// TestIntegration_FirstMessageChannelInConversation testa isolamento de canais por conversa
func TestIntegration_FirstMessageChannelInConversation(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: duas conversas em canais diferentes
	conv1 := &database.Conversation{
		Title:     "Chat Telegram",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv1).Error; err != nil {
		t.Fatalf("falha ao criar conversa1: %v", err)
	}

	conv2 := &database.Conversation{
		Title:     "Chat Signal",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv2).Error; err != nil {
		t.Fatalf("falha ao criar conversa2: %v", err)
	}

	// 2. Primeira mensagem em Telegram
	msg1 := &database.ChatMessage{
		ConversationID: conv1.ID,
		Role:           "user",
		Content:        "Mensagem do Telegram",
		Source:         "telegram",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(msg1).Error; err != nil {
		t.Fatalf("falha ao criar msg1: %v", err)
	}

	// 3. Primeira mensagem em Signal
	msg2 := &database.ChatMessage{
		ConversationID: conv2.ID,
		Role:           "user",
		Content:        "Mensagem do Signal",
		Source:         "signal",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(msg2).Error; err != nil {
		t.Fatalf("falha ao criar msg2: %v", err)
	}

	// 4. Validar que conversas são isoladas por Source
	var conv1Msgs []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv1.ID).Find(&conv1Msgs).Error; err != nil {
		t.Fatalf("falha ao recuperar msgs de conv1: %v", err)
	}

	var conv2Msgs []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv2.ID).Find(&conv2Msgs).Error; err != nil {
		t.Fatalf("falha ao recuperar msgs de conv2: %v", err)
	}

	// 5. Validar que canais são diferentes
	if len(conv1Msgs) > 0 && len(conv2Msgs) > 0 {
		if conv1Msgs[0].Source == conv2Msgs[0].Source {
			t.Error("conversas deveriam ter Sources diferentes")
		}

		if conv1Msgs[0].Source != "telegram" {
			t.Errorf("conv1 deveria ter Source=telegram, obteve %s", conv1Msgs[0].Source)
		}

		if conv2Msgs[0].Source != "signal" {
			t.Errorf("conv2 deveria ter Source=signal, obteve %s", conv2Msgs[0].Source)
		}
	}

	t.Log("✓ Isolamento de conversas por Source (canal) validado")
}

// TestIntegration_FirstMessageChannelResponseRoute testa roteamento de resposta para channel correto
func TestIntegration_FirstMessageChannelResponseRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: mensagem chega via Telegram
	channel := "telegram"

	conv := &database.Conversation{
		Title:     "Test from Telegram",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem chega via Telegram
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Mensagem do usuário",
		Source:         channel,
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Sistema envia resposta
	// A resposta DEVE ser roteada de volta para o MESMO Source (channel)
	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Resposta do assistente",
		Source:         channel, // ← MESMO CHANNEL
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 4. Validar que resposta foi roteada corretamente
	var assistantRetrieved database.ChatMessage
	if err := db.First(&assistantRetrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if assistantRetrieved.Source != channel {
		t.Errorf("resposta deveria ser roteada para %s, obteve %s", channel, assistantRetrieved.Source)
	}

	t.Logf("✓ Resposta roteada corretamente: Source=%s", assistantRetrieved.Source)
}

// TestIntegration_FirstMessageChannelAudioHandling testa áudio em canais
func TestIntegration_FirstMessageChannelAudioHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: mensagem com áudio via Telegram
	channel := "telegram"

	conv := &database.Conversation{
		Title:     "Audio from Telegram",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem com áudio chega via Telegram
	audioBase64 := "SUQzBAAAAAAAI1NTVQAAAAAPAADDTGF2ZjU2LjQwLjEwMQAAAAAAAAAA"

	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Transcrição do áudio: Olá, tudo bem?",
		Source:         channel,
		Audio:          audioBase64,
		AudioMimeType:  "audio/mpeg",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem com áudio: %v", err)
	}

	// 3. Validar que áudio foi persistido
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", userMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar mensagem: %v", err)
	}

	if retrieved.Audio == "" {
		t.Error("áudio não foi persistido")
	}

	if retrieved.AudioMimeType != "audio/mpeg" {
		t.Errorf("AudioMimeType incorreto: %s", retrieved.AudioMimeType)
	}

	if retrieved.Source != channel {
		t.Errorf("Source não foi preservado: esperado %s, obteve %s", channel, retrieved.Source)
	}

	t.Log("✓ Áudio em canal (Telegram via áudio) persistido corretamente")
}

// TestIntegration_FirstMessageChannelMediaHandling testa mídia em canais
func TestIntegration_FirstMessageChannelMediaHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: conversa via Slack com imagem
	channel := "slack"

	conv := &database.Conversation{
		Title:     "Media from Slack",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem com imagem chega via Slack
	mediaJSON := `[{"type":"image","data":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==","mimeType":"image/png"}]`

	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Veja esta imagem",
		Source:         channel,
		Media:          mediaJSON,
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem com mídia: %v", err)
	}

	// 3. Validar que mídia foi persistida
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", userMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar mensagem: %v", err)
	}

	if retrieved.Media == "" {
		t.Error("mídia não foi persistida")
	}

	var mediaData []map[string]interface{}
	if err := json.Unmarshal([]byte(retrieved.Media), &mediaData); err != nil {
		t.Fatalf("falha ao desserializar mídia: %v", err)
	}

	if len(mediaData) != 1 {
		t.Errorf("esperado 1 mídia, obteve %d", len(mediaData))
	}

	t.Log("✓ Mídia em canal (Slack com imagem) persistida corretamente")
}
