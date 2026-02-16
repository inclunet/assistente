package messaging

import (
	"context"
	"encoding/json"
	"fmt"

	msgpkg "assistente/internal/messaging"
	"assistente/internal/tools"
)

// sendMessageArgs são os argumentos da tool send_message.
type sendMessageArgs struct {
	Channel string `json:"channel"` // "telegram", "signal", etc.
	To      string `json:"to"`      // chat_id ou identificador do contato
	Message string `json:"message"` // texto da mensagem a enviar
}

// SendMessageTool permite ao LLM enviar mensagens proativamente via mensageiros.
// Exemplos de uso: "me avise no Telegram quando terminar", "mande um resumo no Telegram".
type SendMessageTool struct {
	gateway *msgpkg.Gateway
}

// NewSendMessageTool cria uma nova instância da tool.
func NewSendMessageTool(gateway *msgpkg.Gateway) *SendMessageTool {
	return &SendMessageTool{gateway: gateway}
}

func (t *SendMessageTool) Name() string {
	return "send_message"
}

func (t *SendMessageTool) Description() string {
	return "Sends a text message via external messaging (Telegram/Signal). Use for proactive notifications when requested. The channel must be configured and the contact authorized."
}

func (t *SendMessageTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"channel": {
				"type": "string",
				"description": "The messaging platform to use (e.g., 'telegram', 'signal')",
				"enum": ["telegram", "signal"]
			},
			"to": {
				"type": "string",
				"description": "Recipient identifier (Telegram chat_id, Signal E.164 phone number, e.g., +5511999999999)"
			},
			"message": {
				"type": "string",
				"description": "The text message to send"
			}
		},
		"required": ["channel", "to", "message"]
	}`)
}

func (t *SendMessageTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params sendMessageArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao parsear argumentos: %v", err),
			IsError: true,
		}, nil
	}

	if params.Channel == "" || params.To == "" || params.Message == "" {
		return tools.ToolResult{
			Content: "Todos os campos são obrigatórios: channel, to, message",
			IsError: true,
		}, nil
	}

	if t.gateway == nil {
		return tools.ToolResult{
			Content: "Gateway de mensageria não está disponível",
			IsError: true,
		}, nil
	}

	messenger, ok := t.gateway.GetMessenger(params.Channel)
	if !ok {
		return tools.ToolResult{
			Content: fmt.Sprintf("Canal '%s' não está configurado ou conectado", params.Channel),
			IsError: true,
		}, nil
	}

	err := messenger.Send(ctx, msgpkg.OutgoingMessage{
		ChatID: params.To,
		Text:   params.Message,
	})
	if err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao enviar mensagem via %s: %v", params.Channel, err),
			IsError: true,
		}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Mensagem enviada com sucesso via %s para %s", params.Channel, params.To),
	}, nil
}
