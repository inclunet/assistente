package profiles

import (
	"fmt"
	"strings"
)

// DefaultProviderSentinel é o valor sentinela usado em profiles para indicar
// "usar o provedor default do sistema". Resolvido em runtime por resolveProfileDefaults.
const DefaultProviderSentinel = "$default"

// Profile representa um perfil de conversa unificado.
// Combina configurações de chat (LLM), voz (TTS) e input (STT/triggers)
// em um único arquivo JSON armazenado em .assistente/profiles/.
type Profile struct {
	BuiltinVersion string         `json:"_builtin_version,omitempty"` // Version for builtin profiles (used by installBuiltinProfiles)
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	Icon           string         `json:"icon,omitempty"`
	Active         bool           `json:"active,omitempty"` // Marca se este é o perfil ativo
	Chat           ChatConfig     `json:"chat"`
	Voice          VoiceConfig    `json:"voice"`
	Input          InputConfig    `json:"input"`
	Channels       ChannelsConfig `json:"channels,omitempty"`
	MediaSupport   *MediaSupport  `json:"media_support,omitempty"` // Suporte a mídia do modelo (auto-detectado)
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
	EnabledTools          []string `json:"enabled_tools"`                      // Ferramentas habilitadas (nil = seleção dinâmica/catalogo quando disponível)
	EnabledSkills         []string `json:"enabled_skills"`                     // Skills autoload ordenados (nil = usa auto_load do skill, [] = nenhum autoload)
	DisableTools          bool     `json:"disable_tools,omitempty"`            // Desabilita completamente tool calling
	DisableSkills         bool     `json:"disable_skills,omitempty"`           // Desabilita injeção de skills no prompt
	DisableOnDemandSkills bool     `json:"disable_on_demand_skills,omitempty"` // Desabilita skills sob demanda (apenas autoload)
	CommandAllowlist      string   `json:"command_allowlist,omitempty"`        // Slug da allowlist de comandos

	// NativeMCP é o override tri-state de suporte a MCP nativo (tools type:"mcp"
	// na Responses API / mcp_servers na Anthropic) para os runs deste perfil
	// (chat normal E sub-agentes que rodam com este profile). Ponteiro para
	// preservar compatibilidade com perfis antigos (AEP-0021):
	//   - nil   → auto: usa o default do provider/endpoint (heurística por URL,
	//             ex.: apenas api.openai.com manda type:"mcp").
	//   - true  → força MCP nativo (envia type:"mcp"), desde que o provider seja
	//             fisicamente capaz (Responses API / Anthropic). Útil para proxies
	//             (LiteLLM/Azure) cujo MODELO selecionado suporta type:"mcp".
	//   - false → força modo adapter (MCP como function/bridge tools, sem type:"mcp").
	//             Útil quando o mesmo endpoint serve um modelo que NÃO suporta
	//             type:"mcp" (ex.: deepseek-v4-flash via LiteLLM), evitando o 400
	//             "unknown variant `mcp`, expected `function`" a cada turno.
	NativeMCP *bool `json:"native_mcp,omitempty"`

	// MaxAgenticIterations define o limite máximo de iterações do loop de agentes
	// Cada tool call conta como uma iteração
	// 0 = usar padrão (25 iterações)
	// >0 = limite customizado (ex: 100 para code generation, 500 para análise profunda)
	// Pode ser combinado com ResponseTimeout para dupla proteção
	MaxAgenticIterations int `json:"max_agentic_iterations,omitempty"`

	// StreamingRecoveryEnabled controla a auto-recuperação de streaming interrompido.
	// Ponteiro para preservar compatibilidade com perfis antigos (nil = usar default).
	StreamingRecoveryEnabled *bool `json:"streaming_recovery_enabled,omitempty"`
	// StreamingRecoveryMaxAttempts define o máximo de tentativas de recuperação.
	// Ponteiro para preservar compatibilidade com perfis antigos (nil = usar default).
	StreamingRecoveryMaxAttempts *int `json:"streaming_recovery_max_attempts,omitempty"`
	// StreamingRecoveryShowContinue controla a exibição da ação manual "Continuar resposta" após falha/cancelamento.
	// Ponteiro para preservar compatibilidade com perfis antigos (nil = usar default).
	StreamingRecoveryShowContinue *bool `json:"streaming_recovery_show_continue,omitempty"`
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

// VoiceRoleConfig configura TTS para uma role específica (assistant, user ou system).
type VoiceRoleConfig struct {
	Enabled       bool    `json:"enabled"`                   // Habilita TTS para esta role
	Provider      string  `json:"provider"`                  // "disabled", "webspeech", "sapi5", "openai"
	LLMProviderID string  `json:"llm_provider_id,omitempty"` // ID do provedor LLM (para credenciais da API)
	VoiceID       string  `json:"voice_id,omitempty"`        // ID da voz (ex: "nova", "alloy", "echo")
	Model         string  `json:"model,omitempty"`           // Modelo TTS (ex: "tts-1", "tts-1-hd")
	SelectionMode string  `json:"selection_mode,omitempty"`  // "model_and_voice" ou "model_only"
	Rate          float64 `json:"rate"`                      // Velocidade (0.5–2.0)
	Pitch         float64 `json:"pitch"`                     // Tom (0.5–2.0)
	Volume        float64 `json:"volume"`                    // Volume (0.0–1.0)
}

// VoiceConfig configura TTS — uma sub-config independente por role.
//   - Assistant: voz para respostas do LLM
//   - User: voz para confirmação/leitura de mensagens do usuário (acessibilidade)
//   - System: voz para notificações, alertas, tool calling, MCP
type VoiceConfig struct {
	Assistant VoiceRoleConfig `json:"assistant"`
	User      VoiceRoleConfig `json:"user"`
	System    VoiceRoleConfig `json:"system"`
}

// InputConfig configura STT e triggers de interação por voz.
// Substitui a antiga InteractionConfig.
type InputConfig struct {
	Enabled        bool            `json:"enabled"`                   // Habilita input por voz
	STTProvider    string          `json:"stt_provider"`              // "webspeech", "whisper_api"
	LLMProviderID  string          `json:"llm_provider_id,omitempty"` // ID do provedor LLM para Whisper API
	STTModel       string          `json:"stt_model,omitempty"`       // Modelo STT (ex: "whisper-1", "whisper-large-v3")
	Language       string          `json:"language"`                  // Idioma (ex: "pt-BR")
	FeedbackSounds bool            `json:"feedback_sounds"`           // Sons de início/fim de gravação
	Triggers       []TriggerConfig `json:"triggers,omitempty"`        // Lista de triggers de ativação
}

// ChannelsConfig configura comportamento para canais externos (Telegram, Signal, etc.).
type ChannelsConfig struct {
	// ResponseMode define como o canal externo responde em relação ao formato de mídia.
	//   "mirror"       (padrão) — espelha o formato: texto→texto, áudio→áudio
	//   "always_text"  — sempre responde em texto, mesmo se recebeu áudio
	//   "always_audio" — sempre responde em áudio (TTS), mesmo se recebeu texto
	ResponseMode string `json:"response_mode,omitempty"`
}

// Channel response mode constants
const (
	ChannelResponseMirror      = "mirror"       // texto→texto, áudio→áudio (padrão)
	ChannelResponseAlwaysText  = "always_text"  // sempre texto
	ChannelResponseAlwaysAudio = "always_audio" // sempre áudio (TTS)
)

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
	defaultRole := VoiceRoleConfig{
		Enabled:  false,
		Provider: "disabled",
		Rate:     1.0,
		Pitch:    1.0,
		Volume:   1.0,
	}
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
			StreamingRecoveryEnabled:     boolPtr(true),
			StreamingRecoveryMaxAttempts: intPtr(3),
			StreamingRecoveryShowContinue: boolPtr(true),
		},
		Voice: VoiceConfig{
			Assistant: VoiceRoleConfig{
				Enabled:       false,
				Provider:      "disabled",
				LLMProviderID: DefaultProviderSentinel,
				Model:         "tts-1",
				Rate:          1.0,
				Pitch:         1.0,
				Volume:        1.0,
			},
			User:   defaultRole,
			System: defaultRole,
		},
		Input: InputConfig{
			Enabled:        true,
			STTProvider:    "webspeech",
			LLMProviderID:  DefaultProviderSentinel,
			Language:       "pt-BR",
			FeedbackSounds: true,
			Triggers:       []TriggerConfig{},
		},
		Channels: ChannelsConfig{
			ResponseMode: ChannelResponseMirror,
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
	if p.Chat.StreamingRecoveryMaxAttempts != nil {
		if *p.Chat.StreamingRecoveryMaxAttempts < 1 {
			return fmt.Errorf("chat.streaming_recovery_max_attempts must be at least 1")
		}
		if *p.Chat.StreamingRecoveryMaxAttempts > 10 {
			return fmt.Errorf("chat.streaming_recovery_max_attempts must be at most 10")
		}
	}
	if p.Chat.ResponseTimeout < 10 {
		return fmt.Errorf("chat.response_timeout must be at least 10 seconds")
	}
	validReasoningEfforts := []string{"", "off", "none", "low", "medium", "high", "max", "ollama"}
	if !containsStr(validReasoningEfforts, p.Chat.ReasoningEffort) {
		return fmt.Errorf("chat.reasoning_effort must be one of: off, low, medium, high")
	}

	// Validação da voz — cada role independente
	validVoiceProviders := []string{"", "disabled", "webspeech", "sapi5", "openai"}
	voiceRoles := map[string]VoiceRoleConfig{
		"voice.assistant": p.Voice.Assistant,
		"voice.user":      p.Voice.User,
		"voice.system":    p.Voice.System,
	}
	for name, role := range voiceRoles {
		if !containsStr(validVoiceProviders, role.Provider) {
			return fmt.Errorf("%s.provider must be one of: disabled, webspeech, sapi5, openai", name)
		}
		if !containsStr([]string{"", "model_and_voice", "model_only"}, role.SelectionMode) {
			return fmt.Errorf("%s.selection_mode must be one of: model_and_voice, model_only", name)
		}
		if role.Enabled && role.Provider == "openai" {
			if role.LLMProviderID == "" {
				return fmt.Errorf("%s.llm_provider_id is required for HTTP TTS", name)
			}
			if role.Model == "" {
				return fmt.Errorf("%s.model is required for HTTP TTS", name)
			}
			if role.SelectionMode == "" {
				return fmt.Errorf("%s.selection_mode is required for HTTP TTS", name)
			}
			expectedSelectionMode := "model_and_voice"
			if strings.HasPrefix(strings.ToLower(role.Model), "voice-") {
				expectedSelectionMode = "model_only"
			}
			if role.SelectionMode != expectedSelectionMode {
				return fmt.Errorf("%s.selection_mode must be %s for model %q", name, expectedSelectionMode, role.Model)
			}
			if role.SelectionMode == "model_and_voice" && role.VoiceID == "" {
				return fmt.Errorf("%s.voice_id is required when selection_mode is model_and_voice", name)
			}
			if role.SelectionMode == "model_only" && role.VoiceID != "" {
				return fmt.Errorf("%s.voice_id must be empty when selection_mode is model_only", name)
			}
		}
	}

	// Validação do modo de resposta para canais
	validChannelModes := []string{"", ChannelResponseMirror, ChannelResponseAlwaysText, ChannelResponseAlwaysAudio}
	if !containsStr(validChannelModes, p.Channels.ResponseMode) {
		return fmt.Errorf("channels.response_mode must be one of: mirror, always_text, always_audio")
	}

	// Validação do input (STT)
	validSTTProviders := []string{"webspeech", "whisper_api", ""}
	if !containsStr(validSTTProviders, p.Input.STTProvider) {
		return fmt.Errorf("input.stt_provider must be one of: webspeech, whisper_api")
	}

	// Validação dos triggers
	validTriggerTypes := []string{TriggerTypeHotkey, TriggerTypeButtonPTT, TriggerTypeButtonToggle, TriggerTypeWakeword, TriggerTypeVAD}
	for i, t := range p.Input.Triggers {
		if !containsStr(validTriggerTypes, t.Type) {
			return fmt.Errorf("input.triggers[%d].type must be one of: hotkey, button_ptt, button_toggle, wakeword, vad", i)
		}
		if t.Type == TriggerTypeHotkey && t.Hotkey == "" {
			return fmt.Errorf("input.triggers[%d].hotkey is required for type hotkey", i)
		}
		if t.Type == TriggerTypeWakeword && t.WakewordKeyword == "" {
			return fmt.Errorf("input.triggers[%d].wakeword_keyword is required for type wakeword", i)
		}
	}

	return nil
}

// ShouldUseAriaLiveForAgent retorna se deve usar aria-live para mensagens do assistente
func (p *Profile) ShouldUseAriaLiveForAgent() bool {
	return !p.Voice.Assistant.Enabled || p.Voice.Assistant.Provider == "disabled"
}

// ShouldUseAriaLiveForUser retorna se deve usar aria-live para mensagens do usuário
func (p *Profile) ShouldUseAriaLiveForUser() bool {
	return !p.Voice.User.Enabled || p.Voice.User.Provider == "disabled"
}

// IsVoiceDisabled retorna true se nenhuma role de TTS está ativa
func (p *Profile) IsVoiceDisabled() bool {
	return !p.Voice.Assistant.Enabled && !p.Voice.User.Enabled && !p.Voice.System.Enabled
}

// GetChannelResponseMode retorna o modo de resposta efetivo para canais externos.
func (p *Profile) GetChannelResponseMode() string {
	if p.Channels.ResponseMode == "" {
		return ChannelResponseMirror
	}
	return p.Channels.ResponseMode
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
