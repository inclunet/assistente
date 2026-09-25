package wailsapi

import (
	"context"
	"errors"
	"sync"

	"assistente/controllers"
	"assistente/internal/llm"
)

// Chat é o bind Wails do domínio chat/envio (AEP-0040, AEP-0088):
// SendMessage e RetryMessage. Auth só via WithUser.
//
// sendMessageFromChannel, ChatController interno, streamMgr, eventos e gateway
// permanecem no *App.
type Chat struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.ChatController
	command ChatCommandHook
}

// ChatCommandHook commits an admitted operation through the original controller.
// It is wired internally, never supplied by the caller. next captures stripped params.
type ChatCommandHook func(context.Context, *llm.ChatCommandMetadata, string, string, func(context.Context) (string, error)) (string, error)

func AttachChatCommandHook(api *Chat, hook ChatCommandHook) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.command = hook
}

func stripChatCommand(params llm.ChatParams) (*llm.ChatCommandMetadata, llm.ChatParams) {
	metadata := params.Command
	params.Command = nil
	return metadata, params
}

func (api *Chat) submit(ctx context.Context, metadata *llm.ChatCommandMetadata, conversationID, retryID string, next func(context.Context) (string, error)) (string, error) {
	// Existing deep-link/non-migrated callers retain the authenticated pipeline.
	// Present but invalid metadata MUST NOT fall back to this legacy ingress.
	if metadata == nil {
		return next(ctx)
	}
	api.mu.RLock()
	hook := api.command
	api.mu.RUnlock()
	if hook != nil {
		return hook(ctx, metadata, conversationID, retryID, next)
	}
	return "", errors.New("chat command hook unavailable")
}

// NewChat cria o bind vazio; AttachChat preenche deps no startup.
func NewChat() *Chat {
	return &Chat{}
}

// AttachChat associa Session e ChatController após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachChat(api *Chat, session Session, ctrl *controllers.ChatController) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.ctrl = ctrl
}

func (api *Chat) deps() (Session, *controllers.ChatController, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.ctrl == nil {
		return nil, nil, ErrChatNotWired
	}
	return api.session, api.ctrl, nil
}

// SendMessage envia uma mensagem do usuário. Source padrão no controller: "wails".
func (api *Chat) SendMessage(conversationID, userContent, userMedia string, params llm.ChatParams) (string, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		metadata, params := stripChatCommand(params)
		return api.submit(ctx, metadata, conversationID, "", func(turnCtx context.Context) (string, error) {
			return ctrl.SendMessage(turnCtx, conversationID, userContent, userMedia, params)
		})
	})
}

// RetryMessage reexecuta a resposta a partir de uma mensagem do usuário já persistida.
func (api *Chat) RetryMessage(conversationID, messageID string, params llm.ChatParams) (string, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		if messageID == "" {
			return "", errors.New("retry message ID required")
		}
		metadata, params := stripChatCommand(params)
		return api.submit(ctx, metadata, conversationID, messageID, func(turnCtx context.Context) (string, error) {
			return ctrl.RetryMessage(turnCtx, conversationID, messageID, params)
		})
	})
}
