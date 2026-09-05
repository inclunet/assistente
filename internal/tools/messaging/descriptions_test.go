package messaging

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessagingDescriptionsDistinguishSendRetryAndPairing(t *testing.T) {
	tests := []struct {
		name        string
		description string
		concepts    []string
	}{
		{
			name:        "send_message",
			description: NewSendMessageTool(nil).Description(),
			concepts: []string{
				"new outbound text",
				"normal reply",
				"inbound channel message",
				"backend-driven",
				"chat.sendmessage",
				"chat.retrymessage",
				"persisted user message",
				"duplicate",
				"example",
			},
		},
		{
			name:        "validate_pairing_code",
			description: NewValidatePairingCodeTool().Description(),
			concepts: []string{
				"pending six-digit",
				"internal recovery flow",
				"messaging gateway",
				"before any message reaches the llm",
				"does not add the contact",
				"limited attempts",
				"single-use",
				"example",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized := strings.ToLower(test.description)
			for _, section := range []string{"use when:", "do not use:", "risk:", "cost:"} {
				if !strings.Contains(normalized, section) {
					t.Errorf("description missing guidance section %q", section)
				}
			}
			for _, concept := range test.concepts {
				if !strings.Contains(normalized, strings.ToLower(concept)) {
					t.Errorf("description missing decision concept %q", concept)
				}
			}
		})
	}
}

func TestMessagingParameterDescriptionsExposeDestinationAndPairingRisks(t *testing.T) {
	tests := []struct {
		name       string
		parameters json.RawMessage
		concepts   map[string][]string
	}{
		{
			name:       "send_message",
			parameters: NewSendMessageTool(nil).Parameters(),
			concepts: map[string][]string{
				"channel": {"configured", "connected", "telegram", "signal", "slack"},
				"to":      {"exact destination", "chat_id", "e.164", "c…/d…", "u…"},
				"message": {"final text", "immediately", "recipient-facing", "retry metadata"},
			},
		},
		{
			name:       "validate_pairing_code",
			parameters: NewValidatePairingCodeTool().Parameters(),
			concepts: map[string][]string{
				"channel":    {"exact external channel", "pending code"},
				"contact_id": {"exact sender/contact identifier", "not a display name"},
				"code":       {"six-digit", "leading zero", "consumes an attempt", "single-use"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var schema struct {
				Properties map[string]struct {
					Description string `json:"description"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(test.parameters, &schema); err != nil {
				t.Fatalf("unmarshal parameters: %v", err)
			}
			for parameter, concepts := range test.concepts {
				description := strings.ToLower(schema.Properties[parameter].Description)
				for _, concept := range concepts {
					if !strings.Contains(description, strings.ToLower(concept)) {
						t.Errorf("parameter %q missing concept %q", parameter, concept)
					}
				}
			}
		})
	}
}
