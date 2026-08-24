package usecases

import (
	"assistente/internal/agent"
	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/logging"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/profileadequacy"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
	"assistente/internal/speech"
	"assistente/internal/toolcatalog"
	"assistente/internal/tools"
	"context"
	"errors"
	"fmt"
	"sync"
)

// profileWithToolsDisabled devolve o perfil do turno com o interruptor de tools
// desligado, em cópia rasa: o perfil salvo continua como o usuário o configurou,
// e a decisão vale só para este turno.
func profileWithToolsDisabled(activeProfile *profiles.Profile) *profiles.Profile {
	if activeProfile == nil || activeProfile.Chat.DisableTools {
		return activeProfile
	}
	turnProfile := *activeProfile
	turnProfile.Chat.DisableTools = true
	return &turnProfile
}

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
	ChatInteractor  *chat.Interactor
	ToolRegistry    *tools.Registry
	LoadedToolStore *tools.LoadedToolStore
	ProviderSvc     *providers.Service
	MCPMgr          *mcpmgr.Manager
	AgentSvc        *agent.Service
	StreamMgr       *chat.StreamingManager
	SpeechSvc       *speech.Service
	Emitter         ports.Emitter
	// OnSpeechRequest é chamado após salvar a mensagem do usuário para disparar TTS proativo.
	OnSpeechRequest func(conversationID string, messageID string, role, text, origin, profileSlug string, interrupt bool)
	// OpenEditorPaths retorna os caminhos de arquivos abertos em abas de editor.
	// Filesystem tools podem ler/editar esses arquivos mesmo fora do workDir.
	OpenEditorPaths func() []string
	// ProfileAdvisor classifica requisitos operacionais do primeiro turno e
	// recomenda um profile instalado sem alterar policy ou carregar tools.
	ProfileAdvisor interface {
		Recommend(ctx context.Context, req profileadequacy.Request) (*profileadequacy.Recommendation, error)
	}
	// QuestionnaireRouter apresenta a confirmação na superfície de origem.
	QuestionnaireRouter interface {
		Ask(ctx context.Context, surface questionnaire.Surface, payload questionnaire.RequestPayload) (questionnaire.Response, error)
	}
	// SwitchTabProfile persiste o override da aba após confirmação explícita.
	SwitchTabProfile func(tabID, conversationID, profileSlug string) error
}

// SendMessageUseCase orquestra o pipeline completo de envio de mensagem ao LLM.
// É agnóstico de framework: zero imports de Wails, CLI ou HTTP.
type SendMessageUseCase struct {
	chatInteractor  *chat.Interactor
	toolRegistry    *tools.Registry
	loadedToolStore *tools.LoadedToolStore
	providerSvc     *providers.Service
	mcpMgr          *mcpmgr.Manager
	agentSvc        *agent.Service
	streamMgr       *chat.StreamingManager
	speechSvc       *speech.Service
	emitter         ports.Emitter
	onSpeechRequest func(conversationID string, messageID string, role, text, origin, profileSlug string, interrupt bool)
	openEditorPaths func() []string
	profileAdvisor  interface {
		Recommend(ctx context.Context, req profileadequacy.Request) (*profileadequacy.Recommendation, error)
	}
	questionnaireRouter interface {
		Ask(ctx context.Context, surface questionnaire.Surface, payload questionnaire.RequestPayload) (questionnaire.Response, error)
	}
	switchTabProfile  func(tabID, conversationID, profileSlug string) error
	conversationLocks struct {
		sync.Mutex
		entries map[string]*sendMessageLockEntry
	}
}

type sendMessageLockEntry struct {
	token chan struct{}
	refs  int
}

// NewSendMessageUseCase cria um SendMessageUseCase com todas as dependências.
func NewSendMessageUseCase(cfg SendMessageConfig) *SendMessageUseCase {
	loadedToolStore := cfg.LoadedToolStore
	if loadedToolStore == nil {
		loadedToolStore = tools.NewLoadedToolStore()
	}
	useCase := &SendMessageUseCase{
		chatInteractor:      cfg.ChatInteractor,
		toolRegistry:        cfg.ToolRegistry,
		loadedToolStore:     loadedToolStore,
		providerSvc:         cfg.ProviderSvc,
		mcpMgr:              cfg.MCPMgr,
		agentSvc:            cfg.AgentSvc,
		streamMgr:           cfg.StreamMgr,
		speechSvc:           cfg.SpeechSvc,
		emitter:             cfg.Emitter,
		onSpeechRequest:     cfg.OnSpeechRequest,
		openEditorPaths:     cfg.OpenEditorPaths,
		profileAdvisor:      cfg.ProfileAdvisor,
		questionnaireRouter: cfg.QuestionnaireRouter,
		switchTabProfile:    cfg.SwitchTabProfile,
	}
	useCase.conversationLocks.entries = make(map[string]*sendMessageLockEntry)
	return useCase
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
	if req.ConversationID != "" {
		unlock, err := uc.lockConversation(ctx, req.ConversationID)
		if err != nil {
			return "", err
		}
		defer unlock()
	}
	if req.Params.AllowAssistantPrefill && req.RetryMessageID == "" {
		errMsg := "continuação explícita requer RetryMessage (mensagem para retry)"
		uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
		return "", fmt.Errorf("%s", errMsg)
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
	})
	if err != nil {
		return "", err
	}
	if req.RetryMessageID == "" && pctx.UserContent == "" && req.UserMedia != "" {
		sttProvider := ""
		if pctx.ActiveProfile != nil {
			sttProvider = pctx.ActiveProfile.Input.STTProvider
		}
		resolved := uc.chatInteractor.ResolveUserContent(ctx, chat.ResolveUserContentRequest{
			Content:     pctx.UserContent,
			Media:       req.UserMedia,
			Source:      req.Source,
			STTProvider: sttProvider,
			Transcribe:  uc.whisperTranscribeFunc(),
		})
		if resolved.Content != "" {
			req.UserContent = resolved.Content
			pctx.UserContent = resolved.Content
		}
	}
	pctx, err = uc.ensureAdequateProfile(ctx, req, pctx)
	if err != nil {
		if errors.Is(err, errProfileAdequacyCancelled) {
			return uc.completeProfileAdequacyCancellation(req, pctx), nil
		}
		return "", err
	}
	activeProfile := pctx.ActiveProfile
	// Turno conduzido por agente externo (AEP-0084 D7): as ferramentas são do
	// agente, e o app planeja o turno com as suas desligadas — pelo mesmo
	// interruptor que o perfil oferece, para que prompt e roteamento enxerguem a
	// mesma decisão. Oferecer tools levaria a conversa ao loop agêntico, que
	// tentaria executar aqui o que não é dele.
	//
	// A marca também diz ao preparo do turno que nada do app acompanha a
	// mensagem (Fase 8): persona, skills, memória e contexto ficam de fora, e a
	// skill invocada por barra deixa de ser processada — num perfil de agente,
	// quem responde por `/` é ele.
	agentDrivenTurn := uc.providerSvc != nil && uc.providerSvc.UsesAgentTurn(ctx, activeProfile)
	if agentDrivenTurn {
		activeProfile = profileWithToolsDisabled(activeProfile)
	}
	params := pctx.Params
	if params.AllowAssistantPrefill {
		// Gating pelo perfil: se o usuário desabilitou a ação manual, o backend deve falhar fechado.
		if activeProfile != nil && activeProfile.Chat.StreamingRecoveryShowContinue != nil && !*activeProfile.Chat.StreamingRecoveryShowContinue {
			errMsg := "ação 'Continuar resposta' desabilitada no perfil ativo"
			uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
			return "", fmt.Errorf("%s", errMsg)
		}
		// Capacidades por provider/modelo (Issue #124): quando o provider/modelo
		// não suporta assistant prefill (ex.: Qwen/LocalAI), NÃO falhamos — fazemos
		// fallback de continuação por mensagem de usuário ("continue a partir deste
		// texto: ..."). O prompt volta a terminar em user e prossegue normalmente.
		if uc.providerSvc == nil || !uc.providerSvc.SupportsAssistantPrefill(ctx, activeProfile) {
			params.AllowAssistantPrefill = false
			params.ContinueViaUserMessage = true
			logging.Infof(ctx, "core.usecases.send-message", "[SendMessage] provider/modelo sem suporte a assistant prefill — usando fallback de continuação por mensagem de usuário (conversa %s)", req.ConversationID)
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
			ConversationID:     req.ConversationID,
			Source:             req.Source,
			ActiveProfile:      activeProfile,
			Transcribe:         uc.whisperTranscribeFunc(),
			MaxContextMessages: params.MaxContextMessages,
		}, retryUserMsg)
		if err != nil {
			return "", err
		}
		userMsg = rmsg.UserMsg
		messages = rmsg.Messages
		conversationSummary = rmsg.ConversationSummary
		userContent = userMsg.Content
		if !agentDrivenTurn {
			if err := uc.chatInteractor.ValidateSkillInvocation(activeProfile, userContent, req.ConversationID, surfaceOrigin); err != nil {
				return "", err
			}
		}
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
		if !agentDrivenTurn {
			if err := uc.chatInteractor.ValidateSkillInvocation(activeProfile, userContent, req.ConversationID, surfaceOrigin); err != nil {
				return "", err
			}
		}

		// Persiste mensagem do usuário, emite ready e carrega histórico.
		rmsg, err := uc.chatInteractor.RecordUserMessage(ctx, chat.RecordUserMessageRequest{
			ConversationID:     req.ConversationID,
			Content:            userContent,
			Media:              req.UserMedia,
			AudioBase64:        resolved.AudioBase64,
			AudioMimeType:      resolved.AudioMimeType,
			Source:             req.Source,
			SurfaceOrigin:      surfaceOrigin,
			ActiveProfile:      activeProfile,
			Transcribe:         uc.whisperTranscribeFunc(),
			MaxContextMessages: params.MaxContextMessages,
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
		TurnID:              userMsg.ID,
		Params:              params,
		ActiveProfile:       activeProfile,
		SurfaceOrigin:       surfaceOrigin,
		Transcribe:          uc.whisperTranscribeFunc(),
		AgentTurn:           agentDrivenTurn,
	})
	if prepResult.Err != nil {
		return "", prepResult.Err
	}
	messages = prepResult.Messages
	// A conversa acompanha o turno até o provider: o agente de código guarda o
	// histórico na sessão dela, e é assim que ele sabe qual sessão usar
	// (AEP-0084 D4).
	params.ConversationID = req.ConversationID
	params.DebugDump.ConversationID = req.ConversationID
	params.DebugDump.TurnID = userMsg.ID
	if params.DebugDump.ProfileSlug == "" {
		params.DebugDump.ProfileSlug = params.ProfileSlug
	}
	invokedSkillSlug := prepResult.InvokedSkillSlug
	invokedFilesystemScope := prepResult.InvokedScope
	invokedExecutionContext := prepResult.InvokedExecutionContext

	// Constrói tool definitions para o LLM.
	disableTools := activeProfile != nil && activeProfile.Chat.DisableTools
	var profileEnabledTools []string
	var profileToolPolicy map[string]string
	var profileToolPolicyDefault string
	var toolSchemaBudgetBytes int
	var preferredToolPackages []string
	if activeProfile != nil {
		profileEnabledTools = activeProfile.Chat.EnabledTools
		profileToolPolicy = activeProfile.Chat.ToolPolicy
		profileToolPolicyDefault = activeProfile.Chat.ToolPolicyDefault
		toolSchemaBudgetBytes = activeProfile.Chat.ToolSchemaBudgetBytes
		preferredToolPackages = activeProfile.Chat.PreferredToolPackages
	}
	var runtimeTools []string
	if prepResult.ModelOnDemandSkillAvailable {
		runtimeTools = append(runtimeTools, tools.LoadSkillName)
	}
	// Política única de seleção de tools por perfil/superfície (AEP-0077 F3, #119).
	// O override de MCP nativo (toolCfg.NativeMCP) é preenchido abaixo, após
	// resolver o provider/override do perfil; a montagem final dos conjuntos
	// native/adapter (com ToolPlanner) é feita por PlanTurnToolDefs.
	toolPolicy := chat.NewToolSelectionPolicy(uc.toolRegistry)
	toolCfg := chat.ProfileToolConfig{
		EnabledTools:      profileEnabledTools,
		ToolPolicy:        profileToolPolicy,
		ToolPolicyDefault: profileToolPolicyDefault,
		DisableTools:      disableTools,
		RuntimeTools:      runtimeTools,
		SchemaBytesBudget: toolSchemaBudgetBytes,
		PreferredPackages: preferredToolPackages,
	}
	if disableTools && uc.loadedToolStore != nil {
		uc.loadedToolStore.ResetConversation(req.ConversationID)
	}
	baseEffectiveToolPolicy := toolPolicy.ResolveEffectiveToolPolicy(toolCfg)
	basePreloadedToolNames := baseEffectiveToolPolicy.PreloadedNames()
	toolCatalogVisibleNames := baseEffectiveToolPolicy.CatalogVisibleNames()
	controlPlaneToolNames := controlPlaneNamesFromPolicy(baseEffectiveToolPolicy)
	if uc.loadedToolStore != nil && !disableTools {
		loadedRuntimeTools := uc.loadedToolStore.Loaded(req.ConversationID, params.ProfileSlug, toolCatalogVisibleNames)
		loadedRuntimeTools = availableLoadedRuntimeTools(ctx, loadedRuntimeTools)
		if len(loadedRuntimeTools) > 0 {
			runtimeTools = append(runtimeTools, loadedRuntimeTools...)
			toolCfg.RuntimeTools = runtimeTools
		}
	}

	// Resolve o ChatProvider para o provedor do perfil ativo.
	if activeProfile == nil || activeProfile.Chat.LLMProvider == "" {
		errMsg := "Nenhum provedor LLM configurado no perfil ativo."
		uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
		return "", fmt.Errorf("%s", errMsg)
	}

	requestStreamer, err := uc.providerSvc.GetChatProvider(ctx, activeProfile.Chat.LLMProvider)
	if err != nil {
		errMsg := fmt.Sprintf("Provedor LLM não disponível: %v", err)
		logging.Errorf(ctx, "core.usecases.send-message", "[SendMessage] ERRO: %s", errMsg)
		uc.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
		return "", fmt.Errorf("%s", errMsg)
	}
	logging.Infof(ctx, "core.usecases.send-message", "[SendMessage] ChatProvider resolvido para provedor: %s", activeProfile.Chat.LLMProvider)

	// MCP nativo: configura servidores MCP HTTP no provider e remove suas tools da lista padrão.
	// O override é por PERFIL (AEP-0021) e vale igualmente para chat e sub-agentes,
	// já que ambos resolvem o mesmo activeProfile neste pipeline. Aqui activeProfile
	// já é não-nil (validado acima, onde LLMProvider vazio/nil retorna erro).
	nativeMCPOverride := activeProfile.Chat.NativeMCP
	toolCfg.NativeMCP = nativeMCPOverride

	// Resolução coesa native/adapter + ToolPlanner (AEP-0077 F4, #121). O budget
	// é aplicado ao conjunto FINAL de cada caminho: o nativo só conta os schemas
	// realmente enviados como função (bridges nativas vão por passthrough e são
	// removidas ANTES de orçar), evitando que bridges descartadas desloquem
	// builtins fixadas pelo perfil. As alternativas em modo ADAPTER:
	//   - adapterStreamer: provider BASE, sem MCP servers nativos;
	//   - adapterToolDefs: tools COM as bridges MCP (não removidas), já orçadas.
	// Usadas pelo fallback nativo→adapter no MESMO turno (ver NativeMCPFallback).
	adapterStreamer := requestStreamer
	var llmToolDefs, adapterToolDefs []llm.ToolDefinition
	requestStreamer, llmToolDefs, adapterToolDefs = toolPolicy.PlanTurnToolDefs(requestStreamer, uc.mcpMgr, toolCfg)

	// O critério do D7 é o conjunto FINAL que chega ao roteamento: uma tool
	// sobrevivente basta para mandar o turno do agente para o loop errado. O
	// interruptor acima já deveria ter zerado tudo; se algum caminho de runtime
	// escapar, o turno segue sem tools e a divergência aparece no log.
	if agentDrivenTurn && (len(llmToolDefs) > 0 || len(adapterToolDefs) > 0) {
		logging.Warnf(ctx, "core.usecases.send-message", "[SendMessage] turno de agente externo chegou ao roteamento com tools (%d nativas, %d adapter) — descartadas", len(llmToolDefs), len(adapterToolDefs))
		llmToolDefs = nil
		adapterToolDefs = nil
	}

	// Auto-degradação otimista (AEP-0021): no modo AUTO tentamos MCP nativo; se o
	// modelo/endpoint rejeitar type:"mcp", o provider dispara este hook (auto-ajusta e
	// PERSISTE o perfil nil→false para os próximos turnos) e, quando há
	// NativeMCPFallback, o loop agêntico re-tenta o MESMO turno em modo adapter com as
	// bridge tools presentes. Override explícito (true/false) não é sobrescrito.
	resolvedProfileSlug := params.ProfileSlug
	resolvedModel := params.Model
	params.OnNativeMCPUnsupported = func() {
		uc.chatInteractor.HandleNativeMCPUnsupported(resolvedProfileSlug, resolvedModel, nativeMCPOverride)
	}
	params.OnPromptCacheHintUnsupported = func() {
		uc.chatInteractor.HandlePromptCacheHintUnsupported(resolvedProfileSlug, resolvedModel)
	}
	if params.PromptCacheKey != "" {
		params.PromptCacheHintFallback = &llm.PromptCacheHintFallback{}
	}

	// Só há o que degradar quando ApplyNativeMCP de fato anexou MCP servers nativos
	// (requestStreamer trocado). Prepara o fallback adapter para o mesmo turno.
	if requestStreamer != adapterStreamer {
		// Fallback adapter: mesma política de expansão, porém forçando MCP nativo
		// desligado (as bridges voltam a ser function tools neste turno).
		adapterFalse := false
		adapterToolCfg := toolCfg
		adapterToolCfg.NativeMCP = &adapterFalse
		params.NativeMCPFallback = &llm.NativeMCPAdapterFallback{
			Streamer: adapterStreamer,
			ToolDefs: adapterToolDefs,
			ResolveToolDefs: func(active []llm.ToolDefinition, names []string) []llm.ToolDefinition {
				return toolPolicy.ResolveExpandedToolDefs(adapterStreamer, uc.mcpMgr, active, names, adapterToolCfg)
			},
		}
	}

	// Cria contexto cancelável por conversa — permite barge-in cancelar o LLM em andamento.
	convCtx, convCancel := context.WithCancel(ctx)
	uc.streamMgr.Register(req.ConversationID, convCancel)

	// Roteia para o loop agêntico sempre que houver tools em modo adapter (inclui o
	// caso em que o caminho nativo removeu todas as bridges): assim o fallback
	// nativo→adapter consegue restaurar as tools no mesmo turno.
	if len(llmToolDefs) > 0 || len(adapterToolDefs) > 0 {
		recoveryEnabled, recoveryMaxAttempts := resolveStreamingRecoverySettings(activeProfile)
		agentCtx := convCtx
		if invokedExecutionContext != nil {
			agentCtx = tools.WithExecutionContext(agentCtx, *invokedExecutionContext)
		} else if invokedSkillSlug != "" {
			agentCtx = tools.WithExecutionContext(agentCtx, tools.ExecutionContext{
				InvokedSkillSlug: invokedSkillSlug,
				Filesystem:       invokedFilesystemScope,
			})
		}
		effectiveToolPolicy := toolPolicy.ResolveEffectiveToolPolicy(toolCfg)
		agentCtx = tools.WithToolCatalogVisibleNames(agentCtx, effectiveToolPolicy.CatalogVisibleNames())
		agentCtx = tools.WithToolCatalogRuntime(agentCtx, tools.ToolCatalogRuntime{
			Store:          uc.loadedToolStore,
			ConversationID: req.ConversationID,
			ProfileSlug:    params.ProfileSlug,
			VisibleNames:   effectiveToolPolicy.CatalogVisibleNames(),
			PreloadedNames: basePreloadedToolNames,
			ControlPlane:   controlPlaneToolNames,
		})
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
				uc.agentSvc.HandleRecoveredPanic(agentCtx, req.ConversationID, userMsg.ID, "runAgenticLoop", r, surfaceOrigin)
			}()
			defer uc.streamMgr.Unregister(req.ConversationID)
			uc.agentSvc.RunAgenticLoop(agentCtx, messages, params, req.ConversationID, userMsg.ID, llmToolDefs, requestStreamer, surfaceOrigin,
				func(convID string, iter int) agent.IterationHandler {
					return agent.NewAgenticStreamHandler(uc.emitter, convID, iter, surfaceOrigin, userMsg.ID)
				},
				func(active []llm.ToolDefinition, names []string) []llm.ToolDefinition {
					return toolPolicy.ResolveExpandedToolDefs(requestStreamer, uc.mcpMgr, active, names, toolCfg)
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
				uc.agentSvc.HandleRecoveredPanic(convCtx, req.ConversationID, userMsg.ID, "StreamChat", r, surfaceOrigin)
			}()
			defer uc.streamMgr.Unregister(req.ConversationID)
			uc.agentSvc.StreamSimpleWithRecovery(convCtx, requestStreamer, messages, params, req.ConversationID, userMsg.ID, params.ProfileSlug, surfaceOrigin, recoveryEnabled, recoveryMaxAttempts)
		}()
	}
	return req.ConversationID, nil
}

func (uc *SendMessageUseCase) lockConversation(ctx context.Context, conversationID string) (func(), error) {
	uc.conversationLocks.Lock()
	if uc.conversationLocks.entries == nil {
		uc.conversationLocks.entries = make(map[string]*sendMessageLockEntry)
	}
	entry := uc.conversationLocks.entries[conversationID]
	if entry == nil {
		entry = &sendMessageLockEntry{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		uc.conversationLocks.entries[conversationID] = entry
	}
	entry.refs++
	uc.conversationLocks.Unlock()

	select {
	case <-entry.token:
	case <-ctx.Done():
		uc.conversationLocks.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(uc.conversationLocks.entries, conversationID)
		}
		uc.conversationLocks.Unlock()
		return nil, ctx.Err()
	}
	return func() {
		entry.token <- struct{}{}
		uc.conversationLocks.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(uc.conversationLocks.entries, conversationID)
		}
		uc.conversationLocks.Unlock()
	}, nil
}

const (
	profileAdequacySwitchAction   = "switch-profile"
	profileAdequacyContinueAction = "continue-current"
)

var errProfileAdequacyCancelled = errors.New("profile adequacy decision cancelled")

func (uc *SendMessageUseCase) completeProfileAdequacyCancellation(
	req SendMessageRequest,
	pctx *chat.PrepareContextResponse,
) string {
	if uc.emitter == nil || pctx == nil {
		return req.ConversationID
	}
	uc.emitter.Emit("chat:done", ports.DoneEvent{
		ConversationID: req.ConversationID,
		Reason:         "cancelled",
		SurfaceOrigin: ports.NewChatSurfaceOrigin(
			req.ConversationID,
			pctx.Params.SurfaceSessionKey,
			pctx.Params.SurfaceID,
			pctx.Params.SurfaceType,
			pctx.Params.SurfaceTabID,
		),
	})
	return req.ConversationID
}

func (uc *SendMessageUseCase) ensureAdequateProfile(
	ctx context.Context,
	req SendMessageRequest,
	pctx *chat.PrepareContextResponse,
) (*chat.PrepareContextResponse, error) {
	if req.RetryMessageID != "" ||
		req.Source != "wails" ||
		pctx == nil ||
		pctx.ActiveProfile == nil ||
		uc.profileAdvisor == nil ||
		uc.questionnaireRouter == nil ||
		uc.switchTabProfile == nil ||
		pctx.Params.SurfaceTabID == "" {
		return pctx, nil
	}

	empty, err := uc.chatInteractor.IsConversationEmpty(ctx, req.ConversationID)
	if err != nil {
		logging.Warnf(ctx, "core.usecases.send-message", "[ProfileAdequacy] Não foi possível verificar o primeiro turno: %v", err)
		return pctx, nil
	}
	if !empty {
		return pctx, nil
	}

	recommendation, err := uc.profileAdvisor.Recommend(ctx, profileadequacy.Request{
		UserContent:    pctx.UserContent,
		SurfaceType:    pctx.Params.SurfaceType,
		TabType:        pctx.Params.TabType,
		ActiveFilePath: pctx.Params.ActiveFilePath,
		CurrentSlug:    pctx.Params.ProfileSlug,
		CurrentProfile: pctx.ActiveProfile,
		Model:          pctx.Params.Model,
	})
	if err != nil {
		if errors.Is(err, profileadequacy.ErrAuxiliaryClassificationUnavailable) {
			logging.Debugf(ctx, "core.usecases.send-message", "[ProfileAdequacy] Provider sem papel auxiliar; mantendo profile atual")
			return pctx, nil
		}
		logging.Warnf(ctx, "core.usecases.send-message", "[ProfileAdequacy] Classificação ignorada: %v", err)
		return pctx, nil
	}
	if recommendation == nil {
		return pctx, nil
	}

	textParams := map[string]any{
		"currentProfile":   recommendation.CurrentName,
		"suggestedProfile": recommendation.SuggestedName,
	}
	response, err := uc.questionnaireRouter.Ask(
		ctx,
		questionnaire.DesktopSurface(req.ConversationID),
		questionnaire.RequestPayload{
			Kind: questionnaire.KindDecision,
			Title: questionnaire.Keyed(
				"app.questionnaire.profileAdequacy.title",
				"Trocar perfil para este pedido?",
			),
			Description: questionnaire.KeyedWith(
				"app.questionnaire.profileAdequacy.description",
				textParams,
				fmt.Sprintf(
					"O perfil %s parece menos adequado para este pedido. Deseja usar %s nesta aba?",
					recommendation.CurrentName,
					recommendation.SuggestedName,
				),
			),
			AllowCancel: true,
			Actions: []questionnaire.DecisionAction{
				{
					ID: profileAdequacySwitchAction,
					Label: questionnaire.KeyedWith(
						"app.questionnaire.profileAdequacy.switch",
						map[string]any{"suggestedProfile": recommendation.SuggestedName},
						"Trocar para "+recommendation.SuggestedName,
					),
					Variant: "primary",
					Primary: true,
				},
				{
					ID: profileAdequacyContinueAction,
					Label: questionnaire.KeyedWith(
						"app.questionnaire.profileAdequacy.keep",
						map[string]any{"currentProfile": recommendation.CurrentName},
						"Continuar com "+recommendation.CurrentName,
					),
					Variant: "secondary",
				},
			},
		},
	)
	if err != nil {
		logging.Warnf(ctx, "core.usecases.send-message", "[ProfileAdequacy] Decisão indisponível; mantendo profile atual: %v", err)
		return pctx, nil
	}
	actionID, ok := questionnaire.DecisionActionID(response)
	if !ok {
		return nil, errProfileAdequacyCancelled
	}
	if actionID == profileAdequacyContinueAction {
		return pctx, nil
	}
	if actionID != profileAdequacySwitchAction {
		return nil, fmt.Errorf("ação de adequação de perfil inválida: %q", actionID)
	}

	if err := uc.switchTabProfile(pctx.Params.SurfaceTabID, req.ConversationID, recommendation.SuggestedSlug); err != nil {
		return nil, fmt.Errorf("persistir troca de perfil confirmada: %w", err)
	}
	if uc.loadedToolStore != nil {
		uc.loadedToolStore.ResetConversation(req.ConversationID)
	}
	req.Params.ProfileSlug = recommendation.SuggestedSlug
	return uc.chatInteractor.PrepareContext(ctx, chat.PrepareContextRequest{
		ConversationID: req.ConversationID,
		UserContent:    req.UserContent,
		UserMedia:      req.UserMedia,
		Params:         req.Params,
		Source:         req.Source,
	})
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

func availableLoadedRuntimeTools(ctx context.Context, names []string) []string {
	if len(names) == 0 || database.DB() == nil {
		return nil
	}
	entries, err := toolcatalog.NewDBRepository(database.DB()).ListTools(ctx, tools.ToolCatalogFilter{
		NameIn:             names,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	})
	if err != nil {
		logging.Errorf(ctx, "core.usecases.send-message", "[SendMessage] falha ao revalidar tools carregadas: %v", err)
		return nil
	}
	available := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		available[entry.Name] = struct{}{}
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := available[name]; ok {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func controlPlaneNamesFromPolicy(policy chat.EffectiveToolPolicy) []string {
	names := make([]string, 0, 2)
	if policy.State(tools.ToolCatalogName) == chat.ToolPolicyPreloaded {
		names = append(names, tools.ToolCatalogName)
	}
	if policy.State(tools.LoadSkillName) == chat.ToolPolicyPreloaded {
		names = append(names, tools.LoadSkillName)
	}
	return names
}
