package profiles

// UnifiedProfile representa o profile unificado que combina chat, voz e interação
type UnifiedProfile struct {
	Name        string             `json:"name" yaml:"name"`
	Description string             `json:"description,omitempty" yaml:"description,omitempty"`
	Icon        string             `json:"icon,omitempty" yaml:"icon,omitempty"`
	Chat        ChatSection        `json:"chat" yaml:"chat"`
	Voice       VoiceSection       `json:"voice,omitempty" yaml:"voice,omitempty"`
	Interaction InteractionSection `json:"interaction,omitempty" yaml:"interaction,omitempty"`

	// Metadados internos (não serializados no YAML)
	FilePath string `json:"file_path,omitempty" yaml:"-"`
}

// ChatSection contém configurações de chat/modelo
type ChatSection struct {
	Model                string   `json:"model,omitempty" yaml:"model,omitempty"`
	Temperature          float64  `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	MaxTokens            int      `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	TopP                 float64  `json:"top_p,omitempty" yaml:"top_p,omitempty"`
	ResponseTimeout      int      `json:"response_timeout,omitempty" yaml:"response_timeout,omitempty"`
	EnableThinking       bool     `json:"enable_thinking,omitempty" yaml:"enable_thinking,omitempty"`
	UseTools             bool     `json:"use_tools" yaml:"use_tools"`
	ToolsList            []string `json:"tools_list,omitempty" yaml:"tools_list,omitempty"`
	SystemPrompt         string   `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`
	SystemPromptPosition string   `json:"system_prompt_position,omitempty" yaml:"system_prompt_position,omitempty"`
	IncludeCoreMemories  bool     `json:"include_core_memories" yaml:"include_core_memories"`
	EmbeddingsModel      string   `json:"embeddings_model,omitempty" yaml:"embeddings_model,omitempty"`
	EmbeddingsDimensions int      `json:"embeddings_dimensions,omitempty" yaml:"embeddings_dimensions,omitempty"`
	ImageModel           string   `json:"image_model,omitempty" yaml:"image_model,omitempty"`
	ShowInternalMessages bool     `json:"show_internal_messages,omitempty" yaml:"show_internal_messages,omitempty"`
}

// VoiceSection contém configurações de TTS
type VoiceSection struct {
	Provider        string  `json:"provider,omitempty" yaml:"provider,omitempty"`
	VoiceID         string  `json:"voice_id,omitempty" yaml:"voice_id,omitempty"`
	Rate            float64 `json:"rate,omitempty" yaml:"rate,omitempty"`
	Pitch           float64 `json:"pitch,omitempty" yaml:"pitch,omitempty"`
	Volume          float64 `json:"volume,omitempty" yaml:"volume,omitempty"`
	EnabledForAgent bool    `json:"enabled_for_agent,omitempty" yaml:"enabled_for_agent,omitempty"`
	EnabledForUser  bool    `json:"enabled_for_user,omitempty" yaml:"enabled_for_user,omitempty"`
}

// InteractionSection contém configurações de interação por voz
type InteractionSection struct {
	STTProvider    string    `json:"stt_provider,omitempty" yaml:"stt_provider,omitempty"`
	Language       string    `json:"language,omitempty" yaml:"language,omitempty"`
	FeedbackSounds bool      `json:"feedback_sounds,omitempty" yaml:"feedback_sounds,omitempty"`
	Triggers       []Trigger `json:"triggers,omitempty" yaml:"triggers,omitempty"`
}

// Trigger representa um trigger de interação
type Trigger struct {
	Type                string  `json:"type" yaml:"type"`
	Enabled             bool    `json:"enabled" yaml:"enabled"`
	AutoStop            bool    `json:"auto_stop,omitempty" yaml:"auto_stop,omitempty"`
	Hotkey              string  `json:"hotkey,omitempty" yaml:"hotkey,omitempty"`
	HotkeyGlobal        bool   `json:"hotkey_global,omitempty" yaml:"hotkey_global,omitempty"`
	HotkeyBringToFront   bool   `json:"hotkey_bring_to_front,omitempty" yaml:"hotkey_bring_to_front,omitempty"`
	WakewordKeyword     string  `json:"wakeword_keyword,omitempty" yaml:"wakeword_keyword,omitempty"`
	WakewordSensitivity float64 `json:"wakeword_sensitivity,omitempty" yaml:"wakeword_sensitivity,omitempty"`
	VADSilenceThreshold float64 `json:"vad_silence_threshold,omitempty" yaml:"vad_silence_threshold,omitempty"`
	VADSilenceDuration  int     `json:"vad_silence_duration,omitempty" yaml:"vad_silence_duration,omitempty"`
}

// DefaultProfile retorna um profile com valores padrão sensatos
func DefaultProfile() *UnifiedProfile {
	return &UnifiedProfile{
		Name:        "default",
		Description: "Perfil padrão",
		Icon:        "💬",
		Chat: ChatSection{
			Model:               "gpt-4o-mini",
			Temperature:         0.7,
			MaxTokens:           4096,
			TopP:                1.0,
			ResponseTimeout:     180,
			UseTools:            true,
			IncludeCoreMemories: true,
			EmbeddingsModel:     "text-embedding-3-small",
			ImageModel:          "dall-e-3",
			SystemPromptPosition: "before",
		},
		Voice: VoiceSection{
			Provider: "disabled",
			Rate:     1.0,
			Pitch:    1.0,
			Volume:   1.0,
		},
		Interaction: InteractionSection{
			STTProvider:    "webspeech",
			Language:       "pt-BR",
			FeedbackSounds: true,
		},
	}
}
