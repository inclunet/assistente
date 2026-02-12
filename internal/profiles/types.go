package profiles

import "fmt"

// Profile representa um perfil de conversa unificado.
// Combina configurações de chat (LLM), voz (TTS) e interação (STT/triggers)
// em um único arquivo JSON armazenado em .assistente/profiles/.
type Profile struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	Icon         string            `json:"icon,omitempty"`
	Chat         ChatConfig        `json:"chat"`
	Voice        VoiceConfig       `json:"voice"`
	Interaction  InteractionConfig `json:"interaction"`
	MediaSupport *MediaSupport     `json:"media_support,omitempty"` // Suporte a mídia do modelo (auto-detectado)
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
	TopP                 float64  `json:"top_p"`                            // 0.0 a 1.0
	ResponseTimeout      int      `json:"response_timeout"`                 // Timeout em segundos
	EnableThinking       bool     `json:"enable_thinking"`                  // Habilita reasoning/thinking
	SystemPrompt         string   `json:"system_prompt,omitempty"`          // Prompt customizado
	SystemPromptPosition string   `json:"system_prompt_position,omitempty"` // "before" ou "after"
	EnabledTools         []string `json:"enabled_tools"`                    // Ferramentas habilitadas (nil = todas)
	EnabledSkills        []string `json:"enabled_skills"`                   // Skills habilitados (nil = todos, [] = nenhum)
	CommandAllowlist     string   `json:"command_allowlist,omitempty"`      // Slug da allowlist de comandos
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
}

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

	// Validação da voz
	validVoiceProviders := []string{"disabled", "webspeech", "sapi5", "openai"}
	if p.Voice.Provider != "" && !containsStr(validVoiceProviders, p.Voice.Provider) {
		return fmt.Errorf("voice.provider must be one of: disabled, webspeech, sapi5, openai")
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

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
