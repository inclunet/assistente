package main

import (
	"testing"

	"assistente/internal/profiles"
)

func TestEffectiveVoiceProviderID(t *testing.T) {
	tests := []struct {
		name string
		cfg  profiles.VoiceRoleConfig
		want string
	}{
		{
			name: "webspeech stays local",
			cfg:  profiles.VoiceRoleConfig{Provider: "webspeech", LLMProviderID: "openai-default"},
			want: "webspeech",
		},
		{
			name: "sapi5 stays local",
			cfg:  profiles.VoiceRoleConfig{Provider: "sapi5", LLMProviderID: "openai-default"},
			want: "sapi5",
		},
		{
			name: "llm provider prefers effective registry id",
			cfg:  profiles.VoiceRoleConfig{Provider: "openai", LLMProviderID: "openai-default"},
			want: "openai-default",
		},
		{
			name: "fallbacks to provider when llm id missing",
			cfg:  profiles.VoiceRoleConfig{Provider: "openai"},
			want: "openai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveVoiceProviderID(tt.cfg); got != tt.want {
				t.Fatalf("effectiveVoiceProviderID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildChatSpeakEventUsesBackendAudioForRemoteAssistantVoice(t *testing.T) {
	app := &App{}
	req := ChatSpeakRequest{
		ConversationID: 7,
		MessageID:      9,
		Role:           "assistant",
		Text:           "**Olá**",
		Origin:         ChatSpeakOriginAssistantMessage,
	}
	cfg := profiles.VoiceRoleConfig{
		Enabled:       true,
		Provider:      "openai",
		LLMProviderID: "openai-default",
		VoiceID:       "nova",
		Model:         "tts-1",
		Rate:          1.25,
		Pitch:         1.1,
		Volume:        0.8,
	}

	event := app.buildChatSpeakEvent(req, "assistant", "Olá", cfg)

	if event.ConversationID != 7 {
		t.Fatalf("conversationId = %d, want 7", event.ConversationID)
	}
	if event.MessageID != 9 {
		t.Fatalf("messageId = %d, want 9", event.MessageID)
	}
	if event.Role != "assistant" {
		t.Fatalf("role = %q, want assistant", event.Role)
	}
	if event.Text != "Olá" {
		t.Fatalf("text = %q, want Olá", event.Text)
	}
	if event.Strategy != ChatSpeakStrategyBackendAudio {
		t.Fatalf("strategy = %q, want %q", event.Strategy, ChatSpeakStrategyBackendAudio)
	}
	if event.FallbackStrategy != ChatSpeakStrategyAnnounce {
		t.Fatalf("fallbackStrategy = %q, want %q", event.FallbackStrategy, ChatSpeakStrategyAnnounce)
	}
	if !event.AutoRead {
		t.Fatal("autoRead = false, want true (maps to Enabled)")
	}
	if event.ProviderID != "openai-default" {
		t.Fatalf("providerId = %q, want openai-default", event.ProviderID)
	}
	if event.VoiceID != "nova" {
		t.Fatalf("voiceId = %q, want nova", event.VoiceID)
	}
	if event.Model != "tts-1" {
		t.Fatalf("model = %q, want tts-1", event.Model)
	}
	if event.Rate != 1.25 {
		t.Fatalf("rate = %v, want 1.25", event.Rate)
	}
	if event.Pitch != 1.1 {
		t.Fatalf("pitch = %v, want 1.1", event.Pitch)
	}
	if event.Volume != 0.8 {
		t.Fatalf("volume = %v, want 0.8", event.Volume)
	}
}

func TestBuildChatSpeakEventUsesAnnounceWhenDisabled(t *testing.T) {
	app := &App{}
	req := ChatSpeakRequest{
		ConversationID: 12,
		Role:           "user",
		Text:           "Oi",
		Origin:         ChatSpeakOriginUserMessage,
	}
	cfg := profiles.VoiceRoleConfig{
		Enabled:       false,
		Provider:      "openai",
		LLMProviderID: "openai-default",
		VoiceID:       "alloy",
	}

	event := app.buildChatSpeakEvent(req, "user", "Oi", cfg)

	if event.Strategy != ChatSpeakStrategyNone {
		t.Fatalf("strategy = %q, want %q", event.Strategy, ChatSpeakStrategyNone)
	}
	if event.AutoRead {
		t.Fatal("autoRead = true, want false")
	}
}

func TestBuildChatSpeakEventUsesLocalStrategies(t *testing.T) {
	app := &App{}

	webspeechEvent := app.buildChatSpeakEvent(
		ChatSpeakRequest{ConversationID: 1, Role: "system", Text: "pensando", Origin: ChatSpeakOriginThinking},
		"system",
		"pensando",
		profiles.VoiceRoleConfig{Enabled: true, Provider: "webspeech", VoiceID: "pt-BR"},
	)
	if webspeechEvent.Strategy != ChatSpeakStrategyWebSpeech {
		t.Fatalf("webspeech strategy = %q, want %q", webspeechEvent.Strategy, ChatSpeakStrategyWebSpeech)
	}

	sapiEvent := app.buildChatSpeakEvent(
		ChatSpeakRequest{ConversationID: 1, Role: "system", Text: "pensando", Origin: ChatSpeakOriginThinking},
		"system",
		"pensando",
		profiles.VoiceRoleConfig{Enabled: true, Provider: "sapi5", VoiceID: "Microsoft Maria"},
	)
	if sapiEvent.Strategy != ChatSpeakStrategyBackendAudio {
		t.Fatalf("sapi5 strategy = %q, want %q (unified via backend_audio)", sapiEvent.Strategy, ChatSpeakStrategyBackendAudio)
	}
	if sapiEvent.ProviderID != "sapi5" {
		t.Fatalf("sapi5 providerID = %q, want %q", sapiEvent.ProviderID, "sapi5")
	}
	if sapiEvent.FallbackStrategy != ChatSpeakStrategyAnnounce {
		t.Fatalf("sapi5 fallbackStrategy = %q, want %q", sapiEvent.FallbackStrategy, ChatSpeakStrategyAnnounce)
	}
}
