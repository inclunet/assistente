package speech

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// ProviderRegistry abstrai o acesso ao registro de provedores LLM.
type ProviderRegistry interface {
	Get(id string) *llm.ProviderConfig
	List() []*llm.ProviderConfig
}

// ProfileProvider abstrai o acesso ao perfil ativo e resolução de defaults.
type ProfileProvider interface {
	GetActive() (*profiles.Profile, error)
	ResolveDefaults(p *profiles.Profile) *profiles.Profile
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

// GetSpeechManager retorna o speech manager atual (pode ser nil).
func (s *Service) GetSpeechManager() *SpeechManager {
	return s.speechManager
}

// SetSpeechManager permite que a camada Wails injete um speech manager.
func (s *Service) SetSpeechManager(sm *SpeechManager) {
	s.speechManager = sm
}

// InitFromProfile inicializa o speech manager a partir do perfil ativo.
func (s *Service) InitFromProfile() error {
	p, err := s.profileProvider.GetActive()
	if err != nil || p == nil {
		return fmt.Errorf("perfil ativo não encontrado: %w", err)
	}
	resolved := s.profileProvider.ResolveDefaults(p)
	sm := NewSpeechManagerFromProfile(resolved, s.registry, s.credMgr)
	if sm == nil {
		return fmt.Errorf("falha ao criar speech manager para perfil ativo")
	}
	s.speechManager = sm
	return nil
}

// EnsureSpeechManager garante que o speechManager está inicializado.
func (s *Service) EnsureSpeechManager() bool {
	if s.speechManager != nil {
		return true
	}
	if err := s.InitFromProfile(); err != nil {
		log.Printf("[Speech] Erro ao inicializar speechManager do perfil: %v", err)
		return false
	}
	return s.speechManager != nil
}

// CreateManagerForProfile cria um SpeechManager configurado a partir de um perfil.
// Resolve defaults ($default → ID real) e delega para NewSpeechManagerFromProfile.
// Retorna nil se p for nil ou se a criação falhar.
func (s *Service) CreateManagerForProfile(p *profiles.Profile) *SpeechManager {
	if p == nil {
		return nil
	}
	resolved := s.profileProvider.ResolveDefaults(p)
	return NewSpeechManagerFromProfile(resolved, s.registry, s.credMgr)
}

// CreateTTSClient cria um TTSClient para um provider LLM específico.
func (s *Service) CreateTTSClient(providerID string, model string) *TTSClient {
	cfg := s.registry.Get(providerID)
	if cfg == nil {
		log.Printf("[TTS] Provider %s não encontrado", providerID)
		return nil
	}
	return NewTTSClient(TTSConfig{
		BaseURL:           cfg.BaseURL,
		CredentialPattern: cfg.CredentialPattern,
		Model:             TTSModel(model),
	}, s.credMgr)
}

// FindOpenAILikeProvider procura um provider LLM com API OpenAI-compatible que suporte TTS.
func (s *Service) FindOpenAILikeProvider() *llm.ProviderConfig {
	isOpenAILike := func(cfg *llm.ProviderConfig) bool {
		if cfg == nil {
			return false
		}
		format := cfg.GetAPIFormat()
		return format == llm.APIFormatOpenAI || format == llm.APIFormatOpenAIResponses
	}

	isOfficialOpenAI := func(cfg *llm.ProviderConfig) bool {
		return cfg.BaseURL == "" || strings.Contains(cfg.BaseURL, "api.openai.com")
	}

	// Tenta o provider do perfil ativo primeiro (voice → chat → default)
	if profile, err := s.profileProvider.GetActive(); err == nil && profile != nil {
		resolved := s.profileProvider.ResolveDefaults(profile)
		if resolved.Voice.Assistant.LLMProviderID != "" {
			if cfg := s.registry.Get(resolved.Voice.Assistant.LLMProviderID); isOpenAILike(cfg) {
				return cfg
			}
		}
		if resolved.Chat.LLMProvider != "" {
			if cfg := s.registry.Get(resolved.Chat.LLMProvider); isOpenAILike(cfg) {
				return cfg
			}
		}
	}

	// Tenta o provider default do sistema
	for _, cfg := range s.registry.List() {
		if cfg.IsDefault && isOpenAILike(cfg) {
			return cfg
		}
	}

	// Último recurso: prefere providers com URL oficial do OpenAI
	var fallbackProxy *llm.ProviderConfig
	for _, cfg := range s.registry.List() {
		if isOpenAILike(cfg) {
			if isOfficialOpenAI(cfg) {
				return cfg
			}
			if fallbackProxy == nil {
				fallbackProxy = cfg
			}
		}
	}

	return fallbackProxy
}

// SpeakMessage retorna o áudio de uma mensagem, usando cache do DB se disponível.
func (s *Service) SpeakMessage(messageID uint, providerID string, voiceID string, model string, rate float64) (*AudioResult, error) {
	// 1. Checa cache no DB
	audio, mime, err := s.audioRepo.GetMessageAudio(messageID)
	if err == nil && audio != "" {
		log.Printf("[TTS] SpeakMessage(%d): cache hit (%d bytes)", messageID, len(audio))
		return &AudioResult{Audio: audio, MimeType: mime, Cached: true}, nil
	}

	// 2. Busca o conteúdo textual da mensagem
	content, err := s.audioRepo.GetMessageContent(messageID)
	if err != nil {
		log.Printf("[TTS] SpeakMessage(%d): mensagem não encontrada: %v", messageID, err)
		return nil, fmt.Errorf("mensagem %d não encontrada: %w", messageID, err)
	}
	if strings.TrimSpace(content) == "" {
		log.Printf("[TTS] SpeakMessage(%d): conteúdo vazio", messageID)
		return nil, fmt.Errorf("mensagem %d sem conteúdo textual", messageID)
	}

	// 3. Cria TTS client com os parâmetros
	log.Printf("[TTS] SpeakMessage(%d): cache miss, gerando TTS (%d chars) provider=%s voice=%s model=%s", messageID, len(content), providerID, voiceID, model)

	if model == "" {
		model = voiceID
	}
	speed := rate
	if speed < 0.25 {
		speed = 1.0
	}

	client := s.CreateTTSClient(providerID, model)
	if client == nil {
		return nil, fmt.Errorf("provider TTS %q não encontrado", providerID)
	}
	client.SetVoice(TTSVoice(voiceID))
	client.SetSpeed(speed)

	audioData, err := client.Synthesize(content)
	if err != nil {
		log.Printf("[TTS] SpeakMessage(%d): erro ao gerar: %v", messageID, err)
		return nil, fmt.Errorf("generate audio for message %d: %w", messageID, err)
	}

	audioBase64 := base64.StdEncoding.EncodeToString(audioData)
	mimeType := "audio/mpeg"

	cached := true
	if saveErr := s.audioRepo.SaveMessageAudio(messageID, audioBase64, mimeType); saveErr != nil {
		log.Printf("[TTS] WARN: falha ao salvar áudio no DB (messageID=%d): %v — áudio será retornado mas não persistido", messageID, saveErr)
		cached = false
	}

	return &AudioResult{Audio: audioBase64, MimeType: mimeType, Cached: cached}, nil
}

// GenerateAndSaveMessageAudio gera áudio TTS para uma mensagem e salva no DB.
func (s *Service) GenerateAndSaveMessageAudio(messageID uint, text string) (*AudioResult, error) {
	if !s.EnsureSpeechManager() {
		return nil, fmt.Errorf("speech manager indisponível")
	}

	result, err := s.speechManager.Synthesize(text)
	if err != nil {
		return nil, fmt.Errorf("generate audio for message %d: %w", messageID, err)
	}

	mimeType := "audio/mpeg"
	cached := true
	if err := s.audioRepo.SaveMessageAudio(messageID, result.AudioBase64, mimeType); err != nil {
		log.Printf("[TTS] WARN: falha ao salvar áudio no DB (messageID=%d): %v — áudio será retornado mas não persistido", messageID, err)
		cached = false
	}

	return &AudioResult{Audio: result.AudioBase64, MimeType: mimeType, Cached: cached}, nil
}

// GetTTSVoices retorna vozes TTS disponíveis para um provedor.
func (s *Service) GetTTSVoices(providerID string) []TTSVoiceEntry {
	if providerID == "" {
		return []TTSVoiceEntry{}
	}

	// SAPI5: vozes do Windows
	if providerID == "sapi5" {
		manager := GetSAPI5Manager()
		if err := manager.Initialize(); err != nil {
			log.Printf("[GetTTSVoices] erro SAPI5: %v", err)
			return []TTSVoiceEntry{}
		}
		voices := manager.GetVoices()
		result := make([]TTSVoiceEntry, len(voices))
		for i, v := range voices {
			result[i] = TTSVoiceEntry{
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
		return []TTSVoiceEntry{}
	}

	// Provedores LLM: consulta via TTSClient
	client := s.CreateTTSClient(providerID, "")
	if client == nil {
		log.Printf("[GetTTSVoices] não foi possível criar client para provider %s", providerID)
		return []TTSVoiceEntry{}
	}

	voices, err := client.FetchVoices()
	if err != nil {
		log.Printf("[GetTTSVoices] erro ao buscar vozes para %s: %v", providerID, err)
		return []TTSVoiceEntry{}
	}

	result := make([]TTSVoiceEntry, len(voices))
	for i, v := range voices {
		result[i] = TTSVoiceEntry{
			ID:          v.ID,
			Name:        v.Name,
			Gender:      v.Gender,
			Description: v.Description,
		}
	}
	return result
}

// GetTTSModels retorna modelos TTS disponíveis para um provedor.
func (s *Service) GetTTSModels(providerID string) []SpeechModelInfo {
	if providerID == "" {
		return []SpeechModelInfo{}
	}
	client := s.CreateTTSClient(providerID, "")
	if client == nil {
		return StaticTTSModels()
	}
	return client.FetchTTSModels()
}

// GetSTTModels retorna modelos STT disponíveis para um provedor.
func (s *Service) GetSTTModels(providerID string) []SpeechModelInfo {
	if providerID == "" {
		return []SpeechModelInfo{}
	}
	client := s.CreateTTSClient(providerID, "")
	if client == nil {
		return StaticSTTModels()
	}
	return client.FetchSTTModels()
}

// SynthesizeStream executa síntese com streaming, emitindo eventos via Emitter.
func (s *Service) SynthesizeStream(text string, voice string, sessionID string) error {
	if !s.EnsureSpeechManager() {
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
				log.Printf("[TTS] Stream error: %v", err)
				s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), CalcTTSTimeout(len(text)))
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

// SpeakPreview faz preview de voz para configurações de perfil.
func (s *Service) SpeakPreview(providerID string, voiceID string, model string, rate float64, volume float64, text string, sessionID string) error {
	if text == "" {
		text = "Este é um teste das configurações de voz"
	}

	if rate <= 0 {
		rate = 1.0
	}
	if volume <= 0 {
		volume = 1.0
	}

	log.Printf("[SpeakPreview] provider=%s, voice=%s, model=%s, rate=%.2f, volume=%.2f", providerID, voiceID, model, rate, volume)

	switch providerID {
	case "webspeech":
		return fmt.Errorf("webspeech preview deve ser feito no frontend")
	case "sapi5":
		return s.previewSAPI5(text, voiceID, rate, volume)
	default:
		return s.previewLLM(providerID, text, voiceID, model, rate, sessionID)
	}
}

func (s *Service) previewSAPI5(text, voiceID string, rate, volume float64) error {
	manager := GetSAPI5Manager()
	sapiRate := int((rate - 1.0) * 10)
	sapiVolume := int(volume * 100)
	if err := manager.SetRate(sapiRate); err != nil {
		log.Printf("[SpeakPreview] SetRate error: %v", err)
	}
	if err := manager.SetVolume(sapiVolume); err != nil {
		log.Printf("[SpeakPreview] SetVolume error: %v", err)
	}
	return manager.Speak(text, voiceID)
}

func (s *Service) previewLLM(providerID, text, voiceID, model string, rate float64, sessionID string) error {
	client := s.CreateTTSClient(providerID, model)
	if client == nil {
		if fallback := s.FindOpenAILikeProvider(); fallback != nil {
			client = s.CreateTTSClient(fallback.ID, model)
		}
	}
	if client == nil {
		s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
			SessionID: sessionID,
			Error:     "nenhum provedor OpenAI com credenciais encontrado",
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
				log.Printf("[SpeakPreview] Stream error: %v", err)
				s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), CalcTTSTimeout(len(text)))
		defer cancel()

		voice := TTSVoice(voiceID)
		if IsDynamicTTSModel(voiceID) {
			client.SetModel(TTSModel(voiceID))
			voice = TTSVoice(voiceID)
		}

		if err := client.SynthesizeStreamWithVoice(ctx, text, voice, callbacks); err != nil {
			s.emitter.Emit(EventTTSStreamError, TTSStreamEvent{
				SessionID: sessionID,
				Error:     err.Error(),
			})
		}
	}()

	return nil
}

// Transcribe transcreve áudio via speech manager (Whisper STT).
func (s *Service) Transcribe(audioBase64, filename string) (*TranscriptionResult, error) {
	if !s.EnsureSpeechManager() {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
	}
	return s.speechManager.Transcribe(audioBase64, filename)
}

// Synthesize sintetiza texto via speech manager (TTS padrão).
func (s *Service) Synthesize(text string) (*SynthesisResult, error) {
	if !s.EnsureSpeechManager() {
		return nil, fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
	}
	return s.speechManager.Synthesize(text)
}

// SynthesizeWithVoice sintetiza texto com voz específica via speech manager.
func (s *Service) SynthesizeWithVoice(text, voice string) (*SynthesisResult, error) {
	if !s.EnsureSpeechManager() {
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

// GetSAPI5Voices retorna as vozes SAPI5 instaladas.
func (s *Service) GetSAPI5Voices() []Voice {
	manager := GetSAPI5Manager()
	if err := manager.Initialize(); err != nil {
		log.Printf("SAPI5 Initialize error (may be expected on non-Windows): %v", err)
		return nil
	}
	return manager.GetVoices()
}

// SpeakSAPI5 sintetiza texto usando SAPI5.
func (s *Service) SpeakSAPI5(text, voiceName string) error {
	return GetSAPI5Manager().Speak(text, voiceName)
}

// StopSAPI5 para a síntese SAPI5 atual.
func (s *Service) StopSAPI5() error {
	return GetSAPI5Manager().Stop()
}

// SetSAPI5Volume define volume SAPI5 (0-100).
func (s *Service) SetSAPI5Volume(volume int) error {
	return GetSAPI5Manager().SetVolume(volume)
}

// SetSAPI5Rate define velocidade SAPI5 (-10 a 10).
func (s *Service) SetSAPI5Rate(rate int) error {
	return GetSAPI5Manager().SetRate(rate)
}

// IsSAPI5Speaking verifica se SAPI5 está falando.
func (s *Service) IsSAPI5Speaking() bool {
	return GetSAPI5Manager().IsSpeaking()
}

// GetAvailableVoices retorna vozes OpenAI disponíveis.
func (s *Service) GetAvailableVoices() []TTSVoiceInfo {
	return GetAvailableVoices()
}

// PreviewVoiceSettings reproduz texto de teste com configurações ad-hoc (legacy).
func (s *Service) PreviewVoiceSettings(provider, voiceID string, rate, volume float64, sampleText string) error {
	if sampleText == "" {
		sampleText = "Este é um teste das configurações de voz"
	}
	if !s.EnsureSpeechManager() {
		return fmt.Errorf("speech manager não disponível - configure um provedor no perfil")
	}
	if provider == "openai" {
		s.speechManager.SetTTSVoice(voiceID)
	}
	result, err := s.speechManager.SynthesizeWithVoice(sampleText, voiceID)
	if err != nil {
		return fmt.Errorf("erro ao sintetizar: %w", err)
	}
	s.emitter.Emit("voice_profile:preview", map[string]interface{}{
		"audio_base64": result.AudioBase64,
		"format":       result.Format,
	})
	return nil
}

// GetMessageAudio retorna o áudio cached de uma mensagem.
func (s *Service) GetMessageAudio(messageID uint) (*AudioResult, error) {
	audio, mime, err := s.audioRepo.GetMessageAudio(messageID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar áudio: %w", err)
	}
	if audio == "" {
		return nil, nil
	}
	return &AudioResult{Audio: audio, MimeType: mime, Cached: true}, nil
}

// SaveMessageAudio salva áudio (base64) numa mensagem existente.
func (s *Service) SaveMessageAudio(messageID uint, audioBase64, mimeType string) error {
	return s.audioRepo.SaveMessageAudio(messageID, audioBase64, mimeType)
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

// TTSVoiceEntry é o formato de voz retornado para o frontend.
type TTSVoiceEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Gender      string `json:"gender"`
	Description string `json:"description"`
}
