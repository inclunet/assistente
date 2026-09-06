package profiles

import (
	"encoding/json"
	"strings"
)

// UnmarshalJSON mantém a leitura dos perfis publicados pela 0.1.9, cujo
// layout de voz, entrada e MCP precede as estruturas atuais. Releases 0.2.0+
// já usam o layout corrente e seguem pelo decode direto.
func (p *Profile) UnmarshalJSON(data []byte) error {
	type currentProfile Profile
	var current currentProfile
	if err := json.Unmarshal(data, &current); err != nil {
		return err
	}

	var shape struct {
		Interaction json.RawMessage `json:"interaction"`
	}
	if err := json.Unmarshal(data, &shape); err != nil {
		return err
	}
	if len(shape.Interaction) == 0 || string(shape.Interaction) == "null" {
		*p = Profile(current)
		return nil
	}

	var legacy legacyProfile019
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	migrated := Profile(current)
	migrated.Input = InputConfig{
		Enabled:        !legacy.Interaction.Disabled,
		STTProvider:    legacy.Interaction.STTProvider,
		LLMProviderID:  legacy.Interaction.LLMProviderID,
		Language:       legacy.Interaction.Language,
		FeedbackSounds: legacy.Interaction.FeedbackSounds,
		Triggers:       legacy.Interaction.Triggers,
	}
	migrated.Voice = migrateLegacyVoice019(legacy.Voice)
	if migrated.Channels.ResponseMode == "" {
		migrated.Channels.ResponseMode = legacy.Voice.ChannelResponseMode
	}
	switch strings.TrimSpace(legacy.Chat.MCPMode) {
	case "adapter":
		enabled := false
		migrated.Chat.NativeMCP = &enabled
	case "native":
		enabled := true
		migrated.Chat.NativeMCP = &enabled
	case "auto":
		if legacy.Chat.MCPNativeTested != nil && !*legacy.Chat.MCPNativeTested {
			enabled := false
			migrated.Chat.NativeMCP = &enabled
		}
	}
	*p = migrated
	return nil
}

type legacyProfile019 struct {
	Chat struct {
		MCPMode         string `json:"mcp_mode,omitempty"`
		MCPNativeTested *bool  `json:"mcp_native_tested,omitempty"`
	} `json:"chat"`
	Voice       legacyVoice019       `json:"voice"`
	Interaction legacyInteraction019 `json:"interaction"`
}

type legacyVoice019 struct {
	Disabled            bool    `json:"disabled,omitempty"`
	Provider            string  `json:"provider"`
	LLMProviderID       string  `json:"llm_provider_id,omitempty"`
	VoiceID             string  `json:"voice_id,omitempty"`
	Rate                float64 `json:"rate"`
	Pitch               float64 `json:"pitch"`
	Volume              float64 `json:"volume"`
	EnabledForAgent     bool    `json:"enabled_for_agent"`
	EnabledForUser      bool    `json:"enabled_for_user"`
	ChannelResponseMode string  `json:"channel_response_mode,omitempty"`
}

type legacyInteraction019 struct {
	Disabled       bool            `json:"disabled,omitempty"`
	STTProvider    string          `json:"stt_provider"`
	LLMProviderID  string          `json:"llm_provider_id,omitempty"`
	Language       string          `json:"language"`
	FeedbackSounds bool            `json:"feedback_sounds"`
	Triggers       []TriggerConfig `json:"triggers,omitempty"`
}

func migrateLegacyVoice019(old legacyVoice019) VoiceConfig {
	role := VoiceRoleConfig{
		Provider:      old.Provider,
		LLMProviderID: old.LLMProviderID,
		VoiceID:       old.VoiceID,
		Rate:          old.Rate,
		Pitch:         old.Pitch,
		Volume:        old.Volume,
	}
	if role.Provider == "" {
		role.Provider = "disabled"
	}
	if role.Provider == "openai" {
		role.Model = "tts-1"
		role.SelectionMode = "model_and_voice"
	}
	assistant := role
	assistant.Enabled = !old.Disabled && old.EnabledForAgent
	user := role
	user.Enabled = !old.Disabled && old.EnabledForUser
	return VoiceConfig{
		Assistant: assistant,
		User:      user,
		System: VoiceRoleConfig{
			Provider: "disabled",
			Rate:     old.Rate,
			Pitch:    old.Pitch,
			Volume:   old.Volume,
		},
	}
}
