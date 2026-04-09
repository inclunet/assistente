package controllers

import (
	"context"
	"fmt"
	"log"

	"assistente/internal/agent"
	"assistente/internal/chat"
	"assistente/internal/config"
	"assistente/internal/core/ports"
	"assistente/internal/events"
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
}

// ChatController é o adapter primário (Inbound) para o pipeline de envio de mensagens.
// Contém toda a orquestração de sendMessageInternal, desacoplada do *App e do Wails.
type ChatController struct {
	emitter          ports.Emitter
	chatInteractor   *chat.Interactor
	toolRegistry     *tools.Registry
	providerSvc      *providers.Service
	mcpMgr           *mcpmgr.Manager
	agentSvc         *agent.Service
	streamMgr        *chat.StreamingManager
	speechSvc        *speech.Service
	settingsSvc      *config.SettingsService
	convRepo         chat.ConversationRepository
	msgGateway       *messaging.Gateway
	responseNotifier *messaging.ResponseNotifier
}

// NewChatController cria um ChatController com todas as suas dependências.
func NewChatController(cfg ChatControllerConfig) *ChatController {
	return &ChatController{
		emitter:          cfg.Emitter,
		chatInteractor:   cfg.ChatInteractor,
		toolRegistry:     cfg.ToolRegistry,
		providerSvc:      cfg.ProviderSvc,
		mcpMgr:           cfg.MCPMgr,
		agentSvc:         cfg.AgentSvc,
		streamMgr:        cfg.StreamMgr,
		speechSvc:        cfg.SpeechSvc,
		settingsSvc:      cfg.SettingsSvc,
		convRepo:         cfg.ConvRepo,
		msgGateway:       cfg.MsgGateway,
		responseNotifier: cfg.ResponseNotifier,
	}
}

// SendMessage é o ponto de entrada para mensagens originadas pelo frontend Wails.
// Registra o bridge canal↔Wails antes de delegar para o pipeline interno.
func (c *ChatController) SendMessage(ctx context.Context, conversationID uint, userContent, userMedia string, params llm.ChatParams) (uint, error) {
	if conversationID > 0 && c.msgGateway != nil && c.responseNotifier != nil {
		c.registerChannelBridge(conversationID)
	}
	return c.sendMessage(ctx, conversationID, userContent, userMedia, params, "wails")
}

// SendMessageFromChannel é chamado pelo Gateway de mensageria (Telegram, Signal, etc.).
func (c *ChatController) SendMessageFromChannel(ctx context.Context, conversationID uint, content, media string, params llm.ChatParams, source string) (uint, error) {
	return c.sendMessage(ctx, conversationID, content, media, params, source)
}

// CancelStreamingForConversation cancela um streaming LLM em andamento (barge-in).
func (c *ChatController) CancelStreamingForConversation(conversationID uint) {
	c.streamMgr.Cancel(conversationID)
}

// sendMessage contém toda a lógica do pipeline de mensagens, desacoplada do *App.
func (c *ChatController) sendMessage(ctx context.Context, conversationID uint, userContent, userMedia string, params llm.ChatParams, source string) (uint, error) {
	// Resolve modelo padrão do config como fallback.
	var defaultModel string
	if cfg, cfgErr := c.settingsSvc.GetConfig(); cfgErr == nil {
		defaultModel = cfg.DefaultModel
	}

	// Delega validação, renaming e resolução de perfil para o ChatInteractor.
	pctx, err := c.chatInteractor.PrepareContext(context.Background(), chat.PrepareContextRequest{
		ConversationID: conversationID,
		UserContent:    userContent,
		UserMedia:      userMedia,
		Params:         params,
		Source:         source,
		DefaultModel:   defaultModel,
	})
	if err != nil {
		return 0, err
	}
	activeProfile := pctx.ActiveProfile
	params = pctx.Params
	userContent = pctx.UserContent

	// Resolve conteúdo: extrai áudio do media e aplica STT fallback para canais.
	var sttProvider string
	if activeProfile != nil {
		sttProvider = activeProfile.Input.STTProvider
	}
	resolved := c.chatInteractor.ResolveUserContent(context.Background(), chat.ResolveUserContentRequest{
		Content:     userContent,
		Media:       userMedia,
		Source:      source,
		STTProvider: sttProvider,
		Transcribe:  c.whisperTranscribeFunc(),
	})
	userContent = resolved.Content

	// Persiste mensagem do usuário, emite ready e carrega histórico.
	rmsg, err := c.chatInteractor.RecordUserMessage(context.Background(), chat.RecordUserMessageRequest{
		ConversationID: conversationID,
		Content:        userContent,
		Media:          userMedia,
		AudioBase64:    resolved.AudioBase64,
		AudioMimeType:  resolved.AudioMimeType,
		Source:         source,
		ActiveProfile:  activeProfile,
		Transcribe:     c.whisperTranscribeFunc(),
	})
	if err != nil {
		return 0, err
	}
	userMsg := rmsg.UserMsg
	messages := rmsg.Messages
	conversationSummary := rmsg.ConversationSummary

	// Detecta slash skill, compõe system prompt e pré-processa mídia.
	prepResult := c.chatInteractor.PrepareMessages(chat.PrepareMessagesRequest{
		Messages:            messages,
		UserContent:         userContent,
		ConversationSummary: conversationSummary,
		ConversationID:      conversationID,
		Params:              params,
		ActiveProfile:       activeProfile,
		Transcribe:          c.whisperTranscribeFunc(),
	})
	messages = prepResult.Messages
	invokedSkillSlug := prepResult.InvokedSkillSlug
	invokedFilesystemScope := prepResult.InvokedScope

	// Constrói tool definitions para o LLM.
	disableTools := activeProfile != nil && activeProfile.Chat.DisableTools
	var enabledTools []string
	if activeProfile != nil {
		enabledTools = activeProfile.Chat.EnabledTools
	}
	llmToolDefs := chat.BuildLLMToolDefs(c.toolRegistry, enabledTools, disableTools)

	// Resolve o ChatProvider para o provedor do perfil ativo.
	if activeProfile == nil || activeProfile.Chat.LLMProvider == "" {
		errMsg := "Nenhum provedor LLM configurado no perfil ativo."
		c.emitter.Emit("chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}

	requestStreamer, err := c.providerSvc.GetChatProvider(activeProfile.Chat.LLMProvider)
	if err != nil {
		errMsg := fmt.Sprintf("Provedor LLM não disponível: %v", err)
		log.Printf("[SendMessage] ERRO: %s", errMsg)
		c.emitter.Emit("chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}
	log.Printf("[SendMessage] ChatProvider resolvido para provedor: %s", activeProfile.Chat.LLMProvider)

	// MCP nativo: configura servidores MCP HTTP no provider e remove suas tools da lista padrão.
	requestStreamer, llmToolDefs = chat.ApplyNativeMCP(requestStreamer, llmToolDefs, c.mcpMgr, enabledTools, disableTools)

	// Cria contexto cancelável por conversa — permite barge-in cancelar o LLM em andamento.
	convCtx, convCancel := context.WithCancel(ctx)
	c.streamMgr.Register(conversationID, convCancel)

	if len(llmToolDefs) > 0 {
		agentCtx := convCtx
		if invokedSkillSlug != "" {
			agentCtx = tools.WithExecutionContext(agentCtx, tools.ExecutionContext{
				InvokedSkillSlug: invokedSkillSlug,
				Filesystem:       invokedFilesystemScope,
			})
		}
		go func() {
			defer func() {
				r := recover()
				events.HandlePanic(c.emitter, conversationID, "runAgenticLoop", r)
			}()
			defer c.streamMgr.Unregister(conversationID)
			c.agentSvc.RunAgenticLoop(agentCtx, messages, params, conversationID, userMsg.ID, llmToolDefs, requestStreamer,
				func(convID uint, iter int) agent.IterationHandler {
					return agent.NewAgenticStreamHandler(c.emitter, convID, iter)
				},
			)
		}()
	} else {
		handler := c.agentSvc.NewSimpleStreamHandler(conversationID, userMsg.ID)
		go func() {
			defer func() {
				r := recover()
				events.HandlePanic(c.emitter, conversationID, "StreamChat", r)
			}()
			defer c.streamMgr.Unregister(conversationID)
			requestStreamer.StreamChat(convCtx, messages, params, handler)
		}()
	}
	return conversationID, nil
}

// registerChannelBridge registra um callback para reenviar a resposta do assistente
// ao canal de mensageria de origem (bridge Wails → canal externo).
func (c *ChatController) registerChannelBridge(conversationID uint) {
	conv, err := c.convRepo.GetConversationInfo(conversationID)
	if err != nil || conv == nil || conv.Channel == "" || conv.ContactID == "" {
		return // Conversa local do Wails, não precisa de bridge.
	}

	messenger, ok := c.msgGateway.GetMessenger(conv.Channel)
	if !ok {
		return // Messenger não registrado.
	}

	log.Printf("[Bridge] Registrando bridge Wails→%s para conversa %d (contato: %s)", conv.Channel, conversationID, conv.ContactID)

	c.responseNotifier.Register(conversationID, messaging.ResponseCallback{
		Channel: conv.Channel,
		ChatID:  conv.ContactID,
		Callback: func(response string, assistantMsgID uint) {
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

// whisperTranscribeFunc cria o callback de transcrição STT para o pipeline de mensagens.
func (c *ChatController) whisperTranscribeFunc() chat.TranscribeFunc {
	return func(audioBase64, filename string) (string, error) {
		result, err := c.speechSvc.Transcribe(audioBase64, filename)
		if err != nil {
			return "", err
		}
		if result == nil {
			return "", nil
		}
		return result.Text, nil
	}
}
