package profiles

import (
	"testing"
)

// TestValidateLLMProvider testa se a validação de LLMProvider funciona corretamente
func TestValidateLLMProvider(t *testing.T) {
	tests := []struct {
		name             string
		llmProvider      string
		shouldError      bool
		expectedErrorMsg string
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
					Assistant: VoiceRoleConfig{Provider: "disabled", Rate: 1.0, Pitch: 1.0, Volume: 1.0},
					User:      VoiceRoleConfig{Provider: "disabled", Rate: 1.0, Pitch: 1.0, Volume: 1.0},
					System:    VoiceRoleConfig{Provider: "disabled", Rate: 1.0, Pitch: 1.0, Volume: 1.0},
				},
				Input: InputConfig{
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

func TestValidateHTTPVoiceContract(t *testing.T) {
	tests := []struct {
		name      string
		role      VoiceRoleConfig
		wantError string
	}{
		{
			name: "valid model and voice",
			role: VoiceRoleConfig{
				Enabled:       true,
				Provider:      "openai",
				LLMProviderID: "localai-default",
				Model:         "qwen3-tts-0.6b-custom-voice",
				SelectionMode: "model_and_voice",
				VoiceID:       "Aiden",
				Rate:          1,
				Pitch:         1,
				Volume:        1,
			},
		},
		{
			name: "valid model only",
			role: VoiceRoleConfig{
				Enabled:       true,
				Provider:      "openai",
				LLMProviderID: "localai-default",
				Model:         "voice-pt_BR-cadu-medium",
				SelectionMode: "model_only",
				Rate:          1,
				Pitch:         1,
				Volume:        1,
			},
		},
		{
			name: "missing model",
			role: VoiceRoleConfig{
				Enabled:       true,
				Provider:      "openai",
				LLMProviderID: "localai-default",
				SelectionMode: "model_and_voice",
				VoiceID:       "Aiden",
				Rate:          1,
				Pitch:         1,
				Volume:        1,
			},
			wantError: "voice.assistant.model is required for HTTP TTS",
		},
		{
			name: "missing voice for model and voice",
			role: VoiceRoleConfig{
				Enabled:       true,
				Provider:      "openai",
				LLMProviderID: "localai-default",
				Model:         "qwen3-tts-0.6b-custom-voice",
				SelectionMode: "model_and_voice",
				Rate:          1,
				Pitch:         1,
				Volume:        1,
			},
			wantError: "voice.assistant.voice_id is required when selection_mode is model_and_voice",
		},
		{
			name: "voice set for model only",
			role: VoiceRoleConfig{
				Enabled:       true,
				Provider:      "openai",
				LLMProviderID: "localai-default",
				Model:         "voice-pt_BR-cadu-medium",
				SelectionMode: "model_only",
				VoiceID:       "Aiden",
				Rate:          1,
				Pitch:         1,
				Volume:        1,
			},
			wantError: "voice.assistant.voice_id must be empty when selection_mode is model_only",
		},
		{
			name: "invalid selection mode",
			role: VoiceRoleConfig{
				Enabled:       true,
				Provider:      "openai",
				LLMProviderID: "localai-default",
				Model:         "tts-1",
				SelectionMode: "legacy",
				VoiceID:       "alloy",
				Rate:          1,
				Pitch:         1,
				Volume:        1,
			},
			wantError: "voice.assistant.selection_mode must be one of: model_and_voice, model_only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validTestProfile()
			p.Voice.Assistant = tt.role

			err := p.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() expected error %q, got nil", tt.wantError)
			}
			if err.Error() != tt.wantError {
				t.Fatalf("Validate() error = %q, want %q", err.Error(), tt.wantError)
			}
		})
	}
}

func validTestProfile() *Profile {
	return &Profile{
		Name: "Test",
		Chat: ChatConfig{
			LLMProvider:     "openai-default",
			Temperature:     0.7,
			MaxTokens:       4096,
			TopP:            1.0,
			ResponseTimeout: 30,
		},
		Voice: VoiceConfig{
			Assistant: VoiceRoleConfig{Provider: "disabled", Rate: 1.0, Pitch: 1.0, Volume: 1.0},
			User:      VoiceRoleConfig{Provider: "disabled", Rate: 1.0, Pitch: 1.0, Volume: 1.0},
			System:    VoiceRoleConfig{Provider: "disabled", Rate: 1.0, Pitch: 1.0, Volume: 1.0},
		},
		Input: InputConfig{
			STTProvider: "webspeech",
			Language:    "pt-BR",
		},
	}
}
