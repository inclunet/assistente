package speech

import (
	"context"
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
		{"qwen3-0.6b-custom-voice", true},
		{"vllm-omni-qwen3-tts-custom-voice", true},
		// Kokoro/LocalAI
		{"kokoro", true},
		{"kokoros", true},
		{"localai-kokoro-v1", true},
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

	wantTTS := map[string]bool{
		"voice-pt_PT-voice3":               true,
		"qwen3-tts-0.6b-custom-voice":      true,
		"qwen3-tts-1.7b-custom-voice":      true,
		"qwen3-0.6b-custom-voice":          true,
		"kokoro":                           true,
		"kokoros":                          true,
		"voice-en_US-amy-medium":           true,
		"voice-en_US-kathleen-low":         true,
		"voice-pt_BR-cadu-medium":          true,
		"voice-pt_BR-faber-medium":         true,
		"voice-pt_PT-voice4":               true,
		"vllm-omni-qwen3-tts-custom-voice": true,
		"voice-pt_BR-edresson-low":         true,
		"voice-pt_BR-jeff-medium":          true,
		"voice-pt_PT-tugao-medium":         true,
		"voice-en_US-lessac-medium":        true,
		"voice-pt_BR-dii":                  true,
		"voice-pt_BR-miro":                 true,
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

func TestSelectionModeForTTSModel(t *testing.T) {
	tests := []struct {
		modelID string
		want    TTSSelectionMode
	}{
		{modelID: "tts-1", want: TTSSelectionModelAndVoice},
		{modelID: "tts-1-hd", want: TTSSelectionModelAndVoice},
		{modelID: "gpt-4o-mini-tts", want: TTSSelectionModelAndVoice},
		{modelID: "qwen3-tts-0.6b-custom-voice", want: TTSSelectionModelAndVoice},
		{modelID: "vllm-omni-qwen3-tts-custom-voice", want: TTSSelectionModelAndVoice},
		{modelID: "kokoro", want: TTSSelectionModelAndVoice},
		{modelID: "kokoros", want: TTSSelectionModelAndVoice},
		{modelID: "voice-pt_BR-cadu-medium", want: TTSSelectionModelOnly},
		{modelID: "Voice-en_US-amy-medium", want: TTSSelectionModelOnly},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			if got := selectionModeForTTSModel(tt.modelID); got != tt.want {
				t.Errorf("selectionModeForTTSModel(%q) = %q, want %q", tt.modelID, got, tt.want)
			}
		})
	}
}

func TestFetchVoicesForDynamicTTSFamilies(t *testing.T) {
	client := &TTSClient{}

	qwenVoices, err := client.FetchVoices(context.Background(), "qwen3-tts-0.6b-custom-voice")
	if err != nil {
		t.Fatalf("FetchVoices(qwen): %v", err)
	}
	if !hasTTSVoiceID(qwenVoices, "Aiden") || !hasTTSVoiceID(qwenVoices, "Vivian") {
		t.Fatalf("Qwen CustomVoice voices missing expected speakers: %+v", qwenVoices)
	}

	kokoroVoices, err := client.FetchVoices(context.Background(), "kokoros")
	if err != nil {
		t.Fatalf("FetchVoices(kokoro): %v", err)
	}
	if !hasTTSVoiceID(kokoroVoices, "af_heart") || !hasTTSVoiceID(kokoroVoices, "pf_dora") {
		t.Fatalf("Kokoro voices missing expected speakers: %+v", kokoroVoices)
	}

	modelOnlyVoices, err := client.FetchVoices(context.Background(), "voice-pt_BR-cadu-medium")
	if err != nil {
		t.Fatalf("FetchVoices(model-only): %v", err)
	}
	if len(modelOnlyVoices) != 0 {
		t.Fatalf("model-only voice model returned separate voices: %+v", modelOnlyVoices)
	}
}

func hasTTSVoiceID(voices []TTSVoiceInfo, id string) bool {
	for _, voice := range voices {
		if voice.ID == id {
			return true
		}
	}
	return false
}
