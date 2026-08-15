package app

import (
	"context"

	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/speech"
	"strings"
	"testing"
)

// mockAudioRepo implementa speech.AudioRepository para testes.
type mockAudioRepo struct {
	audio   map[string]struct{ base64, mime string }
	content map[string]string
}

func newMockAudioRepo() *mockAudioRepo {
	return &mockAudioRepo{
		audio:   make(map[string]struct{ base64, mime string }),
		content: make(map[string]string),
	}
}

func (m *mockAudioRepo) GetMessageAudio(_ context.Context, id string) (string, string, error) {
	if a, ok := m.audio[id]; ok {
		return a.base64, a.mime, nil
	}
	return "", "", nil
}

func (m *mockAudioRepo) SaveMessageAudio(_ context.Context, id string, base64, mime string) error {
	m.audio[id] = struct{ base64, mime string }{base64, mime}
	return nil
}

func (m *mockAudioRepo) GetMessageContent(_ context.Context, id string) (string, error) {
	if c, ok := m.content[id]; ok {
		return c, nil
	}
	return "", nil
}

// testProfileProvider implementa speech.ProfileProvider para testes.
type testProfileProvider struct{}

func (testProfileProvider) GetActive() (*profiles.Profile, error) {
	return nil, nil
}
func (testProfileProvider) ResolveDefaults(_ context.Context, p *profiles.Profile) *profiles.Profile {
	return p
}

// newTestSpeechSvc cria um speech.Service para testes.
func newTestSpeechSvc(repo speech.AudioRepository, reg speech.ProviderRegistry) *speech.Service {
	return speech.NewService(speech.ServiceConfig{
		Emitter:         events.NoopEmitter{},
		Registry:        reg,
		ProfileProvider: testProfileProvider{},
		AudioRepo:       repo,
	})
}

// ---------- Testes ----------

func TestSpeakMessage_ReturnsCachedAudio(t *testing.T) {
	repo := newMockAudioRepo()
	repo.audio["1"] = struct{ base64, mime string }{"cached_audio", "audio/mpeg"}
	repo.content["1"] = "Hello world"
	reg := llm.NewProviderRegistry()

	app := &App{audioSvc: repo, speechSvc: newTestSpeechSvc(repo, reg), currentUserID: "test-user"}

	// Cache hit — provider params são ignorados
	result, err := app.speechSvc.SpeakMessage(context.Background(), "1", "any-provider", "any-model", "any-voice", 1.0, "")
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
	repo.content["2"] = "Hello world"
	reg := llm.NewProviderRegistry()

	app := &App{
		audioSvc:       repo,
		llmRegistry:    reg,
		profileManager: profiles.NewManager(),
		speechSvc:      newTestSpeechSvc(repo, reg),
		currentUserID:  "test-user",
	}

	_, err := app.speechSvc.SpeakMessage(context.Background(), "2", "nonexistent-provider", "tts-1", "voice", 1.0, "")
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
	reg := llm.NewProviderRegistry()

	app := &App{audioSvc: repo, speechSvc: newTestSpeechSvc(repo, reg), currentUserID: "test-user"}

	_, err := app.speechSvc.SpeakMessage(context.Background(), "999", "provider", "", "voice", 1.0, "")
	if err == nil {
		t.Fatal("esperava erro para mensagem inexistente")
	}
}

func TestSpeakMessage_ErrorWhenContentEmpty(t *testing.T) {
	repo := newMockAudioRepo()
	repo.content["3"] = "   " // só espaços
	reg := llm.NewProviderRegistry()

	app := &App{audioSvc: repo, speechSvc: newTestSpeechSvc(repo, reg), currentUserID: "test-user"}

	_, err := app.speechSvc.SpeakMessage(context.Background(), "3", "provider", "", "voice", 1.0, "")
	if err == nil {
		t.Fatal("esperava erro para mensagem com conteúdo vazio")
	}
	if !strings.Contains(err.Error(), "sem conteúdo textual") {
		t.Errorf("erro inesperado: %v", err)
	}
}

func TestSpeakMessage_CacheHitSkipsGeneration(t *testing.T) {
	repo := newMockAudioRepo()
	repo.audio["5"] = struct{ base64, mime string }{"audio_data", "audio/mpeg"}
	// NÃO precisa de content — cache hit pula geração
	// Nem provider — não deve ser chamado
	reg := llm.NewProviderRegistry()

	app := &App{audioSvc: repo, speechSvc: newTestSpeechSvc(repo, reg), currentUserID: "test-user"}

	result, err := app.speechSvc.SpeakMessage(context.Background(), "5", "", "", "", 1.0, "")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if result.Audio != "audio_data" {
		t.Errorf("esperava audio_data, obteve %q", result.Audio)
	}

	// O cache é por mensagem: provider, voz, rate e idioma não fazem parte da
	// chave. Trocar o idioma não regera o áudio já persistido.
	result, err = app.speechSvc.SpeakMessage(context.Background(), "5", "", "", "", 1.0, "es-ES")
	if err != nil {
		t.Fatalf("erro inesperado com outro idioma: %v", err)
	}
	if result.Audio != "audio_data" || !result.Cached {
		t.Errorf("esperava o mesmo áudio em cache, obteve %+v", result)
	}
}

func TestSpeakMessage_ErrorWhenHTTPModelMissing(t *testing.T) {
	repo := newMockAudioRepo()
	repo.content["10"] = "Teste com piper"

	reg := llm.NewProviderRegistry()
	_ = reg.Register(&llm.ProviderConfig{
		ID:      "local-piper",
		Name:    "Local Piper",
		BaseURL: "http://localhost:9999",
	})

	app := &App{
		audioSvc:       repo,
		llmRegistry:    reg,
		profileManager: profiles.NewManager(),
		speechSvc:      newTestSpeechSvc(repo, reg),
		currentUserID:  "test-user",
	}

	_, err := app.speechSvc.SpeakMessage(context.Background(), "10", "local-piper", "", "pt_BR-dii", 1.0, "")
	if err == nil {
		t.Fatal("esperava erro quando model está vazio")
	}
	if !strings.Contains(err.Error(), "TTS model is required") {
		t.Errorf("erro inesperado: %v", err)
	}
}

func TestSpeakMessage_ModelOnlyRejectsVoiceID(t *testing.T) {
	repo := newMockAudioRepo()
	repo.content["12"] = "Teste model-only"

	reg := llm.NewProviderRegistry()
	_ = reg.Register(&llm.ProviderConfig{
		ID:      "local-piper",
		Name:    "Local Piper",
		BaseURL: "http://localhost:9999",
	})

	app := &App{
		audioSvc:       repo,
		llmRegistry:    reg,
		profileManager: profiles.NewManager(),
		speechSvc:      newTestSpeechSvc(repo, reg),
		currentUserID:  "test-user",
	}

	_, err := app.speechSvc.SpeakMessage(context.Background(), "12", "local-piper", "voice-pt_BR-dii", "pt_BR-dii", 1.0, "")
	if err == nil {
		t.Fatal("esperava erro quando voice_id é enviado para model-only")
	}
	if !strings.Contains(err.Error(), "voice_id must be empty") {
		t.Errorf("erro inesperado: %v", err)
	}
}

func TestSpeakMessage_SpeedNormalization(t *testing.T) {
	// Rate < 0.25 é normalizado para 1.0
	repo := newMockAudioRepo()
	repo.content["11"] = "Teste speed"

	reg := llm.NewProviderRegistry()
	_ = reg.Register(&llm.ProviderConfig{
		ID:      "test-provider",
		Name:    "Test Provider",
		BaseURL: "http://localhost:9999",
	})

	app := &App{
		audioSvc:       repo,
		llmRegistry:    reg,
		profileManager: profiles.NewManager(),
		speechSvc:      newTestSpeechSvc(repo, reg),
		currentUserID:  "test-user",
	}

	// Rate 0 deve ser normalizada para 1.0 — o provider será criado mas síntese falhará
	_, err := app.speechSvc.SpeakMessage(context.Background(), "11", "test-provider", "tts-1", "voice", 0.0, "")
	if err == nil {
		t.Fatal("esperava erro de síntese (sem server)")
	}
	// O importante é que não deu erro de provider/validation
	if strings.Contains(err.Error(), "não encontrado") {
		t.Errorf("erro deveria ser de síntese, não de provider: %v", err)
	}
}
