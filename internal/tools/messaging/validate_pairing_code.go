package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	"assistente/internal/contacts"
	"assistente/internal/tools"
)

// ValidatePairingCodeArgs são os argumentos da tool validate_pairing_code
type ValidatePairingCodeArgs struct {
	Channel   string `json:"channel"`
	ContactID string `json:"contact_id"`
	Code      string `json:"code"`
}

// ValidatePairingCodeTool valida um código de pareamento
type ValidatePairingCodeTool struct{}

func NewValidatePairingCodeTool() *ValidatePairingCodeTool {
	return &ValidatePairingCodeTool{}
}

func (t *ValidatePairingCodeTool) Name() string {
	return "validate_pairing_code"
}

func (t *ValidatePairingCodeTool) Description() string {
	return `Validate one pending six-digit external-channel pairing code against its exact channel and contact identifier.
Use when: only an internal recovery flow explicitly asks to check a code that is already pending for that same channel/contact pair. The normal inbound pairing flow is handled directly by the messaging gateway before any message reaches the LLM.
Do not use: do not initiate pairing, guess a code, validate ordinary message text, or treat this as message sending or retry. This operation only validates and consumes pairing state; by itself it does not add the contact to the authorized contacts list.
Risk: a wrong code consumes one of the limited attempts, and a correct code is single-use and removed after validation. Confirm channel, contact_id, and all six digits before calling.
Cost: local in-memory validation only; no network request or LLM call.
Example: {"channel":"telegram","contact_id":"123456789","code":"042731"}.`
}

func (t *ValidatePairingCodeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"channel": {"type": "string", "description": "Exact external channel on which the pending code was generated, such as telegram, signal, or slack."},
			"contact_id": {"type": "string", "description": "Exact sender/contact identifier associated with the pending code on that channel; not a display name or username."},
			"code": {"type": "string", "description": "Exact six-digit code as a string, preserving any leading zero. A wrong value consumes an attempt; a correct value is single-use."}
		},
		"required": ["channel", "contact_id", "code"]
	}`)
}

func (t *ValidatePairingCodeTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params ValidatePairingCodeArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao parsear argumentos: %v", err),
			IsError: true,
		}, nil
	}

	if params.Channel == "" || params.ContactID == "" || params.Code == "" {
		return tools.ToolResult{
			Content: "Channel, contact_id e code são obrigatórios",
			IsError: true,
		}, nil
	}

	valid, err := contacts.ValidatePairingCode(params.Channel, params.ContactID, params.Code)
	if err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("❌ Código inválido: %s", err.Error()),
			IsError: true,
		}, nil
	}

	if !valid {
		return tools.ToolResult{
			Content: "❌ Código inválido",
			IsError: true,
		}, nil
	}

	return tools.ToolResult{
		Content: "✅ Pareamento bem-sucedido! Seu contato foi autorizado.",
		IsError: false,
	}, nil
}
