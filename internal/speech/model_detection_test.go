package speech

import (
	"strings"
	"testing"
)

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
		// Piper/LocalAI (prefixo voice-)
		{"voice-pt_BR-cadu-medium", true},
		{"voice-pt_BR-faber-medium", true},
		{"voice-en_US-amy-medium", true},
		{"voice-en_US-kathleen-low", true},
		{"voice-pt_PT-tugao-medium", true},
		{"voice-pt_BR-dii", true},
		{"voice-pt_BR-miro", true},
		{"Voice-pt_BR-jeff-medium", true},
		// Infixo -tts- (qwen, vllm, etc)
		{"qwen3-tts-0.6b-custom-voice", true},
		{"qwen3-tts-1.7b-custom-voice", true},
		{"vllm-omni-qwen3-tts-custom-voice", true},
		// Não é TTS
		{"gpt-4o", false},
		{"whisper-1", false},
		{"dall-e-3", false},
		{"text-embedding-3-small", false},
		{"gemma-3-27b-it", false},
		{"gpt-oss-20b", false},
		{"Qwen3.5-9B-GGUF", false},
		{"qwen3-8b", false},
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
		"voice-en_US-amy-medium",
		"voice-en_US-kathleen-low",
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

	wantTTS := map[string]bool{
		"voice-pt_PT-voice3":               true,
		"qwen3-tts-0.6b-custom-voice":      true,
		"qwen3-tts-1.7b-custom-voice":      true,
		"voice-en_US-amy-medium":            true,
		"voice-en_US-kathleen-low":          true,
		"voice-pt_BR-cadu-medium":           true,
		"voice-pt_BR-faber-medium":          true,
		"voice-pt_PT-voice4":                true,
		"vllm-omni-qwen3-tts-custom-voice": true,
		"voice-pt_BR-edresson-low":          true,
		"voice-pt_BR-jeff-medium":           true,
		"voice-pt_PT-tugao-medium":          true,
		"voice-en_US-lessac-medium":         true,
		"voice-pt_BR-dii":                   true,
		"voice-pt_BR-miro":                  true,
	}

	wantSTT := map[string]bool{
		"whisper-1": true,
	}

	var gotTTS, gotSTT []string
	for _, id := range allModels {
		if isTTSModel(id) {
			gotTTS = append(gotTTS, id)
		}
		if isSTTModel(id) {
			gotSTT = append(gotSTT, id)
		}
	}

	for _, id := range gotTTS {
		if !wantTTS[id] {
			t.Errorf("isTTSModel(%q) = true, não esperado como TTS", id)
		}
	}
	for id := range wantTTS {
		found := false
		for _, g := range gotTTS {
			if g == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("isTTSModel(%q) = false, esperado como TTS", id)
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
		if isTTSModel(id) {
			t.Errorf("isTTSModel(%q) = true, é LLM e não TTS", id)
		}
		if isSTTModel(id) {
			t.Errorf("isSTTModel(%q) = true, é LLM e não STT", id)
		}
	}
}

// TestIsDynamicTTSModel verifica que modelos dinâmicos (Piper, qwen-tts) são
// detectados, mas modelos padrão OpenAI (tts-1, tts-1-hd) não são.
func TestIsDynamicTTSModel(t *testing.T) {
	dynamic := []string{
		"voice-pt_BR-cadu-medium",
		"voice-en_US-amy-medium",
		"voice-pt_PT-voice4",
		"qwen3-tts-0.6b",
	}
	for _, id := range dynamic {
		if !IsDynamicTTSModel(id) {
			t.Errorf("IsDynamicTTSModel(%q) = false, deveria ser true (modelo dinâmico)", id)
		}
	}

	notDynamic := []string{
		"tts-1",
		"tts-1-hd",
		"tts-1-1106",
		"tts-1-hd-1106",
		"alloy",       // voz OpenAI, não modelo
		"nova",        // voz OpenAI, não modelo
		"gpt-4o-mini", // modelo LLM
	}
	for _, id := range notDynamic {
		if IsDynamicTTSModel(id) {
			t.Errorf("IsDynamicTTSModel(%q) = true, não deveria ser dinâmico", id)
		}
	}
}

// TestClassifyVoicesFromModels verifica a lógica usada por FetchVoices para
// separar modelos TTS padrão (knownTTSModels → vozes estáticas OpenAI) dos
// modelos personalizados (Piper, qwen-tts → retornados como vozes dinâmicas).
func TestClassifyVoicesFromModels(t *testing.T) {
	tests := []struct {
		name            string
		modelIDs        []string
		wantDynamic     []string // IDs que seria retornados como vozes dinâmicas
		wantHasStandard bool     // se há modelo TTS padrão (knownTTSModels)
	}{
		{
			name:            "apenas modelos OpenAI padrão",
			modelIDs:        []string{"tts-1", "tts-1-hd", "gpt-4o", "whisper-1"},
			wantDynamic:     nil,
			wantHasStandard: true,
		},
		{
			name:            "apenas modelos Piper LocalAI",
			modelIDs:        []string{"voice-pt_BR-cadu-medium", "voice-en_US-amy-medium", "whisper-1", "gemma-3-27b-it"},
			wantDynamic:     []string{"voice-pt_BR-cadu-medium", "voice-en_US-amy-medium"},
			wantHasStandard: false,
		},
		{
			name:            "misto: OpenAI + Piper",
			modelIDs:        []string{"tts-1", "voice-pt_BR-cadu-medium", "tts-1-hd"},
			wantDynamic:     []string{"voice-pt_BR-cadu-medium"},
			wantHasStandard: true,
		},
		{
			name:            "qwen-tts modelos como vozes",
			modelIDs:        []string{"qwen3-tts-0.6b-custom-voice", "qwen3-8b"},
			wantDynamic:     []string{"qwen3-tts-0.6b-custom-voice"},
			wantHasStandard: false,
		},
		{
			name:            "real LocalAI completo",
			modelIDs:        []string{"voice-pt_BR-cadu-medium", "voice-pt_BR-faber-medium", "voice-en_US-amy-medium", "qwen3-tts-0.6b-custom-voice", "whisper-1", "gemma-3-27b-it", "gpt-oss-20b"},
			wantDynamic:     []string{"voice-pt_BR-cadu-medium", "voice-pt_BR-faber-medium", "voice-en_US-amy-medium", "qwen3-tts-0.6b-custom-voice"},
			wantHasStandard: false,
		},
		{
			name:            "sem modelos TTS",
			modelIDs:        []string{"gpt-4o", "whisper-1", "dall-e-3"},
			wantDynamic:     nil,
			wantHasStandard: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dynamicVoices []string
			hasStandard := false

			for _, id := range tt.modelIDs {
				if isTTSModel(id) {
					lower := strings.ToLower(id)
					if knownTTSModels[lower] {
						hasStandard = true
					} else {
						dynamicVoices = append(dynamicVoices, id)
					}
				}
			}

			if hasStandard != tt.wantHasStandard {
				t.Errorf("hasStandard = %v, want %v", hasStandard, tt.wantHasStandard)
			}
			if len(dynamicVoices) != len(tt.wantDynamic) {
				t.Errorf("dynamicVoices = %v (len %d), want %v (len %d)", dynamicVoices, len(dynamicVoices), tt.wantDynamic, len(tt.wantDynamic))
				return
			}
			for i, id := range dynamicVoices {
				if id != tt.wantDynamic[i] {
					t.Errorf("dynamicVoices[%d] = %q, want %q", i, id, tt.wantDynamic[i])
				}
			}
		})
	}
}
