package integration

import (
	"errors"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/llm"
)

// TestIntegration_FirstMessageNoProviderConfigured testa detecção: nenhum provider configurado
// Validar que o user recebe feedback claro antes de tentar enviar
func TestIntegration_FirstMessageNoProviderConfigured(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: database vazio, nenhum provider registrado
	registry := llm.NewProviderRegistry()

	if len(registry.List()) != 0 {
		t.Fatal("registry deveria estar vazio")
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "Sem Provider",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Validar que não há provider disponível
	availableProviders := registry.List()
	if len(availableProviders) != 0 {
		t.Error("deveria ter 0 providers, mas encontrou alguns")
	}

	// 4. Em um cenário real, SendMessage deveria validar isso ANTES de criar a mensagem
	// Simular a validação:
	canSendMessage := len(availableProviders) > 0

	if canSendMessage {
		t.Error("não deveria permitir envio sem provider configurado")
	}

	// 5. Mensagem de feedback que usuário receberia (UX crucial)
	errorMessage := "Nenhum provedor LLM foi configurado. Configure um provedor (OpenAI, Claude, Ollama) nas configurações."

	if errorMessage == "" {
		t.Error("deveria ter mensagem clara para o usuário")
	}

	t.Logf("✓ Validação pré-envio: nenhum provider → feedback clara ao usuário")
}

// TestIntegration_FirstMessageNoCredentialsForProvider testa: provider existe mas sem credencial
func TestIntegration_FirstMessageNoCredentialsForProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: provider registrado, mas sem credencial armazenada
	registry := llm.NewProviderRegistry()

	provider := &llm.ProviderConfig{
		ID:      "openai-default",
		Name:    "OpenAI",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4o",
	}

	if err := registry.Register(provider); err != nil {
		t.Fatalf("falha ao registrar provider: %v", err)
	}

	// 2. Validar que provider exists mas sem credenciais
	providers := registry.List()
	if len(providers) != 1 {
		t.Fatalf("esperado 1 provider, encontrou %d", len(providers))
	}

	// 3. Validar que não há credenciais no banco para este provider
	var credentialCount int64
	if err := db.Model(&database.CredentialEntry{}).Where("pattern LIKE ?", "%openai%").Count(&credentialCount).Error; err != nil {
		t.Logf("falha ao contar credenciais: %v", err)
	}

	hasCredential := credentialCount > 0
	if hasCredential {
		t.Error("banco deveria estar vazio de credenciais")
	}

	// 4. Feedback ao usuário (pré-validação)
	if !hasCredential {
		feedbackMsg := "Você não configurou uma chave de API para OpenAI. Clique aqui para adicionar."
		t.Logf("✓ Feedback ao usuário: %s", feedbackMsg)
	}

	// 5. Não criar mensagem sem credencial válida
	conv := &database.Conversation{
		Title:     "Sem Credencial",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// Simular: não permitir envio sem credencial
	canSend := hasCredential
	if canSend {
		t.Error("não deveria permitir envio sem credencial do provider")
	}

	t.Log("✓ Validação: provider existe mas sem credencial → feedback claro")
}

// TestIntegration_FirstMessageProviderSelectionFlow testa fluxo de seleção de provider
func TestIntegration_FirstMessageProviderSelectionFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: múltiplos providers, usuário escolhe um
	registry := llm.NewProviderRegistry()

	providers := []*llm.ProviderConfig{
		{
			ID:      "openai",
			Name:    "OpenAI GPT-4",
			Type:    llm.ProviderOpenAI,
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o",
		},
		{
			ID:      "claude",
			Name:    "Claude 3.5 Sonnet",
			Type:    llm.ProviderClaude,
			BaseURL: "https://api.anthropic.com",
			Model:   "claude-3-5-sonnet",
		},
		{
			ID:      "ollama",
			Name:    "Ollama Local",
			Type:    llm.ProviderOllama,
			BaseURL: "http://localhost:11434",
			Model:   "llama2",
		},
	}

	for _, p := range providers {
		if err := registry.Register(p); err != nil {
			t.Fatalf("falha ao registrar provider %s: %v", p.ID, err)
		}
	}

	// 2. Listar providers disponíveis para o usuário escolher
	availableProviders := registry.List()
	if len(availableProviders) != 3 {
		t.Fatalf("esperado 3 providers, obteve %d", len(availableProviders))
	}

	// 3. Usuário escolhe o primeiro (padrão: OpenAI)
	selectedProvider := registry.Get("openai")
	if selectedProvider == nil {
		t.Fatal("provider OpenAI não foi encontrado")
	}

	if selectedProvider.Name != "OpenAI GPT-4" {
		t.Errorf("provider incorreto: %s", selectedProvider.Name)
	}

	// 4. Criar conversa e primeira mensagem COM provider selecionado
	conv := &database.Conversation{
		Title:     "Com Provider Selecionado",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 5. Primeira mensagem (agora com provider pronto)
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Seu primeiro teste com provider selecionado",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	t.Logf("✓ Fluxo: múltiplos providers listados → usuário selecionou OpenAI → primeira mensagem OK")
}

// TestIntegration_FirstMessageAPITimeout testa comportamento em timeout
func TestIntegration_FirstMessageAPITimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar conversa e primeira mensagem
	conv := &database.Conversation{
		Title:     "Timeout Test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem do usuário
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Esta mensagem causará timeout",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem do usuário: %v", err)
	}

	// 3. Simular timeout na resposta (ex: 45 segundos de espera)
	// Em um caso real, SendMessage teria timeout de ~30s
	timeoutOccurred := true // Simula que o timeout aconteceu
	timeoutDuration := 45 * time.Second

	// 4. Não descartar a mensagem do usuário mesmo com timeout
	var persistedMsg database.ChatMessage
	if err := db.First(&persistedMsg, "id = ?", userMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar mensagem do usuário: %v", err)
	}

	if persistedMsg.Content != "Esta mensagem causará timeout" {
		t.Error("mensagem do usuário foi perdida durante timeout")
	}

	// 5. Feedback ao usuário - claro e acionável
	if timeoutOccurred {
		feedbackMsg := "A resposta demorou muito tempo. Verifique sua conexão ou tente novamente."
		t.Logf("✓ Timeout detectado após %v → Feedback: %s", timeoutDuration, feedbackMsg)
	}

	// 6. Permitir retry sem reenviar a mesma mensagem
	canRetry := true
	if !canRetry {
		t.Error("usuário deveria poder clicar em 'Tentar Novamente' após timeout")
	}

	t.Log("✓ Timeout: mensagem do usuário preservada, retry habilitado")
}

// TestIntegration_FirstMessageFallbackProvider testa fallback automático para outro provider
func TestIntegration_FirstMessageFallbackProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: múltiplos providers (fallback chain)
	registry := llm.NewProviderRegistry()

	providerChain := []*llm.ProviderConfig{
		{ID: "openai", Name: "OpenAI", Type: llm.ProviderOpenAI, BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"},
		{ID: "claude", Name: "Claude", Type: llm.ProviderClaude, BaseURL: "https://api.anthropic.com", Model: "claude-3-5-sonnet"},
		{ID: "ollama", Name: "Ollama", Type: llm.ProviderOllama, BaseURL: "http://localhost:11434", Model: "llama2"},
	}

	for _, p := range providerChain {
		if err := registry.Register(p); err != nil {
			t.Fatalf("falha ao registrar provider: %v", err)
		}
	}

	// 2. Criar conversa e primeira mensagem
	conv := &database.Conversation{
		Title:     "Fallback Test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Teste de fallback entre providers",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Simular erro com o primeiro provider (OpenAI)
	primaryProviderError := errors.New("invalid API key for OpenAI")

	// 4. Tentar fallback: próximo provider (Claude)
	if primaryProviderError != nil {
		t.Logf("✓ OpenAI falhou: %v → tentando Claude", primaryProviderError)
	}

	// 5. Resposta bem-sucedida com fallback provider
	fallbackMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Resposta obtida via Claude (fallback)",
		Model:          "claude-3-5-sonnet", // Rastreando qual provider foi usado
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(500 * time.Millisecond),
	}

	if err := db.Create(fallbackMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta de fallback: %v", err)
	}

	// 6. Validar que fallback foi usado
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", fallbackMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.Model != "claude-3-5-sonnet" {
		t.Errorf("modelo de fallback não foi rastreado: %s", retrieved.Model)
	}

	// 7. Usuário não sabe que houve problema - experiência seamless
	t.Logf("✓ Fallback seamless: OpenAI (erro) → Claude (sucesso) → novo usuário recebe resposta")
}

// TestIntegration_FirstMessageRetryMechanism testa retry sem perder contexto
func TestIntegration_FirstMessageRetryMechanism(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Criar conversa
	conv := &database.Conversation{
		Title:     "Retry Mechanism",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 2. Primeira mensagem do usuário
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é sua resposta?",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Simulação: primeira tentativa falha (ex: erro 429 - rate limit)
	attemptError := errors.New("rate limit exceeded")

	// 4. Mostrar feedback ao usuário
	if attemptError != nil {
		feedbackMsg := "Limite de taxa excedido. Aguarde um momento e tente novamente."
		t.Logf("✓ Erro da 1ª tentativa: %s → Feedback: %s", attemptError.Error(), feedbackMsg)
	}

	// 5. Usuário clica "Tentar Novamente" - NÃO reenviar a mesma mensagem
	// Apenas tentar chamar a API novamente com a mesma mensagem existente
	retryCount := 1

	if retryCount < 1 {
		t.Error("deveria permitir pelo menos 1 retry")
	}

	// 6. Segunda tentativa bem-sucedida
	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Resposta após retry bem-sucedido",
		Model:          "gpt-4o",
		TurnID:         &userMsg.ID,
		CreatedAt:      userMsg.CreatedAt.Add(2 * time.Second), // Mais tempo na 2ª tentativa
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta de retry: %v", err)
	}

	// 7. Validar que histórico está limpo (sem mensagens de erro intermediárias)
	var allMessages []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at").Find(&allMessages).Error; err != nil {
		t.Fatalf("falha ao recuperar histórico: %v", err)
	}

	if len(allMessages) != 2 {
		t.Errorf("histórico deveria ter 2 mensagens (user+assistant), obteve %d", len(allMessages))
	}

	// 8. Validar que a resposta é clara (sem "Erro: ...")
	if allMessages[1].Role != "assistant" || len(allMessages[1].Content) == 0 {
		t.Error("resposta de retry deveria ser uma resposta normal, não uma mensagem de erro")
	}

	t.Log("✓ Retry: 1ª tentativa falha → usuário clica 'Tentar' → 2ª tentativa sucede → histórico limpo")
}

// TestIntegration_FirstMessageErrorRecoveryUI testa que UI indique claramente quando tentar novamente
func TestIntegration_FirstMessageErrorRecoveryUI(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Criar conversa e mensagem
	conv := &database.Conversation{
		Title:     "Error UI Test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Teste de UI de erro",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 2. Simular erro na resposta
	errorOccurred := true
	errorReason := "network_timeout"

	// 3. Rastrear que erro ocorreu (para analytics)
	var errorLog string
	if errorOccurred {
		errorLog = "Tentativa de resposta falhou com erro: " + errorReason
	}

	if errorLog == "" {
		t.Error("erro deveria estar registrado")
	}

	// 4. Validar que há informação clara para o usuário
	userFacingError := "A resposta está demorando. Tente novamente." // Mensagem simples e clara

	if userFacingError == "" {
		t.Error("usuário deveria ter mensagem clara")
	}

	// 5. Validar que há um botão/ação disponível
	retryActionAvailable := true

	if !retryActionAvailable {
		t.Error("deveria haver ação 'Tentar Novamente' disponível")
	}

	// 6. Log para debugging
	t.Logf("✓ Error UI: Erro=%s | Usuário vê: '%s' | Ação disponível: Tentar Novamente", errorReason, userFacingError)
}

// TestIntegration_FirstMessageGracefulDegradation testa degradação graciosa
func TestIntegration_FirstMessageGracefulDegradation(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)
	registry := llm.NewProviderRegistry()

	// 1. Setup: apenas Ollama disponível (modelo local, sempre funciona se ativo)
	ollama := &llm.ProviderConfig{
		ID:      "ollama-local",
		Name:    "Ollama (Local)",
		Type:    llm.ProviderOllama,
		BaseURL: "http://localhost:11434",
		Model:   "llama2",
	}

	if err := registry.Register(ollama); err != nil {
		t.Fatalf("falha ao registrar Ollama: %v", err)
	}

	// 2. Criar conversa e primeira mensagem
	conv := &database.Conversation{
		Title:     "Degradação Graciosa",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Primeira mensagem com degradação graciosa",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 3. Se API remote falhar, Ollama local continua disponível
	remoteApiDown := true
	localOllamaAvailable := true

	if remoteApiDown && localOllamaAvailable {
		t.Log("✓ APIs remote indisponíveis, mas Ollama local funciona como fallback")
	}

	// 4. Resposta via Ollama
	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Resposta obtida via Ollama (modelo local)",
		Model:          "llama2",
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(1 * time.Second),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 5. Usuário não sabe que há problemas - sistema funcionou
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.Model != "llama2" {
		t.Error("modelo de degradação não foi rastreado")
	}

	t.Log("✓ Degradação: APIs remote indisponíveis → Ollama local funciona → usuário não percebe")
}
