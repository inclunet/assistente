package speech

import "testing"

func TestIsTTSModel(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		// Modelos OpenAI conhecidos
		{"tts-1", true},
		{"tts-1-hd", true},
		{"tts-1-1106", true},
		{"tts-1-hd-1106", true},
		// Case-insensitive
		{"TTS-1", true},
		{"Tts-1-HD", true},
		// Prefixo genérico tts-
		{"tts-2", true},
		{"tts-custom", true},
		// Não é TTS
		{"gpt-4o", false},
		{"whisper-1", false},
		{"dall-e-3", false},
		{"text-embedding-3-small", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := isTTSModel(tt.id); got != tt.want {
				t.Errorf("isTTSModel(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsSTTModel(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		// Modelos conhecidos (exact match)
		{"whisper-1", true},
		{"whisper-large-v3", true},
		{"gpt-4o-transcribe", true},
		{"gpt-4o-mini-transcribe", true},
		// Case-insensitive
		{"Whisper-1", true},
		{"GPT-4o-Transcribe", true},
		// Prefixo whisper (providers alternativos)
		{"whisper-large-v3-turbo", true},
		{"whisper-medium", true},
		// Sufixo -transcribe (novos modelos futuros)
		{"gpt-5-transcribe", true},
		{"some-model-transcribe", true},
		// Sufixo -asr (providers alternativos)
		{"model-asr", true},
		{"faster-whisper-asr", true},
		// Não é STT
		{"gpt-4o", false},
		{"tts-1", false},
		{"dall-e-3", false},
		{"text-embedding-3-small", false},
		{"", false},
		// Parcialmente match mas não deve casar
		{"transcribe-gpt", false},
		{"asr-model", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := isSTTModel(tt.id); got != tt.want {
				t.Errorf("isSTTModel(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}
