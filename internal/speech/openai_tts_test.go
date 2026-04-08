package speech

import (
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
		{5999, 90 * time.Second},  // floor(5999/4000) = 1
		{7999, 90 * time.Second},  // floor(7999/4000) = 1
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

	params := client.buildParams("Hello world", VoiceNova)

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

	params := client.buildParams("Test", VoiceEcho)

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

	params := client.buildParams("Test", VoiceNova)

	// Speed == 1.0 → não deve estar setado (usa default da API)
	if params.Speed != (param.Opt[float64]{}) {
		t.Error("Speed deveria ser zero-value quando Speed==1.0")
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
