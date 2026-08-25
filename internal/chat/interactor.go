package chat

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"assistente/internal/contextprovider"
	"assistente/internal/core/ports"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"assistente/internal/skills"
	"assistente/internal/tools"
	"assistente/internal/workspace"
	"gorm.io/gorm"
)

// DefaultMaxContextMessages é o limite padrão de mensagens carregadas no contexto.
const DefaultMaxContextMessages = 50

const (
	MaxMessageContentSize = 512 * 1024       // 512 KB
	MaxMediaSize          = 20 * 1024 * 1024 // 20 MB
)

// ChatParams is an alias for llm.ChatParams — single source of truth.
type ChatParams = llm.ChatParams

// SystemPromptBuilder builds the full system prompt for the chat pipeline.
// Implemented by *prompt.Builder. Defined as an interface here so that internal/chat
// does not need to import internal/prompt (which already imports internal/chat).
type SystemPromptBuilder interface {
	BuildWithContextBlocks(messages []llm.Message, enabledSkills []string, disableSkills bool, disableOnDemand bool, tplData any, contextBlocks []contextprovider.Block) []llm.Message
	BuildTemplateData(activeProfile *profiles.Profile, params llm.ChatParams, conversationID string) TemplateData
}

type WorkspaceProvider interface {
	Active() *workspace.Workspace
}

type SkillRuntimeManager interface {
	skills.InvokerManager
	GetAllSkillsFull() ([]skills.Skill, error)
}

// InteractorConfig groups all dependencies for Interactor.
type InteractorConfig struct {
	Emitter       events.Emitter
	Repo          MessageRepository
	ConvRepo      ConversationRepository
	ProviderSvc   *providers.Service
	ProfileMgr    *profiles.Manager
	Workspace     WorkspaceProvider
	SkillMgr      SkillRuntimeManager // optional during startup; safe to be nil
	PromptBuilder SystemPromptBuilder // optional during startup; safe to be nil
	// ContextProviders monta blocos dinâmicos de prompt (memory, workspace,
	// tasklists). Opcional em testes/startup; nil desabilita esses blocos.
	ContextProviders *contextprovider.Registry
	// LinkedTaskLists resolve as task lists vinculadas a uma conversa para o
	// Context Provider tasklist. Opcional: nil produz contexto vazio.
	LinkedTaskLists func(ctx context.Context, conversationID string) []contextprovider.LinkedTaskList
}

// Interactor orchestrates the core chat use cases, free of Wails dependencies.
type Interactor struct {
	emitter          events.Emitter
	repo             MessageRepository
	convRepo         ConversationRepository
	providerSvc      *providers.Service
	profileMgr       *profiles.Manager
	workspace        WorkspaceProvider
	skillMgr         SkillRuntimeManager
	promptBuilder    SystemPromptBuilder
	contextProviders *contextprovider.Registry
	linkedTaskLists  func(ctx context.Context, conversationID string) []contextprovider.LinkedTaskList

	// nativeMCPAdjustMu serializa o read-modify-write do auto-ajuste de MCP nativo
	// do perfil (nil→false), garantindo idempotência sob concorrência (vários runs
	// do mesmo perfil falhando ao mesmo tempo). Ver HandleNativeMCPUnsupported.
	nativeMCPAdjustMu sync.Mutex
	// promptCacheAdjustMu serializa o auto-ajuste de provider_hints=false quando
	// um provider rejeita explicitamente prompt_cache_key.
	promptCacheAdjustMu sync.Mutex
}

// NewInteractor creates an Interactor with its required dependencies.
func NewInteractor(cfg InteractorConfig) *Interactor {
	return &Interactor{
		emitter:          cfg.Emitter,
		repo:             cfg.Repo,
		convRepo:         cfg.ConvRepo,
		providerSvc:      cfg.ProviderSvc,
		profileMgr:       cfg.ProfileMgr,
		workspace:        cfg.Workspace,
		skillMgr:         cfg.SkillMgr,
		promptBuilder:    cfg.PromptBuilder,
		contextProviders: cfg.ContextProviders,
		linkedTaskLists:  cfg.LinkedTaskLists,
	}
}

func (i *Interactor) resolveWorkspaceProfileSlug(conversationID string, params ChatParams) string {
	if i.workspace == nil {
		return ""
	}
	ws := i.workspace.Active()
	if ws == nil {
		return ""
	}
	if params.SurfaceTabID != "" {
		if slug := profileSlugFromWorkspaceTab(ws.FindTab(params.SurfaceTabID)); slug != "" {
			return slug
		}
	}
	if conversationID != "" {
		if slug := profileSlugFromWorkspaceTab(ws.FindTabByConversation(conversationID)); slug != "" {
			return slug
		}
	}
	return strings.TrimSpace(ws.Profile)
}

func profileSlugFromWorkspaceTab(tab *workspace.Tab) string {
	if tab == nil || tab.ProfileOverride == nil {
		return ""
	}
	slug, _ := tab.ProfileOverride["slug"].(string)
	return strings.TrimSpace(slug)
}

// PrepareContextRequest carries the raw inputs for a message send request.
type PrepareContextRequest struct {
	ConversationID string
	UserContent    string
	UserMedia      string
	Params         ChatParams
	Source         string
	DefaultModel   string // fallback model when profile doesn't specify one
}

// PrepareContextResponse holds the resolved profile and hydrated params for the chat pipeline.
type PrepareContextResponse struct {
	ActiveProfile *profiles.Profile
	Params        ChatParams
	UserContent   string
}

// PrepareContext validates limits, checks credentials, applies auto-rename, resolves
// the profile and applies all profile-level chat defaults onto Params.
// This replaces lines 38-160 of the legacy sendMessageInternal in app_chat.go.
func (i *Interactor) PrepareContext(ctx context.Context, req PrepareContextRequest) (*PrepareContextResponse, error) {
	// 1. Validate content / media size
	if len(req.UserContent) > MaxMessageContentSize {
		errMsg := fmt.Sprintf("Mensagem muito grande (%d bytes). Máximo permitido: %d bytes", len(req.UserContent), MaxMessageContentSize)
		i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
		return nil, errors.New(errMsg)
	}
	if len(req.UserMedia) > MaxMediaSize {
		errMsg := fmt.Sprintf("Mídia muito grande (%d bytes). Máximo permitido: %d bytes", len(req.UserMedia), MaxMediaSize)
		i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: errMsg})
		return nil, errors.New(errMsg)
	}

	// 2. Verify that at least one LLM provider is configured
	if i.providerSvc != nil {
		providerCount, _ := i.providerSvc.Count(ctx)
		if providerCount == 0 {
			msg := "Nenhum provedor LLM configurado. Configure um provedor nas configurações."
			i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: msg})
			return nil, fmt.Errorf("nenhum provedor LLM configurado")
		}
	}

	// 3. Validate conversation ID
	if req.ConversationID == "" {
		const errMsg = "conversationID é obrigatório — conversas devem ser criadas ao criar/resetar a tab"
		i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: "", Error: errMsg})
		return nil, errors.New(errMsg)
	}

	// 4. Auto-rename conversation if it still has the generic default title
	if req.UserContent != "" {
		conv, convErr := i.convRepo.GetConversationInfo(ctx, req.ConversationID)
		if convErr == nil && conv != nil && conv.Title == DefaultConversationTitle {
			title := automaticTitle(req.UserContent)
			if err := i.convRepo.UpdateConversation(ctx, req.ConversationID, title, ""); err == nil {
				i.emitter.Emit("conversation:renamed", ports.ConversationRenamedEvent{
					ConversationID: req.ConversationID,
					NewTitle:       title,
				})
			}
		}
	}

	// 5. Resolve active profile
	var err error
	var activeProfile *profiles.Profile
	resolvedProfileSlug := strings.TrimSpace(req.Params.ProfileSlug)
	profileIsAuthoritative := resolvedProfileSlug != "" &&
		(req.Source == "wails" || req.Source == "subagent")
	if resolvedProfileSlug == "" && req.Source == "wails" {
		resolvedProfileSlug = i.resolveWorkspaceProfileSlug(req.ConversationID, req.Params)
		req.Params.ProfileSlug = resolvedProfileSlug
		profileIsAuthoritative = resolvedProfileSlug != ""
	}
	if i.profileMgr == nil {
		logging.Errorf(ctx, "chat.interactor", "[PrepareContext] profileManager não inicializado — continuando sem perfil")
	} else if resolvedProfileSlug != "" {
		activeProfile, err = i.profileMgr.Get(resolvedProfileSlug)
		if err != nil {
			// Subagentes e overrides do workspace recebem um profile já
			// escolhido (e, quando necessário, autorizado). Fazer fallback
			// para o global executaria com configuração diferente da decisão
			// caso o profile fosse removido entre o diálogo e o turno
			// (AEP-0101).
			if profileIsAuthoritative {
				return nil, fmt.Errorf("profile solicitado indisponível %q: %w", resolvedProfileSlug, err)
			}
			logging.Warnf(ctx, "chat.interactor", "[PrepareContext] Erro ao obter perfil '%s': %v — usando perfil ativo global", resolvedProfileSlug, err)
			var active *profiles.ActiveProfile
			active, err = i.profileMgr.GetActiveAndSlug()
			if err == nil && active != nil {
				activeProfile = active.Profile
				resolvedProfileSlug = active.Slug
				req.Params.ProfileSlug = resolvedProfileSlug
			}
		}
	} else {
		var active *profiles.ActiveProfile
		active, err = i.profileMgr.GetActiveAndSlug()
		if err == nil && active != nil {
			activeProfile = active.Profile
			resolvedProfileSlug = active.Slug
			req.Params.ProfileSlug = resolvedProfileSlug
		}
	}
	if err != nil {
		logging.Errorf(ctx, "chat.interactor", "[PrepareContext] Erro ao obter perfil: %v", err)
	}

	// 6. Resolve $default sentinels (provider/model)
	if activeProfile != nil && i.providerSvc != nil {
		activeProfile = i.providerSvc.ResolveProfileDefaults(ctx, activeProfile)
	}

	// 7. Apply profile-level chat defaults onto Params
	params := req.Params
	if activeProfile != nil {
		params.ProfileSlug = strings.TrimSpace(resolvedProfileSlug)
		logging.Infof(ctx, "chat.interactor", "[PrepareContext] Usando perfil: %s", activeProfile.Name)
		if params.Model == "" && activeProfile.Chat.Model != "" {
			params.Model = activeProfile.Chat.Model
		}
		if activeProfile.Chat.Temperature > 0 {
			params.Temperature = activeProfile.Chat.Temperature
		}
		if activeProfile.Chat.MaxTokens > 0 {
			params.MaxTokens = activeProfile.Chat.MaxTokens
		}
		if activeProfile.Chat.TopP > 0 {
			params.TopP = activeProfile.Chat.TopP
		}
		if activeProfile.Chat.MaxTokensMode != "" {
			params.MaxTokensMode = activeProfile.Chat.MaxTokensMode
		}
		params.ReasoningEffort = activeProfile.Chat.ReasoningEffort
		if activeProfile.Chat.MaxAgenticIterations > 0 {
			params.MaxAgenticIterations = activeProfile.Chat.MaxAgenticIterations
		}
		if activeProfile.Chat.ResponseTimeout > 0 {
			params.ResponseTimeout = activeProfile.Chat.ResponseTimeout
		}
		params.RateLimitEnabled = activeProfile.Chat.RateLimitEnabled
		params.RateLimitRPM = activeProfile.GetLLMRateLimitRPM()
		params.RateLimitBurst = activeProfile.GetLLMRateLimitBurst()
		if activeProfile.Chat.ContextWindow > 0 {
			params.ContextWindow = activeProfile.Chat.ContextWindow
		}
		params.ExplicitCacheControl = activeProfile.Chat.PromptCache.Enabled &&
			activeProfile.Chat.PromptCache.ExplicitCacheControl &&
			i.providerSvc != nil &&
			i.providerSvc.SupportsExplicitCacheControl(ctx, activeProfile)
		debug := activeProfile.Chat.EffectiveDebug()
		params.DebugDump = llm.DebugDumpConfig{
			Enabled:        debug.Enabled,
			DumpRequests:   debug.DumpRequests,
			DumpResponses:  debug.DumpResponses,
			MaxFiles:       debug.MaxFiles,
			ProfileSlug:    strings.TrimSpace(params.ProfileSlug),
			ConversationID: req.ConversationID,
		}
	}

	// 8. Fall back to config default model if still empty
	if params.Model == "" && req.DefaultModel != "" {
		params.Model = req.DefaultModel
		logging.Debugf(ctx, "chat.interactor", "[PrepareContext] Usando modelo padrão: %s", params.Model)
	}
	if activeProfile != nil {
		cacheProfileSlug := strings.TrimSpace(params.ProfileSlug)
		if cacheProfileSlug == "" && i.profileMgr != nil {
			cacheProfileSlug = i.profileMgr.GetActiveSlug()
		}
		if params.DebugDump.ProfileSlug == "" {
			params.DebugDump.ProfileSlug = cacheProfileSlug
		}
		params.PromptCacheKey = ResolvePromptCacheHintKey(activeProfile, cacheProfileSlug, req.ConversationID, params.Model)
	}

	return &PrepareContextResponse{
		ActiveProfile: activeProfile,
		Params:        params,
		UserContent:   req.UserContent,
	}, nil
}

// HandleNativeMCPUnsupported é o hook chamado pelo pipeline de streaming quando uma
// request com MCP nativo falha porque o modelo/endpoint rejeita type:"mcp" (AEP-0021).
//
// Semântica de "memória" = auto-ajuste PERSISTIDO do perfil (não cache em runtime):
//   - override == nil (AUTO otimista): grava Profile.Chat.NativeMCP=false e persiste,
//     para que os próximos turnos usem adapter diretamente, sem repetir o 400. O
//     perfil já fixa o modelo, então é a granularidade certa e fica visível/editável
//     na UI. Idempotente: relê do disco e só grava na transição nil→false.
//   - override == true (Forçar nativo): NÃO sobrescreve a escolha explícita do
//     usuário; apenas loga o aviso (a request já degradou para adapter neste turno).
//   - override == false: nada a fazer (já é adapter).
//
// profileSlug é o slug resolvido para o turno (params.ProfileSlug); quando vazio,
// recai sobre o perfil ativo global. Funciona igual para chat e sub-agentes, pois
// ambos carregam o slug efetivo do run.
func (i *Interactor) HandleNativeMCPUnsupported(profileSlug, model string, override *bool) {
	if override != nil {
		if *override {
			// Resolve o slug efetivo (trim + fallback para o perfil ativo) também aqui,
			// senão o log imprimiria perfil "" no caso comum do chat normal — justamente
			// o cenário em que esse aviso de incompatibilidade de MCP nativo é útil.
			slug := strings.TrimSpace(profileSlug)
			if slug == "" && i.profileMgr != nil {
				slug = i.profileMgr.GetActiveSlug()
			}
			logging.Infof(context.Background(), "chat.interactor", "[MCP] modelo %s do perfil %q não suporta MCP nativo; usando adapter neste turno (perfil em 'forçar nativo')", model, slug)
		}
		return
	}
	if i.profileMgr == nil {
		return
	}

	slug := strings.TrimSpace(profileSlug)
	if slug == "" {
		slug = i.profileMgr.GetActiveSlug()
	}
	if slug == "" {
		return
	}

	// Serializa o read-modify-write: dois runs simultâneos do mesmo perfil não
	// gravam em corrida e o segundo encontra o disco já em false (idempotente).
	i.nativeMCPAdjustMu.Lock()
	defer i.nativeMCPAdjustMu.Unlock()

	profile, err := i.profileMgr.Get(slug)
	if err != nil {
		logging.Errorf(context.Background(), "chat.interactor", "[MCP] auto-ajuste abortado: erro ao ler perfil %q: %v", slug, err)
		return
	}
	if profile.Chat.NativeMCP != nil {
		// Já ajustado (false) ou explicitamente definido entre o início do turno e
		// agora — não regrava (transição nil→false já ocorreu ou não se aplica).
		return
	}

	adapter := false
	profile.Chat.NativeMCP = &adapter
	if err := i.profileMgr.Update(slug, profile); err != nil {
		logging.Errorf(context.Background(), "chat.interactor", "[MCP] auto-ajuste abortado: erro ao persistir perfil %q: %v", slug, err)
		return
	}
	logging.Warnf(context.Background(), "chat.interactor", "[MCP] perfil %q (modelo %s) ajustado para adapter automaticamente após erro de MCP nativo não suportado", slug, model)
}

// HandlePromptCacheHintUnsupported é chamado quando um provider/gateway rejeita
// explicitamente o hint prompt_cache_key. Diferente de cache miss ou ausência de
// métricas, isso indica incompatibilidade de payload para o modelo/rota atual.
// O turno já degradou sem hint; aqui persistimos provider_hints=false para evitar
// repetir o erro.
func (i *Interactor) HandlePromptCacheHintUnsupported(profileSlug, model string) {
	if i.profileMgr == nil {
		return
	}

	slug := strings.TrimSpace(profileSlug)
	if slug == "" {
		slug = i.profileMgr.GetActiveSlug()
	}
	if slug == "" {
		return
	}

	i.promptCacheAdjustMu.Lock()
	defer i.promptCacheAdjustMu.Unlock()

	profile, err := i.profileMgr.Get(slug)
	if err != nil {
		logging.Errorf(context.Background(), "chat.interactor", "[PromptCache] auto-ajuste abortado: erro ao ler perfil %q: %v", slug, err)
		return
	}
	if !profile.Chat.PromptCache.ProviderHints {
		return
	}

	profile.Chat.PromptCache.ProviderHints = false
	if err := i.profileMgr.Update(slug, profile); err != nil {
		logging.Errorf(context.Background(), "chat.interactor", "[PromptCache] auto-ajuste abortado: erro ao persistir perfil %q: %v", slug, err)
		return
	}
	logging.Infof(context.Background(), "chat.interactor", "[PromptCache] perfil %q (modelo %s) ajustado para provider_hints=false após rejeição explícita de prompt_cache_key", slug, model)
}

// RecordUserMessageRequest contém a entrada do usuário já processada (incluindo STT) pronta para ser persistida.
type RecordUserMessageRequest struct {
	ConversationID string
	Content        string
	Media          string
	AudioBase64    string
	AudioMimeType  string
	Source         string
	SurfaceOrigin  *ports.ChatSurfaceOrigin
	ActiveProfile  *profiles.Profile
	Transcribe     TranscribeFunc
	// MaxContextMessages, se > 0, sobrescreve o limite do perfil ao carregar histórico.
	MaxContextMessages int
}

// RecordUserMessageResponse contém a mensagem salva e o histórico da conversa carregado.
type RecordUserMessageResponse struct {
	UserMsg             *Message
	Messages            []llm.Message
	ConversationSummary string
}

// GetRetryableUserMessage retorna uma mensagem existente validando que ela pode ser reenviada.
func (i *Interactor) GetRetryableUserMessage(ctx context.Context, conversationID string, messageID string) (*Message, error) {
	if i.repo == nil {
		return nil, errors.New("repositório de mensagens indisponível")
	}
	userMsg, err := i.repo.GetMessage(ctx, messageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("mensagem não encontrada")
		}
		return nil, err
	}
	if userMsg == nil {
		return nil, errors.New("mensagem não encontrada")
	}
	if userMsg.ConversationID != conversationID {
		return nil, fmt.Errorf("mensagem %s não pertence à conversa %s", messageID, conversationID)
	}
	if userMsg.Role != "user" {
		return nil, fmt.Errorf("mensagem %s não é do usuário", messageID)
	}
	return userMsg, nil
}

// RecordUserMessage persiste a mensagem do usuário, emite o evento ready e carrega o histórico da conversa.
func (i *Interactor) RecordUserMessage(ctx context.Context, req RecordUserMessageRequest) (*RecordUserMessageResponse, error) {
	userMsg, err := i.repo.CreateMessage(ctx, MessageOptions{
		ConversationID: req.ConversationID,
		Role:           "user",
		Content:        req.Content,
		Media:          req.Media,
		Audio:          req.AudioBase64,
		AudioMimeType:  req.AudioMimeType,
		Source:         req.Source,
	})
	if err != nil {
		i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: "Erro ao salvar mensagem: " + err.Error()})
		return nil, err
	}

	i.emitter.Emit("chat:messages_ready", ports.MessagesReadyEvent{
		ConversationID: req.ConversationID,
		UserMessageID:  userMsg.ID,
		TurnID:         userMsg.ID,
		UserContent:    userMsg.Content,
		SurfaceOrigin:  req.SurfaceOrigin,
	})

	return i.ReuseLoadedUserMessage(ctx, req, userMsg)
}

// ReuseLoadedUserMessage monta a resposta de retry a partir de uma mensagem já validada/carregada.
func (i *Interactor) ReuseLoadedUserMessage(ctx context.Context, req RecordUserMessageRequest, userMsg *Message) (*RecordUserMessageResponse, error) {
	if userMsg == nil {
		return nil, errors.New("mensagem não encontrada")
	}

	maxCtxMsgs := DefaultMaxContextMessages
	if req.MaxContextMessages > 0 {
		maxCtxMsgs = req.MaxContextMessages
	} else if req.ActiveProfile != nil {
		maxCtxMsgs = req.ActiveProfile.GetMaxContextMessages()
	}
	loader := MediaHistoryLoader{
		Repo:       i.repo,
		Transcribe: req.Transcribe,
		MaxMsgs:    maxCtxMsgs,
	}
	messages, summary, err := loader.Load(ctx, req.ConversationID)
	if err != nil {
		i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: "Erro ao carregar histórico: " + err.Error()})
		return nil, err
	}

	return &RecordUserMessageResponse{
		UserMsg:             userMsg,
		Messages:            messages,
		ConversationSummary: summary,
	}, nil
}

// ReuseUserMessage carrega uma mensagem de usuário já persistida para um retry sem duplicá-la no banco.
func (i *Interactor) ReuseUserMessage(ctx context.Context, req RecordUserMessageRequest, messageID string) (*RecordUserMessageResponse, error) {
	userMsg, err := i.GetRetryableUserMessage(ctx, req.ConversationID, messageID)
	if err != nil {
		i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: "Erro ao carregar mensagem para retry: " + err.Error()})
		return nil, err
	}
	return i.ReuseLoadedUserMessage(ctx, req, userMsg)
}

// ResolveUserContentRequest contém os dados brutos para resolução de conteúdo do usuário.
type ResolveUserContentRequest struct {
	Content     string
	Media       string
	Source      string
	STTProvider string // activeProfile.Input.STTProvider (pode ser "")
	Transcribe  TranscribeFunc
}

// ResolveUserContentResponse contém o conteúdo resolvido e os dados de áudio extraídos.
type ResolveUserContentResponse struct {
	Content       string
	AudioBase64   string
	AudioMimeType string
}

// ResolveUserContent extrai o áudio do media, aplica fallback STT para canais não-Wails
// e transcreve automaticamente quando o conteúdo está vazio e há mídia de áudio.
// Esta é lógica pura de domínio — sem acesso a banco ou I/O externo além de Transcribe.
func (i *Interactor) ResolveUserContent(ctx context.Context, req ResolveUserContentRequest) ResolveUserContentResponse {
	audioBase64, audioMime := ExtractAudio(req.Media)

	content := req.Content
	if content == "" && req.Media != "" {
		if req.Source != "wails" {
			stt := req.STTProvider
			if stt == "webspeech" || stt == "" {
				logging.Infof(ctx, "chat.interactor", "[ResolveUserContent] Canal %s: STT '%s' não suporta transcrição server-side — usando placeholder", req.Source, stt)
				content = "[Mensagem de áudio recebida, mas transcrição automática não está configurada. Configure Whisper no perfil deste canal para processar mensagens de voz.]"
			}
		}
		if content == "" && req.Transcribe != nil {
			if text, err := req.Transcribe(ctx, audioBase64, WhisperFilename(strings.TrimPrefix(audioMime, "audio/"))); err == nil {
				content = text
			}
		}
	}

	return ResolveUserContentResponse{
		Content:       content,
		AudioBase64:   audioBase64,
		AudioMimeType: audioMime,
	}
}

// PrepareMessagesRequest carries inputs for the PrepareMessages pipeline.
type PrepareMessagesRequest struct {
	Messages            []llm.Message
	UserContent         string
	ConversationSummary string
	ConversationID      string
	TurnID              string
	Params              ChatParams
	ActiveProfile       *profiles.Profile
	SurfaceOrigin       *ports.ChatSurfaceOrigin
	Transcribe          TranscribeFunc
	// AgentTurn diz que quem conduz o turno é um agente de código (AEP-0084
	// D4, revisto na Fase 8). Ele leva só a mensagem da pessoa: nada de
	// persona, skills, memória ou blocos de contexto, que o agente resolve com
	// recursos próprios e acesso direto à árvore de arquivos.
	AgentTurn bool
}

// PrepareMessagesResponse carries the outputs of PrepareMessages.
type PrepareMessagesResponse struct {
	Messages                    []llm.Message
	InvokedSkillSlug            string
	InvokedScope                *tools.FilesystemScope
	InvokedExecutionContext     *tools.ExecutionContext
	ModelOnDemandSkillAvailable bool
	Err                         error
}

// PrepareMessages detects slash skill invocation, injects the full system prompt,
// and preprocesses media messages (audio transcription, unsupported format fallbacks).
// It replaces the app-layer helpers prepareMessages, buildFullSystemPrompt,
// and effectivePromptBuilder.
func (i *Interactor) PrepareMessages(ctx context.Context, req PrepareMessagesRequest) PrepareMessagesResponse {
	if req.AgentTurn {
		return i.prepareAgentMessages(ctx, req)
	}
	var skillTplData TemplateData
	if i.promptBuilder != nil {
		skillTplData = i.promptBuilder.BuildTemplateData(req.ActiveProfile, req.Params, req.ConversationID)
	}

	var slashSkillContent string
	var pendingInvokedSkillSlug string
	var pendingInvokedDisplayName string
	var pendingInvokedMode string
	var pendingInvokedScope *tools.FilesystemScope
	var pendingInvokedExecutionContext *tools.ExecutionContext
	var taskListContextEnabled bool
	_, _, isSlashCommand := skills.ParseSlashCommand(req.UserContent)
	skillPolicy, policyReady, policyErr := i.BuildSkillSelectionPolicy(req.ActiveProfile)
	if policyErr != nil {
		if !isSlashCommand {
			logging.Errorf(ctx, "chat.interactor", "[chat] erro ao carregar política de skills; seguindo sem autoativação/contexto de skills: %v", policyErr)
		} else {
			if i.emitter != nil {
				i.emitter.Emit("chat:error", ports.ErrorEvent{
					ConversationID: req.ConversationID,
					Error:          policyErr.Error(),
					SurfaceOrigin:  req.SurfaceOrigin,
				})
			}
			return PrepareMessagesResponse{Messages: req.Messages, Err: policyErr}
		}
	}
	if policyErr != nil {
		policyReady = false
	}

	var inv *skills.InvocationResult
	var found bool
	var err error
	var modelOnDemandSkillAvailable bool
	if policyReady {
		modelOnDemandSkillAvailable = skillPolicy.HasModelOnDemandSkill()
		taskListContextEnabled = skillPolicy.IsEnabled("tasklist-manager")
		inv, found, err = skills.Invoke(req.UserContent, i.skillMgr, skillTplData, req.ConversationID, skillPolicy)
	}
	if found {
		if err != nil {
			if i.emitter != nil {
				i.emitter.Emit("chat:error", ports.ErrorEvent{
					ConversationID: req.ConversationID,
					Error:          err.Error(),
					SurfaceOrigin:  req.SurfaceOrigin,
				})
			}
			return PrepareMessagesResponse{Messages: req.Messages, Err: err}
		}
		if inv.Mode == skills.SkillModeBase {
			if args := strings.TrimSpace(inv.Arguments); args != "" {
				slashSkillContent = formatBaseSkillArguments(inv.SkillSlug, args)
			}
		} else {
			slashSkillContent = inv.Content
		}
		pendingInvokedSkillSlug = inv.SkillSlug
		pendingInvokedDisplayName = inv.DisplayName
		pendingInvokedMode = string(inv.Mode)
		pendingInvokedExecutionContext = executionContextFromInvocation(inv)
		if inv.Filesystem != nil {
			pendingInvokedScope = &tools.FilesystemScope{
				Read:  append([]string{}, inv.Filesystem.Read...),
				Write: append([]string{}, inv.Filesystem.Write...),
				Deny:  append([]string{}, inv.Filesystem.Deny...),
			}
		}
	}

	var linkedTaskLists []contextprovider.LinkedTaskList
	if taskListContextEnabled && i.promptBuilder != nil && i.contextProviders != nil && i.linkedTaskLists != nil && strings.TrimSpace(req.ConversationID) != "" {
		linkedTaskLists = i.linkedTaskLists(ctx, req.ConversationID)
	}

	var enabledSkills []string
	var disableOnDemand bool
	var disableSkills bool
	if req.ActiveProfile != nil {
		enabledSkills = req.ActiveProfile.Chat.EnabledSkills
		disableOnDemand = req.ActiveProfile.Chat.DisableOnDemandSkills
		disableSkills = req.ActiveProfile.Chat.DisableSkills
	}

	contextBlocks := i.buildDynamicContext(ctx, skillTplData, req.UserContent, req.ConversationSummary, slashSkillContent, linkedTaskLists, taskListContextEnabled, req.ActiveProfile)
	slashSkillInjected := strings.TrimSpace(slashSkillContent) == "" || (i.promptBuilder != nil && hasContextBlock(contextBlocks, "slash_skill", "slash_skill"))

	var invokedSkillSlug string
	var invokedScope *tools.FilesystemScope
	var invokedExecutionContext *tools.ExecutionContext
	if slashSkillInjected {
		invokedSkillSlug = pendingInvokedSkillSlug
		invokedScope = pendingInvokedScope
		invokedExecutionContext = pendingInvokedExecutionContext
		if strings.TrimSpace(slashSkillContent) != "" && i.emitter != nil {
			i.emitter.Emit("chat:skill_loaded", ports.SkillLoadedEvent{
				ConversationID: req.ConversationID,
				TurnID:         req.TurnID,
				Slug:           pendingInvokedSkillSlug,
				DisplayName:    pendingInvokedDisplayName,
				Mode:           pendingInvokedMode,
				SurfaceOrigin:  req.SurfaceOrigin,
			})
		}
	}

	var messages []llm.Message
	if i.promptBuilder != nil {
		messages = i.promptBuilder.BuildWithContextBlocks(markTurnContextTarget(req.Messages, req.TurnID), enabledSkills, disableSkills, disableOnDemand, skillTplData, contextBlocks)
	} else {
		messages = req.Messages
	}

	var audioSupported, docSupported *bool
	if req.ActiveProfile != nil && req.ActiveProfile.MediaSupport != nil {
		audioSupported = req.ActiveProfile.MediaSupport.Audio
		docSupported = req.ActiveProfile.MediaSupport.Document
	}
	messages = PreprocessMessages(ctx, messages, req.Transcribe, audioSupported, docSupported)

	return PrepareMessagesResponse{
		Messages:                    messages,
		InvokedSkillSlug:            invokedSkillSlug,
		InvokedScope:                invokedScope,
		InvokedExecutionContext:     invokedExecutionContext,
		ModelOnDemandSkillAvailable: modelOnDemandSkillAvailable,
		Err:                         nil,
	}
}

// prepareAgentMessages prepara o turno conduzido por um agente de código. Nada
// do que o app sabe entra: sem system prompt, sem blocos de contexto e sem
// skill invocada por barra — num perfil de agente o menu da barra é dele
// (AEP-0084, Fase 8). O texto vai como a pessoa escreveu, e `/algo` que ele
// entenda chega intacto.
//
// A mídia continua passando pelo mesmo pré-processamento: anexo é conteúdo da
// pessoa, e transcrever áudio que o agente não recebe é o que permite mandar o
// texto no lugar dele.
func (i *Interactor) prepareAgentMessages(ctx context.Context, req PrepareMessagesRequest) PrepareMessagesResponse {
	var audioSupported, docSupported *bool
	if req.ActiveProfile != nil && req.ActiveProfile.MediaSupport != nil {
		audioSupported = req.ActiveProfile.MediaSupport.Audio
		docSupported = req.ActiveProfile.MediaSupport.Document
	}
	return PrepareMessagesResponse{
		Messages: PreprocessMessages(ctx, req.Messages, req.Transcribe, audioSupported, docSupported),
	}
}

func markTurnContextTarget(messages []llm.Message, turnID string) []llm.Message {
	out := append([]llm.Message(nil), messages...)
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return out
	}
	for idx := range out {
		out[idx].TurnContextTarget = false
		if out[idx].MessageID == turnID {
			out[idx].TurnContextTarget = true
		}
	}
	return out
}

func executionContextFromInvocation(inv *skills.InvocationResult) *tools.ExecutionContext {
	if inv == nil {
		return nil
	}
	ec := &tools.ExecutionContext{InvokedSkillSlug: inv.SkillSlug}
	if inv.Filesystem != nil {
		ec.Filesystem = &tools.FilesystemScope{
			Read:  append([]string{}, inv.Filesystem.Read...),
			Write: append([]string{}, inv.Filesystem.Write...),
			Deny:  append([]string{}, inv.Filesystem.Deny...),
		}
	}
	if inv.Tools != nil {
		ec.AllowedTools = append([]string{}, inv.Tools.Allowed...)
		ec.DeniedTools = append([]string{}, inv.Tools.Denied...)
		if inv.Tools.BashCommands != nil {
			ec.AllowedBash = append([]string{}, inv.Tools.BashCommands.Allowed...)
			ec.DeniedBash = append([]string{}, inv.Tools.BashCommands.Denied...)
		}
	}
	if inv.Network != nil {
		ec.NetworkAllowedHost = append([]string{}, inv.Network.AllowedHosts...)
		ec.NetworkDeniedHost = append([]string{}, inv.Network.DeniedHosts...)
	}
	return ec
}

func formatBaseSkillArguments(slug, args string) string {
	var sb strings.Builder
	sb.WriteString("<invoked_skill_arguments>\n")
	sb.WriteString("Skill: ")
	sb.WriteString(slug)
	sb.WriteString("\n")
	sb.WriteString("For this turn, apply these slash-command arguments to the already-loaded base skill. ")
	sb.WriteString("Treat `$ARGUMENTS` in that base skill as the full arguments string below and `$1`, `$2`, ... as whitespace-separated positional arguments.\n")
	sb.WriteString("Arguments:\n")
	sb.WriteString(args)
	sb.WriteString("\n</invoked_skill_arguments>")
	return sb.String()
}

func (i *Interactor) BuildSkillSelectionPolicy(activeProfile *profiles.Profile) (skills.SelectionPolicy, bool, error) {
	if i.skillMgr == nil {
		return skills.SelectionPolicy{}, false, nil
	}
	allSkills, err := i.skillMgr.GetAllSkillsFull()
	if err != nil {
		return skills.SelectionPolicy{}, false, fmt.Errorf("erro ao carregar política de skills: %w", err)
	}
	var enabledSkills []string
	var disableOnDemand bool
	var disableSkills bool
	if activeProfile != nil {
		enabledSkills = activeProfile.Chat.EnabledSkills
		disableOnDemand = activeProfile.Chat.DisableOnDemandSkills
		disableSkills = activeProfile.Chat.DisableSkills
	}
	return skills.ResolveSelectionPolicy(allSkills, enabledSkills, disableSkills, disableOnDemand), true, nil
}

func (i *Interactor) ValidateSkillInvocation(activeProfile *profiles.Profile, userContent string, conversationID string, surfaceOrigin *ports.ChatSurfaceOrigin) error {
	slug, _, ok := skills.ParseSlashCommand(userContent)
	if !ok {
		return nil
	}
	policy, policyReady, err := i.BuildSkillSelectionPolicy(activeProfile)
	if err != nil {
		if i.emitter != nil {
			i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: conversationID, Error: err.Error(), SurfaceOrigin: surfaceOrigin})
		}
		return err
	}
	if !policyReady {
		return nil
	}
	skill, err := i.skillMgr.Get(slug)
	if err != nil || skill == nil || !skill.IsUserInvocable() {
		return nil
	}
	if policy.ModeFor(slug) == skills.SkillModeDisabled {
		err := fmt.Errorf("skill /%s está desabilitada no perfil ativo", slug)
		if i.emitter != nil {
			i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: conversationID, Error: err.Error(), SurfaceOrigin: surfaceOrigin})
		}
		return err
	}
	return nil
}

func (i *Interactor) buildDynamicContext(ctx context.Context, data TemplateData, currentUserText string, conversationSummary string, slashSkillContent string, linkedTaskLists []contextprovider.LinkedTaskList, taskListContextEnabled bool, activeProfile *profiles.Profile) []contextprovider.Block {
	if i.contextProviders == nil {
		return nil
	}
	providerBudgets, providerEnabled, providerSettings := resolveContextProviderProfileConfig(i.contextProviders.Metadata(), activeProfile)
	req := contextprovider.BuildRequest{
		ConversationID:         data.ConversationID,
		WorkspaceID:            data.WorkspaceID,
		ProjectID:              data.ProjectID,
		WorkspaceName:          data.WorkspaceName,
		WorkspaceProfile:       data.WorkspaceProfile,
		TabCount:               data.TabCount,
		ActiveTabTitle:         data.ActiveTabTitle,
		ActiveTabType:          data.ActiveTabType,
		Tabs:                   make([]contextprovider.Tab, 0, len(data.Tabs)),
		CurrentUserText:        currentUserText,
		ConversationSummary:    conversationSummary,
		SlashSkillContent:      slashSkillContent,
		EnabledSkills:          enabledSkillList(activeProfile),
		DisableSkills:          activeProfile != nil && activeProfile.Chat.DisableSkills,
		DisableOnDemand:        activeProfile != nil && activeProfile.Chat.DisableOnDemandSkills,
		ToolCallingEnabled:     data.ToolCallingEnabled,
		EnabledTools:           append([]string(nil), data.EnabledTools...),
		ProviderBudgets:        providerBudgets,
		ProviderEnabled:        providerEnabled,
		ProviderSettings:       providerSettings,
		TaskListContextEnabled: taskListContextEnabled,
		LinkedTaskLists:        linkedTaskLists,
	}
	for _, tab := range data.Tabs {
		req.Tabs = append(req.Tabs, contextprovider.Tab{
			Title:     tab.Title,
			Type:      tab.Type,
			ContentID: tab.ContentID,
			IsActive:  tab.IsActive,
			State:     tab.State,
		})
	}
	if data.Surface != nil {
		req.Surface = &contextprovider.Surface{
			Type:    data.Surface.Type,
			Title:   data.Surface.Title,
			State:   data.Surface.State,
			Context: data.Surface.Context,
		}
	}
	blocks, err := i.contextProviders.Build(ctx, req)
	if err != nil {
		logging.Errorf(ctx, "chat.interactor", "[context/providers] erro ao montar blocos dinâmicos: %v", err)
		return nil
	}
	return blocks
}

func enabledSkillList(activeProfile *profiles.Profile) []string {
	if activeProfile == nil {
		return nil
	}
	return activeProfile.Chat.EnabledSkills
}

func hasContextBlock(blocks []contextprovider.Block, provider string, name string) bool {
	for _, block := range blocks {
		if block.Provider == provider && block.Name == name && strings.TrimSpace(block.Content) != "" {
			return true
		}
	}
	return false
}

func resolveContextProviderProfileConfig(metadata []contextprovider.ProviderMetadata, activeProfile *profiles.Profile) (map[string]int, map[string]bool, map[string]map[string]any) {
	budgets := make(map[string]int, len(metadata))
	enabled := make(map[string]bool, len(metadata))
	settings := make(map[string]map[string]any)
	var overrides map[string]profiles.ContextProviderProfileConfig
	if activeProfile != nil {
		overrides = activeProfile.ContextProviders
	}
	for _, item := range metadata {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		budget := item.DefaultBudget
		isEnabled := item.DefaultEnabled
		if cfg, ok := overrides[item.Name]; ok {
			if cfg.Enabled != nil {
				isEnabled = *cfg.Enabled
			}
			if cfg.Budget > 0 {
				budget = cfg.Budget
			}
			if len(cfg.Settings) > 0 {
				settings[item.Name] = cfg.Settings
			}
		}
		budgets[item.Name] = budget
		enabled[item.Name] = isEnabled
	}
	return budgets, enabled, settings
}
