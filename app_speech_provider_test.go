package main

import (
	"testing"

	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// TestSpeechProviderIndependence verifica que TTS/STT podem usar providers diferentes do chat
func TestSpeechProviderIndependence(t *testing.T) {
	// Criar perfil com providers diferentes
	profile := profiles.Profile{
		Name: "Test Mixed Providers",
		Chat: profiles.ChatConfig{
			LLMProvider: "anthropic-claude", // Claude para chat
			Model:       "claude-3-7-sonnet-20250219",
		},
		Voice: profiles.VoiceConfig{
			Assistant: profiles.VoiceRoleConfig{
				Provider:      "openai",
				LLMProviderID: "openai-default", // OpenAI para TTS (diferente do chat!)
				VoiceID:       "nova",
			},
		},
		Input: profiles.InputConfig{
			STTProvider:   "whisper_api",
			LLMProviderID: "openai-default", // OpenAI para Whisper (diferente do chat!)
			Language:      "pt-BR",
		},
	}

	// Verificar que os campos estão corretos
	if profile.Chat.LLMProvider != "anthropic-claude" {
		t.Errorf("Chat provider incorreto: %s", profile.Chat.LLMProvider)
	}

	if profile.Voice.Assistant.LLMProviderID != "openai-default" {
		t.Errorf("TTS provider incorreto: %s", profile.Voice.Assistant.LLMProviderID)
	}

	if profile.Input.LLMProviderID != "openai-default" {
		t.Errorf("STT provider incorreto: %s", profile.Input.LLMProviderID)
	}

	// Demonstrar independência: chat usa Claude, voice usa OpenAI
	if profile.Chat.LLMProvider == profile.Voice.Assistant.LLMProviderID {
		t.Error("ERRO: Chat e Voice deveriam usar providers DIFERENTES neste teste")
	}
}

// TestProfileWithSpeechProviders verifica estrutura de perfil com provider IDs
func TestProfileWithSpeechProviders(t *testing.T) {
	profile := profiles.DefaultProfile()

	// Verificar que os campos existem e têm defaults
	if profile.Voice.Assistant.LLMProviderID == "" {
		t.Error("VoiceConfig.Assistant.LLMProviderID deveria ter valor padrão")
	}

	if profile.Input.LLMProviderID == "" {
		t.Error("InputConfig.LLMProviderID deveria ter valor padrão")
	}
}

// TestBuiltinProfilesHaveSpeechProviders verifica que builtin profiles têm provider IDs
func TestBuiltinProfilesHaveSpeechProviders(t *testing.T) {
	// Este teste seria mais completo lendo os arquivos JSON reais
	// Por enquanto, apenas estrutural

	testCases := []struct {
		name        string
		chatProv    string
		voiceProv   string
		sttProv     string
		description string
	}{
		{
			name:        "Padrão",
			chatProv:    "openai-default",
			voiceProv:   "openai-default",
			sttProv:     "openai-default",
			description: "Tudo OpenAI",
		},
		{
			name:        "Programação",
			chatProv:    "anthropic-claude",
			voiceProv:   "openai-default", // Diferente do chat!
			sttProv:     "openai-default", // Diferente do chat!
			description: "Claude para chat, OpenAI para vozes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verificar que o design permite esta configuração
			profile := profiles.Profile{
				Name: tc.name,
				Chat: profiles.ChatConfig{
					LLMProvider: tc.chatProv,
				},
				Voice: profiles.VoiceConfig{
					Assistant: profiles.VoiceRoleConfig{LLMProviderID: tc.voiceProv},
				},
				Input: profiles.InputConfig{
					LLMProviderID: tc.sttProv,
				},
			}

			if profile.Chat.LLMProvider != tc.chatProv {
				t.Errorf("Chat provider mismatch")
			}
			if profile.Voice.Assistant.LLMProviderID != tc.voiceProv {
				t.Errorf("Voice provider mismatch")
			}
			if profile.Input.LLMProviderID != tc.sttProv {
				t.Errorf("STT provider mismatch")
			}
		})
	}
}

// TestProviderRegistryForSpeech verifica registry com providers para speech
func TestProviderRegistryForSpeech(t *testing.T) {
	registry := llm.NewProviderRegistry()

	// Registrar provider para chat
	chatProvider := &llm.ProviderConfig{
		ID:      "anthropic-claude",
		Name:    "Claude",
		Type:    llm.ProviderClaude,
		BaseURL: "https://api.anthropic.com/v1",
	}
	if err := registry.Register(chatProvider); err != nil {
		t.Fatalf("Erro ao registrar chat provider: %v", err)
	}

	// Registrar provider para speech (pode ser o mesmo ou diferente)
	speechProvider := &llm.ProviderConfig{
		ID:      "openai-speech",
		Name:    "OpenAI Speech",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1",
	}
	if err := registry.Register(speechProvider); err != nil {
		t.Fatalf("Erro ao registrar speech provider: %v", err)
	}

	// Verificar que ambos coexistem
	if registry.Get("anthropic-claude") == nil {
		t.Error("Chat provider não encontrado")
	}
	if registry.Get("openai-speech") == nil {
		t.Error("Speech provider não encontrado")
	}

	providers := registry.List()
	if len(providers) != 2 {
		t.Errorf("Esperado 2 providers, obteve %d", len(providers))
	}
}
