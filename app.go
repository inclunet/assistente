package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/llm"
	"assistente/internal/speech"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx                   context.Context
	llmClient             *llm.SyncClient
	speechManager         *speech.SpeechManager
	hotkeyManager         *hotkey.Manager
	voiceHotkeyID         int
	currentConversationID uint // ID da conversa atual

	// Throttle para hotkeys - evita disparo repetido quando segura a tecla
	hotkeyLastFired  map[uint]time.Time
	hotkeyThrottleMs int64 // tempo mínimo entre disparos (em ms)
}

// ==================== Tipos para Threads ====================

// EnrichedMessage é ChatMessage + campos derivados calculados no backend
// Todos os campos são definidos explicitamente para evitar conflitos de embedding
type EnrichedMessage struct {
	ID               string    `json:"id"` // String para JS safety (números grandes)
	ConversationID   uint      `json:"conversationId"`
	ParentID         *string   `json:"parentId,omitempty"` // String para evitar undefined no TypeScript
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	Reasoning        string    `json:"reasoning,omitempty"` // Reasoning/thinking do modelo (DeepSeek, Claude, o1)
	Media            string    `json:"media,omitempty"`
	PromptTokens     int       `json:"promptTokens,omitempty"`
	CompletionTokens int       `json:"completionTokens,omitempty"`
	TotalTokens      int       `json:"totalTokens,omitempty"`
	Model            string    `json:"model,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	Timestamp        int64     `json:"timestamp"`   // Milliseconds desde epoch
	IsStreaming      bool      `json:"isStreaming"`  // Sempre false do DB
	Internal         bool      `json:"internal"`     // Se tem parentId (é resposta de thread)
}

// MessageNode representa uma mensagem com seus filhos na hierarquia
type MessageNode struct {
	Message    EnrichedMessage `json:"message"` // Mensagem enriquecida
	Children   []MessageNode   `json:"children,omitempty"`
	Level      int             `json:"level"`
	ChildCount int             `json:"childCount"` // Para lazy loading
}

// ConversationWithThreads representa uma conversa com mensagens organizadas em árvore
type ConversationWithThreads struct {
	ID          uint                      `json:"id"`
	Title       string                    `json:"title"`
	Preferences *database.ChatPreferences `json:"preferences,omitempty"`
	Threads     []MessageNode             `json:"threads"`
}

// StreamEvent representa um evento de streaming simplificado
type StreamEvent struct {
	MessageID      uint   `json:"messageId"`
	ConversationId uint   `json:"conversationId"`
	Content        string `json:"content"`
	Done           bool   `json:"done"`
	FullResponse   string `json:"fullResponse,omitempty"`
	Error          string `json:"error,omitempty"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		hotkeyLastFired:  make(map[uint]time.Time),
		hotkeyThrottleMs: 1000, // 1000ms entre disparos (1 segundo)
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Inicializa o banco de dados
	if err := InitDatabase(); err != nil {
		log.Printf("Erro ao inicializar banco de dados: %v", err)
	}

	// Inicializa o cliente LLM
	a.initLLMClient()

	// Inicializa hotkeys globais
	a.initGlobalHotkeys()
}

// initLLMClient inicializa o cliente LLM
func (a *App) initLLMClient() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Erro ao carregar config para LLM: %v", err)
		return
	}

	// Configura o timeout de resposta HTTP baseado na config
	llm.ConfigureResponseTimeout(cfg.GetResponseTimeout())
	log.Printf("HTTP Response Timeout configurado para %d segundos", cfg.GetResponseTimeout())

	if cfg.APIKey == "" {
		log.Printf("API Key não configurada")
		return
	}

	a.llmClient = llm.NewSyncClient(cfg.APIBaseURL, cfg.APIKey)
	log.Printf("LLM Client inicializado")
}

// ReloadLLMClient recarrega o cliente LLM (chamado quando config muda)
func (a *App) ReloadLLMClient() {
	a.initLLMClient()
}

// shutdown é chamado quando o app fecha
func (a *App) shutdown(ctx context.Context) {
	// Para hotkeys globais
	if a.hotkeyManager != nil {
		a.hotkeyManager.Stop()
	}
}

// initGlobalHotkeys inicializa o gerenciador de hotkeys
// Os hotkeys são registrados pelos triggers dos perfis de interação
func (a *App) initGlobalHotkeys() {
	if !hotkey.IsSupported() {
		log.Println("[Hotkey] Hotkeys globais não suportados neste sistema")
		return
	}

	a.hotkeyManager = hotkey.GetManager()
	log.Println("[Hotkey] Manager inicializado. Hotkeys serão registrados pelos triggers dos perfis.")
}

// ============================================================================
// Global Hotkey API
// ============================================================================

// HotkeyInfo informações sobre um hotkey
type HotkeyInfo struct {
	ID          int    `json:"id"`
	Modifiers   string `json:"modifiers"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// IsGlobalHotkeySupported verifica se hotkeys globais são suportados
func (a *App) IsGlobalHotkeySupported() bool {
	return hotkey.IsSupported()
}

// GetGlobalHotkeys retorna os hotkeys globais configurados
// DEPRECATED: Use os triggers dos perfis de interação para configurar hotkeys
func (a *App) GetGlobalHotkeys() []HotkeyInfo {
	// Hotkeys são agora gerenciados pelos triggers dos perfis de interação
	return []HotkeyInfo{}
}

// SetVoiceHotkey altera o hotkey de voz
// DEPRECATED: Use os triggers dos perfis de interação para configurar hotkeys
func (a *App) SetVoiceHotkey(modifiers string, key string) error {
	return fmt.Errorf("deprecated: use interaction profile triggers to configure hotkeys")
}

// DisableVoiceHotkey desativa o hotkey de voz
// DEPRECATED: Use os triggers dos perfis de interação para configurar hotkeys
func (a *App) DisableVoiceHotkey() error {
	return nil
}

// EnableVoiceHotkey reativa o hotkey de voz com configuração padrão
// DEPRECATED: Use os triggers dos perfis de interação para configurar hotkeys
func (a *App) EnableVoiceHotkey() error {
	return fmt.Errorf("deprecated: use interaction profile triggers to configure hotkeys")
}

// ==================== Interaction Profile Hotkeys ====================

// RegisterInteractionProfileHotkeys registra os hotkeys de um perfil de interação
// Itera pelos triggers do perfil e registra hotkeys para triggers que possuem
func (a *App) RegisterInteractionProfileHotkeys(profileID uint) error {
	if a.hotkeyManager == nil {
		log.Printf("[Hotkey] Manager não inicializado!")
		return fmt.Errorf("hotkey manager not initialized")
	}

	// Busca o perfil com triggers
	profile, err := database.GetInteractionProfile(profileID)
	if err != nil {
		log.Printf("[Hotkey] Perfil %d não encontrado: %v", profileID, err)
		return fmt.Errorf("profile not found: %w", err)
	}

	log.Printf("[Hotkey] Perfil %d (%s) tem %d triggers", profileID, profile.Name, len(profile.Triggers))

	// Remove hotkeys anteriores deste perfil
	a.hotkeyManager.UnregisterProfileHotkeys(int(profileID))

	// Registra hotkeys para cada trigger que possui hotkey configurada
	hotkeyCount := 0
	for _, trigger := range profile.Triggers {
		log.Printf("[Hotkey] Trigger %d: type=%s, enabled=%v, hotkey='%s'", trigger.ID, trigger.Type, trigger.Enabled, trigger.Hotkey)
		if !trigger.Enabled || trigger.Hotkey == "" {
			log.Printf("[Hotkey] Trigger %d ignorado (enabled=%v, hotkey vazio=%v)", trigger.ID, trigger.Enabled, trigger.Hotkey == "")
			continue
		}
		hotkeyCount++

		// Captura variáveis para closure
		t := trigger

		log.Printf("[Hotkey] Registrando hotkey '%s' para trigger %d...", t.Hotkey, t.ID)
		_, err := a.hotkeyManager.RegisterProfileHotkey(
			int(profileID),
			t.Hotkey,
			t.Type == database.TriggerTypeHotkey, // isPrimary: só hotkey direto é "principal"
			t.HotkeyBringToFront,
			func() {
				// Throttle: ignora se disparou recentemente (evita loop quando segura tecla)
				now := time.Now()
				if lastFired, ok := a.hotkeyLastFired[t.ID]; ok {
					elapsed := now.Sub(lastFired).Milliseconds()
					if elapsed < a.hotkeyThrottleMs {
						log.Printf("[Hotkey] BLOQUEADO por throttle: trigger %d, elapsed=%dms < %dms", t.ID, elapsed, a.hotkeyThrottleMs)
						return // Ignora - muito rápido
					}
				}
				a.hotkeyLastFired[t.ID] = now

				log.Printf("[Hotkey] HOTKEY ACIONADA! Trigger %d, perfil %d (throttle OK)", t.ID, profileID)
				// Emite evento para frontend com informações do trigger
				runtime.EventsEmit(a.ctx, "interaction:hotkey:triggered", map[string]interface{}{
					"triggerId":    t.ID,
					"profileId":    profileID,
					"triggerType":  t.Type,
					"bringToFront": t.HotkeyBringToFront,
				})

				// Se deve trazer janela para frente
				if t.HotkeyGlobal && t.HotkeyBringToFront {
					runtime.WindowShow(a.ctx)
				}
			},
		)
		if err != nil {
			log.Printf("[Hotkey] ERRO ao registrar hotkey do trigger %d (perfil %d): %v", t.ID, profileID, err)
			// Continua para os outros triggers
		} else {
			log.Printf("[Hotkey] Hotkey '%s' registrada com sucesso para trigger %d", t.Hotkey, t.ID)
		}
	}

	log.Printf("[Hotkey] Total: %d hotkeys registradas para perfil %d", hotkeyCount, profileID)
	return nil
}

// UnregisterInteractionProfileHotkeys remove os hotkeys de um perfil
func (a *App) UnregisterInteractionProfileHotkeys(profileID uint) error {
	if a.hotkeyManager == nil {
		return nil
	}
	return a.hotkeyManager.UnregisterProfileHotkeys(int(profileID))
}

// GetActiveInteractionProfile retorna o perfil de interação atualmente ativo
func (a *App) GetActiveInteractionProfile() *database.InteractionProfile {
	profile, err := database.GetActiveInteractionProfile()
	if err != nil {
		log.Printf("[App] Erro ao buscar perfil ativo: %v", err)
		return nil
	}
	return profile
}

// SetActiveInteractionProfile define e ativa um perfil de interação
// Persiste no banco, registra os hotkeys do perfil e emite evento
func (a *App) SetActiveInteractionProfile(profileID uint) error {
	log.Printf("[SetActiveInteractionProfile] Ativando perfil %d", profileID)

	// Persiste no banco
	if err := database.SetActiveInteractionProfile(profileID); err != nil {
		log.Printf("[SetActiveInteractionProfile] Erro ao persistir: %v", err)
		return err
	}

	// Desregistra hotkeys de todos os perfis
	if a.hotkeyManager != nil {
		log.Printf("[SetActiveInteractionProfile] Desregistrando todas hotkeys...")
		a.hotkeyManager.UnregisterAllProfileHotkeys()
	} else {
		log.Printf("[SetActiveInteractionProfile] AVISO: hotkeyManager é nil!")
	}

	// Registra hotkeys do novo perfil (se não for 0 = desativado)
	if profileID > 0 {
		log.Printf("[SetActiveInteractionProfile] Registrando hotkeys do perfil %d...", profileID)
		if err := a.RegisterInteractionProfileHotkeys(profileID); err != nil {
			log.Printf("[SetActiveInteractionProfile] Erro ao registrar hotkeys: %v", err)
			return err
		}
	} else {
		log.Printf("[SetActiveInteractionProfile] Perfil 0 = desativado, não registrando hotkeys")
	}

	// Emite evento de mudança de perfil
	runtime.EventsEmit(a.ctx, "interaction:profile:activated", map[string]interface{}{
		"profileId": profileID,
	})

	return nil
}

// GetActiveProfileHotkeys retorna os hotkeys registrados para um perfil
func (a *App) GetActiveProfileHotkeys(profileID uint) []map[string]interface{} {
	hotkeys := hotkey.GetProfileHotkeys(int(profileID))
	result := make([]map[string]interface{}, 0, len(hotkeys))

	for _, hk := range hotkeys {
		result = append(result, map[string]interface{}{
			"profileId":    hk.ProfileID,
			"isPrimary":    hk.IsPrimary,
			"combination":  hk.Combination,
			"bringToFront": hk.BringToFront,
			"hotkeyId":     hk.HotkeyID,
		})
	}

	return result
}

// ============================================================================
// SAPI5 Voice Methods (Windows only)
// ============================================================================

// SAPI5VoiceInfo representa informações de uma voz SAPI5 para o frontend
type SAPI5VoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Language    string `json:"language"`
	Gender      string `json:"gender"`
	Age         string `json:"age"`
	Vendor      string `json:"vendor"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// GetSAPI5Voices retorna a lista de vozes SAPI5 instaladas
// Em sistemas não-Windows, retorna lista vazia sem erro
func (a *App) GetSAPI5Voices() ([]SAPI5VoiceInfo, error) {
	manager := speech.GetSAPI5Manager()

	if err := manager.Initialize(); err != nil {
		log.Printf("SAPI5 Initialize error (may be expected on non-Windows): %v", err)
		return []SAPI5VoiceInfo{}, nil
	}

	voices := manager.GetVoices()
	result := make([]SAPI5VoiceInfo, len(voices))

	for i, v := range voices {
		result[i] = SAPI5VoiceInfo{
			ID:          v.ID,
			Name:        v.Name,
			Language:    v.Language,
			Gender:      v.Gender,
			Age:         v.Age,
			Vendor:      v.Vendor,
			Description: v.Description,
			Source:      v.Source,
		}
	}

	return result, nil
}

// SpeakSAPI5 sintetiza texto usando uma voz SAPI5
// Em sistemas não-Windows, não faz nada
func (a *App) SpeakSAPI5(text string, voiceName string) error {
	manager := speech.GetSAPI5Manager()
	return manager.Speak(text, voiceName)
}

// StopSAPI5 para a síntese SAPI5 atual
func (a *App) StopSAPI5() error {
	manager := speech.GetSAPI5Manager()
	return manager.Stop()
}

// SetSAPI5Volume define o volume (0-100)
func (a *App) SetSAPI5Volume(volume int) error {
	manager := speech.GetSAPI5Manager()
	return manager.SetVolume(volume)
}

// SetSAPI5Rate define a velocidade (-10 a 10, 0 é normal)
func (a *App) SetSAPI5Rate(rate int) error {
	manager := speech.GetSAPI5Manager()
	return manager.SetRate(rate)
}

// IsSAPI5Speaking verifica se está falando
func (a *App) IsSAPI5Speaking() bool {
	manager := speech.GetSAPI5Manager()
	return manager.IsSpeaking()
}

// ============================================================================
// OpenAI Speech API Methods (Whisper STT + OpenAI TTS)
// ============================================================================

// OpenAITTSVoiceInfo representa uma voz OpenAI TTS para o frontend
type OpenAITTSVoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Gender      string `json:"gender"`
	Provider    string `json:"provider"`
}

// TranscriptionResultInfo resultado da transcrição para o frontend
type TranscriptionResultInfo struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Provider string  `json:"provider"`
}

// SynthesisResultInfo resultado da síntese para o frontend
type SynthesisResultInfo struct {
	AudioBase64 string `json:"audioBase64"`
	Format      string `json:"format"`
	Provider    string `json:"provider"`
}

// InitSpeechManager inicializa o gerenciador de speech com as configurações
func (a *App) InitSpeechManager(apiKey, apiBaseURL, whisperLanguage, ttsVoice, ttsModel string) error {
	config := speech.SpeechConfig{
		STTProvider:      speech.STTProviderWhisper,
		TTSProvider:      speech.TTSProviderOpenAI,
		OpenAIAPIKey:     apiKey,
		OpenAIAPIBaseURL: apiBaseURL,
		WhisperModel:     "whisper-1",
		WhisperLanguage:  whisperLanguage,
		TTSModel:         ttsModel,
		TTSVoice:         ttsVoice,
	}

	a.speechManager = speech.NewSpeechManager(config)
	log.Printf("Speech Manager inicializado (STT: whisper, TTS: openai)")
	return nil
}

// TranscribeWhisper transcreve áudio usando OpenAI Whisper
// audioBase64: áudio codificado em base64
// filename: nome do arquivo com extensão (ex: "audio.webm")
func (a *App) TranscribeWhisper(audioBase64 string, filename string) (*TranscriptionResultInfo, error) {
	if a.speechManager == nil {
		// Tenta inicializar com as configurações salvas
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")
	}

	result, err := a.speechManager.Transcribe(audioBase64, filename)
	if err != nil {
		return nil, err
	}

	return &TranscriptionResultInfo{
		Text:     result.Text,
		Language: result.Language,
		Duration: result.Duration,
		Provider: result.Provider,
	}, nil
}

// SynthesizeOpenAI sintetiza texto usando OpenAI TTS
func (a *App) SynthesizeOpenAI(text string) (*SynthesisResultInfo, error) {
	if a.speechManager == nil {
		// Tenta inicializar com as configurações salvas
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")
	}

	result, err := a.speechManager.Synthesize(text)
	if err != nil {
		return nil, err
	}

	return &SynthesisResultInfo{
		AudioBase64: result.AudioBase64,
		Format:      result.Format,
		Provider:    result.Provider,
	}, nil
}

// SynthesizeOpenAIWithVoice sintetiza texto usando OpenAI TTS com uma voz específica
func (a *App) SynthesizeOpenAIWithVoice(text string, voice string) (*SynthesisResultInfo, error) {
	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", voice, "tts-1")
	}

	result, err := a.speechManager.SynthesizeWithVoice(text, voice)
	if err != nil {
		return nil, err
	}

	return &SynthesisResultInfo{
		AudioBase64: result.AudioBase64,
		Format:      result.Format,
		Provider:    result.Provider,
	}, nil
}

// TTSStreamEvent evento de streaming de TTS (interface unificada para todos os provedores)
type TTSStreamEvent struct {
	SessionID   string `json:"sessionId"`   // Identificador único da sessão
	ChunkBase64 string `json:"chunkBase64"` // Chunk de áudio em base64 (apenas em tts:stream:chunk)
	Format      string `json:"format"`      // Formato do áudio (mp3, opus, etc)
	Done        bool   `json:"done"`        // True quando streaming terminou
	Error       string `json:"error"`       // Mensagem de erro (apenas em tts:stream:error)
}

// SynthesizeOpenAIStream sintetiza texto usando OpenAI TTS com streaming
// Emite eventos Wails conforme recebe chunks de áudio:
// - "tts:stream:start"  -> { sessionId, format }
// - "tts:stream:chunk"  -> { sessionId, chunkBase64, format }
// - "tts:stream:done"   -> { sessionId, done: true }
// - "tts:stream:error"  -> { sessionId, error }
// IMPORTANTE: Este método retorna imediatamente e executa o streaming em background
func (a *App) SynthesizeOpenAIStream(text string, voice string, sessionID string) error {
	if a.speechManager == nil {
		cfg, err := config.Load()
		if err != nil {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionID,
				Error:     "speech manager not initialized",
			})
			return fmt.Errorf("speech manager not initialized")
		}
		if cfg.APIKey == "" {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionID,
				Error:     "API key not configured",
			})
			return fmt.Errorf("API key not configured")
		}
		a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", voice, "tts-1")
	}

	// Verifica se o provedor suporta streaming
	if !a.speechManager.SupportsStreaming() {
		// Fallback em goroutine separada
		go func() {
			result, err := a.speechManager.SynthesizeWithVoice(text, voice)
			if err != nil {
				runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
				return
			}

			// Emite como streaming de um único chunk
			runtime.EventsEmit(a.ctx, "tts:stream:start", TTSStreamEvent{
				SessionID: sessionID,
				Format:    result.Format,
			})
			runtime.EventsEmit(a.ctx, "tts:stream:chunk", TTSStreamEvent{
				SessionID:   sessionID,
				ChunkBase64: result.AudioBase64,
				Format:      result.Format,
			})
			runtime.EventsEmit(a.ctx, "tts:stream:done", TTSStreamEvent{
				SessionID: sessionID,
				Done:      true,
			})
		}()
		return nil
	}

	// Executa streaming em goroutine separada para não bloquear
	go func() {
		// Emite evento de início
		runtime.EventsEmit(a.ctx, "tts:stream:start", TTSStreamEvent{
			SessionID: sessionID,
			Format:    "mp3",
		})

		// Inicia streaming com callbacks
		callbacks := speech.StreamCallbacks{
			OnChunk: func(chunkBase64 string) {
				runtime.EventsEmit(a.ctx, "tts:stream:chunk", TTSStreamEvent{
					SessionID:   sessionID,
					ChunkBase64: chunkBase64,
					Format:      "mp3",
				})
			},
			OnDone: func() {
				runtime.EventsEmit(a.ctx, "tts:stream:done", TTSStreamEvent{
					SessionID: sessionID,
					Done:      true,
				})
			},
			OnError: func(err error) {
				log.Printf("[TTS] Stream error: %v", err)
				runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
					SessionID: sessionID,
					Error:     err.Error(),
				})
			},
		}

		// Usa contexto com timeout
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := a.speechManager.SynthesizeStream(ctx, text, voice, callbacks)
		if err != nil {
			runtime.EventsEmit(a.ctx, "tts:stream:error", TTSStreamEvent{
				SessionID: sessionID,
				Error:     err.Error(),
			})
		}
	}()

	return nil
}

// GetOpenAITTSVoices retorna as vozes disponíveis do OpenAI TTS
func (a *App) GetOpenAITTSVoices() []OpenAITTSVoiceInfo {
	voices := speech.GetAvailableVoices()
	result := make([]OpenAITTSVoiceInfo, len(voices))

	for i, v := range voices {
		result[i] = OpenAITTSVoiceInfo{
			ID:          v.ID,
			Name:        v.Name,
			Description: v.Description,
			Gender:      v.Gender,
			Provider:    v.Provider,
		}
	}

	return result
}

// SetOpenAITTSVoice altera a voz do OpenAI TTS
func (a *App) SetOpenAITTSVoice(voice string) {
	if a.speechManager != nil {
		a.speechManager.SetTTSVoice(voice)
	}
}

// SetOpenAITTSSpeed altera a velocidade do OpenAI TTS
func (a *App) SetOpenAITTSSpeed(rate int) {
	if a.speechManager != nil {
		a.speechManager.SetTTSSpeed(rate)
	}
}

// ==================== Interaction Profiles ====================

// GetInteractionProfiles retorna todos os perfis de interação
func (a *App) GetInteractionProfiles() ([]database.InteractionProfile, error) {
	return database.GetAllInteractionProfiles()
}

// GetInteractionProfile retorna um perfil de interação por ID
func (a *App) GetInteractionProfile(id uint) (*database.InteractionProfile, error) {
	return database.GetInteractionProfile(id)
}

// GetDefaultInteractionProfile retorna o perfil de interação padrão
func (a *App) GetDefaultInteractionProfile() (*database.InteractionProfile, error) {
	return database.GetDefaultInteractionProfile()
}

// CreateInteractionProfile cria um novo perfil de interação
func (a *App) CreateInteractionProfile(profile database.InteractionProfile) (*database.InteractionProfile, error) {
	created, err := database.CreateInteractionProfile(&profile)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:profile:created", map[string]interface{}{
		"id":   created.ID,
		"name": created.Name,
	})

	return created, nil
}

// UpdateInteractionProfile atualiza um perfil de interação
func (a *App) UpdateInteractionProfile(id uint, profile database.InteractionProfile) (*database.InteractionProfile, error) {
	updated, err := database.UpdateInteractionProfile(id, &profile)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:profile:updated", map[string]interface{}{
		"id":   updated.ID,
		"name": updated.Name,
	})

	return updated, nil
}

// DeleteInteractionProfile deleta um perfil de interação
func (a *App) DeleteInteractionProfile(id uint) error {
	err := database.DeleteInteractionProfile(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:profile:deleted", map[string]interface{}{
		"id": id,
	})

	return nil
}

// SetDefaultInteractionProfile define um perfil como padrão
func (a *App) SetDefaultInteractionProfile(id uint) error {
	err := database.SetDefaultInteractionProfile(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:profile:default_changed", map[string]interface{}{
		"id": id,
	})

	return nil
}

// SearchInteractionProfiles busca perfis por nome ou descrição
func (a *App) SearchInteractionProfiles(query string) ([]database.InteractionProfile, error) {
	return database.SearchInteractionProfiles(query)
}

// ==================== Interaction Triggers ====================

// GetTriggersByProfile retorna todos os triggers de um perfil
func (a *App) GetTriggersByProfile(profileID uint) ([]database.InteractionTrigger, error) {
	return database.GetTriggersByProfile(profileID)
}

// GetInteractionTrigger retorna um trigger por ID
func (a *App) GetInteractionTrigger(id uint) (*database.InteractionTrigger, error) {
	return database.GetInteractionTrigger(id)
}

// CreateInteractionTrigger cria um novo trigger
func (a *App) CreateInteractionTrigger(trigger database.InteractionTrigger) (*database.InteractionTrigger, error) {
	created, err := database.CreateInteractionTrigger(&trigger)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:trigger:created", map[string]interface{}{
		"id":        created.ID,
		"profileId": created.ProfileID,
		"type":      created.Type,
	})

	return created, nil
}

// UpdateInteractionTrigger atualiza um trigger
func (a *App) UpdateInteractionTrigger(id uint, trigger database.InteractionTrigger) (*database.InteractionTrigger, error) {
	updated, err := database.UpdateInteractionTrigger(id, &trigger)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:trigger:updated", map[string]interface{}{
		"id":        updated.ID,
		"profileId": updated.ProfileID,
		"type":      updated.Type,
	})

	return updated, nil
}

// DeleteInteractionTrigger deleta um trigger
func (a *App) DeleteInteractionTrigger(id uint) error {
	// Busca trigger para obter profileId antes de deletar
	trigger, err := database.GetInteractionTrigger(id)
	if err != nil {
		return err
	}

	err = database.DeleteInteractionTrigger(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "interaction:trigger:deleted", map[string]interface{}{
		"id":        id,
		"profileId": trigger.ProfileID,
	})

	return nil
}

// ==================== Chat Profiles ====================

// GetChatProfiles retorna todos os perfis de conversa
func (a *App) GetChatProfiles() ([]database.ChatProfile, error) {
	return database.GetAllChatProfiles()
}

// GetChatProfile retorna um perfil de conversa por ID
func (a *App) GetChatProfile(id uint) (*database.ChatProfile, error) {
	return database.GetChatProfile(id)
}

// GetDefaultChatProfile retorna o perfil de conversa padrão
func (a *App) GetDefaultChatProfile() (*database.ChatProfile, error) {
	return database.GetDefaultChatProfile()
}

// CreateChatProfile cria um novo perfil de conversa
func (a *App) CreateChatProfile(profile database.ChatProfile) (*database.ChatProfile, error) {
	created, err := database.CreateChatProfile(&profile)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:created", created)

	return created, nil
}

// UpdateChatProfile atualiza um perfil de conversa
func (a *App) UpdateChatProfile(id uint, profile database.ChatProfile) (*database.ChatProfile, error) {
	updated, err := database.UpdateChatProfile(id, &profile)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:updated", updated)

	return updated, nil
}

// DeleteChatProfile deleta um perfil de conversa
func (a *App) DeleteChatProfile(id uint) error {
	err := database.DeleteChatProfile(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:deleted", id)

	return nil
}

// SetDefaultChatProfile define um perfil como padrão
func (a *App) SetDefaultChatProfile(id uint) error {
	err := database.SetDefaultChatProfile(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:default_changed", id)

	return nil
}

// SetConversationChatProfile define o perfil de conversa para uma conversa
func (a *App) SetConversationChatProfile(conversationID uint, profileID uint) error {
	err := database.SetConversationChatProfile(conversationID, profileID)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:conversation_changed", map[string]interface{}{
		"conversation_id": conversationID,
		"profile_id":      profileID,
	})

	return nil
}

// ClearConversationChatProfile remove o perfil customizado de uma conversa
func (a *App) ClearConversationChatProfile(conversationID uint) error {
	err := database.ClearConversationChatProfile(conversationID)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "chat:profile:conversation_changed", map[string]interface{}{
		"conversation_id": conversationID,
		"profile_id":      0, // 0 indica usar padrão
	})

	return nil
}

// GetEffectiveChatProfile retorna o perfil efetivo de uma conversa
func (a *App) GetEffectiveChatProfile(conversationID uint) (*database.ChatProfile, error) {
	return database.GetEffectiveChatProfile(conversationID)
}

// ==================== Chat Tabs ====================

// GetAllTabs retorna todas as abas de chat
func (a *App) GetAllTabs() ([]database.ChatTab, error) {
	return database.GetAllTabs()
}

// GetActiveTab retorna a aba ativa
func (a *App) GetActiveTab() (*database.ChatTab, error) {
	return database.GetActiveTab()
}

// CreateTab cria uma nova aba de chat
// setAsActive: se true, marca a nova aba como ativa; se false, mantém a aba atual ativa
func (a *App) CreateTab(title, icon string, setAsActive bool) (*database.ChatTab, error) {
	tab, err := database.CreateTab(title, icon, setAsActive)
	if err != nil {
		return nil, err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "tab_created", map[string]interface{}{
		"id":       tab.ID,
		"title":    tab.Title,
		"icon":     tab.Icon,
		"position": tab.Position,
		"isActive": tab.IsActive,
	})

	return tab, nil
}

// CloseTab fecha uma aba
func (a *App) CloseTab(id uint) error {
	err := database.CloseTab(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "tab_closed", map[string]interface{}{
		"id": id,
	})

	return nil
}

// SetActiveTab define a aba ativa
func (a *App) SetActiveTab(id uint) error {
	err := database.SetActiveTab(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "tab_activated", map[string]interface{}{
		"id": id,
	})

	return nil
}

// UpdateTabTitle atualiza o título de uma aba e da conversa associada
func (a *App) UpdateTabTitle(id uint, title string) error {
	// Busca a tab para verificar se tem conversa associada
	tab, err := database.GetTab(id)
	if err != nil {
		return err
	}

	err = database.UpdateTabTitle(id, title)
	if err != nil {
		return err
	}

	// Se há conversa associada, emite evento unificado para atualizar todas as referências
	if tab.ConversationID != nil && *tab.ConversationID > 0 {
		runtime.EventsEmit(a.ctx, "conversation:renamed", map[string]interface{}{
			"conversation_id": *tab.ConversationID,
			"new_title":       title,
		})
	}

	return nil
}

// LoadConversationInTab carrega uma conversa em uma aba
func (a *App) LoadConversationInTab(tabId, conversationId uint) error {
	err := database.LoadConversationInTab(tabId, conversationId)
	if err != nil {
		return err
	}

	// Obtém a conversa para emitir evento completo
	conv, err := database.GetConversation(conversationId)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "conversation_loaded_in_tab", map[string]interface{}{
		"tabId":          tabId,
		"conversationId": conv.ID,
		"title":          conv.Title,
	})

	return nil
}

// ClearTab limpa a conversa de uma aba
func (a *App) ClearTab(id uint) error {
	err := database.ClearTab(id)
	if err != nil {
		return err
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "tab_cleared", map[string]interface{}{
		"id": id,
	})

	return nil
}

// ReorderTabs reordena as abas
func (a *App) ReorderTabs(orderedIds []uint) error {
	return database.ReorderTabs(orderedIds)
}
