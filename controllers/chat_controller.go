package controllers

import (
	"context"
	"log"

	"assistente/internal/agent"
	"assistente/internal/chat"
	"assistente/internal/config"
	"assistente/internal/core/ports"
	"assistente/internal/core/usecases"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/messaging"
	"assistente/internal/providers"
	"assistente/internal/speech"
	"assistente/internal/tools"
)

// ChatControllerConfig agrupa todas as dependências do ChatController.
type ChatControllerConfig struct {
	Emitter          ports.Emitter
	ChatInteractor   *chat.Interactor
	ToolRegistry     *tools.Registry
	ProviderSvc      *providers.Service
	MCPMgr           *mcpmgr.Manager
	AgentSvc         *agent.Service
	StreamMgr        *chat.StreamingManager
	SpeechSvc        *speech.Service
	SettingsSvc      *config.SettingsService
	ConvRepo         chat.ConversationRepository
	MsgGateway       *messaging.Gateway
	ResponseNotifier *messaging.ResponseNotifier
	OnSpeechRequest  func(conversationID string, messageID string, role, text, origin, profileSlug string, interrupt bool)
	OpenEditorPaths  func() []string
}

// ChatController é o adapter primário (Inbound) para o pipeline de envio de mensagens.
// Orquestra o bridge canal↔Wails e delega o pipeline de mensagem ao SendMessageUseCase.
type ChatController struct {
	emitter          ports.Emitter
	streamMgr        *chat.StreamingManager
	convRepo         chat.ConversationRepository
	msgGateway       *messaging.Gateway
	responseNotifier *messaging.ResponseNotifier
	sendMsgUC        *usecases.SendMessageUseCase
}

// NewChatController cria um ChatController com todas as suas dependências.
func NewChatController(cfg ChatControllerConfig) *ChatController {
	return &ChatController{
		emitter:          cfg.Emitter,
		streamMgr:        cfg.StreamMgr,
		convRepo:         cfg.ConvRepo,
		msgGateway:       cfg.MsgGateway,
		responseNotifier: cfg.ResponseNotifier,
		sendMsgUC: usecases.NewSendMessageUseCase(usecases.SendMessageConfig{
			ChatInteractor:  cfg.ChatInteractor,
			ToolRegistry:    cfg.ToolRegistry,
			ProviderSvc:     cfg.ProviderSvc,
			MCPMgr:          cfg.MCPMgr,
			AgentSvc:        cfg.AgentSvc,
			StreamMgr:       cfg.StreamMgr,
			SpeechSvc:       cfg.SpeechSvc,
			SettingsSvc:     cfg.SettingsSvc,
			Emitter:         cfg.Emitter,
			OnSpeechRequest: cfg.OnSpeechRequest,
			OpenEditorPaths: cfg.OpenEditorPaths,
		}),
	}
}

// SendMessage é o ponto de entrada para mensagens originadas pelo frontend Wails.
// Registra o bridge canal↔Wails antes de delegar para o Use Case.
func (c *ChatController) SendMessage(ctx context.Context, conversationID string, userContent, userMedia string, params llm.ChatParams) (string, error) {
	if conversationID != "" && c.msgGateway != nil && c.responseNotifier != nil {
		c.registerChannelBridge(ctx, conversationID)
	}
	return c.sendMsgUC.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: conversationID,
		UserContent:    userContent,
		UserMedia:      userMedia,
		Params:         params,
		Source:         "wails",
	})
}

// RetryMessage reexecuta o turno a partir de uma mensagem já persistida, sem duplicar a mensagem do usuário.
func (c *ChatController) RetryMessage(ctx context.Context, conversationID string, messageID string, params llm.ChatParams) (string, error) {
	if conversationID != "" && c.msgGateway != nil && c.responseNotifier != nil {
		c.registerChannelBridge(ctx, conversationID)
	}
	return c.sendMsgUC.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: conversationID,
		RetryMessageID: messageID,
		Params:         params,
		Source:         "wails",
	})
}

// SendMessageFromChannel é chamado pelo Gateway de mensageria (Telegram, Signal, etc.).
func (c *ChatController) SendMessageFromChannel(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
	return c.sendMsgUC.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: conversationID,
		UserContent:    content,
		UserMedia:      media,
		Params:         params,
		Source:         source,
	})
}

// CancelStreamingForConversation cancela um streaming LLM em andamento (barge-in).
func (c *ChatController) CancelStreamingForConversation(conversationID string) {
	c.streamMgr.Cancel(conversationID)
}

// registerChannelBridge registra um callback para reenviar a resposta do assistente
// ao canal de mensageria de origem (bridge Wails → canal externo).
func (c *ChatController) registerChannelBridge(ctx context.Context, conversationID string) {
	conv, err := c.convRepo.GetConversationInfo(ctx, conversationID)
	if err != nil || conv == nil || conv.Channel == "" || conv.ContactID == "" {
		return // Conversa local do Wails, não precisa de bridge.
	}

	messenger, ok := c.msgGateway.GetMessenger(conv.Channel)
	if !ok {
		return // Messenger não registrado.
	}

	log.Printf("[Bridge] Registrando bridge Wails→%s para conversa %s (contato: %s)", conv.Channel, conversationID, conv.ContactID)

	c.responseNotifier.Register(conversationID, messaging.ResponseCallback{
		Channel: conv.Channel,
		ChatID:  conv.ContactID,
		Callback: func(response string, assistantMsgID string) {
			err := messenger.Send(context.Background(), messaging.OutgoingMessage{
				ChatID: conv.ContactID,
				Text:   response,
			})
			if err != nil {
				log.Printf("[Bridge] Erro ao reenviar resposta para %s/%s: %v", conv.Channel, conv.ContactID, err)
			} else {
				log.Printf("[Bridge] Resposta reenviada para %s/%s", conv.Channel, conv.ContactID)
			}
		},
	})
}
