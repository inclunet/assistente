package speech

import (
	"testing"
)

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
		{"voice-pt_BR-cadu-medium", false},
		{"gemma-3-27b-it", false},
		{"qwen3-8b", false},
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

// TestDetectionWithRealLocalAIModels valida a detecção contra a lista real de
// modelos retornados por /v1/models de um LocalAI (inclunet:30090).
func TestDetectionWithRealLocalAIModels(t *testing.T) {
	allModels := []string{
		"voice-pt_PT-voice3",
		"whisper-1",
		"gemma-3-27b-it",
		"gpt-oss-20b",
		"qwen3-tts-0.6b-custom-voice",
		"qwen3-tts-1.7b-custom-voice",
		"qwen3-0.6b-custom-voice",
		"voice-en_US-amy-medium",
		"voice-en_US-kathleen-low",
		"kokoro",
		"kokoros",
		"voice-pt_BR-cadu-medium",
		"voice-pt_BR-faber-medium",
		"voice-pt_PT-voice4",
		"Qwen3.5-9B-GGUF",
		"vllm-omni-qwen3-tts-custom-voice",
		"voice-pt_BR-edresson-low",
		"voice-pt_BR-jeff-medium",
		"voice-pt_PT-tugao-medium",
		"qwen3-8b",
		"voice-en_US-lessac-medium",
		"voice-pt_BR-dii",
		"voice-pt_BR-miro",
	}

	wantSTT := map[string]bool{
		"whisper-1": true,
	}

	var gotSTT []string
	for _, id := range allModels {
		if isSTTModel(id) {
			gotSTT = append(gotSTT, id)
		}
	}

	for _, id := range gotSTT {
		if !wantSTT[id] {
			t.Errorf("isSTTModel(%q) = true, não esperado como STT", id)
		}
	}
	for id := range wantSTT {
		found := false
		for _, g := range gotSTT {
			if g == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("isSTTModel(%q) = false, esperado como STT", id)
		}
	}

	// Modelos LLM NÃO devem ser detectados como TTS nem STT
	llmModels := []string{"gemma-3-27b-it", "gpt-oss-20b", "Qwen3.5-9B-GGUF", "qwen3-8b"}
	for _, id := range llmModels {
		if isSTTModel(id) {
			t.Errorf("isSTTModel(%q) = true, é LLM e não STT", id)
		}
	}
}
