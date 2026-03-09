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
	return "Validates a pairing code for an unknown contact. Used internally to authorize contacts after they provide the correct 6-digit pairing code received in their first message."
}

func (t *ValidatePairingCodeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"channel": {"type": "string", "description": "Canal de mensageria (signal, telegram, etc)"},
			"contact_id": {"type": "string", "description": "ID do contato"},
			"code": {"type": "string", "description": "Código de pareamento de 6 dígitos"}
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
