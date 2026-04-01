package profiles

import (
	"fmt"
)

// DefaultProviderSentinel é o valor sentinela usado em profiles para indicar
// "usar o provedor default do sistema". Resolvido em runtime por resolveProfileDefaults.
const DefaultProviderSentinel = "$default"

// Profile representa um perfil de conversa unificado.
// Combina configurações de chat (LLM), voz (TTS) e interação (STT/triggers)
// em um único arquivo JSON armazenado em .assistente/profiles/.
type Profile struct {
	BuiltinVersion string            `json:"_builtin_version,omitempty"` // Version for builtin profiles (used by installBuiltinProfiles)
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Icon           string            `json:"icon,omitempty"`
	Active         bool              `json:"active,omitempty"` // Marca se este é o perfil ativo
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
	LLMProvider           string   `json:"llm_provider"` // ID do provedor LLM a usar (ex: "openai-default")
	Model                 string   `json:"model,omitempty"`
	Temperature           float64  `json:"temperature"`                        // 0.0 a 2.0
	MaxTokens             int      `json:"max_tokens"`                         // Limite de tokens na resposta
	MaxTokensMode         string   `json:"max_tokens_mode,omitempty"`          // "legacy" (max_tokens) ou "completion_tokens" (max_completion_tokens)
	ContextWindow         int      `json:"context_window,omitempty"`           // Tamanho da janela de contexto do modelo (0 = não definido)
	MaxContextMessages    int      `json:"max_context_messages,omitempty"`     // Máx de mensagens no contexto (0 = padrão 50)
	MinContextMessages    int      `json:"min_context_messages,omitempty"`     // Mín de mensagens preservadas após sumarização (0 = padrão 10)
	TopP                  float64  `json:"top_p"`                              // 0.0 a 1.0
	ResponseTimeout       int      `json:"response_timeout"`                   // Timeout em segundos
	ReasoningEffort       string   `json:"reasoning_effort,omitempty"`         // off, low, medium, high (vazio = off)
	EnabledTools          []string `json:"enabled_tools"`                      // Ferramentas habilitadas (nil = todas)
	EnabledSkills         []string `json:"enabled_skills"`                     // Skills autoload ordenados (nil = usa auto_load do skill, [] = nenhum autoload)
	DisableTools          bool     `json:"disable_tools,omitempty"`            // Desabilita completamente tool calling
	DisableSkills         bool     `json:"disable_skills,omitempty"`           // Desabilita injeção de skills no prompt
	DisableOnDemandSkills bool     `json:"disable_on_demand_skills,omitempty"` // Desabilita skills sob demanda (apenas autoload)
	CommandAllowlist string `json:"command_allowlist,omitempty"` // Slug da allowlist de comandos

	// MaxAgenticIterations define o limite máximo de iterações do loop de agentes
	// Cada tool call conta como uma iteração
	// 0 = usar padrão (25 iterações)
	// >0 = limite customizado (ex: 100 para code generation, 500 para análise profunda)
	// Pode ser combinado com ResponseTimeout para dupla proteção
	MaxAgenticIterations int `json:"max_agentic_iterations,omitempty"`
}

// VoiceConfig define as configurações de voz TTS
type VoiceConfig struct {
	Disabled        bool    `json:"disabled,omitempty"`        // Desabilita completamente TTS neste perfil
	Provider        string  `json:"provider"`                  // disabled, webspeech, sapi5, openai
	LLMProviderID   string  `json:"llm_provider_id,omitempty"` // ID do provedor LLM para TTS (ex: "openai-default", independente do chat)
	VoiceID         string  `json:"voice_id,omitempty"`        // ID da voz
	Rate            float64 `json:"rate"`                      // Velocidade
	Pitch           float64 `json:"pitch"`                     // Tom
	Volume          float64 `json:"volume"`                    // Volume (0-1)
	EnabledForAgent bool    `json:"enabled_for_agent"`         // TTS para mensagens do assistente
	EnabledForUser  bool    `json:"enabled_for_user"`          // TTS para mensagens do usuário

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


// InteractionConfig define as configurações de interação por voz
type InteractionConfig struct {
	Disabled       bool            `json:"disabled,omitempty"`        // Desabilita completamente STT/interação neste perfil
	STTProvider    string          `json:"stt_provider"`              // webspeech, whisper_api
	LLMProviderID  string          `json:"llm_provider_id,omitempty"` // ID do provedor LLM para Whisper (ex: "openai-default", independente do chat)
	Language       string          `json:"language"`                  // Idioma (ex: pt-BR)
	FeedbackSounds bool            `json:"feedback_sounds"`           // Sons de início/fim
	Triggers       []TriggerConfig `json:"triggers,omitempty"`        // Lista de triggers
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

// DefaultProfile retorna um perfil com valores padrão.
// Usa $default para provedor e modelo — resolvido em runtime pelo sistema de default provider.
func DefaultProfile() *Profile {
	return &Profile{
		Name:        "Padrão",
		Description: "Configuração padrão.",
		Icon:        "chatbox",
		Chat: ChatConfig{
			LLMProvider:     DefaultProviderSentinel,
			Model:           DefaultProviderSentinel,
			Temperature:     0.7,
			MaxTokens:       4096,
			TopP:            1.0,
			ResponseTimeout: 180,
			ReasoningEffort: "",
		},
		Voice: VoiceConfig{
			Provider:        "disabled",
			LLMProviderID:   DefaultProviderSentinel,
			VoiceID:         "",
			Rate:            1.0,
			Pitch:           1.0,
			Volume:          1.0,
			EnabledForAgent: false,
			EnabledForUser:  false,
		},
		Interaction: InteractionConfig{
			STTProvider:    "webspeech",
			LLMProviderID:  DefaultProviderSentinel,
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
	if p.Chat.LLMProvider == "" {
		return fmt.Errorf("chat.llm_provider is required (ex: 'openai-default' or '$default')")
	}
	if p.Chat.Temperature < 0 || p.Chat.Temperature > 2 {
		return fmt.Errorf("chat.temperature must be between 0 and 2")
	}
	if p.Chat.MaxTokens < 1 {
		return fmt.Errorf("chat.max_tokens must be at least 1")
	}
	if p.Chat.ContextWindow < 0 {
		return fmt.Errorf("chat.context_window must be 0 (auto) or a positive number")
	}
	if p.Chat.MaxContextMessages < 0 {
		return fmt.Errorf("chat.max_context_messages must be 0 (default) or a positive number")
	}
	if p.Chat.MinContextMessages < 0 {
		return fmt.Errorf("chat.min_context_messages must be 0 (default) or a positive number")
	}
	if p.Chat.MinContextMessages > 0 && p.Chat.MaxContextMessages > 0 && p.Chat.MinContextMessages >= p.Chat.MaxContextMessages {
		return fmt.Errorf("chat.min_context_messages must be less than max_context_messages")
	}
	if p.Chat.TopP < 0 || p.Chat.TopP > 1 {
		return fmt.Errorf("chat.top_p must be between 0 and 1")
	}
	if p.Chat.ResponseTimeout < 10 {
		return fmt.Errorf("chat.response_timeout must be at least 10 seconds")
	}
	validReasoningEfforts := []string{"", "off", "none", "low", "medium", "high", "max", "ollama"}
	if !containsStr(validReasoningEfforts, p.Chat.ReasoningEffort) {
		return fmt.Errorf("chat.reasoning_effort must be one of: off, low, medium, high")
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
	return p.Voice.Disabled || p.Voice.Provider == "disabled" || !p.Voice.EnabledForAgent
}

// ShouldUseAriaLiveForUser retorna se deve usar aria-live para mensagens do usuário
func (p *Profile) ShouldUseAriaLiveForUser() bool {
	return p.Voice.Disabled || p.Voice.Provider == "disabled" || !p.Voice.EnabledForUser
}

// IsVoiceDisabled retorna true se o perfil não usa TTS
func (p *Profile) IsVoiceDisabled() bool {
	return p.Voice.Disabled || p.Voice.Provider == "disabled" || (!p.Voice.EnabledForAgent && !p.Voice.EnabledForUser)
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

// GetMaxContextMessages retorna o limite efetivo de mensagens no contexto.
func (p *Profile) GetMaxContextMessages() int {
	if p.Chat.MaxContextMessages > 0 {
		return p.Chat.MaxContextMessages
	}
	return 50
}

// GetMinContextMessages retorna o mínimo de mensagens preservadas após sumarização.
func (p *Profile) GetMinContextMessages() int {
	if p.Chat.MinContextMessages > 0 {
		return p.Chat.MinContextMessages
	}
	return 10
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

