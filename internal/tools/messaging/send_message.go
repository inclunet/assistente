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
	return `Send one new outbound text immediately through a configured external Telegram, Signal, or Slack adapter.
Use when: the user explicitly asks to notify or message a known external destination, and you already have the exact channel-specific recipient identifier.
Do not use: do not use for the assistant's normal reply to the current chat or an inbound channel message; the backend-driven conversation pipeline delivers those replies. This tool is also not Chat.SendMessage, does not create a conversation or persisted user message, and must not replace Chat.RetryMessage, which re-runs a response from an existing persisted user message.
Risk: this performs an external side effect immediately. Verify the channel, recipient, and final text before calling; repeating a call can send a duplicate.
Cost: one adapter/network send; it does not invoke the LLM or wait for a reply.
Example: {"channel":"signal","to":"+5511999999999","message":"O relatório ficou pronto."}.`
}

func (t *SendMessageTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"channel": {
				"type": "string",
				"description": "Configured and connected external adapter that will send the message. Choose exactly one of telegram, signal, or slack.",
				"enum": ["telegram", "signal", "slack"]
			},
			"to": {
				"type": "string",
				"description": "Exact destination identifier already known for the selected channel: Telegram chat_id, Signal E.164 phone number, or Slack channel/DM ID such as C…/D…. Do not pass a Slack user ID (U…)."
			},
			"message": {
				"type": "string",
				"description": "Final text to send immediately. Pass only recipient-facing content, not tool instructions or retry metadata."
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
