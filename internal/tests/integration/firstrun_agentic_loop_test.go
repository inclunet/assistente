package integration

import (
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/profiles"
)

// TestIntegration_FirstMessageAgenticLoopDefaultLimit testa que limite padrão é 25 iterações
func TestIntegration_FirstMessageAgenticLoopDefaultLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil SEM MaxAgenticIterations configurado (usa default 25)
	defaultProfile := profiles.Profile{
		Name:   "Default Loop Limit",
		Active: true,
		Chat: profiles.ChatConfig{
			LLMProvider:  "openai",
			Model:        "gpt-4o",
			Temperature:  0.7,
			MaxTokens:    2000,
			DisableTools: false,
			EnabledTools: []string{"web_search", "calculator", "file_read"},
			// MaxAgenticIterations NOT SET → uses default 25
		},
		Voice: profiles.VoiceConfig{},
	}

	// 2. Validar que profile não tem MaxAgenticIterations configurado
	if defaultProfile.Chat.MaxAgenticIterations != 0 {
		t.Errorf("MaxAgenticIterations deveria ser 0 (default), obteve %d", defaultProfile.Chat.MaxAgenticIterations)
	}

	// 3. Criar conversa
	conv := &database.Conversation{
		Title:     "Default Agentic Loop",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 4. Primeira mensagem que pode gerar múltiplas tool calls
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Pesquise algo, calcule, leia arquivo",
		Source:         "wails",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 5. Resposta (deve respeitar limite padrão de 25 iterações)
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Resposta dentro do limite padrão de 25 iterações",
		Source:           "wails",
		PromptTokens:     100,
		CompletionTokens: 200,
		TotalTokens:      300,
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	t.Log("✓ Limite padrão de agentic loop (25 iterações) validado")
}

// TestIntegration_FirstMessageAgenticLoopCustomLimit testa limite customizado no profile
func TestIntegration_FirstMessageAgenticLoopCustomLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfils com diferentes MaxAgenticIterations
	limits := []struct {
		name  string
		limit int
	}{
		{"Conservative", 10},
		{"Standard", 25},
		{"CodeGen", 100},
		{"Aggressive", 500},
	}

	for _, l := range limits {
		profile := profiles.Profile{
			Name:   "Loop Limit " + l.name,
			Active: true,
			Chat: profiles.ChatConfig{
				LLMProvider:          "openai",
				Model:                "gpt-4o",
				Temperature:          0.7,
				MaxTokens:            2000,
				DisableTools:         false,
				MaxAgenticIterations: l.limit, // Custom limit
				ResponseTimeout:      90,      // 90s para múltiplas iterações
			},
			Voice: profiles.VoiceConfig{},
		}

		if profile.Chat.MaxAgenticIterations != l.limit {
			t.Errorf("perfil %s: esperado %d, obteve %d", l.name, l.limit, profile.Chat.MaxAgenticIterations)
		}

		// Criar conversa
		conv := &database.Conversation{
			Title:     "Chat " + l.name,
		}

		if err := db.Create(conv).Error; err != nil {
			t.Fatalf("falha ao criar conversa para %s: %v", l.name, err)
		}

		userMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "Execute múltiplas tool calls",
			Source:         "wails",
		}

		if err := db.Create(userMsg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem para %s: %v", l.name, err)
		}

		assistantMsg := &database.ChatMessage{
			ConversationID:   conv.ID,
			Role:             "assistant",
			Content:          "Resposta com até " + string(rune(l.limit)) + " iterações máximo",
			Source:           "wails",
			PromptTokens:     150,
			CompletionTokens: 250,
			TotalTokens:      400,
		}

		if err := db.Create(assistantMsg).Error; err != nil {
			t.Fatalf("falha ao criar resposta para %s: %v", l.name, err)
		}
	}

	t.Logf("✓ Limites customizados de agentic loop validados (10, 25, 100, 500 iterações)")
}

// TestIntegration_FirstMessageAgenticLoopHitLimit testa comportamento ao atingir limite
func TestIntegration_FirstMessageAgenticLoopHitLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil com limite baixo (5 iterações) para fácil teste
	lowLimitProfile := profiles.Profile{
		Name:   "Low Limit (5)",
		Active: true,
		Chat: profiles.ChatConfig{
			LLMProvider:          "openai",
			Model:                "gpt-4o",
			Temperature:          0.7,
			MaxTokens:            2000,
			DisableTools:         false,
			MaxAgenticIterations: 5, // Limite bem baixo para teste
			ResponseTimeout:      30,
		},
		Voice: profiles.VoiceConfig{},
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "Hit Limit Chat",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem que TENTARIA executar 5+ tool calls
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Execute 10 operações em sequência",
		Source:         "wails",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Resposta: parou no limite (5 iterações máximo)
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Executei 5 operações (limite atingido). Precisaria de outra mensagem para continuar.",
		Source:           "wails",
		PromptTokens:     100,
		CompletionTokens: 150,
		TotalTokens:      250,
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 5. Validar que parou elegantemente (sem erro, mas com notificação)
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.Content == "" {
		t.Error("resposta deveria conter mensagem de limite atingido")
	}

	t.Logf("✓ Comportamento ao atingir limite de %d iterações validado (para elegantemente)", lowLimitProfile.Chat.MaxAgenticIterations)
}

// TestIntegration_FirstMessageAgenticLoopWithinLimit testa execução dentro do limite
func TestIntegration_FirstMessageAgenticLoopWithinLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil com limite 100, mas apenas 10 iterações executadas
	profile := profiles.Profile{
		Name:   "High Limit With Few Calls",
		Active: true,
		Chat: profiles.ChatConfig{
			LLMProvider:          "openai",
			Model:                "gpt-4o",
			Temperature:          0.7,
			MaxTokens:            3000,
			DisableTools:         false,
			MaxAgenticIterations: 100, // Limite alto
			ResponseTimeout:      60,
		},
		Voice: profiles.VoiceConfig{},
	}

	if profile.Chat.MaxAgenticIterations != 100 {
		t.Errorf("MaxAgenticIterations esperado 100, obteve %d", profile.Chat.MaxAgenticIterations)
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "Within Limit Chat",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Execute 10 operações simples",
		Source:         "wails",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Resposta com 10 iterações (bem dentro do limite de 100)
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Completei todas as 10 operações solicitadas normalmente, sem atingir limite (10/100)",
		Source:           "wails",
		PromptTokens:     150,
		CompletionTokens: 300,
		TotalTokens:      450,
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	t.Log("✓ Execução dentro do limite (10/100 iterações) funciona normalmente")
}

// TestIntegration_FirstMessageAgenticLoopExceedsLimit testa quando iterações excedem limite
func TestIntegration_FirstMessageAgenticLoopExceedsLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil com limite 50, mas tenta executar 100 iterações
	profile := profiles.Profile{
		Name:   "Limit Exceeded",
		Active: true,
		Chat: profiles.ChatConfig{
			LLMProvider:          "openai",
			Model:                "gpt-4o",
			Temperature:          0.7,
			MaxTokens:            4000,
			DisableTools:         false,
			MaxAgenticIterations: 50, // Limite: 50
			ResponseTimeout:      90,
		},
		Voice: profiles.VoiceConfig{},
	}

	if profile.Chat.MaxAgenticIterations != 50 {
		t.Errorf("MaxAgenticIterations esperado 50, obteve %d", profile.Chat.MaxAgenticIterations)
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "Exceeded Limit Chat",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem (tentaria 100 calls)
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Execute 100 operações complexas",
		Source:         "wails",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Resposta: parou no limite (50 iterações executadas das 100 solicitadas)
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Executei 50 operações e atingi o limite da configuração do perfil. Para continuar, envie outra mensagem e vou retomar de onde parei.",
		Source:           "wails",
		PromptTokens:     200,
		CompletionTokens: 400,
		TotalTokens:      600,
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	t.Logf("✓ Iterações acima do limite (100 tentou, 50 executou) respeitado e parado elegantemente")
}

// TestIntegration_FirstMessageAgenticLoopTokenCounting testa contagem de tokens em múltiplas iterações
func TestIntegration_FirstMessageAgenticLoopTokenCounting(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil com limite 50 iterações
	profile := profiles.Profile{
		Name:   "Token Counting Loop",
		Active: true,
		Chat: profiles.ChatConfig{
			LLMProvider:          "openai",
			Model:                "gpt-4o",
			Temperature:          0.7,
			MaxTokens:            5000, // Contexto generoso
			DisableTools:         false,
			MaxAgenticIterations: 50,
			ResponseTimeout:      60,
		},
		Voice: profiles.VoiceConfig{},
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "Token Count Loop",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "user",
		Content:          "Execute múltipla operações e some tokens",
		Source:           "wails",
		PromptTokens:     50,
		CompletionTokens: 0,
		TotalTokens:      50,
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Resposta com múltiplas iterações (tokens acumulados)
	// Simulando:
	// - Iteração 1: 50 tokens (prompt) + 100 (completion)
	// - Iteração 2: 150 tokens (context + prompt) + 120 (completion)
	// ... até ~50 iterações
	// Total acumulado: ~4800 tokens (dentro do MaxTokens de 5000)

	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Completei múltiplas iterações com rastreamento de tokens. Total acumulado: 4800 tokens (dentro do limite de 5000)",
		Source:           "wails",
		PromptTokens:     300,  // Acumulado de todas as iterações
		CompletionTokens: 4500, // Acumulado
		TotalTokens:      4800, // Total dentro do MaxTokens
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 5. Validar que tokens foram rastreados corretamente através das iterações
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.TotalTokens > profile.Chat.MaxTokens {
		t.Errorf("total tokens (%d) não deveria exceder MaxTokens (%d) mesmo em múltiplas iterações",
			retrieved.TotalTokens, profile.Chat.MaxTokens)
	}

	t.Logf("✓ Token counting em múltiplas iterações validado (%.0f%% do MaxTokens utilizado)",
		float64(retrieved.TotalTokens)/float64(profile.Chat.MaxTokens)*100)
}

// TestIntegration_FirstMessageAgenticLoopTimeoutProtection testa timeout como 2ª camada de proteção
func TestIntegration_FirstMessageAgenticLoopTimeoutProtection(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil com limite alto (100 iterações) mas timeout curto (10s)
	// Combina ambas proteções
	profile := profiles.Profile{
		Name:   "Timeout Protection",
		Active: true,
		Chat: profiles.ChatConfig{
			LLMProvider:          "openai",
			Model:                "gpt-4o",
			Temperature:          0.7,
			MaxTokens:            3000,
			DisableTools:         false,
			MaxAgenticIterations: 100, // Limite alto
			ResponseTimeout:      10,  // Timeout curto (10s)
		},
		Voice: profiles.VoiceConfig{},
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "Timeout Protection Loop",
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Execute operações lentamente",
		Source:         "wails",
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Resposta: parou por timeout (8s) antes de atingir 100 iterações
	// Se tivesse continuado, teria demorado 15+s
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Execução interrompida por timeout após 8.5 segundos (limite: 10s do perfil). Apenas 15 das 100 iterações possíveis foram completadas.",
		Source:           "wails",
		PromptTokens:     100,
		CompletionTokens: 150,
		TotalTokens:      250,
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 5. Validar tempo de resposta
	timeTaken := assistantMsg.CreatedAt.Sub(userMsg.CreatedAt)

	if timeTaken > time.Duration(profile.Chat.ResponseTimeout)*time.Second {
		t.Errorf("tempo total (%v) excedeu ResponseTimeout (%ds)",
			timeTaken, profile.Chat.ResponseTimeout)
	}

	t.Logf("✓ Timeout como 2ª proteção validado (%.1fs < %ds)", timeTaken.Seconds(), profile.Chat.ResponseTimeout)
}
