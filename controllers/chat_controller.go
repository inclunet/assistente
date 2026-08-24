package controllers

import (
	"assistente/internal/agent"
	"assistente/internal/channels"
	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/core/usecases"
	"assistente/internal/llm"
	"assistente/internal/logging"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/messaging"
	"assistente/internal/profileadequacy"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
	"assistente/internal/speech"
	"assistente/internal/subagent"
	"assistente/internal/tools"
	"context"

	"github.com/google/uuid"
)

// ChatControllerConfig agrupa todas as dependências do ChatController.
type ChatControllerConfig struct {
	Emitter             ports.Emitter
	ChatInteractor      *chat.Interactor
	ToolRegistry        *tools.Registry
	ProviderSvc         *providers.Service
	MCPMgr              *mcpmgr.Manager
	AgentSvc            *agent.Service
	StreamMgr           *chat.StreamingManager
	SpeechSvc           *speech.Service
	ConvRepo            chat.ConversationRepository
	MsgGateway          *messaging.Gateway
	ResponseNotifier    *messaging.ResponseNotifier
	OnSpeechRequest     func(conversationID string, messageID string, role, text, origin, profileSlug string, interrupt bool)
	OpenEditorPaths     func() []string
	ProfileAdvisor      *profileadequacy.Advisor
	QuestionnaireRouter *questionnaire.Router
	SwitchTabProfile    func(tabID, conversationID, profileSlug string) error
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
	loadedToolStore  *tools.LoadedToolStore
}

// NewChatController cria um ChatController com todas as suas dependências.
func NewChatController(cfg ChatControllerConfig) *ChatController {
	loadedToolStore := tools.NewLoadedToolStore()
	return &ChatController{
		emitter:          cfg.Emitter,
		streamMgr:        cfg.StreamMgr,
		convRepo:         cfg.ConvRepo,
		msgGateway:       cfg.MsgGateway,
		responseNotifier: cfg.ResponseNotifier,
		loadedToolStore:  loadedToolStore,
		sendMsgUC: usecases.NewSendMessageUseCase(usecases.SendMessageConfig{
			ChatInteractor:      cfg.ChatInteractor,
			ToolRegistry:        cfg.ToolRegistry,
			LoadedToolStore:     loadedToolStore,
			ProviderSvc:         cfg.ProviderSvc,
			MCPMgr:              cfg.MCPMgr,
			AgentSvc:            cfg.AgentSvc,
			StreamMgr:           cfg.StreamMgr,
			SpeechSvc:           cfg.SpeechSvc,
			Emitter:             cfg.Emitter,
			OnSpeechRequest:     cfg.OnSpeechRequest,
			OpenEditorPaths:     cfg.OpenEditorPaths,
			ProfileAdvisor:      cfg.ProfileAdvisor,
			QuestionnaireRouter: cfg.QuestionnaireRouter,
			SwitchTabProfile:    cfg.SwitchTabProfile,
		}),
	}
}

// SendMessage é o ponto de entrada para mensagens originadas pelo frontend Wails.
// Registra o bridge canal↔Wails antes de delegar para o Use Case.
func (c *ChatController) SendMessage(ctx context.Context, conversationID string, userContent, userMedia string, params llm.ChatParams) (string, error) {
	bridgeTrace := ""
	if conversationID != "" && c.msgGateway != nil && c.responseNotifier != nil {
		bridgeTrace = c.registerChannelBridge(ctx, conversationID)
	}
	msgID, err := c.sendMsgUC.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: conversationID,
		UserContent:    userContent,
		UserMedia:      userMedia,
		Params:         params,
		Source:         "wails",
	})
	if err != nil && bridgeTrace != "" && c.responseNotifier != nil {
		// Erro síncrono antes do Notify: remove só este bridge (não o gateway).
		c.responseNotifier.CancelTrace(conversationID, bridgeTrace)
	}
	return msgID, err
}

// RetryMessage reexecuta o turno a partir de uma mensagem já persistida, sem duplicar a mensagem do usuário.
func (c *ChatController) RetryMessage(ctx context.Context, conversationID string, messageID string, params llm.ChatParams) (string, error) {
	bridgeTrace := ""
	if conversationID != "" && c.msgGateway != nil && c.responseNotifier != nil {
		bridgeTrace = c.registerChannelBridge(ctx, conversationID)
	}
	msgID, err := c.sendMsgUC.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: conversationID,
		RetryMessageID: messageID,
		Params:         params,
		Source:         "wails",
	})
	if err != nil && bridgeTrace != "" && c.responseNotifier != nil {
		c.responseNotifier.CancelTrace(conversationID, bridgeTrace)
	}
	return msgID, err
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

// SendForSubagent dispara um envio de sub-agente (AEP-0068) pela MESMA
// SendMessageUseCase usada pelo chat e canais — sem fluxo alternativo de envio
// (AEP-0040). A conversa (sub-conversa) deve já existir; o Manager de
// sub-agentes a cria antes de chamar este método.
func (c *ChatController) SendForSubagent(ctx context.Context, conversationID, prompt, media, profileSlug, model string) (string, error) {
	return c.sendMsgUC.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: conversationID,
		UserContent:    prompt,
		UserMedia:      media,
		Params:         llm.ChatParams{ProfileSlug: profileSlug, Model: model},
		Source:         subagent.Source,
	})
}

// CancelStreamingForConversation cancela um streaming LLM em andamento (barge-in).
func (c *ChatController) CancelStreamingForConversation(conversationID string) {
	c.streamMgr.Cancel(conversationID)
}

// ResetLoadedToolsForConversation descarta tools carregadas sob demanda para uma
// conversa que foi recriada, reciclada ou removida logicamente.
func (c *ChatController) ResetLoadedToolsForConversation(conversationID string) {
	if c == nil || c.loadedToolStore == nil {
		return
	}
	c.loadedToolStore.ResetConversation(conversationID)
}

// registerChannelBridge registra um callback para reenviar a resposta do assistente
// ao canal de mensageria de origem (bridge Wails → canal externo).
// Retorna o TraceID do bridge (vazio se não registrou) para CancelTrace em erro síncrono.
func (c *ChatController) registerChannelBridge(ctx context.Context, conversationID string) string {
	conv, err := c.convRepo.GetConversationInfo(ctx, conversationID)
	if err != nil || conv == nil || conv.Channel == "" || conv.ContactID == "" {
		return "" // Conversa local do Wails, não precisa de bridge.
	}

	messenger, ok := c.msgGateway.GetMessenger(conv.Channel)
	if !ok {
		return "" // Messenger não registrado.
	}

	logging.Infof(ctx, "controllers.chat-controller", "[Bridge] Registrando bridge Wails→%s para conversa %s (contact=%s)", conv.Channel, conversationID, conv.ContactID)

	channelName := conv.Channel
	contactID := conv.ContactID
	// Snapshot do destino no Register (igual ao gateway). Re-resolver no
	// Callback via GetReplyChatID permitiria que outra mensagem Slack do
	// mesmo user em outro channel sobrescrevesse o destino mid-flight.
	replyChatID := channels.GetReplyChatID(channelName, contactID)
	traceID := uuid.NewString()
	c.responseNotifier.Register(conversationID, messaging.ResponseCallback{
		Channel:     channelName,
		ChatID:      replyChatID,
		OwnerUserID: conv.UserID,
		TraceID:     traceID,
		// SkipPersist: persistência M14 fica no Register do gateway.
		SkipPersist: true,
		Callback: func(response string, assistantMsgID string) {
			err := messenger.Send(context.Background(), messaging.OutgoingMessage{
				ChatID: replyChatID,
				Text:   response,
			})
			if err != nil {
				logging.Errorf(ctx, "controllers.chat-controller", "[Bridge] Erro ao reenviar resposta para %s contact=%s replyChat=%s: %v", channelName, contactID, replyChatID, err)
			} else {
				logging.Infof(ctx, "controllers.chat-controller", "[Bridge] Resposta reenviada para %s contact=%s replyChat=%s", channelName, contactID, replyChatID)
			}
		},
	})
	return traceID
}
