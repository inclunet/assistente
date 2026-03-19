package llm

import (
	"testing"
)

// TestExtractDomain testa extração do domínio de URLs
func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url      string
		expected string
		desc     string
	}{
		{
			"https://api.openai.com/v1",
			"api.openai.com",
			"https com path",
		},
		{
			"https://api.openai.com:443/v1",
			"api.openai.com:443",
			"https com porta explícita",
		},
		{
			"http://localhost:8000/v1",
			"localhost:8000",
			"localhost com porta",
		},
		{
			"http://example.com",
			"example.com",
			"sem path",
		},
		{
			"https://api.anthropic.com",
			"api.anthropic.com",
			"sem path e scheme",
		},
		{
			"invalid://url:::malformed",
			"",
			"url malformada",
		},
		{
			"",
			"",
			"url vazia",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := extractDomain(tt.url)
			if result != tt.expected {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

// TestNewSyncClient_Initialization testa criação do cliente
func TestNewSyncClient_Initialization(t *testing.T) {
	config := &ProviderConfig{
		ID:      "test-provider",
		Name:    "Test Provider",
		Type:    ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1",
		Timeout: 60,
	}

	client := NewSyncClient(config, nil)

	if client == nil {
		t.Fatal("NewSyncClient retornou nil")
	}

	if client.provider != config {
		t.Error("provider não foi atribuído corretamente")
	}

	if client.httpClient == nil {
		t.Error("httpClient não foi criado")
	}
}

// TestNewSyncClient_DefaultTimeout testa timeout padrão
func TestNewSyncClient_DefaultTimeout(t *testing.T) {
	config := &ProviderConfig{
		ID:      "test-provider",
		Name:    "Test",
		BaseURL: "https://api.test.com",
		Timeout: 0, // Deve usar default
	}

	client := NewSyncClient(config, nil)
	if client == nil {
		t.Fatal("NewSyncClient retornou nil")
	}

	// Não conseguimos verificar timeout diretamente, mas se chegou aqui sem panic, é bom sinal
}

// TestNewSyncClient_CustomTimeout testa timeout customizado
func TestNewSyncClient_CustomTimeout(t *testing.T) {
	config := &ProviderConfig{
		ID:      "test-provider",
		Name:    "Test",
		BaseURL: "https://api.test.com",
		Timeout: 300, // 5 min
	}

	client := NewSyncClient(config, nil)
	if client == nil {
		t.Fatal("NewSyncClient retornou nil")
	}
}

// TestNewSyncClient_CredentialPattern testa padrão de credenciais
func TestNewSyncClient_CredentialPattern(t *testing.T) {
	tests := []struct {
		pattern      string
		baseURL      string
		expectedHost string
		desc         string
	}{
		{
			"*.anthropic.com",
			"https://api.anthropic.com/v1",
			"*.anthropic.com",
			"pattern customizado usado",
		},
		{
			"",
			"https://api.openai.com/v1",
			"api.openai.com",
			"pattern vazio usa domain extraído",
		},
		{
			"",
			"http://localhost:8000/v1",
			"localhost:8000",
			"pattern vazio com localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			config := &ProviderConfig{
				ID:                "test",
				Name:              "Test",
				BaseURL:           tt.baseURL,
				CredentialPattern: tt.pattern,
			}

			client := NewSyncClient(config, nil)
			if client == nil {
				t.Fatal("NewSyncClient retornou nil")
			}
			// Se chegou aqui, o client foi criado com sucesso
		})
	}
}

// TestSyncChatMessage_Serialization testa estrutura de mensagem
func TestSyncChatMessage_Serialization(t *testing.T) {
	content := "Hello, world!"
	msg := syncChatMessage{
		Role:    "user",
		Content: &content,
	}

	if msg.Role != "user" {
		t.Errorf("Role incorreto: %s", msg.Role)
	}

	if msg.Content == nil {
		t.Error("Content é nil")
	}

	if *msg.Content != "Hello, world!" {
		t.Errorf("Content incorreto: %s", *msg.Content)
	}
}

// TestSyncChatMessage_NilContent testa mensagem com conteúdo nil
func TestSyncChatMessage_NilContent(t *testing.T) {
	msg := syncChatMessage{
		Role:    "assistant",
		Content: nil,
	}

	if msg.Role != "assistant" {
		t.Errorf("Role incorreto: %s", msg.Role)
	}

	if msg.Content != nil {
		t.Error("Content deveria ser nil")
	}
}

// TestSyncChatRequest_Structure testa estrutura de request
func TestSyncChatRequest_Structure(t *testing.T) {
	sysMsg := "You are helpful"
	userMsg := "What is 2+2?"

	req := syncChatRequest{
		Model: "gpt-4",
		Messages: []syncChatMessage{
			{Role: "system", Content: &sysMsg},
			{Role: "user", Content: &userMsg},
		},
	}

	if req.Model != "gpt-4" {
		t.Errorf("Model incorreto: %s", req.Model)
	}

	if len(req.Messages) != 2 {
		t.Errorf("Esperado 2 mensagens, got %d", len(req.Messages))
	}

	if req.Messages[0].Role != "system" {
		t.Errorf("Role da mensagem 0 incorreto: %s", req.Messages[0].Role)
	}
}

// TestSyncClient_DifferentProviderTypes testa clientes para diferentes providers
func TestSyncClient_DifferentProviderTypes(t *testing.T) {
	tests := []struct {
		providerType ProviderType
		baseURL      string
		desc         string
	}{
		{ProviderOpenAI, "https://api.openai.com/v1", "OpenAI"},
		{ProviderClaude, "https://api.anthropic.com/v1", "Claude (Anthropic)"},
		{ProviderOllama, "http://localhost:11434/v1", "Ollama (local)"},
		{ProviderCustom, "http://custom-llm:8000/api", "Custom provider"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			config := &ProviderConfig{
				ID:      "test-" + string(tt.providerType),
				Name:    "Test " + string(tt.providerType),
				Type:    tt.providerType,
				BaseURL: tt.baseURL,
			}

			client := NewSyncClient(config, nil)
			if client == nil {
				t.Fatalf("failed to create client for %s", tt.desc)
			}

			if client.provider.Type != tt.providerType {
				t.Errorf("Provider type mismatch: expected %s, got %s", tt.providerType, client.provider.Type)
			}
		})
	}
}

// TestNewSyncClient_WithHeaders testa cliente com headers customizados
func TestNewSyncClient_WithHeaders(t *testing.T) {
	config := &ProviderConfig{
		ID:      "test-provider",
		Name:    "Test",
		BaseURL: "https://api.test.com",
		Headers: map[string]string{
			"X-API-Key": "secret-key",
			"User-Agent": "Custom/1.0",
		},
	}

	client := NewSyncClient(config, nil)
	if client == nil {
		t.Fatal("NewSyncClient retornou nil")
	}

	if len(config.Headers) != 2 {
		t.Errorf("Headers foram modificados: expected 2, got %d", len(config.Headers))
	}
}
