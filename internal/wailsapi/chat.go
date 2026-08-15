package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/llm"
	"context"
	"sync"
)

// Chat é o bind Wails do domínio chat/envio (AEP-0088): SendMessage e
// RetryMessage. Auth só via WithUser.
//
// SendMessageSync, sendMessageFromChannel, ChatController, streamMgr, eventos e
// gateway permanecem no *App — fora do escopo desta migração (AEP-0040).
type Chat struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.ChatController
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
		return ctrl.SendMessage(ctx, conversationID, userContent, userMedia, params)
	})
}

// RetryMessage reexecuta a resposta a partir de uma mensagem do usuário já persistida.
func (api *Chat) RetryMessage(conversationID, messageID string, params llm.ChatParams) (string, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return ctrl.RetryMessage(ctx, conversationID, messageID, params)
	})
}
