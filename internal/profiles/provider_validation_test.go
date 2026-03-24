package profiles

import (
	"testing"
)

// TestValidateLLMProvider testa se a validação de LLMProvider funciona corretamente
func TestValidateLLMProvider(t *testing.T) {
	tests := []struct {
		name              string
		llmProvider       string
		shouldError       bool
		expectedErrorMsg  string
	}{
		{
			name:        "valid: openai-default provider",
			llmProvider: "openai-default",
			shouldError: false,
		},
		{
			name:        "valid: anthropic-claude provider",
			llmProvider: "anthropic-claude",
			shouldError: false,
		},
		{
			name:        "valid: ollama-local provider",
			llmProvider: "ollama-local",
			shouldError: false,
		},
		{
			name:        "valid: custom provider ID",
			llmProvider: "my-custom-provider",
			shouldError: false,
		},
		{
			name:        "valid: $default sentinel",
			llmProvider: DefaultProviderSentinel,
			shouldError: false,
		},
		{
			name:             "invalid: empty provider",
			llmProvider:      "",
			shouldError:      true,
			expectedErrorMsg: "chat.llm_provider is required (ex: 'openai-default' or '$default')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Profile{
				Name: "Test",
				Chat: ChatConfig{
					LLMProvider:     tt.llmProvider,
					Temperature:     0.7,
					MaxTokens:       4096,
					TopP:            1.0,
					ResponseTimeout: 30,
				},
				Voice: VoiceConfig{
					Provider: "disabled",
					Rate:     1.0,
					Pitch:    1.0,
					Volume:   1.0,
				},
				Interaction: InteractionConfig{
					STTProvider: "webspeech",
					Language:    "pt-BR",
				},
			}

			err := p.Validate()

			if tt.shouldError && err == nil {
				t.Error("expected error but got none")
			} else if !tt.shouldError && err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if tt.shouldError && err != nil && tt.expectedErrorMsg != "" {
				if tt.expectedErrorMsg != err.Error() {
					t.Errorf("expected error message '%s', got '%s'", tt.expectedErrorMsg, err.Error())
				}
			}
		})
	}
}

// TestDefaultProfileHasLLMProvider verifica se o perfil padrão possui um provedor LLM
func TestDefaultProfileHasLLMProvider(t *testing.T) {
	p := DefaultProfile()

	if p.Chat.LLMProvider == "" {
		t.Error("DefaultProfile should have a non-empty LLMProvider")
	}

	// Verifica se o perfil padrão é válido
	if err := p.Validate(); err != nil {
		t.Errorf("DefaultProfile should be valid, but got error: %v", err)
	}
}
