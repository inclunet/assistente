package usecases

import (
	"context"
	"fmt"
	"log"

	"assistente/internal/agent"
	"assistente/internal/chat"
	"assistente/internal/config"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"assistente/internal/speech"
	"assistente/internal/tools"
)

func resolveStreamingRecoverySettings(activeProfile *profiles.Profile) (enabled bool, maxAttempts int) {
	// Defaults (AEP-0064): enabled + 3 tentativas
	enabled = true
	maxAttempts = 3
	if activeProfile == nil {
		return enabled, maxAttempts
	}
	if activeProfile.Chat.StreamingRecoveryEnabled != nil {
		enabled = *activeProfile.Chat.StreamingRecoveryEnabled
	}
	if activeProfile.Chat.StreamingRecoveryMaxAttempts != nil {
		maxAttempts = *activeProfile.Chat.StreamingRecoveryMaxAttempts
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if maxAttempts > 10 {
		maxAttempts = 10
	}
	return enabled, maxAttempts
}

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
	OnSpeechRequest func(conversationID string, messageID string, role, text, origin, profileSlug string, interrupt bool)
	// OpenEditorPaths retorna os caminhos de arquivos abertos em abas de editor.
	// Filesystem tools podem ler/editar esses arquivos mesmo fora do workDir.
	OpenEditorPaths func() []string
}

// SendMessageUseCase orquestra o pipeline completo de envio de mensagem ao LLM.
// É agnóstico de framework: zero imports de Wails, CLI ou HTTP.
type SendMessageUseCase struct {
	chatInteractor  *chat.Interactor
	toolRegistry    *tools.Registry
	providerSvc     *providers.Service
	mcpMgr          *mcpmgr.Manager
	agentSvc        *agent.Service
	streamMgr       *chat.StreamingManager
	speechSvc       *speech.Service
	settingsSvc     *config.SettingsService
	emitter         ports.Emitter
	onSpeechRequest func(conversationID string, messageID string, role, text, origin, profileSlug string, interrupt bool)
	openEditorPaths func() []string
}

// NewSendMessageUseCase cria um SendMessageUseCase com todas as dependências.
func NewSendMessageUseCase(cfg SendMessageConfig) *SendMessageUseCase {
	return &SendMessageUseCase{
		chatInteractor:  cfg.ChatInteractor,
		toolRegistry:    cfg.ToolRegistry,
		providerSvc:     cfg.ProviderSvc,
		mcpMgr:          cfg.MCPMgr,
		agentSvc:        cfg.AgentSvc,
		streamMgr:       cfg.StreamMgr,
		speechSvc:       cfg.SpeechSvc,
		settingsSvc:     cfg.SettingsSvc,
		emitter:         cfg.Emitter,
		onSpeechRequest: cfg.OnSpeechRequest,
		openEditorPaths: cfg.OpenEditorPaths,
	}
}

// SendMessageRequest encapsula os parâmetros de entrada do Use Case.
type SendMessageRequest struct {
	Ctx            context.Context
	ConversationID string
	RetryMessageID string
	UserContent    string
	UserMedia      string
	Params         llm.ChatParams
	Source         string
}

// Execute executa o pipeline de mensagem: prepara contexto → persiste → monta prompt
// → resolve LLM → lança goroutine de streaming (agêntico ou simples).
// Retorna o conversationID ou erro síncrono (problemas de configuração, banco, etc.).
//
// SECURITY (B14 / AEP-0052): rejeita ctx sem userID antes de tocar qualquer
// camada de dados. O ctx aqui propaga para chat repository, agent loop,
// summarization em background e tools — todos os caminhos que dependem do
// escopo. Quem chama do controller Wails recebe ctx de
// requireAuthenticatedContext; quem chama do gateway de canais recebe ctx
// carimbado com OwnerUserID. Qualquer ctx puro chegando aqui é bug de
// wiring, não input legítimo.
func (uc *SendMessageUseCase) Execute(req SendMessageRequest) (string, error) {
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return "", err
	}
	if req.Params.AllowAssistantPrefill && req.RetryMessageID == "" {
		errMsg := "continuação explícita requer RetryMessage (mensagem para retry)"
		uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
		return "", fmt.Errorf("%s", errMsg)
	}

	// Resolve modelo padrão do config como fallback.
	var defaultModel string
	if cfg, cfgErr := uc.settingsSvc.GetConfig(); cfgErr == nil {
		defaultModel = cfg.DefaultModel
	}

	var retryUserMsg *chat.Message
	if req.RetryMessageID != "" {
		reused, err := uc.chatInteractor.GetRetryableUserMessage(ctx, req.ConversationID, req.RetryMessageID)
		if err != nil {
			uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: "Erro ao carregar mensagem para retry: " + err.Error()})
			return "", err
		}
		retryUserMsg = reused
		req.UserContent = reused.Content
		req.UserMedia = reused.Media
		if reused.Source != "" {
			req.Source = reused.Source
		}
	}

	// Delega validação, renaming e resolução de perfil para o ChatInteractor.
	pctx, err := uc.chatInteractor.PrepareContext(ctx, chat.PrepareContextRequest{
		ConversationID: req.ConversationID,
		UserContent:    req.UserContent,
		UserMedia:      req.UserMedia,
		Params:         req.Params,
		Source:         req.Source,
		DefaultModel:   defaultModel,
	})
	if err != nil {
		return "", err
	}
	activeProfile := pctx.ActiveProfile
	params := pctx.Params
	if params.AllowAssistantPrefill {
		// Gating pelo perfil: se o usuário desabilitou a ação manual, o backend deve falhar fechado.
		if activeProfile != nil && activeProfile.Chat.StreamingRecoveryShowContinue != nil && !*activeProfile.Chat.StreamingRecoveryShowContinue {
			errMsg := "ação 'Continuar resposta' desabilitada no perfil ativo"
			uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
			return "", fmt.Errorf("%s", errMsg)
		}
	}
	userContent := pctx.UserContent
	surfaceOrigin := ports.NewChatSurfaceOrigin(
		req.ConversationID,
		params.SurfaceSessionKey,
		params.SurfaceID,
		params.SurfaceType,
		params.SurfaceTabID,
	)

	var userMsg *chat.Message
	var messages []llm.Message
	var conversationSummary string
	if req.RetryMessageID != "" {
		rmsg, err := uc.chatInteractor.ReuseLoadedUserMessage(ctx, chat.RecordUserMessageRequest{
			ConversationID: req.ConversationID,
			Source:         req.Source,
			ActiveProfile:  activeProfile,
			Transcribe:     uc.whisperTranscribeFunc(),
		}, retryUserMsg)
		if err != nil {
			return "", err
		}
		userMsg = rmsg.UserMsg
		messages = rmsg.Messages
		conversationSummary = rmsg.ConversationSummary
		userContent = userMsg.Content
	} else {
		// Resolve conteúdo: extrai áudio do media e aplica STT fallback para canais.
		var sttProvider string
		if activeProfile != nil {
			sttProvider = activeProfile.Input.STTProvider
		}
		resolved := uc.chatInteractor.ResolveUserContent(ctx, chat.ResolveUserContentRequest{
			Content:     userContent,
			Media:       req.UserMedia,
			Source:      req.Source,
			STTProvider: sttProvider,
			Transcribe:  uc.whisperTranscribeFunc(),
		})
		userContent = resolved.Content

		// Persiste mensagem do usuário, emite ready e carrega histórico.
		rmsg, err := uc.chatInteractor.RecordUserMessage(ctx, chat.RecordUserMessageRequest{
			ConversationID: req.ConversationID,
			Content:        userContent,
			Media:          req.UserMedia,
			AudioBase64:    resolved.AudioBase64,
			AudioMimeType:  resolved.AudioMimeType,
			Source:         req.Source,
			SurfaceOrigin:  surfaceOrigin,
			ActiveProfile:  activeProfile,
			Transcribe:     uc.whisperTranscribeFunc(),
		})
		if err != nil {
			return "", err
		}
		userMsg = rmsg.UserMsg
		messages = rmsg.Messages
		conversationSummary = rmsg.ConversationSummary

		// TTS proativo: verbaliza a mensagem do usuário (síncrono para garantir ordem dos eventos)
		if uc.onSpeechRequest != nil && userContent != "" {
			uc.onSpeechRequest(req.ConversationID, userMsg.ID, "user", userContent, "user_message", params.ProfileSlug, true)
		}
	}

	// Detecta slash skill, compõe system prompt e pré-processa mídia.
	prepResult := uc.chatInteractor.PrepareMessages(ctx, chat.PrepareMessagesRequest{
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
	var profileEnabledTools []string
	if activeProfile != nil {
		profileEnabledTools = activeProfile.Chat.EnabledTools
	}
	initialEnabledTools := chat.ResolveInitialEnabledTools(uc.toolRegistry, profileEnabledTools, disableTools)
	llmToolDefs := chat.BuildLLMToolDefs(uc.toolRegistry, initialEnabledTools, disableTools)

	// Resolve o ChatProvider para o provedor do perfil ativo.
	if activeProfile == nil || activeProfile.Chat.LLMProvider == "" {
		errMsg := "Nenhum provedor LLM configurado no perfil ativo."
		uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
		return "", fmt.Errorf("%s", errMsg)
	}

	requestStreamer, err := uc.providerSvc.GetChatProvider(ctx, activeProfile.Chat.LLMProvider)
	if err != nil {
		errMsg := fmt.Sprintf("Provedor LLM não disponível: %v", err)
		log.Printf("[SendMessage] ERRO: %s", errMsg)
		uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
		return "", fmt.Errorf("%s", errMsg)
	}
	log.Printf("[SendMessage] ChatProvider resolvido para provedor: %s", activeProfile.Chat.LLMProvider)

	// MCP nativo: configura servidores MCP HTTP no provider e remove suas tools da lista padrão.
	requestStreamer, llmToolDefs = chat.ApplyNativeMCP(requestStreamer, llmToolDefs, uc.mcpMgr, profileEnabledTools, disableTools)

	// Cria contexto cancelável por conversa — permite barge-in cancelar o LLM em andamento.
	convCtx, convCancel := context.WithCancel(ctx)
	uc.streamMgr.Register(req.ConversationID, convCancel)

	if len(llmToolDefs) > 0 {
		recoveryEnabled, recoveryMaxAttempts := resolveStreamingRecoverySettings(activeProfile)
		agentCtx := convCtx
		if invokedSkillSlug != "" {
			agentCtx = tools.WithExecutionContext(agentCtx, tools.ExecutionContext{
				InvokedSkillSlug: invokedSkillSlug,
				Filesystem:       invokedFilesystemScope,
			})
		}
		// Injeta caminhos de arquivos abertos em abas de editor para que
		// filesystem tools possam ler/editar esses arquivos fora do workDir.
		if uc.openEditorPaths != nil {
			if paths := uc.openEditorPaths(); len(paths) > 0 {
				agentCtx = tools.WithOpenEditorPaths(agentCtx, paths)
			}
		}
		go func() {
			defer func() {
				r := recover()
				events.HandlePanic(uc.emitter, req.ConversationID, "runAgenticLoop", r)
			}()
			defer uc.streamMgr.Unregister(req.ConversationID)
			uc.agentSvc.RunAgenticLoop(agentCtx, messages, params, req.ConversationID, userMsg.ID, llmToolDefs, requestStreamer, surfaceOrigin,
				func(convID string, iter int) agent.IterationHandler {
					return agent.NewAgenticStreamHandler(uc.emitter, convID, iter, surfaceOrigin, userMsg.ID)
				},
				func(names []string) []llm.ToolDefinition {
					names = filterExpandedToolNames(uc.toolRegistry, names, profileEnabledTools, disableTools)
					names = chat.FilterToolNamesForNativeMCP(requestStreamer, uc.mcpMgr, names, disableTools)
					return chat.BuildLLMToolDefsByNames(uc.toolRegistry, names, disableTools)
				},
				recoveryEnabled,
				recoveryMaxAttempts,
			)
		}()
	} else {
		recoveryEnabled, recoveryMaxAttempts := resolveStreamingRecoverySettings(activeProfile)
		go func() {
			defer func() {
				r := recover()
				events.HandlePanic(uc.emitter, req.ConversationID, "StreamChat", r)
			}()
			defer uc.streamMgr.Unregister(req.ConversationID)
			uc.agentSvc.StreamSimpleWithRecovery(convCtx, requestStreamer, messages, params, req.ConversationID, userMsg.ID, params.ProfileSlug, surfaceOrigin, recoveryEnabled, recoveryMaxAttempts)
		}()
	}
	return req.ConversationID, nil
}

func filterExpandedToolNames(registry *tools.Registry, names []string, profileEnabledTools []string, disableTools bool) []string {
	names = chat.FilterToolNamesByEnabledTools(names, profileEnabledTools, disableTools)
	if profileEnabledTools == nil && registry != nil {
		names = registry.FilterOutOptInNames(names)
	}
	return names
}

// whisperTranscribeFunc cria o callback de transcrição STT para o pipeline.
func (uc *SendMessageUseCase) whisperTranscribeFunc() chat.TranscribeFunc {
	return func(ctx context.Context, audioBase64, filename string) (string, error) {
		result, err := uc.speechSvc.Transcribe(ctx, audioBase64, filename)
		if err != nil {
			return "", err
		}
		if result == nil {
			return "", nil
		}
		return result.Text, nil
	}
}
