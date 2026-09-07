package app

import (
	"fmt"
	"runtime"
	"strings"

	"assistente/internal/apidto"
	"assistente/internal/profiles"
	"assistente/internal/textutil"
)

func stripMarkdownForTTSInLanguage(text, language string) string {
	return textutil.StripMarkdownForSpeechLabeled(text, textutil.CodeBlockSpeechLabel(language))
}

// Aliases da borda (AEP-0088 D5): tipos canônicos em apidto; helpers internos do App
// e testes do pacote continuam usando estes nomes curtos.
type (
	ChatSpeakStrategy = apidto.ChatSpeakStrategy
	ChatSpeakOrigin   = apidto.ChatSpeakOrigin
	ChatSpeakRequest  = apidto.ChatSpeakRequest
	ChatSpeakEvent    = apidto.ChatSpeakEvent
)

const (
	ChatSpeakStrategyNone         = apidto.ChatSpeakStrategyNone
	ChatSpeakStrategyAnnounce     = apidto.ChatSpeakStrategyAnnounce
	ChatSpeakStrategyWebSpeech    = apidto.ChatSpeakStrategyWebSpeech
	ChatSpeakStrategyBackendAudio = apidto.ChatSpeakStrategyBackendAudio

	ChatSpeakOriginAssistantMessage = apidto.ChatSpeakOriginAssistantMessage
	ChatSpeakOriginUserMessage      = apidto.ChatSpeakOriginUserMessage
	ChatSpeakOriginSystemMessage    = apidto.ChatSpeakOriginSystemMessage
	ChatSpeakOriginThinking         = apidto.ChatSpeakOriginThinking
	ChatSpeakOriginToolStatus       = apidto.ChatSpeakOriginToolStatus
	ChatSpeakOriginSegment          = apidto.ChatSpeakOriginSegment
)

func boolValueOrDefault(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func effectiveVoiceProviderID(cfg profiles.VoiceRoleConfig) string {
	switch cfg.Provider {
	case "webspeech", "sapi5":
		return cfg.Provider
	default:
		if cfg.LLMProviderID != "" {
			return cfg.LLMProviderID
		}
		return cfg.Provider
	}
}

// speechDispatcher adapta helpers lowercase do App para o bind wailsapi.Speech.
type speechDispatcher struct{ app *App }

func (d speechDispatcher) DispatchSpeech(req apidto.ChatSpeakRequest) error {
	_, err := d.app.dispatchSpeechEvent(req)
	return err
}

func (a *App) dispatchSpeechEvent(req ChatSpeakRequest) (*ChatSpeakEvent, error) {
	if strings.TrimSpace(req.Text) == "" {
		return nil, nil
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "system"
	}

	// O perfil é resolvido antes do strip para localizar o marcador de bloco
	// de código no idioma da fala.
	profile, err := a.resolveSpeechProfile(req.ConversationID, req.ProfileSlug)
	if err != nil {
		return nil, err
	}

	text := stripMarkdownForTTSInLanguage(req.Text, profile.Input.Language)
	if strings.TrimSpace(text) == "" {
		// Strip pode zerar só-sintaxe; fallback ao texto original (SpeakMessage/gateway).
		text = strings.TrimSpace(req.Text)
	}
	if text == "" {
		return nil, nil
	}

	var voiceCfg profiles.VoiceRoleConfig
	switch role {
	case "assistant":
		voiceCfg = profile.Voice.Assistant
	case "user":
		voiceCfg = profile.Voice.User
	case "system":
		voiceCfg = profile.Voice.System
	default:
		return nil, fmt.Errorf("role de fala inválida: %s", role)
	}

	event := a.buildChatSpeakEvent(req, role, text, voiceCfg)
	event.SpeechLanguage = profile.Input.Language
	a.emitter.Emit("chat:speak", event)
	return &event, nil
}

func (a *App) resolveSpeechProfile(conversationID string, profileSlug string) (*profiles.Profile, error) {
	if profileSlug != "" {
		p, err := a.profileManager.Get(profileSlug)
		if err != nil {
			return nil, fmt.Errorf("perfil %q não encontrado: %w", profileSlug, err)
		}
		return a.resolveProfileDefaults(p), nil
	}

	p, err := a.profileManager.GetActive()
	if err != nil {
		return nil, fmt.Errorf("perfil ativo não encontrado: %w", err)
	}
	return a.resolveProfileDefaults(p), nil
}

func (a *App) buildChatSpeakEvent(req ChatSpeakRequest, role, text string, cfg profiles.VoiceRoleConfig) ChatSpeakEvent {
	event := ChatSpeakEvent{
		MessageID:      req.MessageID,
		ConversationID: req.ConversationID,
		Role:           role,
		Text:           text,
		Origin:         req.Origin,
		Interrupt:      boolValueOrDefault(req.Interrupt, true),
		ProviderID:     effectiveVoiceProviderID(cfg),
		VoiceID:        cfg.VoiceID,
		Model:          cfg.Model,
		Rate:           cfg.Rate,
		Pitch:          cfg.Pitch,
		Volume:         cfg.Volume,
		AutoRead:       cfg.Enabled,
	}

	switch {
	case !cfg.Enabled || cfg.Provider == "disabled":
		// Sem TTS: ainda verbalizar no leitor de ecrã (região aria-live)
		event.Strategy = ChatSpeakStrategyAnnounce
	case cfg.Provider == "":
		// Nenhum provider configurado → anuncia via screen reader como fallback acessível
		event.Strategy = ChatSpeakStrategyAnnounce
	case cfg.Provider == "webspeech":
		event.Strategy = ChatSpeakStrategyWebSpeech
	case cfg.Provider == "sapi5":
		if runtime.GOOS != "windows" {
			// SAPI5 indisponível fora do Windows — fallback acessível
			event.Strategy = ChatSpeakStrategyAnnounce
		} else {
			// SAPI5 unificado: backend gera WAV via COM, frontend reproduz como backend_audio
			event.Strategy = ChatSpeakStrategyBackendAudio
			event.ProviderID = "sapi5"
			event.FallbackStrategy = ChatSpeakStrategyAnnounce
		}
	case effectiveVoiceProviderID(cfg) == "":
		event.Strategy = ChatSpeakStrategyAnnounce
	default:
		event.Strategy = ChatSpeakStrategyBackendAudio
		event.FallbackStrategy = ChatSpeakStrategyAnnounce
	}

	return event
}
