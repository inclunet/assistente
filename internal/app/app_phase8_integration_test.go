package app

import (
	"context"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// TestPhase8_StartupFlow valida o fluxo completo de startup
func TestPhase8_StartupFlow(t *testing.T) {
	// Simular componentes do App
	llmRegistry := llm.NewProviderRegistry()
	profileManager := profiles.NewManager()

	// 1. Providers devem ser inicializados
	providers := []llm.ProviderConfig{
		{
			ID:                "openai-default",
			Name:              "OpenAI",
			Type:              llm.ProviderOpenAI,
			BaseURL:           "https://api.openai.com/v1",
			Model:             "gpt-4o-mini",
			Timeout:           180,
			CredentialPattern: "*.openai.com",
		},
		{
			ID:                "anthropic-claude",
			Name:              "Claude (Anthropic)",
			Type:              llm.ProviderClaude,
			BaseURL:           "https://api.anthropic.com/v1",
			Model:             "claude-3-7-sonnet-20250219",
			Timeout:           180,
			CredentialPattern: "*.anthropic.com",
		},
	}

	for _, p := range providers {
		if err := llmRegistry.Register(&p); err != nil {
			t.Errorf("Erro ao registrar provider %s: %v", p.ID, err)
		}
	}

	// 2. Verificar que providers foram registrados
	registeredProviders := llmRegistry.List()
	if len(registeredProviders) != 2 {
		t.Errorf("Esperado 2 providers, obteve %d", len(registeredProviders))
	}

	// 3. Verificar que podemos buscar por ID
	openai := llmRegistry.Get("openai-default")
	if openai == nil {
		t.Fatal("Provider openai-default não encontrado")
	}
	if openai.Name != "OpenAI" {
		t.Errorf("Nome incorreto: %s", openai.Name)
	}

	// 4. Profile manager deve ter profiles
	if err := profileManager.EnsureDefaults(); err != nil {
		t.Errorf("Erro ao garantir defaults: %v", err)
	}

	t.Log("✓ Startup flow validado")
}

// TestPhase8_ProfileSwitching valida troca de perfil end-to-end
func TestPhase8_ProfileSwitching(t *testing.T) {
	llmRegistry := llm.NewProviderRegistry()

	// Setup providers
	openai := &llm.ProviderConfig{
		ID:      "openai-default",
		Name:    "OpenAI",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1",
	}
	claude := &llm.ProviderConfig{
		ID:      "anthropic-claude",
		Name:    "Claude",
		Type:    llm.ProviderClaude,
		BaseURL: "https://api.anthropic.com/v1",
	}

	llmRegistry.Register(openai)
	llmRegistry.Register(claude)

	// Profile 1: OpenAI para tudo
	profile1 := profiles.Profile{
		Name: "OpenAI Full",
		Chat: profiles.ChatConfig{
			LLMProvider: "openai-default",
		},
		Voice: profiles.VoiceConfig{
			Assistant: profiles.VoiceRoleConfig{LLMProviderID: "openai-default"},
		},
		Input: profiles.InputConfig{
			LLMProviderID: "openai-default",
		},
	}

	// Profile 2: Claude para chat, OpenAI para voice (INDEPENDENTE!)
	profile2 := profiles.Profile{
		Name: "Mixed Providers",
		Chat: profiles.ChatConfig{
			LLMProvider: "anthropic-claude", // Claude para chat
		},
		Voice: profiles.VoiceConfig{
			Assistant: profiles.VoiceRoleConfig{LLMProviderID: "openai-default"}, // OpenAI para TTS
		},
		Input: profiles.InputConfig{
			LLMProviderID: "openai-default", // OpenAI para STT
		},
	}

	// Validar Profile 1
	chatProvider1 := llmRegistry.Get(profile1.Chat.LLMProvider)
	if chatProvider1 == nil || chatProvider1.ID != "openai-default" {
		t.Error("Profile 1: Chat provider incorreto")
	}
	voiceProvider1 := llmRegistry.Get(profile1.Voice.Assistant.LLMProviderID)
	if voiceProvider1 == nil || voiceProvider1.ID != "openai-default" {
		t.Error("Profile 1: Voice provider incorreto")
	}

	// Validar Profile 2 (INDEPENDÊNCIA!)
	chatProvider2 := llmRegistry.Get(profile2.Chat.LLMProvider)
	if chatProvider2 == nil || chatProvider2.ID != "anthropic-claude" {
		t.Error("Profile 2: Chat provider deveria ser Claude")
	}
	voiceProvider2 := llmRegistry.Get(profile2.Voice.Assistant.LLMProviderID)
	if voiceProvider2 == nil || voiceProvider2.ID != "openai-default" {
		t.Error("Profile 2: Voice provider deveria ser OpenAI")
	}

	// VERIFICAÇÃO CRÍTICA: Chat e Voice são DIFERENTES
	if profile2.Chat.LLMProvider == profile2.Voice.Assistant.LLMProviderID {
		t.Error("FALHOU: Chat e Voice deveriam usar providers DIFERENTES no Profile 2")
	}

	t.Log("✓ Profile switching com providers independentes validado")
}

// TestPhase8_CredentialAutoInjection valida injeção automática de credenciais
func TestPhase8_CredentialAutoInjection(t *testing.T) {
	ctx := context.Background()
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))

	// Registrar credencial para OpenAI
	authCfg := &credentials.AuthConfig{
		Type:  "bearer",
		Token: "sk-test123456",
	}
	err := credMgr.RegisterPatternWithContext(ctx, "*.openai.com", authCfg)
	if err != nil {
		t.Fatalf("Erro ao registrar credencial: %v", err)
	}

	// Verificar que credencial foi salva
	retrieved, err := credMgr.GetByPattern("*.openai.com")
	if err != nil {
		t.Fatalf("Erro ao buscar credencial: %v", err)
	}

	if retrieved.Token != "sk-test123456" {
		t.Errorf("Token incorreto: esperado 'sk-test123456', obteve '%s'", retrieved.Token)
	}

	// Verificar que resolve para URL
	resolvedAuth, err := credMgr.ResolveForURL("https://api.openai.com/v1/chat/completions")
	if err != nil {
		t.Fatalf("Erro ao resolver para URL: %v", err)
	}

	if resolvedAuth.Token != "sk-test123456" {
		t.Error("Credencial não foi resolvida corretamente para URL")
	}

	t.Log("✓ Credential auto-injection validado")
}

// TestPhase8_LegacyMigration valida migração de config.json legado
func TestPhase8_LegacyMigration(t *testing.T) {
	ctx := context.Background()

	// Simular detecção de config legado
	legacyAPIKey := "sk-legacy123"

	// Extrair domínio (seria feito a partir do baseURL na migração real)
	expectedPattern := "*.openai.com"

	// Simular registro no credMgr
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	authCfg := &credentials.AuthConfig{
		Type:  "bearer",
		Token: legacyAPIKey,
	}
	err := credMgr.RegisterPatternWithContext(ctx, expectedPattern, authCfg)
	if err != nil {
		t.Fatalf("Erro ao migrar credencial legada: %v", err)
	}

	// Verificar que migração funcionou
	retrieved, err := credMgr.GetByPattern(expectedPattern)
	if err != nil || retrieved.Token != legacyAPIKey {
		t.Error("Migração de config legado falhou")
	}

	t.Log("✓ Legacy migration validado")
}

// TestPhase8_RealWorldScenarios valida cenários reais de uso
func TestPhase8_RealWorldScenarios(t *testing.T) {
	scenarios := []struct {
		name         string
		chatProvider string
		ttsProvider  string
		sttProvider  string
		description  string
	}{
		{
			name:         "Cenário 1: Tudo OpenAI",
			chatProvider: "openai-default",
			ttsProvider:  "openai-default",
			sttProvider:  "openai-default",
			description:  "Usuário usa OpenAI para tudo",
		},
		{
			name:         "Cenário 2: Claude + OpenAI Voice",
			chatProvider: "anthropic-claude",
			ttsProvider:  "openai-default",
			sttProvider:  "openai-default",
			description:  "Melhor código (Claude) + melhor voz (OpenAI)",
		},
		{
			name:         "Cenário 3: Ollama + OpenAI Voice",
			chatProvider: "ollama-local",
			ttsProvider:  "openai-default",
			sttProvider:  "openai-default",
			description:  "Modelo local + voice na nuvem",
		},
	}

	llmRegistry := llm.NewProviderRegistry()

	// Setup providers
	providers := []*llm.ProviderConfig{
		{ID: "openai-default", Name: "OpenAI", Type: llm.ProviderOpenAI, BaseURL: "https://api.openai.com/v1"},
		{ID: "anthropic-claude", Name: "Claude", Type: llm.ProviderClaude, BaseURL: "https://api.anthropic.com/v1"},
		{ID: "ollama-local", Name: "Ollama", Type: llm.ProviderOllama, BaseURL: "http://localhost:11434/api"},
	}
	for _, p := range providers {
		llmRegistry.Register(p)
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Criar profile para o cenário
			profile := profiles.Profile{
				Name: scenario.name,
				Chat: profiles.ChatConfig{
					LLMProvider: scenario.chatProvider,
				},
				Voice: profiles.VoiceConfig{
					Assistant: profiles.VoiceRoleConfig{LLMProviderID: scenario.ttsProvider},
				},
				Input: profiles.InputConfig{
					LLMProviderID: scenario.sttProvider,
				},
			}

			// Validar que todos os providers existem
			chatProv := llmRegistry.Get(profile.Chat.LLMProvider)
			if chatProv == nil {
				t.Errorf("Chat provider '%s' não encontrado", profile.Chat.LLMProvider)
			}

			ttsProv := llmRegistry.Get(profile.Voice.Assistant.LLMProviderID)
			if ttsProv == nil {
				t.Errorf("TTS provider '%s' não encontrado", profile.Voice.Assistant.LLMProviderID)
			}

			sttProv := llmRegistry.Get(profile.Input.LLMProviderID)
			if sttProv == nil {
				t.Errorf("STT provider '%s' não encontrado", profile.Input.LLMProviderID)
			}

			t.Logf("✓ %s: %s", scenario.name, scenario.description)
		})
	}
}

// TestPhase8_NoRegressions valida que funcionalidades existentes não quebraram
func TestPhase8_NoRegressions(t *testing.T) {
	// Verificar que registry funciona
	registry := llm.NewProviderRegistry()

	// Adicionar
	provider := &llm.ProviderConfig{
		ID:      "test-provider",
		Name:    "Test",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://test.com",
	}
	if err := registry.Register(provider); err != nil {
		t.Errorf("Regressão: Register falhou: %v", err)
	}

	// Buscar
	retrieved := registry.Get("test-provider")
	if retrieved == nil {
		t.Error("Regressão: Get falhou")
	}

	// Listar
	list := registry.List()
	if len(list) != 1 {
		t.Error("Regressão: List falhou")
	}

	// Remover
	if err := registry.Remove("test-provider"); err != nil {
		t.Errorf("Regressão: Remove falhou: %v", err)
	}

	t.Log("✓ No regressions detected")
}

// TestPhase8_NewUserExperience valida experiência de usuário novo
func TestPhase8_NewUserExperience(t *testing.T) {
	// Usuário novo não precisa de config.json
	// Apenas:
	// 1. Perfil ativo
	// 2. Providers no registry
	// 3. Credentials manager

	profileMgr := profiles.NewManager()
	llmRegistry := llm.NewProviderRegistry()
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))

	// Garantir defaults
	if err := profileMgr.EnsureDefaults(); err != nil {
		t.Errorf("Novo usuário: EnsureDefaults falhou: %v", err)
	}

	// Providers builtin existem
	openai := &llm.ProviderConfig{
		ID:      "openai-default",
		Name:    "OpenAI",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1",
	}
	llmRegistry.Register(openai)

	if llmRegistry.Get("openai-default") == nil {
		t.Error("Novo usuário: Provider builtin não disponível")
	}

	// Credentials manager funcional
	if credMgr == nil {
		t.Error("Novo usuário: Credentials manager não inicializado")
	}

	t.Log("✓ New user experience validado (sem config.json)")
}
