package usecases

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
	"assistente/internal/providers"
	"assistente/internal/speech"
	"assistente/internal/tools"
)

// SendMessageConfig agrupa as dependências do SendMessageUseCase.
type SendMessageConfig struct {
	ChatInteractor *chat.Interactor
	ToolRegistry   *tools.Registry
	ProviderSvc    *providers.Service
	MCPMgr         *mcpmgr.Manager
	AgentSvc       *agent.Service
	StreamMgr      *chat.StreamingManager
	SpeechSvc      *speech.Service
	SettingsSvc    *config.SettingsService
	Emitter        ports.Emitter
	// OnSpeechRequest é chamado após salvar a mensagem do usuário para disparar TTS proativo.
	OnSpeechRequest func(conversationID uint, messageID uint, role, text, origin string, interrupt bool)
}

// SendMessageUseCase orquestra o pipeline completo de envio de mensagem ao LLM.
// É agnóstico de framework: zero imports de Wails, CLI ou HTTP.
type SendMessageUseCase struct {
	chatInteractor *chat.Interactor
	toolRegistry   *tools.Registry
	providerSvc    *providers.Service
	mcpMgr         *mcpmgr.Manager
	agentSvc       *agent.Service
	streamMgr      *chat.StreamingManager
	speechSvc      *speech.Service
	settingsSvc    *config.SettingsService
	emitter         ports.Emitter
	onSpeechRequest func(conversationID uint, messageID uint, role, text, origin string, interrupt bool)
}

// NewSendMessageUseCase cria um SendMessageUseCase com todas as dependências.
func NewSendMessageUseCase(cfg SendMessageConfig) *SendMessageUseCase {
	return &SendMessageUseCase{
		chatInteractor: cfg.ChatInteractor,
		toolRegistry:   cfg.ToolRegistry,
		providerSvc:    cfg.ProviderSvc,
		mcpMgr:         cfg.MCPMgr,
		agentSvc:       cfg.AgentSvc,
		streamMgr:      cfg.StreamMgr,
		speechSvc:      cfg.SpeechSvc,
		settingsSvc:    cfg.SettingsSvc,
		emitter:        cfg.Emitter,		onSpeechRequest: cfg.OnSpeechRequest,	}
}

// SendMessageRequest encapsula os parâmetros de entrada do Use Case.
type SendMessageRequest struct {
	Ctx            context.Context
	ConversationID uint
	UserContent    string
	UserMedia      string
	Params         llm.ChatParams
	Source         string
}

// Execute executa o pipeline de mensagem: prepara contexto → persiste → monta prompt
// → resolve LLM → lança goroutine de streaming (agêntico ou simples).
// Retorna o conversationID ou erro síncrono (problemas de configuração, banco, etc.).
func (uc *SendMessageUseCase) Execute(req SendMessageRequest) (uint, error) {
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Resolve modelo padrão do config como fallback.
	var defaultModel string
	if cfg, cfgErr := uc.settingsSvc.GetConfig(); cfgErr == nil {
		defaultModel = cfg.DefaultModel
	}

	// Delega validação, renaming e resolução de perfil para o ChatInteractor.
	pctx, err := uc.chatInteractor.PrepareContext(context.Background(), chat.PrepareContextRequest{
		ConversationID: req.ConversationID,
		UserContent:    req.UserContent,
		UserMedia:      req.UserMedia,
		Params:         req.Params,
		Source:         req.Source,
		DefaultModel:   defaultModel,
	})
	if err != nil {
		return 0, err
	}
	activeProfile := pctx.ActiveProfile
	params := pctx.Params
	userContent := pctx.UserContent

	// Resolve conteúdo: extrai áudio do media e aplica STT fallback para canais.
	var sttProvider string
	if activeProfile != nil {
		sttProvider = activeProfile.Input.STTProvider
	}
	resolved := uc.chatInteractor.ResolveUserContent(context.Background(), chat.ResolveUserContentRequest{
		Content:     userContent,
		Media:       req.UserMedia,
		Source:      req.Source,
		STTProvider: sttProvider,
		Transcribe:  uc.whisperTranscribeFunc(),
	})
	userContent = resolved.Content

	// Persiste mensagem do usuário, emite ready e carrega histórico.
	rmsg, err := uc.chatInteractor.RecordUserMessage(context.Background(), chat.RecordUserMessageRequest{
		ConversationID: req.ConversationID,
		Content:        userContent,
		Media:          req.UserMedia,
		AudioBase64:    resolved.AudioBase64,
		AudioMimeType:  resolved.AudioMimeType,
		Source:         req.Source,
		ActiveProfile:  activeProfile,
		Transcribe:     uc.whisperTranscribeFunc(),
	})
	if err != nil {
		return 0, err
	}
	userMsg := rmsg.UserMsg
	messages := rmsg.Messages
	conversationSummary := rmsg.ConversationSummary

	// TTS proativo: verbaliza a mensagem do usuário (síncrono para garantir ordem dos eventos)
	if uc.onSpeechRequest != nil && userContent != "" {
		uc.onSpeechRequest(req.ConversationID, userMsg.ID, "user", userContent, "user_message", true)
	}

	// Detecta slash skill, compõe system prompt e pré-processa mídia.
	prepResult := uc.chatInteractor.PrepareMessages(chat.PrepareMessagesRequest{
		Messages:            messages,
		UserContent:         userContent,
		ConversationSummary: conversationSummary,
		ConversationID:      req.ConversationID,
		Params:              params,
		ActiveProfile:       activeProfile,
		Transcribe:          uc.whisperTranscribeFunc(),
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
	llmToolDefs := chat.BuildLLMToolDefs(uc.toolRegistry, enabledTools, disableTools)

	// Resolve o ChatProvider para o provedor do perfil ativo.
	if activeProfile == nil || activeProfile.Chat.LLMProvider == "" {
		errMsg := "Nenhum provedor LLM configurado no perfil ativo."
		uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
		return 0, fmt.Errorf("%s", errMsg)
	}

	requestStreamer, err := uc.providerSvc.GetChatProvider(activeProfile.Chat.LLMProvider)
	if err != nil {
		errMsg := fmt.Sprintf("Provedor LLM não disponível: %v", err)
		log.Printf("[SendMessage] ERRO: %s", errMsg)
		uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
		return 0, fmt.Errorf("%s", errMsg)
	}
	log.Printf("[SendMessage] ChatProvider resolvido para provedor: %s", activeProfile.Chat.LLMProvider)

	// MCP nativo: configura servidores MCP HTTP no provider e remove suas tools da lista padrão.
	requestStreamer, llmToolDefs = chat.ApplyNativeMCP(requestStreamer, llmToolDefs, uc.mcpMgr, enabledTools, disableTools)

	// Cria contexto cancelável por conversa — permite barge-in cancelar o LLM em andamento.
	convCtx, convCancel := context.WithCancel(ctx)
	uc.streamMgr.Register(req.ConversationID, convCancel)

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
				events.HandlePanic(uc.emitter, req.ConversationID, "runAgenticLoop", r)
			}()
			defer uc.streamMgr.Unregister(req.ConversationID)
			uc.agentSvc.RunAgenticLoop(agentCtx, messages, params, req.ConversationID, userMsg.ID, llmToolDefs, requestStreamer,
				func(convID uint, iter int) agent.IterationHandler {
					return agent.NewAgenticStreamHandler(uc.emitter, convID, iter)
				},
			)
		}()
	} else {
		handler := uc.agentSvc.NewSimpleStreamHandler(req.ConversationID, userMsg.ID)
		go func() {
			defer func() {
				r := recover()
				events.HandlePanic(uc.emitter, req.ConversationID, "StreamChat", r)
			}()
			defer uc.streamMgr.Unregister(req.ConversationID)
			requestStreamer.StreamChat(convCtx, messages, params, handler)
		}()
	}
	return req.ConversationID, nil
}

// whisperTranscribeFunc cria o callback de transcrição STT para o pipeline.
func (uc *SendMessageUseCase) whisperTranscribeFunc() chat.TranscribeFunc {
	return func(audioBase64, filename string) (string, error) {
		result, err := uc.speechSvc.Transcribe(audioBase64, filename)
		if err != nil {
			return "", err
		}
		if result == nil {
			return "", nil
		}
		return result.Text, nil
	}
}
