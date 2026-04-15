package main

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"assistente/internal/profiles"
)

// stripMarkdownForTTS removes markdown syntax to produce clean text for TTS.
var mdPattern = regexp.MustCompile(`(?m)` +
	`(^#{1,6}\s+)` + // headings
	`|([*_]{1,3})` + // bold/italic
	`|(~~)` + // strikethrough
	"|(```[\\s\\S]*?```)" + // code blocks
	"|(`[^`]+`)" + // inline code
	`|(\[([^\]]+)\]\([^)]+\))` + // links → keep text
	`|(^\s*[-*+]\s)` + // unordered list markers
	`|(^\s*\d+\.\s)` + // ordered list markers
	`|(^>\s?)`, // blockquotes
)

func stripMarkdownForTTS(text string) string {
	result := mdPattern.ReplaceAllStringFunc(text, func(match string) string {
		// For links [text](url), keep just the text
		if strings.HasPrefix(match, "[") {
			idx := strings.Index(match, "]")
			if idx > 0 {
				return match[1:idx]
			}
		}
		return ""
	})
	// Collapse multiple blank lines
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}

type ChatSpeakStrategy string

const (
	ChatSpeakStrategyNone         ChatSpeakStrategy = "none"
	ChatSpeakStrategyAnnounce     ChatSpeakStrategy = "announce"
	ChatSpeakStrategyWebSpeech    ChatSpeakStrategy = "webspeech"
	ChatSpeakStrategyBackendAudio ChatSpeakStrategy = "backend_audio"
)

type ChatSpeakOrigin string

const (
	ChatSpeakOriginAssistantMessage ChatSpeakOrigin = "assistant_message"
	ChatSpeakOriginUserMessage      ChatSpeakOrigin = "user_message"
	ChatSpeakOriginSystemMessage    ChatSpeakOrigin = "system_message"
	ChatSpeakOriginThinking         ChatSpeakOrigin = "thinking"
	ChatSpeakOriginToolStatus       ChatSpeakOrigin = "tool_status"
	ChatSpeakOriginSegment          ChatSpeakOrigin = "segment"
)

type ChatSpeakRequest struct {
	ConversationID uint            `json:"conversationId"`
	MessageID      uint            `json:"messageId,omitempty"`
	ProfileSlug    string          `json:"profileSlug,omitempty"`
	Role           string          `json:"role"`
	Text           string          `json:"text"`
	Origin         ChatSpeakOrigin `json:"origin"`
	Interrupt      *bool           `json:"interrupt,omitempty"`
}

type ChatSpeakEvent struct {
	MessageID        uint              `json:"messageId,omitempty"`
	ConversationID   uint              `json:"conversationId"`
	Role             string            `json:"role"`
	Text             string            `json:"text"`
	Strategy         ChatSpeakStrategy `json:"strategy"`
	FallbackStrategy ChatSpeakStrategy `json:"fallbackStrategy,omitempty"`
	AutoRead         bool              `json:"autoRead"`
	ProviderID       string            `json:"providerId,omitempty"`
	VoiceID          string            `json:"voiceId,omitempty"`
	Model            string            `json:"model,omitempty"`
	Rate             float64           `json:"rate,omitempty"`
	Pitch            float64           `json:"pitch,omitempty"`
	Volume           float64           `json:"volume,omitempty"`
	Origin           ChatSpeakOrigin   `json:"origin"`
	Interrupt        bool              `json:"interrupt"`
}

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

func (a *App) DispatchSpeech(req ChatSpeakRequest) error {
	_, err := a.dispatchSpeechEvent(req)
	return err
}

func (a *App) dispatchSpeechEvent(req ChatSpeakRequest) (*ChatSpeakEvent, error) {
	text := stripMarkdownForTTS(req.Text)
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "system"
	}

	profile, err := a.resolveSpeechProfile(req.ConversationID, req.ProfileSlug)
	if err != nil {
		return nil, err
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
	a.emitter.Emit("chat:speak", event)
	return &event, nil
}

func (a *App) resolveSpeechProfile(conversationID uint, profileSlug string) (*profiles.Profile, error) {
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
