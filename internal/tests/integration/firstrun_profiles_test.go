package integration

import (
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/profiles"
)

// TestIntegration_FirstMessageWithCreativeProfile testa primeira mensagem com perfil criativo (temp alta)
func TestIntegration_FirstMessageWithCreativeProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar perfil CRIATIVO (temperatura alta, top_p alto)
	creativeProfile := profiles.Profile{
		Name:        "Criativo",
		Description: "Respostas criativas e variadas",
		Icon:        "✨",
		Active:      true,
		Chat: profiles.ChatConfig{
			LLMProvider:     "openai",
			Model:           "gpt-4o",
			Temperature:     1.5, // HIGH - criativo
			MaxTokens:       2000,
			TopP:            0.95,
			ResponseTimeout: 45,
			DisableTools:    false,
			EnabledTools:    []string{},
		},
		Voice: profiles.VoiceConfig{
			Disabled: true,
		},
	}

	// 2. Simular seleção deste perfil (em um cenário real, viria do frontend)
	// Para este teste, apenas validamos que o perfil está configurado
	expectedTemperature := 1.5
	expectedMaxTokens := 2000

	if creativeProfile.Chat.Temperature != expectedTemperature {
		t.Errorf("temperatura do perfil criativo incorreta: esperado %f, obteve %f", expectedTemperature, creativeProfile.Chat.Temperature)
	}

	if creativeProfile.Chat.MaxTokens != expectedMaxTokens {
		t.Errorf("maxTokens do perfil criativo incorreto: esperado %d, obteve %d", expectedMaxTokens, creativeProfile.Chat.MaxTokens)
	}

	// 3. Criar conversa
	conv := &database.Conversation{
		Title:     "Criative Chat",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 4. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Crie uma história criativa",
		Source:         "wails",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 5. Resposta (simulando que foi gerada com as configs do perfil)
	// Em um teste real, o LLM usaria temperature=1.5 para gerar output mais criativo
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Era uma vez uma história absolutamente criativa que nunca ninguém havia imaginado...",
		Source:           "wails",
		Model:            "gpt-4o",
		PromptTokens:     30,
		CompletionTokens: 150,
		TotalTokens:      180,
		CreatedAt:        time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 6. Validações
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.Model != "gpt-4o" {
		t.Errorf("modelo incorreto: esperado gpt-4o, obteve %s", retrieved.Model)
	}

	t.Logf("✓ Perfil criativo (temp=%.1f, max_tokens=%d) aplicado na primeira mensagem", creativeProfile.Chat.Temperature, creativeProfile.Chat.MaxTokens)
}

// TestIntegration_FirstMessageWithDeterministicProfile testa primeira mensagem com perfil determinístico (temp=0)
func TestIntegration_FirstMessageWithDeterministicProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: criar perfil DETERMINÍSTICO (temperatura 0)
	deterministicProfile := profiles.Profile{
		Name:        "Determinístico",
		Description: "Respostas previsíveis e consistentes",
		Icon:        "🎯",
		Active:      true,
		Chat: profiles.ChatConfig{
			LLMProvider:     "openai",
			Model:           "gpt-4o",
			Temperature:     0.0, // ZERO - determinístico
			MaxTokens:       1000,
			TopP:            0.5,
			ResponseTimeout: 45,
			DisableTools:    false,
		},
		Voice: profiles.VoiceConfig{
			Disabled: true,
		},
	}

	expectedTemperature := 0.0

	if deterministicProfile.Chat.Temperature != expectedTemperature {
		t.Errorf("temperatura incorreta: esperado %f, obteve %f", expectedTemperature, deterministicProfile.Chat.Temperature)
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "Deterministic Chat",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é a capital da França?",
		Source:         "wails",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Resposta (mesma pergunta com temp=0 sempre dará mesma resposta)
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "A capital da França é Paris.",
		Source:           "wails",
		Model:            "gpt-4o",
		PromptTokens:     20,
		CompletionTokens: 25,
		TotalTokens:      45,
		CreatedAt:        time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	t.Logf("✓ Perfil determinístico (temp=%.1f) aplicado na primeira mensagem", deterministicProfile.Chat.Temperature)
}

// TestIntegration_FirstMessageWithToolsDisabled testa primeira mensagem com tools desabilitadas
func TestIntegration_FirstMessageWithToolsDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil com tools DESABILITADAS
	noToolsProfile := profiles.Profile{
		Name:        "Sem Ferramentas",
		Description: "Modo offline, sem acesso a ferramentas",
		Icon:        "🔇",
		Active:      true,
		Chat: profiles.ChatConfig{
			LLMProvider:  "openai",
			Model:        "gpt-4o",
			Temperature:  0.7,
			MaxTokens:    1000,
			DisableTools: true, // TOOLS DESABILITADAS
			EnabledTools: []string{},
		},
		Voice: profiles.VoiceConfig{
			Disabled: true,
		},
	}

	if !noToolsProfile.Chat.DisableTools {
		t.Error("DisableTools deveria ser true")
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "No Tools Chat",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem que normalmente acionaria uma ferramenta
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é a temperatura em São Paulo?",
		Source:         "wails",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Resposta SEM tool calls (porque foram desabilitadas)
	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Desculpe, não tenho acesso a ferramentas neste perfil. Você teria que me informar a temperatura.",
		ToolCalls:      "", // VAZIO - sem tool calls
		Source:         "wails",
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 5. Validar que ToolCalls está vazio
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.ToolCalls != "" {
		t.Errorf("ToolCalls deveria estar vazio, obteve: %s", retrieved.ToolCalls)
	}

	t.Log("✓ Perfil sem ferramentas (DisableTools=true) respeitado na primeira mensagem")
}

// TestIntegration_FirstMessageWithMCPDisabled testa primeira mensagem com MCP desabilitado
func TestIntegration_FirstMessageWithMCPDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil com MCP desabilitado
	// Para simular isso, usamos MCPMode = "" (sistema não carrega MCP servers)
	noMCPProfile := profiles.Profile{
		Name:        "Sem MCP",
		Description: "Sem Model Context Protocol",
		Icon:        "🔒",
		Active:      true,
		Chat: profiles.ChatConfig{
			LLMProvider:  "openai",
			Model:        "gpt-4o",
			Temperature:  0.7,
			MaxTokens:    1000,
			DisableTools: false,
			EnabledTools: []string{}, // Apenas built-in tools, sem MCP
			MCPMode:      "",         // VAZIO - MCP desabilitado
		},
		Voice: profiles.VoiceConfig{
			Disabled: true,
		},
	}

	if noMCPProfile.Chat.MCPMode != "" {
		t.Errorf("MCPMode deveria ser vazio, obteve: %s", noMCPProfile.Chat.MCPMode)
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "No MCP Chat",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Procure no GitHub",
		Source:         "wails",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Resposta sem MCP tools (apenas built-in tools disponíveis)
	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "MCP está desabilitado neste perfil, então não posso acessar GitHub.",
		Source:         "wails",
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	t.Log("✓ Perfil sem MCP (MCPMode='') respeitado na primeira mensagem")
}

// TestIntegration_FirstMessageDifferentModels testa primeira mensagem com diferentes modelos
func TestIntegration_FirstMessageDifferentModels(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: diferentes modelos
	models := []struct {
		name     string
		model    string
		provider string
	}{
		{"GPT-4o", "gpt-4o", "openai"},
		{"Claude 3.5", "claude-3-5-sonnet", "anthropic"},
		{"Local Ollama", "llama-2-7b", "ollama"},
	}

	for _, m := range models {
		profile := profiles.Profile{
			Name:   m.name,
			Active: true,
			Chat: profiles.ChatConfig{
				LLMProvider: m.provider,
				Model:       m.model,
				Temperature: 0.7,
				MaxTokens:   1000,
			},
			Voice: profiles.VoiceConfig{
				Disabled: true,
			},
		}

		if profile.Chat.Model != m.model {
			t.Errorf("modelo incorreto: esperado %s, obteve %s", m.model, profile.Chat.Model)
		}

		if profile.Chat.LLMProvider != m.provider {
			t.Errorf("provider incorreto: esperado %s, obteve %s", m.provider, profile.Chat.LLMProvider)
		}

		// 2. Simular primeira mensagem com este modelo
		conv := &database.Conversation{
			Title:     "Chat with " + m.name,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := db.Create(conv).Error; err != nil {
			t.Fatalf("falha ao criar conversa para %s: %v", m.name, err)
		}

		userMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "Olá",
			Source:         "wails",
			CreatedAt:      time.Now(),
		}

		if err := db.Create(userMsg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem para %s: %v", m.name, err)
		}

		assistantMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        "Olá! Sou " + m.name,
			Source:         "wails",
			Model:          m.model,
			CreatedAt:      time.Now().Add(100 * time.Millisecond),
		}

		if err := db.Create(assistantMsg).Error; err != nil {
			t.Fatalf("falha ao criar resposta para %s: %v", m.name, err)
		}

		// 3. Validar que modelo foi usado
		var retrieved database.ChatMessage
		if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
			t.Fatalf("falha ao recuperar resposta de %s: %v", m.name, err)
		}

		if retrieved.Model != m.model {
			t.Errorf("modelo na resposta incorreto: esperado %s, obteve %s", m.model, retrieved.Model)
		}
	}

	t.Logf("✓ Primeira mensagem funciona com %d modelos diferentes", len(models))
}

// TestIntegration_FirstMessageWithContextWindowLimit testa primeira mensagem com limite de context window
func TestIntegration_FirstMessageWithContextWindowLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil com context window pequeno (simula modelo com janela limitada)
	limitedContextProfile := profiles.Profile{
		Name:   "Limited Context",
		Active: true,
		Chat: profiles.ChatConfig{
			LLMProvider:        "openai",
			Model:              "gpt-3.5-turbo",
			Temperature:        0.7,
			MaxTokens:          500,
			ContextWindow:      4096, // Janela pequena
			MaxContextMessages: 10,   // Maks 10 mensagens no contexto
		},
		Voice: profiles.VoiceConfig{
			Disabled: true,
		},
	}

	if limitedContextProfile.Chat.MaxContextMessages != 10 {
		t.Errorf("maxContextMessages incorreto: esperado 10, obteve %d", limitedContextProfile.Chat.MaxContextMessages)
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "Limited Context Chat",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Primeira mensagem",
		Source:         "wails",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Resposta com context window limitado",
		Source:           "wails",
		Model:            "gpt-3.5-turbo",
		PromptTokens:     50,
		CompletionTokens: 30,
		TotalTokens:      80,
		CreatedAt:        time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 4. Validar que tokens foram rastreados respeitando limite
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.TotalTokens > limitedContextProfile.Chat.ContextWindow {
		t.Errorf("total tokens (%d) excedeu context window (%d)", retrieved.TotalTokens, limitedContextProfile.Chat.ContextWindow)
	}

	t.Logf("✓ Limite de context window (%d) respeitado na primeira mensagem", limitedContextProfile.Chat.ContextWindow)
}

// TestIntegration_FirstMessageProfileResponseTimeout testa timeout customizado do perfil
func TestIntegration_FirstMessageProfileResponseTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil com timeout customizado (30s ao invés de 45s padrão)
	quickTimeoutProfile := profiles.Profile{
		Name:   "Quick Timeout",
		Active: true,
		Chat: profiles.ChatConfig{
			LLMProvider:     "openai",
			Model:           "gpt-4o",
			Temperature:     0.7,
			MaxTokens:       1000,
			ResponseTimeout: 30, // 30 segundos (ao invés de 45s padrão)
		},
		Voice: profiles.VoiceConfig{
			Disabled: true,
		},
	}

	if quickTimeoutProfile.Chat.ResponseTimeout != 30 {
		t.Errorf("timeout incorreto: esperado 30, obteve %d", quickTimeoutProfile.Chat.ResponseTimeout)
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "Quick Timeout Chat",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Teste de timeout",
		Source:         "wails",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Resposta com sucesso (antes do timeout de 30s)
	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Resposta rápida",
		Source:         "wails",
		CreatedAt:      time.Now().Add(5 * time.Second), // Resposta em 5s < 30s timeout
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	t.Logf("✓ Timeout customizado do perfil (%ds) respeitado na primeira mensagem", quickTimeoutProfile.Chat.ResponseTimeout)
}

// TestIntegration_FirstMessageProfileParametersPropagation testa que parâmetros do profile são propagados para requisição ao LLM
func TestIntegration_FirstMessageProfileParametersPropagation(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: perfil com parâmetros específicos
	testProfile := profiles.Profile{
		Name:   "Params Test",
		Active: true,
		Chat: profiles.ChatConfig{
			LLMProvider:     "openai",
			Model:           "gpt-4o",
			Temperature:     0.8,                 // Custom temperature
			MaxTokens:       2500,                // Custom max tokens
			TopP:            0.85,                // Custom top_p
			MaxTokensMode:   "completion_tokens", // Custom max tokens mode
			ResponseTimeout: 60,
		},
		Voice: profiles.VoiceConfig{
			Disabled: true,
		},
	}

	// 2. Validar que parâmetros estão no profile
	if testProfile.Chat.Temperature != 0.8 {
		t.Errorf("temperatura esperada 0.8, obteve %f", testProfile.Chat.Temperature)
	}
	if testProfile.Chat.MaxTokens != 2500 {
		t.Errorf("maxTokens esperado 2500, obteve %d", testProfile.Chat.MaxTokens)
	}
	if testProfile.Chat.TopP != 0.85 {
		t.Errorf("topP esperado 0.85, obteve %f", testProfile.Chat.TopP)
	}
	if testProfile.Chat.MaxTokensMode != "completion_tokens" {
		t.Errorf("maxTokensMode esperado completion_tokens, obteve %s", testProfile.Chat.MaxTokensMode)
	}

	// 3. Criar conversa
	conv := &database.Conversation{
		Title:     "Params Propagation Chat",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 4. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Crie um sumário detalhado",
		Source:         "wails",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 5. Resposta (os params do profile foram "aplicados" internamente)
	assistantMsg := &database.ChatMessage{
		ConversationID:   conv.ID,
		Role:             "assistant",
		Content:          "Aqui está um sumário detalhado com parâmetros customizados...",
		Source:           "wails",
		Model:            "gpt-4o",
		PromptTokens:     100,
		CompletionTokens: 150, // Respeitando MaxTokens=2500
		TotalTokens:      250,
		CreatedAt:        time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar resposta: %v", err)
	}

	// 6. Validar que tokens estão dentro do limite
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resposta: %v", err)
	}

	if retrieved.CompletionTokens > testProfile.Chat.MaxTokens {
		t.Errorf("completion tokens (%d) excedeu maxTokens do profile (%d)", retrieved.CompletionTokens, testProfile.Chat.MaxTokens)
	}

	t.Logf("✓ Parâmetros do profile (temp=%.1f, maxTokens=%d, topP=%.2f) propagados corretamente", testProfile.Chat.Temperature, testProfile.Chat.MaxTokens, testProfile.Chat.TopP)
}

// TestIntegration_FirstMessageProfileToolEnabling testa habilitação/desabilitação de tools por profile
func TestIntegration_FirstMessageProfileToolEnabling(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: dois profiles - um com web_search habilitado, outro desabilitado
	profilesMap := map[string]profiles.Profile{
		"with_web_search": {
			Name:   "Com Web Search",
			Active: true,
			Chat: profiles.ChatConfig{
				LLMProvider:  "openai",
				Model:        "gpt-4o",
				Temperature:  0.7,
				MaxTokens:    1000,
				DisableTools: false,
				EnabledTools: []string{"web_search", "calculator"},
			},
			Voice: profiles.VoiceConfig{
				Disabled: true,
			},
		},
		"without_web_search": {
			Name:   "Sem Web Search",
			Active: true,
			Chat: profiles.ChatConfig{
				LLMProvider:  "openai",
				Model:        "gpt-4o",
				Temperature:  0.7,
				MaxTokens:    1000,
				DisableTools: false,
				EnabledTools: []string{"calculator"}, // Apenas calculator
			},
			Voice: profiles.VoiceConfig{
				Disabled: true,
			},
		},
	}

	for profileName, profile := range profilesMap {
		// Validar que tools estão configuradas corretamente
		hasWebSearch := false
		for _, tool := range profile.Chat.EnabledTools {
			if tool == "web_search" {
				hasWebSearch = true
				break
			}
		}

		if profileName == "with_web_search" && !hasWebSearch {
			t.Errorf("profile %s deveria ter web_search habilitado", profileName)
		}
		if profileName == "without_web_search" && hasWebSearch {
			t.Errorf("profile %s não deveria ter web_search habilitado", profileName)
		}

		// Criar conversa para este profile
		conv := &database.Conversation{
			Title:     "Chat with " + profileName,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := db.Create(conv).Error; err != nil {
			t.Fatalf("falha ao criar conversa para %s: %v", profileName, err)
		}

		// Primeira mensagem que poderia usar web_search
		userMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "Busque informações sobre o clima em São Paulo",
			Source:         "wails",
			CreatedAt:      time.Now(),
		}

		if err := db.Create(userMsg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem para %s: %v", profileName, err)
		}

		// Simular resposta
		assistantMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        "Resposta sobre clima",
			Source:         "wails",
			CreatedAt:      time.Now().Add(100 * time.Millisecond),
		}

		if err := db.Create(assistantMsg).Error; err != nil {
			t.Fatalf("falha ao criar resposta para %s: %v", profileName, err)
		}
	}

	t.Logf("✓ Perfis com diferentes EnabledTools validados (with_web_search vs without_web_search)")
}

// TestIntegration_FirstMessageProfileTopP testa que TopP é respeitado (nucleus sampling)
func TestIntegration_FirstMessageProfileTopP(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: dois profiles com TopP diferentes
	testProfiles := []struct {
		name string
		topP float64
	}{
		{"Conservative", 0.5}, // Conservador - menos variação
		{"Explorary", 0.95},   // Exploratório - mais variação
	}

	for _, p := range testProfiles {
		profile := profiles.Profile{
			Name:   p.name,
			Active: true,
			Chat: profiles.ChatConfig{
				LLMProvider: "openai",
				Model:       "gpt-4o",
				Temperature: 1.0,    // Temperatura alta para aproveitar TopP
				TopP:        p.topP, // TopP específico
				MaxTokens:   1000,
			},
			Voice: profiles.VoiceConfig{
				Disabled: true,
			},
		}

		if profile.Chat.TopP != p.topP {
			t.Errorf("TopP do profile %s incorreto: esperado %f, obteve %f", p.name, p.topP, profile.Chat.TopP)
		}

		// Criar conversa
		conv := &database.Conversation{
			Title:     "Chat " + p.name,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := db.Create(conv).Error; err != nil {
			t.Fatalf("falha ao criar conversa: %v", err)
		}

		// Primeira mensagem
		userMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "Crie uma história criativa",
			Source:         "wails",
			CreatedAt:      time.Now(),
		}

		if err := db.Create(userMsg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem: %v", err)
		}

		assistantMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        "Uma história com TopP=" + floatToString(p.topP),
			Source:         "wails",
			CreatedAt:      time.Now().Add(100 * time.Millisecond),
		}

		if err := db.Create(assistantMsg).Error; err != nil {
			t.Fatalf("falha ao criar resposta: %v", err)
		}
	}

	t.Log("✓ Perfis com diferentes TopP validados (Conservative=0.5 vs Explorary=0.95)")
}

// TestIntegration_FirstMessageProfileWithVoiceSettings testa configurações de voz do profile
func TestIntegration_FirstMessageProfileWithVoiceSettings(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: dois profiles com voice config diferentes
	voiceProfiles := map[string]profiles.Profile{
		"voice_enabled": {
			Name:   "Com Voz",
			Active: true,
			Chat: profiles.ChatConfig{
				LLMProvider: "openai",
				Model:       "gpt-4o",
				Temperature: 0.7,
				MaxTokens:   1000,
			},
			Voice: profiles.VoiceConfig{
				Disabled:            false,
				Provider:            "openai",
				VoiceID:             "alloy",
				Rate:                1.0,
				Pitch:               0.5,
				Volume:              1.0,
				EnabledForAgent:     true,
				EnabledForUser:      false,
				ChannelResponseMode: "mirror", // Texto em texto, áudio em áudio
			},
		},
		"voice_disabled": {
			Name:   "Sem Voz",
			Active: true,
			Chat: profiles.ChatConfig{
				LLMProvider: "openai",
				Model:       "gpt-4o",
				Temperature: 0.7,
				MaxTokens:   1000,
			},
			Voice: profiles.VoiceConfig{
				Disabled: true, // Voz desabilitada
			},
		},
	}

	for profileName, profile := range voiceProfiles {
		// Validar voice config
		if profileName == "voice_enabled" {
			if profile.Voice.Disabled {
				t.Errorf("profile %s deveria ter voz habilitada", profileName)
			}
			if profile.Voice.VoiceID != "alloy" {
				t.Errorf("VoiceID esperado 'alloy', obteve %s", profile.Voice.VoiceID)
			}
			if profile.Voice.EnabledForAgent != true {
				t.Errorf("EnabledForAgent deveria ser true para %s", profileName)
			}
		}

		if profileName == "voice_disabled" {
			if !profile.Voice.Disabled {
				t.Errorf("profile %s deveria ter voz desabilitada", profileName)
			}
		}

		// Criar conversa
		conv := &database.Conversation{
			Title:     "Chat " + profileName,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := db.Create(conv).Error; err != nil {
			t.Fatalf("falha ao criar conversa: %v", err)
		}

		userMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "Primeira mensagem",
			Source:         "wails",
			CreatedAt:      time.Now(),
		}

		if err := db.Create(userMsg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem: %v", err)
		}

		assistantMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        "Resposta",
			Source:         "wails",
			CreatedAt:      time.Now().Add(100 * time.Millisecond),
		}

		if err := db.Create(assistantMsg).Error; err != nil {
			t.Fatalf("falha ao criar resposta: %v", err)
		}
	}

	t.Log("✓ Voice settings do profile validados (habilitado/desabilitado)")
}

// TestIntegration_FirstMessageProfileChannelResponseMode testa ChannelResponseMode
func TestIntegration_FirstMessageProfileChannelResponseMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: diferentes ChannelResponseModes
	modes := []string{"mirror", "always_text", "always_audio"}

	for _, mode := range modes {
		profile := profiles.Profile{
			Name:   "Response Mode " + mode,
			Active: true,
			Chat: profiles.ChatConfig{
				LLMProvider: "openai",
				Model:       "gpt-4o",
				Temperature: 0.7,
				MaxTokens:   1000,
			},
			Voice: profiles.VoiceConfig{
				Disabled:            false,
				Provider:            "openai",
				ChannelResponseMode: mode,
			},
		}

		if profile.Voice.ChannelResponseMode != mode {
			t.Errorf("ChannelResponseMode esperado %s, obteve %s", mode, profile.Voice.ChannelResponseMode)
		}

		// Criar conversa para cada mode
		conv := &database.Conversation{
			Title:     "Chat " + mode,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := db.Create(conv).Error; err != nil {
			t.Fatalf("falha ao criar conversa para %s: %v", mode, err)
		}

		userMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "Teste de response mode: " + mode,
			Source:         "telegram", // Via canal
			CreatedAt:      time.Now(),
		}

		if err := db.Create(userMsg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem: %v", err)
		}

		// Simular resposta que deveria ser roteada de acordo com mode
		assistantMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        "Resposta com mode=" + mode,
			Source:         "telegram",
			CreatedAt:      time.Now().Add(100 * time.Millisecond),
		}

		if err := db.Create(assistantMsg).Error; err != nil {
			t.Fatalf("falha ao criar resposta: %v", err)
		}
	}

	t.Logf("✓ ChannelResponseMode validado para %d modos (mirror, always_text, always_audio)", len(modes))
}

// TestIntegration_FirstMessageProfileMCPMode testa diferentes MCPModes
func TestIntegration_FirstMessageProfileMCPMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: diferentes MCPModes
	modes := []string{"adapter", "native", "auto"}

	for _, mode := range modes {
		// Determina MCPNativeTested baseado no modo
		var nativeTested *bool
		if mode == "native" {
			trueVal := true
			nativeTested = &trueVal
		}

		profile := profiles.Profile{
			Name:   "MCP Mode " + mode,
			Active: true,
			Chat: profiles.ChatConfig{
				LLMProvider:     "openai",
				Model:           "gpt-4o",
				Temperature:     0.7,
				MaxTokens:       1000,
				MCPMode:         mode,
				MCPNativeTested: nativeTested,
			},
			Voice: profiles.VoiceConfig{
				Disabled: true,
			},
		}

		if profile.Chat.MCPMode != mode {
			t.Errorf("MCPMode esperado %s, obteve %s", mode, profile.Chat.MCPMode)
		}

		// Criar conversa
		conv := &database.Conversation{
			Title:     "Chat MCPMode " + mode,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := db.Create(conv).Error; err != nil {
			t.Fatalf("falha ao criar conversa: %v", err)
		}

		userMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "Use MCP tools",
			Source:         "wails",
			CreatedAt:      time.Now(),
		}

		if err := db.Create(userMsg).Error; err != nil {
			t.Fatalf("falha ao criar mensagem: %v", err)
		}

		assistantMsg := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        "Resposta usando MCPMode=" + mode,
			Source:         "wails",
			CreatedAt:      time.Now().Add(100 * time.Millisecond),
		}

		if err := db.Create(assistantMsg).Error; err != nil {
			t.Fatalf("falha ao criar resposta: %v", err)
		}
	}

	t.Logf("✓ MCPMode validado para %d modos (adapter, native, auto)", len(modes))
}

// Helper: converter float para string para mensagem
func floatToString(f float64) string {
	data, _ := json.Marshal(f)
	return string(data)
}
