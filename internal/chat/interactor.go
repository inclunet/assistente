package chat

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"assistente/internal/config"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"
)

// DefaultMaxContextMessages é o limite padrão de mensagens carregadas no contexto.
const DefaultMaxContextMessages = 50

const (
	MaxMessageContentSize = 512 * 1024       // 512 KB
	MaxMediaSize          = 20 * 1024 * 1024 // 20 MB
)

// ChatParams is an alias for llm.ChatParams — single source of truth.
type ChatParams = llm.ChatParams

// Interactor orchestrates the core chat use cases, free of Wails dependencies.
type Interactor struct {
	emitter     events.Emitter
	repo        MessageRepository
	convRepo    ConversationRepository
	providerSvc *providers.Service
	profileMgr  *profiles.Manager
}

// NewInteractor creates an Interactor with its required dependencies.
func NewInteractor(
	em events.Emitter,
	repo MessageRepository,
	convRepo ConversationRepository,
	providerSvc *providers.Service,
	profileMgr *profiles.Manager,
) *Interactor {
	return &Interactor{
		emitter:     em,
		repo:        repo,
		convRepo:    convRepo,
		providerSvc: providerSvc,
		profileMgr:  profileMgr,
	}
}

// PrepareContextRequest carries the raw inputs for a message send request.
type PrepareContextRequest struct {
	ConversationID uint
	UserContent    string
	UserMedia      string
	Params         ChatParams
	Source         string
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
		i.emitter.Emit("chat:error", errMsg)
		return nil, errors.New(errMsg)
	}
	if len(req.UserMedia) > MaxMediaSize {
		errMsg := fmt.Sprintf("Mídia muito grande (%d bytes). Máximo permitido: %d bytes", len(req.UserMedia), MaxMediaSize)
		i.emitter.Emit("chat:error", errMsg)
		return nil, errors.New(errMsg)
	}

	// 2. Verify credentials
	cfg, err := config.Load()
	if err != nil {
		i.emitter.Emit("chat:error", "Erro ao carregar configuração: "+err.Error())
		return nil, err
	}
	if cfg.APIKey == "" && i.providerSvc != nil {
		providerCount, _ := i.providerSvc.Count()
		if providerCount == 0 {
			msg := "Nenhum provedor LLM configurado. Configure um provedor nas configurações."
			i.emitter.Emit("chat:error", msg)
			return nil, fmt.Errorf("nenhum provedor LLM configurado")
		}
	}

	// 3. Validate conversation ID
	if req.ConversationID == 0 {
		const errMsg = "conversationID é obrigatório — conversas devem ser criadas ao criar/resetar a tab"
		i.emitter.Emit("chat:error", errMsg)
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
				i.emitter.Emit("conversation:renamed", map[string]interface{}{
					"conversation_id": req.ConversationID,
					"new_title":       title,
				})
			}
		}
	}

	// 5. Resolve active profile
	var activeProfile *profiles.Profile
	if i.profileMgr == nil {
		log.Printf("[PrepareContext] profileManager não inicializado — continuando sem perfil")
	} else if req.Params.ProfileSlug != "" {
		activeProfile, err = i.profileMgr.Get(req.Params.ProfileSlug)
		if err != nil {
			log.Printf("[PrepareContext] Erro ao obter perfil '%s': %v — usando perfil ativo global", req.Params.ProfileSlug, err)
			activeProfile, err = i.profileMgr.GetActive()
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
	if params.Model == "" {
		params.Model = cfg.DefaultModel
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
	UserMsg             *database.ChatMessage
	Messages            []llm.Message
	ConversationSummary string
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
		i.emitter.Emit("chat:error", "Erro ao salvar mensagem: "+err.Error())
		return nil, err
	}

	i.emitter.Emit("chat:messages_ready", map[string]interface{}{
		"conversationId": req.ConversationID,
		"userMessageId":  userMsg.ID,
		"userContent":    userMsg.Content,
	})

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
		i.emitter.Emit("chat:error", "Erro ao carregar histórico: "+err.Error())
		return nil, err
	}

	return &RecordUserMessageResponse{
		UserMsg:             userMsg,
		Messages:            messages,
		ConversationSummary: summary,
	}, nil
}

// ResolveUserContentRequest contém os dados brutos para resolução de conteúdo do usuário.
type ResolveUserContentRequest struct {
	Content    string
	Media      string
	Source     string
	STTProvider string   // activeProfile.Input.STTProvider (pode ser "")
	Transcribe TranscribeFunc
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
