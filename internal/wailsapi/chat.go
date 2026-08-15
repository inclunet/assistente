package wailsapi

import (
	"context"
	"sync"

	"assistente/controllers"
	"assistente/internal/llm"
)

// Chat é o bind Wails do domínio chat/envio (AEP-0088): SendMessage,
// RetryMessage e SendMessageSync (probe de acessibilidade). Auth só via WithUser.
//
// SendMessageSync NÃO faz parte do pipeline conversacional (AEP-0040): sem
// streaming, eventos chat:* ou persistência. sendMessageFromChannel,
// ChatController interno, streamMgr, eventos e gateway permanecem no *App.
type Chat struct {
	mu       sync.RWMutex
	session  Session
	ctrl     *controllers.ChatController
	syncCtrl SyncChatSender
}

// SyncChatSender executa um probe LLM síncrono (sem streaming, sem eventos,
// sem persistência). Contrato mínimo para o método de acessibilidade
// SendMessageSync — separado do pipeline conversacional (AEP-0040).
type SyncChatSender interface {
	SendMessageSync(ctx context.Context, messages []llm.Message, params llm.ChatParams) (string, error)
}

// NewChat cria o bind vazio; AttachChat preenche deps no startup.
func NewChat() *Chat {
	return &Chat{}
}

// AttachChat associa Session, ChatController e SyncChatSender após o startup
// montar as deps. Função de pacote (não método) para não entrar no Bind do Wails.
// syncCtrl tipicamente é *controllers.SettingsController (lógica do probe).
func AttachChat(api *Chat, session Session, ctrl *controllers.ChatController, syncCtrl SyncChatSender) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.ctrl = ctrl
	api.syncCtrl = syncCtrl
}

func (api *Chat) deps() (Session, *controllers.ChatController, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.ctrl == nil {
		return nil, nil, ErrChatNotWired
	}
	return api.session, api.ctrl, nil
}

func (api *Chat) syncDeps() (Session, SyncChatSender, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.syncCtrl == nil {
		return nil, nil, ErrChatNotWired
	}
	return api.session, api.syncCtrl, nil
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

// SendMessageSync faz um probe LLM síncrono (acessibilidade/testes). NÃO é o
// caminho de mensagens do chat (AEP-0040): sem streaming, eventos ou
// persistência. Use SendMessage para o fluxo conversacional.
func (api *Chat) SendMessageSync(messages []llm.Message, params llm.ChatParams) (string, error) {
	session, syncCtrl, err := api.syncDeps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return syncCtrl.SendMessageSync(ctx, messages, params)
	})
}
