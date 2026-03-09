package profiles

import (
	"testing"
)

func TestGetMaxContextMessages(t *testing.T) {
	tests := []struct {
		name     string
		value    int
		expected int
	}{
		{"zero returns default 50", 0, 50},
		{"negative returns default 50", -5, 50},
		{"custom value 100", 100, 100},
		{"custom value 10", 10, 10},
		{"value 1", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Profile{Chat: ChatConfig{MaxContextMessages: tt.value}}
			result := p.GetMaxContextMessages()
			if result != tt.expected {
				t.Errorf("GetMaxContextMessages() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestGetMinContextMessages(t *testing.T) {
	tests := []struct {
		name     string
		value    int
		expected int
	}{
		{"zero returns default 10", 0, 10},
		{"negative returns default 10", -3, 10},
		{"custom value 5", 5, 5},
		{"custom value 20", 20, 20},
		{"value 1", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Profile{Chat: ChatConfig{MinContextMessages: tt.value}}
			result := p.GetMinContextMessages()
			if result != tt.expected {
				t.Errorf("GetMinContextMessages() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestValidateContextFields(t *testing.T) {
	validBase := func() *Profile {
		return &Profile{
			Name: "Test",
			Chat: ChatConfig{
				LLMProvider:     "openai-default",
				Temperature:     0.7,
				MaxTokens:       4096,
				TopP:            1.0,
				ResponseTimeout: 30,
			},
			Voice: VoiceConfig{
				Provider: "disabled",
				Rate:     1.0,
				Pitch:    1.0,
				Volume:   1.0,
			},
			Interaction: InteractionConfig{
				STTProvider: "webspeech",
				Language:    "pt-BR",
			},
		}
	}

	t.Run("valid: all context fields zero (defaults)", func(t *testing.T) {
		p := validBase()
		if err := p.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid: context_window positive", func(t *testing.T) {
		p := validBase()
		p.Chat.ContextWindow = 128000
		if err := p.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid: context_window negative", func(t *testing.T) {
		p := validBase()
		p.Chat.ContextWindow = -1
		if err := p.Validate(); err == nil {
			t.Error("expected error for negative context_window")
		}
	})

	t.Run("valid: max_context_messages positive", func(t *testing.T) {
		p := validBase()
		p.Chat.MaxContextMessages = 100
		if err := p.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid: max_context_messages negative", func(t *testing.T) {
		p := validBase()
		p.Chat.MaxContextMessages = -1
		if err := p.Validate(); err == nil {
			t.Error("expected error for negative max_context_messages")
		}
	})

	t.Run("valid: min_context_messages positive", func(t *testing.T) {
		p := validBase()
		p.Chat.MinContextMessages = 5
		if err := p.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid: min_context_messages negative", func(t *testing.T) {
		p := validBase()
		p.Chat.MinContextMessages = -1
		if err := p.Validate(); err == nil {
			t.Error("expected error for negative min_context_messages")
		}
	})

	t.Run("invalid: min >= max context messages", func(t *testing.T) {
		p := validBase()
		p.Chat.MaxContextMessages = 20
		p.Chat.MinContextMessages = 20
		if err := p.Validate(); err == nil {
			t.Error("expected error when min_context_messages >= max_context_messages")
		}
	})

	t.Run("invalid: min > max context messages", func(t *testing.T) {
		p := validBase()
		p.Chat.MaxContextMessages = 10
		p.Chat.MinContextMessages = 15
		if err := p.Validate(); err == nil {
			t.Error("expected error when min_context_messages > max_context_messages")
		}
	})

	t.Run("valid: min < max context messages", func(t *testing.T) {
		p := validBase()
		p.Chat.MaxContextMessages = 50
		p.Chat.MinContextMessages = 10
		if err := p.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid: only min set, max zero (default)", func(t *testing.T) {
		p := validBase()
		p.Chat.MinContextMessages = 10
		p.Chat.MaxContextMessages = 0
		if err := p.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid: only max set, min zero (default)", func(t *testing.T) {
		p := validBase()
		p.Chat.MaxContextMessages = 50
		p.Chat.MinContextMessages = 0
		if err := p.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestContextFieldsInteraction(t *testing.T) {
	t.Run("defaults: min < max", func(t *testing.T) {
		p := &Profile{}
		min := p.GetMinContextMessages()
		max := p.GetMaxContextMessages()
		if min >= max {
			t.Errorf("default min (%d) should be < default max (%d)", min, max)
		}
	})

	t.Run("custom values respected", func(t *testing.T) {
		p := &Profile{
			Chat: ChatConfig{
				MaxContextMessages: 30,
				MinContextMessages: 5,
			},
		}
		if p.GetMaxContextMessages() != 30 {
			t.Errorf("expected 30, got %d", p.GetMaxContextMessages())
		}
		if p.GetMinContextMessages() != 5 {
			t.Errorf("expected 5, got %d", p.GetMinContextMessages())
		}
	})
}
