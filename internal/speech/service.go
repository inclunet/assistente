package speech

import (
	"assistente/internal/logging"
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/textutil"
)

// ProviderRegistry abstrai o acesso ao registro de provedores LLM.
type ProviderRegistry interface {
	Get(id string) *llm.ProviderConfig
	List() []*llm.ProviderConfig
}

// ProfileProvider abstrai o acesso ao perfil ativo e resolução de defaults.
type ProfileProvider interface {
	GetActive() (*profiles.Profile, error)
	ResolveDefaults(ctx context.Context, p *profiles.Profile) *profiles.Profile
}

// ServiceConfig contém dependências injetadas para o Service.
type ServiceConfig struct {
	Emitter         events.Emitter
	Registry        ProviderRegistry
	ProfileProvider ProfileProvider
	CredMgr         *credentials.Manager
	AudioRepo       AudioRepository
}

// Service encapsula a lógica de negócio de speech/TTS/STT,
// desacoplado do framework Wails.
type Service struct {
	emitter         events.Emitter
	registry        ProviderRegistry
	profileProvider ProfileProvider
	credMgr         *credentials.Manager
	audioRepo       AudioRepository
	speechManager   *SpeechManager
}

// NewService cria um Service com as dependências fornecidas.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		emitter:         cfg.Emitter,
		registry:        cfg.Registry,
		profileProvider: cfg.ProfileProvider,
		credMgr:         cfg.CredMgr,
		audioRepo:       cfg.AudioRepo,
	}
}

// speechLanguage devolve o idioma de fala do perfil ativo. Vazio quando o
// perfil não está disponível — os rótulos falados caem no padrão em inglês.
func (s *Service) speechLanguage() string {
	if s.profileProvider == nil {
		return ""
	}
	p, err := s.profileProvider.GetActive()
	if err != nil || p == nil {
		return ""
	}
	return p.Input.Language
}

// GetSpeechManager retorna o speech manager atual (pode ser nil).
func (s *Service) GetSpeechManager() *SpeechManager {
	return s.speechManager
}

// SetSpeechManager permite que a camada Wails injete um speech manager.
func (s *Service) SetSpeechManager(sm *SpeechManager) {
	s.speechManager = sm
}

// InitFromProfile inicializa o speech manager a partir do perfil ativo.
func (s *Service) InitFromProfile(ctx context.Context) error {
	p, err := s.profileProvider.GetActive()
	if err != nil || p == nil {
		return fmt.Errorf("perfil ativo não encontrado: %w", err)
	}
	resolved := s.profileProvider.ResolveDefaults(ctx, p)
	sm := NewSpeechManagerFromProfile(ctx, resolved, s.registry, s.credMgr)
	if sm == nil {
		return fmt.Errorf("falha ao criar speech manager para perfil ativo")
	}
	s.speechManager = sm
	return nil
}

// EnsureSpeechManager garante que o speechManager está inicializado.
func (s *Service) EnsureSpeechManager(ctx context.Context) bool {
	if s.speechManager != nil {
		return true
	}
	if err := s.InitFromProfile(ctx); err != nil {
		logging.Errorf(ctx, "speech.service", "[Speech] Erro ao inicializar speechManager do perfil: %v", err)
		return false
	}
	return s.speechManager != nil
}

// CreateTTSClient cria um TTSClient para um provider LLM específico.
func (s *Service) CreateTTSClient(ctx context.Context, providerID string, model string) *TTSClient {
	cfg := s.registry.Get(providerID)
	if cfg == nil {
		logging.Infof(ctx, "speech.service", "[TTS] Provider %s não encontrado", providerID)
		return nil
	}
	return NewTTSClient(TTSConfig{
		BaseURL:           cfg.BaseURL,
		CredentialPattern: cfg.CredentialPattern,
		Model:             TTSModel(model),
	}, s.credMgr)
}

func (s *Service) CreateTTSClientWithLanguage(ctx context.Context, providerID string, model string, language string) *TTSClient {
	cfg := s.registry.Get(providerID)
	if cfg == nil {
		logging.Infof(ctx, "speech.service", "[TTS] Provider %s não encontrado", providerID)
		return nil
	}
	return NewTTSClient(TTSConfig{
		BaseURL:           cfg.BaseURL,
		CredentialPattern: cfg.CredentialPattern,
		Model:             TTSModel(model),
		Language:          language,
	}, s.credMgr)
}

// SpeakMessage retorna o áudio de uma mensagem, usando cache do DB se disponível.
func (s *Service) SpeakMessage(ctx context.Context, messageID string, providerID string, model string, voiceID string, rate float64) (*AudioResult, error) {
	// 1. Checa cache no DB
	audio, mime, err := s.audioRepo.GetMessageAudio(ctx, messageID)
	if err == nil && audio != "" {
		return &AudioResult{Audio: audio, MimeType: mime, Cached: true}, nil
	}

	// 2. Busca o conteúdo textual da mensagem
	content, err := s.audioRepo.GetMessageContent(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("mensagem %s não encontrada: %w", messageID, err)
	}
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("mensagem %s sem conteúdo textual", messageID)
	}

	rawContent := content
	content = textutil.StripMarkdownForSpeechLabeled(content, textutil.CodeBlockSpeechLabel(s.speechLanguage()))
	if strings.TrimSpace(content) == "" {
		// Strip pode zerar só-sintaxe; fallback ao texto persistido (mesmo padrão do gateway).
		content = strings.TrimSpace(rawContent)
		if content == "" {
			return nil, fmt.Errorf("mensagem %s sem conteúdo falável após remover markdown", messageID)
		}
	}

	// 3. Gera TTS: roteia entre SAPI5 (local) e provedores API (HTTP)
	audioData, mimeType, err := s.synthesizeForProvider(ctx, content, providerID, model, voiceID, rate)
	if err != nil {
		return nil, fmt.Errorf("TTS for message %s: %w", messageID, err)
	}

	// 4. Persiste no DB
	audioBase64 := base64.StdEncoding.EncodeToString(audioData)
	cached := true
	if saveErr := s.audioRepo.SaveMessageAudio(ctx, messageID, audioBase64, mimeType); saveErr != nil {
		logging.Warnf(ctx, "speech.service", "[TTS] WARN: falha ao salvar áudio no DB (messageID=%s): %v", messageID, saveErr)
		cached = false
	}

	return &AudioResult{Audio: audioBase64, MimeType: mimeType, Cached: cached}, nil
}

// synthesizeForProvider roteia a síntese TTS para o provider correto.
func (s *Service) synthesizeForProvider(ctx context.Context, text, providerID, model, voiceID string, rate float64) ([]byte, string, error) {
	if providerID == "sapi5" {
		return s.synthesizeSAPI5(text, voiceID, mapRateToSAPI5(rate))
	}
	return s.synthesizeAPI(ctx, text, providerID, model, voiceID, rate)
}

// mapRateToSAPI5 converte a escala de rate do perfil (0.25–4.0, padrão 1.0)
// para a escala SAPI5 (-10..10, padrão 0). Valores fora do range são clamped.
func mapRateToSAPI5(rate float64) int {
	if rate <= 0 || rate == 1.0 {
		return 0 // padrão
	}
	// rate < 1.0 → negativo (mais lento), rate > 1.0 → positivo (mais rápido)
	// Escala: 0.25 → -10, 1.0 → 0, 4.0 → +10
	var sapi5Rate float64
	if rate < 1.0 {
		// 0.25..1.0 → -10..0
		sapi5Rate = (rate - 1.0) / 0.075 // (1.0-0.25)/10 = 0.075
	} else {
		// 1.0..4.0 → 0..10
		sapi5Rate = (rate - 1.0) / 0.3 // (4.0-1.0)/10 = 0.3
	}
	// Clamp
	if sapi5Rate < -10 {
		sapi5Rate = -10
	}
	if sapi5Rate > 10 {
		sapi5Rate = 10
	}
	return int(math.Round(sapi5Rate))
}

// synthesizeSAPI5 gera áudio WAV via SAPI5 COM local.
func (s *Service) synthesizeSAPI5(text, voiceID string, rate int) ([]byte, string, error) {
	audioData, err := GetSAPI5Manager().SynthesizeToBytes(text, voiceID, rate, 100)
	if err != nil {
		return nil, "", fmt.Errorf("SAPI5: %w", err)
	}
	if len(audioData) == 0 {
		return nil, "", fmt.Errorf("SAPI5 retornou áudio vazio")
	}
	return audioData, "audio/wav", nil
}

// synthesizeAPI gera áudio MP3 via provider OpenAI-compatible (HTTP).
func (s *Service) synthesizeAPI(ctx context.Context, text, providerID, model, voiceID string, rate float64) ([]byte, string, error) {
	if err := validateTTSSelection(model, voiceID, ""); err != nil {
		return nil, "", err
	}
	if rate <= 0 {
		rate = 1.0
	}
	speed := clampSpeed(rate)

	client := s.CreateTTSClient(ctx, providerID, model)
	if client == nil {
		return nil, "", fmt.Errorf("provider TTS %q não encontrado", providerID)
	}
	client.SetVoice(TTSVoice(voiceID))
	client.SetSpeed(speed)

	audioData, err := client.Synthesize(text)
	if err != nil {
		return nil, "", fmt.Errorf("synthesize: %w", err)
	}
	return audioData, "audio/mpeg", nil
}

// GenerateAndSaveMessageAudio gera áudio TTS para uma mensagem e salva no DB.
func (s *Service) GenerateAndSaveMessageAudio(ctx context.Context, messageID string, text string) (*AudioResult, error) {
	if !s.EnsureSpeechManager(ctx) {
		return nil, fmt.Errorf("speech manager indisponível")
	}

	result, err := s.speechManager.Synthesize(text)
	if err != nil {
		return nil, fmt.Errorf("generate audio for message %s: %w", messageID, err)
	}

	mimeType := "audio/mpeg"
	cached := true
	if err := s.audioRepo.SaveMessageAudio(ctx, messageID, result.AudioBase64, mimeType); err != nil {
		logging.Warnf(ctx, "speech.service", "[TTS] WARN: falha ao salvar áudio no DB (messageID=%s): %v — áudio será retornado mas não persistido", messageID, err)
		cached = false
	}

	return &AudioResult{Audio: result.AudioBase64, MimeType: mimeType, Cached: cached}, nil
}

// GetTTSModels retorna modelos TTS disponíveis para um provedor.
func (s *Service) GetTTSModels(ctx context.Context, providerID string) []TTSModelInfo {
	if providerID == "" {
		return []TTSModelInfo{}
	}
	if providerID == "webspeech" || providerID == "sapi5" {
		return []TTSModelInfo{}
	}
	client := s.CreateTTSClient(ctx, providerID, "")
	if client == nil {
		logging.Errorf(ctx, "speech.service", "[GetTTSModels] não foi possível criar client para provider %s", providerID)
		return []TTSModelInfo{}
	}
	models, err := client.FetchTTSModels(ctx)
	if err != nil {
		logging.Errorf(ctx, "speech.service", "[GetTTSModels] erro ao buscar modelos para %s: %v", providerID, err)
		return []TTSModelInfo{}
	}
	return models
}

// GetTTSVoices retorna vozes TTS disponíveis para um provedor e modelo.
func (s *Service) GetTTSVoices(ctx context.Context, providerID, modelID string) []TTSVoiceInfo {
	if providerID == "" {
		return []TTSVoiceInfo{}
	}

	// SAPI5: vozes do Windows
	if providerID == "sapi5" {
		manager := GetSAPI5Manager()
		voices := manager.GetVoices()
		result := make([]TTSVoiceInfo, len(voices))
		for i, v := range voices {
			result[i] = TTSVoiceInfo{
				ID:          v.Name,
				Name:        v.Name,
				Gender:      v.Gender,
				Description: v.Description,
			}
		}
		return result
	}

	// WebSpeech: vozes gerenciadas pelo browser
	if providerID == "webspeech" {
		return []TTSVoiceInfo{}
	}
	if modelID == "" {
		logging.Infof(ctx, "speech.service", "[GetTTSVoices] model é obrigatório para provider %s", providerID)
		return []TTSVoiceInfo{}
	}

	// Provedores LLM: consulta via TTSClient
	client := s.CreateTTSClient(ctx, providerID, modelID)
	if client == nil {
		logging.Errorf(ctx, "speech.service", "[GetTTSVoices] não foi possível criar client para provider %s", providerID)
		return []TTSVoiceInfo{}
	}

	voices, err := client.FetchVoices(ctx, modelID)
	if err != nil {
		logging.Errorf(ctx, "speech.service", "[GetTTSVoices] erro ao buscar vozes para %s: %v", providerID, err)
		return []TTSVoiceInfo{}
	}

	return voices
}

// GetSTTModels retorna modelos STT disponíveis para um provedor.
func (s *Service) GetSTTModels(ctx context.Context, providerID string) []SpeechModelInfo {
	if providerID == "" {
		return []SpeechModelInfo{}
	}
	client := s.CreateTTSClient(ctx, providerID, "")
	if client == nil {
		return StaticSTTModels()
	}
	return client.FetchSTTModels(ctx)
}

// SynthesizeStream executa síntese com streaming, emitindo eventos via Emitter.
func (s *Service) SynthesizeStream(ctx context.Context, text string, voice string, sessionID string) error {
	if !s.EnsureSpeechManager(ctx) {
		s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
			SessionID: sessionID,
			Error:     "speech manager não disponível - configure um provedor no perfil",
		})
		return fmt.Errorf("speech manager não disponível")
	}

	if !s.speechManager.SupportsStreaming() {
		go func() {
			result, err := s.speechManager.SynthesizeWithVoice(text, voice)
			if err != nil {
				s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
				return
			}
			s.emitter.Emit(EventTTSStreamStart, TTSStreamEvent{
				SessionID: sessionID,
				Format:    result.Format,
			})
			s.emitter.Emit(EventTTSStreamChunk, TTSStreamEvent{
				SessionID:   sessionID,
				ChunkBase64: result.AudioBase64,
				Format:      result.Format,
			})
			s.emitter.Emit(EventTTSStreamDone, TTSStreamEvent{
				SessionID: sessionID,
				Done:      true,
			})
		}()
		return nil
	}

	go func() {
		s.emitter.Emit(EventTTSStreamStart, TTSStreamEvent{
			SessionID: sessionID,
			Format:    "mp3",
		})

		callbacks := StreamCallbacks{
			OnChunk: func(chunkBase64 string) {
				s.emitter.Emit(EventTTSStreamChunk, TTSStreamEvent{
					SessionID:   sessionID,
					ChunkBase64: chunkBase64,
					Format:      "mp3",
				})
			},
			OnDone: func() {
				s.emitter.Emit(EventTTSStreamDone, TTSStreamEvent{
					SessionID: sessionID,
					Done:      true,
				})
			},
			OnError: func(err error) {
				logging.Errorf(ctx, "speech.service", "[TTS] Stream error: %v", err)
				s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
			},
		}

		ctx, cancel := context.WithTimeout(ctx, CalcTTSTimeout(len(text)))
		defer cancel()

		err := s.speechManager.SynthesizeStream(ctx, text, voice, callbacks)
		if err != nil {
			s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
				SessionID: sessionID,
				Error:     err.Error(),
			})
		}
	}()

	return nil
}

// SpeakPreviewParams agrupa os parâmetros de preview de voz.
type SpeakPreviewParams struct {
	ProviderID string
	VoiceID    string
	Model      string
	Language   string
	Rate       float64
	Volume     float64
	Text       string
	SessionID  string
}

// SpeakPreview faz preview de voz para configurações de perfil.
func (s *Service) SpeakPreview(ctx context.Context, p SpeakPreviewParams) error {
	text := p.Text
	if text == "" {
		return fmt.Errorf("texto de preview é obrigatório")
	}

	rate := p.Rate
	if rate <= 0 {
		rate = 1.0
	}
	volume := p.Volume
	if volume <= 0 {
		volume = 1.0
	}

	logging.Debugf(ctx, "speech.service", "[SpeakPreview] provider=%s, voice=%s, model=%s, language=%s, rate=%.2f, volume=%.2f", p.ProviderID, p.VoiceID, p.Model, p.Language, rate, volume)

	switch p.ProviderID {
	case "webspeech":
		return fmt.Errorf("webspeech preview deve ser feito no frontend")
	case "sapi5":
		return s.previewSAPI5(text, p.VoiceID, rate, volume)
	default:
		return s.previewLLM(ctx, p.ProviderID, text, p.VoiceID, p.Model, p.Language, rate, p.SessionID)
	}
}

func (s *Service) previewSAPI5(text, voiceID string, rate, volume float64) error {
	manager := GetSAPI5Manager()
	sapiRate := mapRateToSAPI5(rate)
	sapiVolume := int(volume * 100)
	if err := manager.SetRate(sapiRate); err != nil {
		logging.Errorf(context.Background(), "speech.service", "[SpeakPreview] SetRate error: %v", err)
	}
	if err := manager.SetVolume(sapiVolume); err != nil {
		logging.Errorf(context.Background(), "speech.service", "[SpeakPreview] SetVolume error: %v", err)
	}
	return manager.Speak(text, voiceID)
}

func (s *Service) previewLLM(ctx context.Context, providerID, text, voiceID, model, language string, rate float64, sessionID string) error {
	if err := validateTTSSelection(model, voiceID, ""); err != nil {
		return err
	}
	client := s.CreateTTSClientWithLanguage(ctx, providerID, model, language)
	if client == nil {
		s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
			SessionID: sessionID,
			Error:     fmt.Sprintf("provider TTS %q não encontrado", providerID),
		})
		return fmt.Errorf("no TTS provider available for %s", providerID)
	}

	client.SetSpeed(rate)

	go func() {
		s.emitter.Emit(EventTTSStreamStart, TTSStreamEvent{
			SessionID: sessionID,
			Format:    "mp3",
		})

		callbacks := TTSStreamCallbacks{
			OnChunk: func(chunk []byte) {
				chunkBase64 := base64.StdEncoding.EncodeToString(chunk)
				s.emitter.Emit(EventTTSStreamChunk, TTSStreamEvent{
					SessionID:   sessionID,
					ChunkBase64: chunkBase64,
					Format:      "mp3",
				})
			},
			OnDone: func() {
				s.emitter.Emit(EventTTSStreamDone, TTSStreamEvent{
					SessionID: sessionID,
					Done:      true,
				})
			},
			OnError: func(err error) {
				logging.Errorf(ctx, "speech.service", "[SpeakPreview] Stream error: %v", err)
				s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
			},
		}

		ctx, cancel := context.WithTimeout(ctx, CalcTTSTimeout(len(text)))
		defer cancel()

		var err error
		if voiceID == "" {
			err = client.SynthesizeStream(ctx, text, callbacks)
		} else {
			err = client.SynthesizeStreamWithVoice(ctx, text, TTSVoice(voiceID), callbacks)
		}
		if err != nil {
			s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
				SessionID: sessionID,
				Error:     err.Error(),
			})
		}
	}()

	return nil
}

// Transcribe transcreve áudio via speech manager (Whisper STT).
func (s *Service) Transcribe(ctx context.Context, audioBase64, filename string) (*TranscriptionResult, error) {
	if !s.EnsureSpeechManager(ctx) {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
	}
	return s.speechManager.Transcribe(audioBase64, filename)
}

// Synthesize sintetiza texto via speech manager (TTS padrão).
func (s *Service) Synthesize(ctx context.Context, text string) (*SynthesisResult, error) {
	if !s.EnsureSpeechManager(ctx) {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
	}
	return s.speechManager.Synthesize(text)
}

// SynthesizeWithVoice sintetiza texto com voz específica via speech manager.
func (s *Service) SynthesizeWithVoice(ctx context.Context, text, voice string) (*SynthesisResult, error) {
	if !s.EnsureSpeechManager(ctx) {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
	}
	return s.speechManager.SynthesizeWithVoice(text, voice)
}

// SetTTSVoice altera a voz do TTS no speech manager.
func (s *Service) SetTTSVoice(voice string) {
	if s.speechManager != nil {
		s.speechManager.SetTTSVoice(voice)
	}
}

// SetTTSSpeed altera a velocidade do TTS no speech manager.
func (s *Service) SetTTSSpeed(speed float64) {
	if s.speechManager != nil {
		s.speechManager.SetTTSSpeed(speed)
	}
}

// InitFromConfig cria um speech manager a partir de config explícita.
func (s *Service) InitFromConfig(cfg SpeechConfig) {
	s.speechManager = NewSpeechManager(cfg, s.credMgr)
}

// GetAvailableVoices retorna vozes OpenAI disponíveis.
func (s *Service) GetAvailableVoices() []TTSVoiceInfo {
	return GetAvailableVoices()
}

// GetMessageAudio retorna o áudio cached de uma mensagem.
func (s *Service) GetMessageAudio(ctx context.Context, messageID string) (*AudioResult, error) {
	audio, mime, err := s.audioRepo.GetMessageAudio(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar áudio: %w", err)
	}
	if audio == "" {
		return nil, nil
	}
	return &AudioResult{Audio: audio, MimeType: mime, Cached: true}, nil
}

// SaveMessageAudio salva áudio (base64) numa mensagem existente.
func (s *Service) SaveMessageAudio(ctx context.Context, messageID string, audioBase64, mimeType string) error {
	return s.audioRepo.SaveMessageAudio(ctx, messageID, audioBase64, mimeType)
}

// GetSpeechProviders retorna provedores LLM que suportam TTS ou STT.
func (s *Service) GetSpeechProviders() []*llm.ProviderConfig {
	all := s.registry.List()
	result := make([]*llm.ProviderConfig, 0, len(all))
	for _, p := range all {
		if p.SupportsTTS() || p.SupportsSTT() {
			result = append(result, p)
		}
	}
	return result
}

// AudioResult é o resultado de busca/geração de áudio.
type AudioResult struct {
	Audio    string `json:"audio"`
	MimeType string `json:"mimeType"`
	Cached   bool   `json:"cached"`
}

// TTSStreamEvent evento de streaming de TTS.
type TTSStreamEvent struct {
	SessionID   string `json:"sessionId"`
	ChunkBase64 string `json:"chunkBase64"`
	Format      string `json:"format"`
	Done        bool   `json:"done"`
	Error       string `json:"error"`
}
