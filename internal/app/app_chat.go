package app

import (
	"context"
	"fmt"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/profiles"
	"assistente/internal/prompt"
)

// sendMessageFromChannel é chamado pelo Gateway de mensageria com um ctx que
// carrega o OwnerUserID do canal (AEP-0052). NÃO usa requireAuthenticatedContext:
// mensagens de canal precisam funcionar mesmo com a UI fechada/sem login —
// se substituíssemos o ctx pelo derivado de currentUserID, qualquer usuário
// que não estivesse logado na UI veria suas mensagens entrantes descartadas.
//
// O fail-closed aqui é diferente do path Wails: confiamos que o gateway
// carimbou o ctx com o owner do canal antes de chamar; se o ctx chegar sem
// userID é bug do gateway (config sem OwnerUserID, p.ex.). Validamos com
// RequireUserID e devolvemos ErrUserScopeRequired explícito para o caller
// poder logar/notificar.
//
// É deliberadamente não exportado — não é binding Wails, só callback do
// SendMessageFunc do gateway. SendMessage/RetryMessage da UI vivem em
// wailsapi.Chat (AEP-0088).
func (a *App) sendMessageFromChannel(ctx context.Context, conversationID string, content, media string, params ChatParams, source string) (string, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return "", err
	}
	if a.chatCtrl == nil {
		return "", fmt.Errorf("chat controller não inicializado")
	}
	return a.chatCtrl.SendMessageFromChannel(ctx, conversationID, content, media, params, source)
}

// DefaultSystemPrompt é re-exportado de internal/chat para compatibilidade.
var DefaultSystemPrompt = chat.DefaultSystemPrompt

// effectivePromptBuilder retorna a.promptBuilder se inicializado, ou constrói um Builder avulso.
// Protege contra o trap de interface nil em Go (nil *Manager ≠ nil interface).
// Em produção, a.promptBuilder é sempre não-nil após startup(). Usado por testes que criam &App{}.
func (a *App) effectivePromptBuilder() *prompt.Builder {
	if a.promptBuilder != nil {
		return a.promptBuilder
	}
	b := &prompt.Builder{Tools: a.toolRegistry}
	if a.skillMgr != nil {
		b.Skills = a.skillMgr
	}
	if a.workspaceMgr != nil {
		b.Workspace = a.workspaceMgr
	}
	return b
}

// loadConversationHistory carrega o histórico de mensagens de uma conversa.
// Respeita rolling context: se há resumo, exclui mensagens já resumidas do contexto.
func (a *App) loadConversationHistory(conversationID string, profile *profiles.Profile) ([]Message, string, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, "", err
	}
	maxCtxMsgs := chat.DefaultMaxContextMessages
	if profile != nil {
		maxCtxMsgs = profile.GetMaxContextMessages()
	}
	loader := chat.MediaHistoryLoader{
		Repo:       a.msgRepo,
		Transcribe: a.whisperTranscribeFunc(),
		MaxMsgs:    maxCtxMsgs,
	}
	return loader.Load(ctx, conversationID)
}

// whisperTranscribeFunc cria o callback de transcrição para o MediaHistoryLoader e PreprocessMessages.
func (a *App) whisperTranscribeFunc() chat.TranscribeFunc {
	return func(ctx context.Context, audioBase64, filename string) (string, error) {
		result, err := a.speechSvc.Transcribe(ctx, audioBase64, filename)
		if err != nil {
			return "", err
		}
		if result == nil {
			return "", nil
		}
		return result.Text, nil
	}
}
