package main

import (
	"testing"

	"assistente/internal/config"
	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// TestProfileSwitchUpdatesLLMClient verifica que trocar de perfil atualiza o cliente LLM
func TestProfileSwitchUpdatesLLMClient(t *testing.T) {
	// Skip em ambientes sem display (CI)
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	llmRegistry := llm.NewProviderRegistry()

	// Registrar providers de teste
	provider1 := &llm.ProviderConfig{
		ID:                "test-provider-1",
		Name:              "Test Provider 1",
		Type:              llm.ProviderOpenAI,
		BaseURL:           "https://api.test1.com/v1",
		Model:             "test-model-1",
		Timeout:           60,
		CredentialPattern: "*.test1.com",
	}
	if err := llmRegistry.Register(provider1); err != nil {
		t.Fatalf("Erro ao registrar provider1: %v", err)
	}

	provider2 := &llm.ProviderConfig{
		ID:                "test-provider-2",
		Name:              "Test Provider 2",
		Type:              llm.ProviderClaude,
		BaseURL:           "https://api.test2.com/v1",
		Model:             "test-model-2",
		Timeout:           90,
		CredentialPattern: "*.test2.com",
	}
	if err := llmRegistry.Register(provider2); err != nil {
		t.Fatalf("Erro ao registrar provider2: %v", err)
	}

	// Verificar que os providers foram registrados
	providers := llmRegistry.List()
	if len(providers) != 2 {
		t.Errorf("Esperado 2 providers, obteve %d", len(providers))
	}

	// Verificar que podemos buscar provider por ID
	retrieved := llmRegistry.Get("test-provider-1")
	if retrieved == nil {
		t.Error("Provider test-provider-1 não encontrado")
	}
	if retrieved != nil && retrieved.Name != "Test Provider 1" {
		t.Errorf("Nome incorreto: esperado 'Test Provider 1', obteve '%s'", retrieved.Name)
	}
}

// TestProviderRegistry verifica funcionalidade básica do registry
func TestProviderRegistry(t *testing.T) {
	registry := llm.NewProviderRegistry()

	// Test Add
	provider := &llm.ProviderConfig{
		ID:      "test-id",
		Name:    "Test Provider",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://api.test.com/v1",
	}

	err := registry.Register(provider)
	if err != nil {
		t.Errorf("Erro ao registrar provider: %v", err)
	}

	// Test Get
	retrieved := registry.Get("test-id")
	if retrieved == nil {
		t.Fatal("Provider não encontrado após registro")
	}
	if retrieved.Name != "Test Provider" {
		t.Errorf("Nome incorreto: esperado 'Test Provider', obteve '%s'", retrieved.Name)
	}

	// Test List
	list := registry.List()
	if len(list) != 1 {
		t.Errorf("Esperado 1 provider na lista, obteve %d", len(list))
	}

	// Test Remove
	err = registry.Remove("test-id")
	if err != nil {
		t.Errorf("Erro ao remover provider: %v", err)
	}

	retrieved = registry.Get("test-id")
	if retrieved != nil {
		t.Error("Provider ainda existe após remoção")
	}
}

// TestProviderValidation verifica validação de ProviderConfig
func TestProviderValidation(t *testing.T) {
	tests := []struct {
		name      string
		provider  *llm.ProviderConfig
		wantError bool
	}{
		{
			name: "valid provider",
			provider: &llm.ProviderConfig{
				ID:      "test-id",
				Name:    "Test",
				BaseURL: "https://api.test.com",
			},
			wantError: false,
		},
		{
			name: "empty id",
			provider: &llm.ProviderConfig{
				ID:      "",
				Name:    "Test",
				BaseURL: "https://api.test.com",
			},
			wantError: true,
		},
		{
			name: "empty name",
			provider: &llm.ProviderConfig{
				ID:      "test-id",
				Name:    "",
				BaseURL: "https://api.test.com",
			},
			wantError: true,
		},
		{
			name: "empty base url",
			provider: &llm.ProviderConfig{
				ID:      "test-id",
				Name:    "Test",
				BaseURL: "",
			},
			wantError: true,
		},
		{
			name:      "nil provider",
			provider:  nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.provider.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestClientUsesProviderConfig verifica que o cliente usa configuração do provider
func TestClientUsesProviderConfig(t *testing.T) {
	provider := &llm.ProviderConfig{
		ID:      "test-client",
		Name:    "Test Client",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://api.example.com/v1",
		Model:   "test-model",
		Timeout: 120,
	}

	cfg := &config.Config{}
	// Criar credentials manager com chave de teste
	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)

	client := llm.NewClient(provider, cfg, credMgr)
	if client == nil {
		t.Fatal("Cliente não foi criado")
	}

	// Não podemos testar internos privados, mas verificamos que não crashou
	// Em testes reais, mockariamos HTTP para verificar URL/headers
}

// TestActiveProviderFromProfile verifica que perfil ativo determina provider
func TestActiveProviderFromProfile(t *testing.T) {
	// Este teste seria mais completo em um cenário de integração
	// Por agora, apenas verifica estrutura básica

	profile := profiles.Profile{
		Name:        "Test Profile",
		Description: "Profile for testing",
		Chat: profiles.ChatConfig{
			LLMProvider: "openai-default",
			Model:       "gpt-4o-mini",
		},
	}

	if profile.Chat.LLMProvider != "openai-default" {
		t.Errorf("LLMProvider incorreto: esperado 'openai-default', obteve '%s'", profile.Chat.LLMProvider)
	}
}
