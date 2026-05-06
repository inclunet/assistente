package speech

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/openai/openai-go/packages/param"
)

// ============================================================================
// Constantes
// ============================================================================

func TestTTSConstants(t *testing.T) {
	if TtsMaxChunkSize != 4000 {
		t.Errorf("TtsMaxChunkSize = %d, esperado 4000", TtsMaxChunkSize)
	}
	if TtsStreamBufSize != 8192 {
		t.Errorf("TtsStreamBufSize = %d, esperado 8192", TtsStreamBufSize)
	}
	if TtsTimeoutBase != 60*time.Second {
		t.Errorf("TtsTimeoutBase = %v, esperado 60s", TtsTimeoutBase)
	}
	if TtsTimeoutPerChunk != 30*time.Second {
		t.Errorf("TtsTimeoutPerChunk = %v, esperado 30s", TtsTimeoutPerChunk)
	}
}

func TestCalcTTSTimeout(t *testing.T) {
	tests := []struct {
		textLen  int
		expected time.Duration
	}{
		{0, 60 * time.Second},
		{100, 60 * time.Second},
		{3999, 60 * time.Second},
		{4000, 90 * time.Second},
		{8000, 120 * time.Second},
		{12000, 150 * time.Second},
		{5999, 90 * time.Second}, // floor(5999/4000) = 1
		{7999, 90 * time.Second}, // floor(7999/4000) = 1
	}

	for _, tt := range tests {
		result := CalcTTSTimeout(tt.textLen)
		if result != tt.expected {
			t.Errorf("CalcTTSTimeout(%d) = %v, esperado %v", tt.textLen, result, tt.expected)
		}
	}
}

// ============================================================================
// Event Constants
// ============================================================================

func TestEventConstants(t *testing.T) {
	if EventTTSStreamStart != "tts:stream:start" {
		t.Errorf("EventTTSStreamStart = %q", EventTTSStreamStart)
	}
	if EventTTSStreamChunk != "tts:stream:chunk" {
		t.Errorf("EventTTSStreamChunk = %q", EventTTSStreamChunk)
	}
	if EventTTSStreamDone != "tts:stream:done" {
		t.Errorf("EventTTSStreamDone = %q", EventTTSStreamDone)
	}
	if EventTTSStreamError != "tts:stream:error" {
		t.Errorf("EventTTSStreamError = %q", EventTTSStreamError)
	}
}

// ============================================================================
// buildParams
// ============================================================================

func TestBuildParams_DefaultVoice(t *testing.T) {
	client := &TTSClient{
		config: TTSConfig{
			Model:  ModelTTS1,
			Voice:  VoiceNova,
			Format: FormatMP3,
			Speed:  1.0,
		},
	}

	params, err := client.buildParams("Hello world", VoiceNova)
	if err != nil {
		t.Fatalf("buildParams erro inesperado: %v", err)
	}

	if params.Input != "Hello world" {
		t.Errorf("Input = %q, esperado %q", params.Input, "Hello world")
	}
	if string(params.Model) != "tts-1" {
		t.Errorf("Model = %q, esperado %q", params.Model, "tts-1")
	}
	if string(params.Voice) != "nova" {
		t.Errorf("Voice = %q, esperado %q", params.Voice, "nova")
	}
	if string(params.ResponseFormat) != "mp3" {
		t.Errorf("Format = %q, esperado %q", params.ResponseFormat, "mp3")
	}
}

func TestBuildParams_CustomSpeed(t *testing.T) {
	client := &TTSClient{
		config: TTSConfig{
			Model:  ModelTTS1HD,
			Voice:  VoiceAlloy,
			Format: FormatOpus,
			Speed:  1.5,
		},
	}

	params, err := client.buildParams("Test", VoiceEcho)
	if err != nil {
		t.Fatalf("buildParams erro inesperado: %v", err)
	}

	if string(params.Voice) != "echo" {
		t.Errorf("Voice = %q, esperado %q (override)", params.Voice, "echo")
	}
	if string(params.Model) != "tts-1-hd" {
		t.Errorf("Model = %q, esperado %q", params.Model, "tts-1-hd")
	}

	// Speed != 1.0 → deve estar setado
	expectedSpeed := param.NewOpt(1.5)
	if params.Speed != expectedSpeed {
		t.Errorf("Speed = %v, esperado %v", params.Speed, expectedSpeed)
	}
}

func TestBuildParams_DefaultSpeedNotSet(t *testing.T) {
	client := &TTSClient{
		config: TTSConfig{
			Model: ModelTTS1,
			Voice: VoiceNova,
			Speed: 1.0,
		},
	}

	params, err := client.buildParams("Test", VoiceNova)
	if err != nil {
		t.Fatalf("buildParams erro inesperado: %v", err)
	}

	// Speed == 1.0 → não deve estar setado (usa default da API)
	if params.Speed != (param.Opt[float64]{}) {
		t.Error("Speed deveria ser zero-value quando Speed==1.0")
	}
}

func TestBuildParams_ModelOnlyOmitsVoice(t *testing.T) {
	client := &TTSClient{
		config: TTSConfig{
			Model:         TTSModel("voice-pt_BR-cadu-medium"),
			SelectionMode: TTSSelectionModelOnly,
			Format:        FormatMP3,
			Speed:         1.0,
		},
	}

	params, err := client.buildParams("Teste", "")
	if err != nil {
		t.Fatalf("buildParams erro inesperado: %v", err)
	}
	if string(params.Model) != "voice-pt_BR-cadu-medium" {
		t.Errorf("Model = %q, esperado voice-pt_BR-cadu-medium", params.Model)
	}
	if params.Voice != "" {
		t.Errorf("Voice deveria ser omitida/zero-value em model_only, obteve %q", params.Voice)
	}
}

func TestBuildParams_ModelOnlyRejectsVoice(t *testing.T) {
	client := &TTSClient{
		config: TTSConfig{
			Model:         TTSModel("voice-pt_BR-cadu-medium"),
			SelectionMode: TTSSelectionModelOnly,
			Format:        FormatMP3,
			Speed:         1.0,
		},
	}

	_, err := client.buildParams("Teste", "nova")
	if err == nil {
		t.Fatal("esperava erro quando model_only recebe voice")
	}
}

func TestBuildParams_OfficialOpenAIUsesLanguageInstruction(t *testing.T) {
	client := &TTSClient{
		config: TTSConfig{
			BaseURL:  "https://api.openai.com/v1",
			Model:    TTSModel("gpt-4o-mini-tts"),
			Voice:    VoiceNova,
			Language: "pt-BR",
		},
	}

	params, err := client.buildParams("Olá", VoiceNova)
	if err != nil {
		t.Fatalf("buildParams erro inesperado: %v", err)
	}
	if !params.Instructions.Valid() {
		t.Fatal("Instructions deveria estar definido para modelo com suporte")
	}
	if params.Instructions.Value != "Speak in Brazilian Portuguese with a native Brazilian Portuguese accent." {
		t.Errorf("Instructions = %q", params.Instructions.Value)
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal params: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("Unmarshal params: %v", err)
	}
	if _, ok := body["language"]; ok {
		t.Fatalf("OpenAI oficial não deve receber campo language extra: %s", string(data))
	}
}

func TestBuildParams_CompatibleProviderSendsLanguageField(t *testing.T) {
	client := &TTSClient{
		config: TTSConfig{
			BaseURL:  "http://localhost:8080/v1",
			Model:    TTSModel("kokoro"),
			Voice:    TTSVoice("pm_santa"),
			Language: "pt-BR",
		},
	}

	params, err := client.buildParams("Olá", TTSVoice("pm_santa"))
	if err != nil {
		t.Fatalf("buildParams erro inesperado: %v", err)
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal params: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("Unmarshal params: %v", err)
	}
	if body["language"] != "pt" {
		t.Fatalf("language = %v, esperado pt; body=%s", body["language"], string(data))
	}
	if body["lang_code"] != "p" {
		t.Fatalf("lang_code = %v, esperado p; body=%s", body["lang_code"], string(data))
	}
}

func TestBuildParams_KokoroUsesVoicePrefixForLangCode(t *testing.T) {
	client := &TTSClient{
		config: TTSConfig{
			BaseURL:  "http://localhost:8080/v1",
			Model:    TTSModel("kokoro"),
			Voice:    TTSVoice("am_santa"),
			Language: "pt-BR",
		},
	}

	params, err := client.buildParams("Olá", TTSVoice("am_santa"))
	if err != nil {
		t.Fatalf("buildParams erro inesperado: %v", err)
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal params: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("Unmarshal params: %v", err)
	}
	if body["lang_code"] != "a" {
		t.Fatalf("lang_code = %v, esperado a para voz am_santa; body=%s", body["lang_code"], string(data))
	}
}

func TestBuildParams_KokoroNormalizesHyphenatedVoiceID(t *testing.T) {
	client := &TTSClient{
		config: TTSConfig{
			BaseURL:  "http://localhost:8080/v1",
			Model:    TTSModel("kokoro"),
			Voice:    TTSVoice("pm-santa"),
			Language: "pt-BR",
		},
	}

	params, err := client.buildParams("Olá", TTSVoice("pm-santa"))
	if err != nil {
		t.Fatalf("buildParams erro inesperado: %v", err)
	}
	if params.Voice != "pm_santa" {
		t.Fatalf("Voice = %q, esperado pm_santa", params.Voice)
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal params: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("Unmarshal params: %v", err)
	}
	if body["lang_code"] != "p" {
		t.Fatalf("lang_code = %v, esperado p; body=%s", body["lang_code"], string(data))
	}
}

// ============================================================================
// Voice helpers
// ============================================================================

func TestGetAvailableVoices(t *testing.T) {
	voices := GetAvailableVoices()
	if len(voices) < 6 {
		t.Errorf("esperava pelo menos 6 vozes, obteve %d", len(voices))
	}

	// Verifica que retorna uma cópia (não modifica o original)
	voices[0].ID = "modified"
	voicesAgain := GetAvailableVoices()
	if voicesAgain[0].ID == "modified" {
		t.Error("GetAvailableVoices deveria retornar uma cópia")
	}
}

// ============================================================================
// RoleVoiceConfig - Pitch field
// ============================================================================

func TestRoleVoiceConfig_HasPitch(t *testing.T) {
	cfg := RoleVoiceConfig{
		Provider: "webspeech",
		Voice:    "nova",
		Rate:     1.0,
		Pitch:    1.5,
		Volume:   0.8,
	}
	if cfg.Pitch != 1.5 {
		t.Errorf("Pitch = %f, esperado 1.5", cfg.Pitch)
	}
}
