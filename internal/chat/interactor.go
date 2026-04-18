package chat

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"assistente/internal/core/ports"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"assistente/internal/skills"
	"assistente/internal/tools"
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
	Build(messages []llm.Message, enabledSkills []string, disableOnDemand bool, tplData any, slashSkillContent string, conversationSummary string) []llm.Message
	BuildTemplateData(activeProfile *profiles.Profile, params llm.ChatParams, conversationID uint) TemplateData
}

// InteractorConfig groups all dependencies for Interactor.
type InteractorConfig struct {
	Emitter       events.Emitter
	Repo          MessageRepository
	ConvRepo      ConversationRepository
	ProviderSvc   *providers.Service
	ProfileMgr    *profiles.Manager
	SkillMgr      skills.InvokerManager // optional during startup; safe to be nil
	PromptBuilder SystemPromptBuilder   // optional during startup; safe to be nil
}

// Interactor orchestrates the core chat use cases, free of Wails dependencies.
type Interactor struct {
	emitter       events.Emitter
	repo          MessageRepository
	convRepo      ConversationRepository
	providerSvc   *providers.Service
	profileMgr    *profiles.Manager
	skillMgr      skills.InvokerManager
	promptBuilder SystemPromptBuilder
}

func inheritProfileRoutingFields(base *profiles.Profile, fallback *profiles.Profile) *profiles.Profile {
	if base == nil || fallback == nil {
		return base
	}

	merged := *base
	merged.Chat = base.Chat
	merged.Voice = base.Voice
	merged.Input = base.Input

	if strings.TrimSpace(merged.Chat.LLMProvider) == "" {
		merged.Chat.LLMProvider = fallback.Chat.LLMProvider
	}
	if strings.TrimSpace(merged.Chat.Model) == "" {
		merged.Chat.Model = fallback.Chat.Model
	}
	if strings.TrimSpace(merged.Voice.Assistant.LLMProviderID) == "" {
		merged.Voice.Assistant.LLMProviderID = fallback.Voice.Assistant.LLMProviderID
	}
	if strings.TrimSpace(merged.Input.LLMProviderID) == "" {
		merged.Input.LLMProviderID = fallback.Input.LLMProviderID
	}

	return &merged
}

// NewInteractor creates an Interactor with its required dependencies.
func NewInteractor(cfg InteractorConfig) *Interactor {
	return &Interactor{
		emitter:       cfg.Emitter,
		repo:          cfg.Repo,
		convRepo:      cfg.ConvRepo,
		providerSvc:   cfg.ProviderSvc,
		profileMgr:    cfg.ProfileMgr,
		skillMgr:      cfg.SkillMgr,
		promptBuilder: cfg.PromptBuilder,
	}
}

// PrepareContextRequest carries the raw inputs for a message send request.
type PrepareContextRequest struct {
	ConversationID uint
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
		providerCount, _ := i.providerSvc.Count()
		if providerCount == 0 {
			msg := "Nenhum provedor LLM configurado. Configure um provedor nas configurações."
			i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: req.ConversationID, Error: msg})
			return nil, fmt.Errorf("nenhum provedor LLM configurado")
		}
	}

	// 3. Validate conversation ID
	if req.ConversationID == 0 {
		const errMsg = "conversationID é obrigatório — conversas devem ser criadas ao criar/resetar a tab"
		i.emitter.Emit("chat:error", ports.ErrorEvent{ConversationID: 0, Error: errMsg})
		return nil, errors.New(errMsg)
	}

	// 4. Auto-rename conversation if it still has the generic default title
	if req.UserContent != "" {
		conv, convErr := i.convRepo.GetConversationInfo(req.ConversationID)
		if convErr == nil && conv != nil && conv.Title == "Nova Conversa" {
			title := req.UserContent
			if len(title) > 50 {
				title = title[:50]
			}
			if err := i.convRepo.UpdateConversation(req.ConversationID, title, ""); err == nil {
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
	if i.profileMgr == nil {
		log.Printf("[PrepareContext] profileManager não inicializado — continuando sem perfil")
	} else if req.Params.ProfileSlug != "" {
		activeProfile, err = i.profileMgr.Get(req.Params.ProfileSlug)
		if err != nil {
			log.Printf("[PrepareContext] Erro ao obter perfil '%s': %v — usando perfil ativo global", req.Params.ProfileSlug, err)
			activeProfile, err = i.profileMgr.GetActive()
		} else {
			globalActive, globalErr := i.profileMgr.GetActive()
			if globalErr != nil {
				log.Printf("[PrepareContext] Erro ao obter perfil ativo global para fallback: %v", globalErr)
			} else {
				activeProfile = inheritProfileRoutingFields(activeProfile, globalActive)
			}
		}
	} else {
		activeProfile, err = i.profileMgr.GetActive()
	}
	if err != nil {
		log.Printf("[PrepareContext] Erro ao obter perfil: %v", err)
	}

	// 6. Resolve $default sentinels (provider/model)
	if activeProfile != nil && i.providerSvc != nil {
		activeProfile = i.providerSvc.ResolveProfileDefaults(activeProfile)
	}

	// 7. Apply profile-level chat defaults onto Params
	params := req.Params
	if activeProfile != nil {
		log.Printf("[PrepareContext] Usando perfil: %s", activeProfile.Name)
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
	}

	// 8. Fall back to config default model if still empty
	if params.Model == "" && req.DefaultModel != "" {
		params.Model = req.DefaultModel
		log.Printf("[PrepareContext] Usando modelo padrão: %s", params.Model)
	}

	return &PrepareContextResponse{
		ActiveProfile: activeProfile,
		Params:        params,
		UserContent:   req.UserContent,
	}, nil
}

// RecordUserMessageRequest contém a entrada do usuário já processada (incluindo STT) pronta para ser persistida.
type RecordUserMessageRequest struct {
	ConversationID uint
	Content        string
	Media          string
	AudioBase64    string
	AudioMimeType  string
	Source         string
	ActiveProfile  *profiles.Profile
	Transcribe     TranscribeFunc
}

// RecordUserMessageResponse contém a mensagem salva e o histórico da conversa carregado.
type RecordUserMessageResponse struct {
	UserMsg             *Message
	Messages            []llm.Message
	ConversationSummary string
}

// GetRetryableUserMessage retorna uma mensagem existente validando que ela pode ser reenviada.
func (i *Interactor) GetRetryableUserMessage(conversationID uint, messageID uint) (*Message, error) {
	if i.repo == nil {
		return nil, errors.New("repositório de mensagens indisponível")
	}
	userMsg, err := i.repo.GetMessage(messageID)
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
		return nil, fmt.Errorf("mensagem %d não pertence à conversa %d", messageID, conversationID)
	}
	if userMsg.Role != "user" {
		return nil, fmt.Errorf("mensagem %d não é do usuário", messageID)
	}
	return userMsg, nil
}

// RecordUserMessage persiste a mensagem do usuário, emite o evento ready e carrega o histórico da conversa.
func (i *Interactor) RecordUserMessage(ctx context.Context, req RecordUserMessageRequest) (*RecordUserMessageResponse, error) {
	userMsg, err := i.repo.CreateMessage(MessageOptions{
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
		UserContent:    userMsg.Content,
	})

	return i.ReuseLoadedUserMessage(ctx, req, userMsg)
}

// ReuseLoadedUserMessage monta a resposta de retry a partir de uma mensagem já validada/carregada.
func (i *Interactor) ReuseLoadedUserMessage(_ context.Context, req RecordUserMessageRequest, userMsg *Message) (*RecordUserMessageResponse, error) {
	if userMsg == nil {
		return nil, errors.New("mensagem não encontrada")
	}

	maxCtxMsgs := DefaultMaxContextMessages
	if req.ActiveProfile != nil {
		maxCtxMsgs = req.ActiveProfile.GetMaxContextMessages()
	}
	loader := MediaHistoryLoader{
		Repo:       i.repo,
		Transcribe: req.Transcribe,
		MaxMsgs:    maxCtxMsgs,
	}
	messages, summary, err := loader.Load(req.ConversationID)
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
func (i *Interactor) ReuseUserMessage(ctx context.Context, req RecordUserMessageRequest, messageID uint) (*RecordUserMessageResponse, error) {
	userMsg, err := i.GetRetryableUserMessage(req.ConversationID, messageID)
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
func (i *Interactor) ResolveUserContent(_ context.Context, req ResolveUserContentRequest) ResolveUserContentResponse {
	audioBase64, audioMime := ExtractAudio(req.Media)

	content := req.Content
	if content == "" && req.Media != "" {
		if req.Source != "wails" {
			stt := req.STTProvider
			if stt == "webspeech" || stt == "" {
				log.Printf("[ResolveUserContent] Canal %s: STT '%s' não suporta transcrição server-side — usando placeholder", req.Source, stt)
				content = "[Mensagem de áudio recebida, mas transcrição automática não está configurada. Configure Whisper no perfil deste canal para processar mensagens de voz.]"
			}
		}
		if content == "" && req.Transcribe != nil {
			if text, err := req.Transcribe(audioBase64, WhisperFilename(strings.TrimPrefix(audioMime, "audio/"))); err == nil {
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
	ConversationID      uint
	Params              ChatParams
	ActiveProfile       *profiles.Profile
	Transcribe          TranscribeFunc
}

// PrepareMessagesResponse carries the outputs of PrepareMessages.
type PrepareMessagesResponse struct {
	Messages         []llm.Message
	InvokedSkillSlug string
	InvokedScope     *tools.FilesystemScope
}

// PrepareMessages detects slash skill invocation, injects the full system prompt,
// and preprocesses media messages (audio transcription, unsupported format fallbacks).
// It replaces the app-layer helpers prepareMessages, buildFullSystemPrompt,
// and effectivePromptBuilder.
func (i *Interactor) PrepareMessages(req PrepareMessagesRequest) PrepareMessagesResponse {
	var skillTplData TemplateData
	if i.promptBuilder != nil {
		skillTplData = i.promptBuilder.BuildTemplateData(req.ActiveProfile, req.Params, req.ConversationID)
	}

	var slashSkillContent string
	var invokedSkillSlug string
	var invokedScope *tools.FilesystemScope

	if inv, found, _ := skills.Invoke(req.UserContent, i.skillMgr, skillTplData, req.ConversationID); found {
		slashSkillContent = inv.Content
		invokedSkillSlug = inv.SkillSlug
		if inv.Filesystem != nil {
			invokedScope = &tools.FilesystemScope{
				Read:  append([]string{}, inv.Filesystem.Read...),
				Write: append([]string{}, inv.Filesystem.Write...),
				Deny:  append([]string{}, inv.Filesystem.Deny...),
			}
		}
	}

	var enabledSkills []string
	var disableOnDemand bool
	if req.ActiveProfile != nil {
		enabledSkills = req.ActiveProfile.Chat.EnabledSkills
		disableOnDemand = req.ActiveProfile.Chat.DisableOnDemandSkills
		if req.ActiveProfile.Chat.DisableSkills {
			enabledSkills = []string{}
		}
	}

	var messages []llm.Message
	if i.promptBuilder != nil {
		messages = i.promptBuilder.Build(req.Messages, enabledSkills, disableOnDemand, skillTplData, slashSkillContent, req.ConversationSummary)
	} else {
		messages = req.Messages
	}

	var audioSupported, docSupported *bool
	if req.ActiveProfile != nil && req.ActiveProfile.MediaSupport != nil {
		audioSupported = req.ActiveProfile.MediaSupport.Audio
		docSupported = req.ActiveProfile.MediaSupport.Document
	}
	messages = PreprocessMessages(messages, req.Transcribe, audioSupported, docSupported)

	return PrepareMessagesResponse{
		Messages:         messages,
		InvokedSkillSlug: invokedSkillSlug,
		InvokedScope:     invokedScope,
	}
}
