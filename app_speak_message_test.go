package main

import (
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"strings"
	"testing"
)

// mockAudioRepo implementa speech.AudioRepository para testes.
type mockAudioRepo struct {
	audio   map[uint]struct{ base64, mime string }
	content map[uint]string
}

func newMockAudioRepo() *mockAudioRepo {
	return &mockAudioRepo{
		audio:   make(map[uint]struct{ base64, mime string }),
		content: make(map[uint]string),
	}
}

func (m *mockAudioRepo) GetMessageAudio(id uint) (string, string, error) {
	if a, ok := m.audio[id]; ok {
		return a.base64, a.mime, nil
	}
	return "", "", nil
}

func (m *mockAudioRepo) SaveMessageAudio(id uint, base64, mime string) error {
	m.audio[id] = struct{ base64, mime string }{base64, mime}
	return nil
}

func (m *mockAudioRepo) GetMessageContent(id uint) (string, error) {
	if c, ok := m.content[id]; ok {
		return c, nil
	}
	return "", nil
}

// ---------- Testes ----------

func TestSpeakMessage_ReturnsCachedAudio(t *testing.T) {
	repo := newMockAudioRepo()
	repo.audio[1] = struct{ base64, mime string }{"cached_audio", "audio/mpeg"}
	repo.content[1] = "Hello world"

	app := &App{audioSvc: repo}

	// Cache hit — provider params são ignorados
	result, err := app.SpeakMessage(1, "any-provider", "any-voice", "", 1.0)
	if err != nil {
		t.Fatalf("SpeakMessage erro inesperado: %v", err)
	}
	if result.Audio != "cached_audio" {
		t.Errorf("esperava cached_audio, obteve %q", result.Audio)
	}
	if result.MimeType != "audio/mpeg" {
		t.Errorf("esperava audio/mpeg, obteve %q", result.MimeType)
	}
}

func TestSpeakMessage_ErrorWhenProviderNotFound(t *testing.T) {
	repo := newMockAudioRepo()
	repo.content[2] = "Hello world"

	app := &App{
		audioSvc:       repo,
		llmRegistry:    llm.NewProviderRegistry(),
		profileManager: profiles.NewManager(),
	}

	_, err := app.SpeakMessage(2, "nonexistent-provider", "voice", "tts-1", 1.0)
	if err == nil {
		t.Fatal("esperava erro quando provider não existe no registry")
	}
	if !strings.Contains(err.Error(), "não encontrado") {
		t.Errorf("erro inesperado: %v", err)
	}
}

func TestSpeakMessage_ErrorWhenMessageNotFound(t *testing.T) {
	repo := newMockAudioRepo()
	// Mensagem 999 não existe no mock

	app := &App{audioSvc: repo}

	_, err := app.SpeakMessage(999, "provider", "voice", "", 1.0)
	if err == nil {
		t.Fatal("esperava erro para mensagem inexistente")
	}
}

func TestSpeakMessage_ErrorWhenContentEmpty(t *testing.T) {
	repo := newMockAudioRepo()
	repo.content[3] = "   " // só espaços

	app := &App{audioSvc: repo}

	_, err := app.SpeakMessage(3, "provider", "voice", "", 1.0)
	if err == nil {
		t.Fatal("esperava erro para mensagem com conteúdo vazio")
	}
	if !strings.Contains(err.Error(), "sem conteúdo textual") {
		t.Errorf("erro inesperado: %v", err)
	}
}

func TestSpeakMessage_CacheHitSkipsGeneration(t *testing.T) {
	repo := newMockAudioRepo()
	repo.audio[5] = struct{ base64, mime string }{"audio_data", "audio/mpeg"}
	// NÃO precisa de content — cache hit pula geração
	// Nem provider — não deve ser chamado

	app := &App{audioSvc: repo}

	result, err := app.SpeakMessage(5, "", "", "", 1.0)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if result.Audio != "audio_data" {
		t.Errorf("esperava audio_data, obteve %q", result.Audio)
	}
}
