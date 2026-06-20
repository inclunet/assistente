package llm

import (
	"strings"
	"testing"
)

// TestProviderConfigValidate_Valid testa validação com configs válidas
func TestProviderConfigValidate_Valid(t *testing.T) {
	tests := []struct {
		name   string
		config *ProviderConfig
	}{
		{
			"openai_standard",
			&ProviderConfig{
				ID:      "openai-gpt4",
				Name:    "OpenAI GPT-4",
				Type:    ProviderOpenAI,
				BaseURL: "https://api.openai.com/v1",
				Model:   "gpt-4",
			},
		},
		{
			"claude_with_credentials",
			&ProviderConfig{
				ID:                "anthropic-claude",
				Name:              "Anthropic Claude",
				Type:              ProviderClaude,
				BaseURL:           "https://api.anthropic.com/v1",
				CredentialPattern: "*.anthropic.com",
			},
		},
		{
			"custom_with_headers",
			&ProviderConfig{
				ID:      "custom-local",
				Name:    "Custom Local LLM",
				Type:    ProviderCustom,
				BaseURL: "http://localhost:8000/v1",
				Headers: map[string]string{
					"X-API-Key": "local-key",
				},
			},
		},
		{
			"with_timeout",
			&ProviderConfig{
				ID:      "slow-provider",
				Name:    "Slow Provider",
				Type:    ProviderDeepSeek,
				BaseURL: "https://api.deepseek.com/v1",
				Timeout: 300, // 5 min
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != nil {
				t.Errorf("Validate: esperado sucesso, got erro: %v", err)
			}
		})
	}
}

// TestProviderConfigValidate_Invalid testa rejeição de configs inválidas
func TestProviderConfigValidate_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		config *ProviderConfig
		errMsg string
	}{
		{
			"nil_config",
			nil,
			"provider config nil",
		},
		{
			"empty_id",
			&ProviderConfig{
				ID:      "    ",
				Name:    "Test",
				BaseURL: "https://api.test.com",
			},
			"provider id vazio",
		},
		{
			"empty_name",
			&ProviderConfig{
				ID:      "test-provider",
				Name:    "   ",
				BaseURL: "https://api.test.com",
			},
			"provider name vazio",
		},
		{
			"empty_baseurl",
			&ProviderConfig{
				ID:      "test-provider",
				Name:    "Test Provider",
				BaseURL: "   ",
			},
			"provider base_url vazio",
		},
		{
			"all_empty",
			&ProviderConfig{
				ID:      "",
				Name:    "",
				BaseURL: "",
			},
			"provider id vazio",
		},
		{
			"missing_id",
			&ProviderConfig{
				Name:    "Test",
				BaseURL: "https://api.test.com",
			},
			"provider id vazio",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil {
				t.Errorf("Validate: esperado erro, got nil")
			}
			if err != nil && !contains(err.Error(), tt.errMsg) {
				t.Errorf("Validate: esperado erro contendo %q, got %q", tt.errMsg, err.Error())
			}
		})
	}
}

// TestProviderConfigValidate_Trimming testa que espaços em branco são removidos
func TestProviderConfigValidate_Trimming(t *testing.T) {
	config := &ProviderConfig{
		ID:                "  provider-id  ",
		Name:              "  Provider Name  ",
		BaseURL:           "  https://api.example.com/v1  ",
		CredentialPattern: "  *.example.com  ",
	}

	err := config.Validate()
	if err != nil {
		t.Fatalf("Validate retornou erro: %v", err)
	}

	if config.ID != "provider-id" {
		t.Errorf("ID não foi trimmed: %q", config.ID)
	}
	if config.Name != "Provider Name" {
		t.Errorf("Name não foi trimmed: %q", config.Name)
	}
	if config.BaseURL != "https://api.example.com/v1" {
		t.Errorf("BaseURL não foi trimmed: %q", config.BaseURL)
	}
	if config.CredentialPattern != "*.example.com" {
		t.Errorf("CredentialPattern não foi trimmed: %q", config.CredentialPattern)
	}
}

// TestProviderType_Constants verifica que os tipos de provedor são bem-formados
func TestProviderType_Constants(t *testing.T) {
	types := []ProviderType{
		ProviderOpenAI,
		ProviderClaude,
		ProviderGrok,
		ProviderDeepSeek,
		ProviderMistral,
		ProviderGroq,
		ProviderTogether,
		ProviderFireworks,
		ProviderPerplexity,
		ProviderOllama,
		ProviderCustom,
	}

	// Todos devem ser strings não-vazias
	for _, pt := range types {
		if pt == "" {
			t.Error("ProviderType não deve ser vazio")
		}
		// Verificar que é lowercase (padrão para serialização)
		if string(pt) != strings.ToLower(string(pt)) {
			t.Errorf("ProviderType %q deve ser lowercase", pt)
		}
	}

	// Verificar unicidade
	seen := make(map[string]bool)
	for _, pt := range types {
		key := string(pt)
		if seen[key] {
			t.Errorf("ProviderType duplicado: %q", key)
		}
		seen[key] = true
	}
}

// TestProviderConfig_Timeout testa validação de timeout
func TestProviderConfig_Timeout(t *testing.T) {
	tests := []struct {
		timeout int
		desc    string
	}{
		{0, "zero timeout (default será usado)"},
		{30, "30 segundos"},
		{300, "5 minutos"},
		{3600, "1 hora"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			config := &ProviderConfig{
				ID:      "test-provider",
				Name:    "Test",
				BaseURL: "https://api.test.com",
				Timeout: tt.timeout,
			}

			err := config.Validate()
			if err != nil {
				t.Errorf("Timeout %d: esperado sucesso, got erro: %v", tt.timeout, err)
			}
		})
	}
}

// TestProviderConfig_Headers testa que headers customizados são preservados
func TestProviderConfig_Headers(t *testing.T) {
	config := &ProviderConfig{
		ID:      "test-provider",
		Name:    "Test",
		BaseURL: "https://api.test.com",
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"X-Custom":      "value",
			"Accept":        "application/json",
		},
	}

	err := config.Validate()
	if err != nil {
		t.Fatalf("Validate retornou erro: %v", err)
	}

	// Headers não devem ser modificados
	if len(config.Headers) != 3 {
		t.Errorf("Headers foram modificados: esperado 3, got %d", len(config.Headers))
	}

	if config.Headers["X-Custom"] != "value" {
		t.Errorf("Header customizado foi perdido")
	}
}

// TestProviderConfig_NilHeaders testa que nil headers é aceitável
func TestProviderConfig_NilHeaders(t *testing.T) {
	config := &ProviderConfig{
		ID:      "test-provider",
		Name:    "Test Provider",
		BaseURL: "https://api.test.com",
		Headers: nil,
	}

	err := config.Validate()
	if err != nil {
		t.Errorf("Config com nil headers deve ser válido: %v", err)
	}
}

// TestProviderConfig_Model testa que Model é opcional
func TestProviderConfig_Model(t *testing.T) {
	configWithModel := &ProviderConfig{
		ID:      "with-model",
		Name:    "Test",
		BaseURL: "https://api.test.com",
		Model:   "gpt-4",
	}

	configWithoutModel := &ProviderConfig{
		ID:      "without-model",
		Name:    "Test",
		BaseURL: "https://api.test.com",
	}

	if err := configWithModel.Validate(); err != nil {
		t.Errorf("Config com Model deve ser válido: %v", err)
	}

	if err := configWithoutModel.Validate(); err != nil {
		t.Errorf("Config sem Model deve ser válido: %v", err)
	}
}

// TestProviderConfig_BaseURLFormats testa diferentes formatos de BaseURL
func TestProviderConfig_BaseURLFormats(t *testing.T) {
	tests := []struct {
		baseurl string
		desc    string
		valid   bool
	}{
		{"https://api.openai.com/v1", "https com path", true},
		{"http://localhost:8000", "http localhost", true},
		{"http://localhost:8000/v1", "http localhost com path", true},
		{"https://custom-domain.example.com:9000/api/v1", "com porta customizada", true},
		{"", "vazio", false},
		{"   ", "só espaços", false},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			config := &ProviderConfig{
				ID:      "test",
				Name:    "Test",
				BaseURL: tt.baseurl,
			}

			err := config.Validate()
			if tt.valid && err != nil {
				t.Errorf("BaseURL %q: esperado válido, got erro: %v", tt.baseurl, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("BaseURL %q: esperado inválido, got sucesso", tt.baseurl)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestProviderConfig_SupportsTTS verifica SupportsTTS por APIFormat
func TestProviderConfig_SupportsTTS(t *testing.T) {
	tests := []struct {
		name      string
		apiFormat APIFormat
		baseURL   string
		want      bool
	}{
		{"openai explicit", APIFormatOpenAI, "https://litellm.example.com/v1", true},
		{"openai_responses explicit", APIFormatOpenAIResponses, "https://api.openai.com/v1", true},
		{"anthropic", APIFormatAnthropic, "https://api.anthropic.com/v1", false},
		{"google", APIFormatGoogle, "https://generativelanguage.googleapis.com/v1", false},
		{"empty inferred openai", "", "https://litellm.example.com/v1", true},
		{"empty inferred openai_responses", "", "https://api.openai.com/v1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ProviderConfig{APIFormat: tt.apiFormat, BaseURL: tt.baseURL}
			if got := p.SupportsTTS(); got != tt.want {
				t.Errorf("SupportsTTS() = %v, want %v (apiFormat=%q, baseURL=%q)", got, tt.want, tt.apiFormat, tt.baseURL)
			}
		})
	}
}

// TestProviderConfig_SupportsSTT verifica SupportsSTT por APIFormat
func TestProviderConfig_SupportsSTT(t *testing.T) {
	tests := []struct {
		name      string
		apiFormat APIFormat
		want      bool
	}{
		{"openai", APIFormatOpenAI, true},
		{"openai_responses", APIFormatOpenAIResponses, true},
		{"anthropic", APIFormatAnthropic, false},
		{"google", APIFormatGoogle, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ProviderConfig{APIFormat: tt.apiFormat, BaseURL: "https://example.com"}
			if got := p.SupportsSTT(); got != tt.want {
				t.Errorf("SupportsSTT() = %v, want %v (apiFormat=%q)", got, tt.want, tt.apiFormat)
			}
		})
	}
}

func TestProviderConfig_SupportsExplicitCacheControl(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ProviderConfig
		want bool
	}{
		{
			name: "anthropic official",
			cfg:  &ProviderConfig{APIFormat: APIFormatAnthropic, BaseURL: "https://api.anthropic.com/v1"},
			want: true,
		},
		{
			name: "anthropic proxy no capability",
			cfg:  &ProviderConfig{APIFormat: APIFormatAnthropic, BaseURL: "https://litellm.example.com/anthropic"},
		},
		{
			name: "openai compatible uses provider hints instead",
			cfg:  &ProviderConfig{APIFormat: APIFormatOpenAI, BaseURL: "https://api.openai.com/v1"},
		},
		{
			name: "google cachedContent needs lifecycle",
			cfg:  &ProviderConfig{APIFormat: APIFormatGoogle, BaseURL: "https://generativelanguage.googleapis.com/v1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsExplicitCacheControl(tt.cfg); got != tt.want {
				t.Fatalf("SupportsExplicitCacheControl() = %v, want %v", got, tt.want)
			}
		})
	}
}
