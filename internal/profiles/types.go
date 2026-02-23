package profiles

import (
	"fmt"
	"strings"
)

// Profile representa um perfil de conversa unificado.
// Combina configurações de chat (LLM), voz (TTS) e interação (STT/triggers)
// em um único arquivo JSON armazenado em .assistente/profiles/.
type Profile struct {
	BuiltinVersion string            `json:"_builtin_version,omitempty"` // Version for builtin profiles (used by installBuiltinProfiles)
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Icon           string            `json:"icon,omitempty"`
	Chat           ChatConfig        `json:"chat"`
	Voice          VoiceConfig       `json:"voice"`
	Interaction    InteractionConfig `json:"interaction"`
	MediaSupport   *MediaSupport     `json:"media_support,omitempty"` // Suporte a mídia do modelo (auto-detectado)
}

// MediaSupport indica quais tipos de mídia o modelo LLM suporta nativamente.
// Começa nil (não testado). Cada campo é preenchido automaticamente após a
// primeira tentativa — se o modelo rejeitar, marca false e usa fallback
// (Whisper para áudio, extração de texto para documentos).
// O usuário pode forçar valores manualmente no perfil.
type MediaSupport struct {
	Audio    *bool `json:"audio,omitempty"`    // Suporta input_audio? nil=não testado
	Image    *bool `json:"image,omitempty"`    // Suporta image_url? nil=não testado (assume true)
	Document *bool `json:"document,omitempty"` // Suporta arquivos/PDF? nil=não testado
	Video    *bool `json:"video,omitempty"`    // Suporta vídeo? nil=não testado
}

// ChatConfig define as configurações do modelo LLM
type ChatConfig struct {
	Model                string   `json:"model,omitempty"`
	Temperature          float64  `json:"temperature"`                      // 0.0 a 2.0
	MaxTokens            int      `json:"max_tokens"`                       // Limite de tokens na resposta
	ContextWindow        int      `json:"context_window,omitempty"`         // Limite de contexto (tokens totais)
	TopP                 float64  `json:"top_p"`                            // 0.0 a 1.0
	ResponseTimeout      int      `json:"response_timeout"`                 // Timeout em segundos
	EnableThinking       bool     `json:"enable_thinking"`                  // Habilita reasoning/thinking
	SystemPrompt         string   `json:"system_prompt,omitempty"`          // Prompt customizado
	SystemPromptPosition string   `json:"system_prompt_position,omitempty"` // "before" ou "after"
	EnabledTools         []string `json:"enabled_tools"`                    // Ferramentas habilitadas (nil = todas)
	EnabledSkills        []string `json:"enabled_skills"`                   // Skills habilitados (nil = todos, [] = nenhum)
	CommandAllowlist     string   `json:"command_allowlist,omitempty"`      // Slug da allowlist de comandos

	// MCP Mode: "adapter" (padrão) ou "native"
	// - "adapter": Tools MCP via bridge (compatível com qualquer modelo)
	// - "native": MCP direto (requer modelo com suporte nativo como Claude 3.7+)
	// - "auto": Detecta automaticamente se modelo suporta MCP nativo
	MCPMode string `json:"mcp_mode,omitempty"` // adapter, native, auto (padrão: adapter)

	// MCPNativeTested indica se o suporte MCP nativo foi testado para este modelo
	// Evita re-testar toda vez. Valores:
	// - nil: não testado ainda
	// - true: testado e suporta MCP nativo
	// - false: testado e NÃO suporta MCP nativo
	MCPNativeTested *bool `json:"mcp_native_tested,omitempty"`
}

// VoiceConfig define as configurações de voz TTS
type VoiceConfig struct {
	Provider        string  `json:"provider"`           // disabled, webspeech, sapi5, openai
	VoiceID         string  `json:"voice_id,omitempty"` // ID da voz
	Rate            float64 `json:"rate"`               // Velocidade
	Pitch           float64 `json:"pitch"`              // Tom
	Volume          float64 `json:"volume"`             // Volume (0-1)
	EnabledForAgent bool    `json:"enabled_for_agent"`  // TTS para mensagens do assistente
	EnabledForUser  bool    `json:"enabled_for_user"`   // TTS para mensagens do usuário

	// ChannelResponseMode define como o canal externo responde em relação ao formato de mídia.
	// Afeta apenas conversas via canais (Signal, Telegram, etc.) — não afeta o desktop.
	//   "mirror"       (padrão) — espelha o formato: texto→texto, áudio→áudio
	//   "always_text"  — sempre responde em texto, mesmo se recebeu áudio
	//   "always_audio" — sempre responde em áudio (TTS), mesmo se recebeu texto
	ChannelResponseMode string `json:"channel_response_mode,omitempty"`
}

// Channel response mode constants
const (
	ChannelResponseMirror      = "mirror"       // texto→texto, áudio→áudio (padrão)
	ChannelResponseAlwaysText  = "always_text"  // sempre texto
	ChannelResponseAlwaysAudio = "always_audio" // sempre áudio (TTS)
)

// MCP Mode constants
const (
	MCPModeAdapter = "adapter" // Usa bridge de tools (compatível com todos modelos)
	MCPModeNative  = "native"  // MCP direto (requer suporte do modelo)
	MCPModeAuto    = "auto"    // Detecta automaticamente
)

// InteractionConfig define as configurações de interação por voz
type InteractionConfig struct {
	STTProvider    string          `json:"stt_provider"`       // webspeech, whisper_api
	Language       string          `json:"language"`           // Idioma (ex: pt-BR)
	FeedbackSounds bool            `json:"feedback_sounds"`    // Sons de início/fim
	Triggers       []TriggerConfig `json:"triggers,omitempty"` // Lista de triggers
}

// TriggerConfig define uma forma de ativar interação por voz
type TriggerConfig struct {
	Type    string `json:"type"`    // hotkey, button_ptt, button_toggle, wakeword, vad
	Enabled bool   `json:"enabled"` // Se o trigger está ativo

	// Como terminar gravação: true=VAD automático, false=manual
	AutoStop bool `json:"auto_stop,omitempty"`

	// Hotkey
	Hotkey             string `json:"hotkey,omitempty"`                // Ex: "Ctrl+Shift+Space"
	HotkeyGlobal       bool   `json:"hotkey_global,omitempty"`         // Global ou local
	HotkeyBringToFront bool   `json:"hotkey_bring_to_front,omitempty"` // Trazer janela (se global)

	// Wakeword
	WakewordKeyword     string  `json:"wakeword_keyword,omitempty"`     // Ex: "assistente"
	WakewordProvider    string  `json:"wakeword_provider,omitempty"`    // webspeech
	WakewordSensitivity float64 `json:"wakeword_sensitivity,omitempty"` // 0.0 - 1.0

	// VAD Config
	VADSilenceThreshold  float64 `json:"vad_silence_threshold,omitempty"`  // 0-1
	VADSilenceDuration   int     `json:"vad_silence_duration,omitempty"`   // ms
	VADActivityThreshold float64 `json:"vad_activity_threshold,omitempty"` // 0-1
	VADActivityDuration  int     `json:"vad_activity_duration,omitempty"`  // ms
}

// ProfileInfo é um resumo leve de um perfil para listagem
type ProfileInfo struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"` // nome do arquivo sem extensão
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Source      string `json:"source"` // "exe", "home", "workdir"
}

// Trigger types
const (
	TriggerTypeHotkey       = "hotkey"
	TriggerTypeButtonPTT    = "button_ptt"
	TriggerTypeButtonToggle = "button_toggle"
	TriggerTypeWakeword     = "wakeword"
	TriggerTypeVAD          = "vad"
)

// DefaultProfile retorna um perfil com valores padrão
func DefaultProfile() *Profile {
	return &Profile{
		Name:        "Padrão",
		Description: "Configuração padrão.",
		Icon:        "chatbox",
		Chat: ChatConfig{
			Model:                "",
			Temperature:          0.7,
			MaxTokens:            4096,
			TopP:                 1.0,
			ResponseTimeout:      180,
			EnableThinking:       false,
			SystemPrompt:         "",
			SystemPromptPosition: "after",
		},
		Voice: VoiceConfig{
			Provider:        "disabled",
			VoiceID:         "",
			Rate:            1.0,
			Pitch:           1.0,
			Volume:          1.0,
			EnabledForAgent: false,
			EnabledForUser:  false,
		},
		Interaction: InteractionConfig{
			STTProvider:    "webspeech",
			Language:       "pt-BR",
			FeedbackSounds: true,
			Triggers:       []TriggerConfig{},
		},
	}
}

// Validate valida os campos do perfil
func (p *Profile) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Validação do chat
	if p.Chat.Temperature < 0 || p.Chat.Temperature > 2 {
		return fmt.Errorf("chat.temperature must be between 0 and 2")
	}
	if p.Chat.MaxTokens < 1 {
		return fmt.Errorf("chat.max_tokens must be at least 1")
	}
	if p.Chat.TopP < 0 || p.Chat.TopP > 1 {
		return fmt.Errorf("chat.top_p must be between 0 and 1")
	}
	if p.Chat.ResponseTimeout < 10 {
		return fmt.Errorf("chat.response_timeout must be at least 10 seconds")
	}
	if p.Chat.SystemPromptPosition != "" && p.Chat.SystemPromptPosition != "before" && p.Chat.SystemPromptPosition != "after" {
		return fmt.Errorf("chat.system_prompt_position must be 'before' or 'after'")
	}

	// Validação do MCP Mode
	validMCPModes := []string{"", MCPModeAdapter, MCPModeNative, MCPModeAuto}
	if !containsStr(validMCPModes, p.Chat.MCPMode) {
		return fmt.Errorf("chat.mcp_mode must be one of: adapter, native, auto")
	}

	// Validação da voz
	validVoiceProviders := []string{"disabled", "webspeech", "sapi5", "openai"}
	if p.Voice.Provider != "" && !containsStr(validVoiceProviders, p.Voice.Provider) {
		return fmt.Errorf("voice.provider must be one of: disabled, webspeech, sapi5, openai")
	}

	// Validação do modo de resposta para canais
	validChannelModes := []string{"", ChannelResponseMirror, ChannelResponseAlwaysText, ChannelResponseAlwaysAudio}
	if !containsStr(validChannelModes, p.Voice.ChannelResponseMode) {
		return fmt.Errorf("voice.channel_response_mode must be one of: mirror, always_text, always_audio")
	}

	// Validação da interação
	validSTTProviders := []string{"webspeech", "whisper_api", ""}
	if !containsStr(validSTTProviders, p.Interaction.STTProvider) {
		return fmt.Errorf("interaction.stt_provider must be one of: webspeech, whisper_api")
	}

	// Validação dos triggers
	validTriggerTypes := []string{TriggerTypeHotkey, TriggerTypeButtonPTT, TriggerTypeButtonToggle, TriggerTypeWakeword, TriggerTypeVAD}
	for i, t := range p.Interaction.Triggers {
		if !containsStr(validTriggerTypes, t.Type) {
			return fmt.Errorf("interaction.triggers[%d].type must be one of: hotkey, button_ptt, button_toggle, wakeword, vad", i)
		}
		if t.Type == TriggerTypeHotkey && t.Hotkey == "" {
			return fmt.Errorf("interaction.triggers[%d].hotkey is required for type hotkey", i)
		}
		if t.Type == TriggerTypeWakeword && t.WakewordKeyword == "" {
			return fmt.Errorf("interaction.triggers[%d].wakeword_keyword is required for type wakeword", i)
		}
	}

	return nil
}

// ShouldUseAriaLiveForAgent retorna se deve usar aria-live para mensagens do assistente
func (p *Profile) ShouldUseAriaLiveForAgent() bool {
	return p.Voice.Provider == "disabled" || !p.Voice.EnabledForAgent
}

// ShouldUseAriaLiveForUser retorna se deve usar aria-live para mensagens do usuário
func (p *Profile) ShouldUseAriaLiveForUser() bool {
	return p.Voice.Provider == "disabled" || !p.Voice.EnabledForUser
}

// IsVoiceDisabled retorna true se o perfil não usa TTS
func (p *Profile) IsVoiceDisabled() bool {
	return p.Voice.Provider == "disabled" || (!p.Voice.EnabledForAgent && !p.Voice.EnabledForUser)
}

// GetChannelResponseMode retorna o modo de resposta efetivo para canais externos.
func (p *Profile) GetChannelResponseMode() string {
	if p.Voice.ChannelResponseMode == "" {
		return ChannelResponseMirror
	}
	return p.Voice.ChannelResponseMode
}

// ShouldRespondWithAudio retorna se o canal deve responder com áudio dado o modo e se a mensagem original era áudio.
func (p *Profile) ShouldRespondWithAudio(incomingIsAudio bool) bool {
	mode := p.GetChannelResponseMode()
	switch mode {
	case ChannelResponseAlwaysAudio:
		return true
	case ChannelResponseAlwaysText:
		return false
	case ChannelResponseMirror:
		return incomingIsAudio
	default:
		return incomingIsAudio // fallback = mirror
	}
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetMCPMode retorna o modo MCP efetivo do perfil.
// Se não especificado ou inválido, retorna "auto" como padrão.
func (p *Profile) GetMCPMode() string {
	mode := p.Chat.MCPMode

	// Valida se é um modo válido
	if mode == MCPModeAdapter || mode == MCPModeNative || mode == MCPModeAuto {
		return mode
	}

	// Default: auto (testa e decide automaticamente)
	return MCPModeAuto
}

// ShouldUseMCPNative retorna se deve usar modo MCP nativo baseado no modelo.
// Verifica se o modelo suporta MCP nativamente e se o modo está configurado para "native" ou "auto".
func (p *Profile) ShouldUseMCPNative() bool {
	mode := p.GetMCPMode()

	// Se modo é explicitamente adapter, nunca usa nativo
	if mode == MCPModeAdapter {
		return false
	}

	// Se modo é explicitamente native, sempre usa nativo
	if mode == MCPModeNative {
		return true
	}

	// Se modo é auto, usa o resultado do teste (se disponível)
	if mode == MCPModeAuto {
		// Se já foi testado, usa resultado do teste
		if p.Chat.MCPNativeTested != nil {
			return *p.Chat.MCPNativeTested
		}
		// Se não foi testado, assume false (seguro)
		return false
	}

	return false
}

// MCPNativeWasTested retorna true se o suporte MCP nativo já foi testado.
func (p *Profile) MCPNativeWasTested() bool {
	return p.Chat.MCPNativeTested != nil
}

// SetMCPNativeSupport marca se o modelo suporta MCP nativo após teste.
func (p *Profile) SetMCPNativeSupport(supported bool) {
	p.Chat.MCPNativeTested = &supported
}

// ModelSupportsNativeMCP verifica se um modelo suporta MCP nativamente.
// DEPRECATED: Use TestMCPNativeSupport() para teste real em vez de hardcoded.
// Esta função agora apenas fornece "hint" inicial.
func ModelSupportsNativeMCP(modelID string) bool {
	// Normaliza para lowercase para comparação
	model := strings.ToLower(modelID)

	// Claude 3.7+ (inclui sonnet, opus, haiku)
	// Estes são conhecidos por suportar, mas sempre teste para confirmar
	if strings.Contains(model, "claude-3-7") ||
		strings.Contains(model, "claude-3.7") {
		return true
	}

	// Claude 4+ (futuro)
	if strings.Contains(model, "claude-4") {
		return true
	}

	// Para outros modelos, retorna false (teste é necessário)
	return false
}
