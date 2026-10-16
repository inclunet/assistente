package app

import (
	"context"
	"errors"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/workspace"
	"gorm.io/gorm"
)

const (
	commandChatSendID   = "chat.message.send"
	commandChatRetryID  = "chat.message.retry"
	commandChatCancelID = "chat.response.cancel"
)

// Código público sem IDs ou detalhes do banco; ausência e falta de acesso são indistinguíveis.
var errChatConversationUnavailable = errors.New("chat_conversation_unavailable")

func isChatActionCommand(id string) bool {
	return id == commandChatSendID || id == commandChatRetryID || id == commandChatCancelID
}

func commandChatActionRegistrations() []commandcatalog.Registration {
	var registrations []commandcatalog.Registration
	for _, item := range []struct{ id, route, pt, en, es string }{
		{commandChatSendID, "send", "Enviar mensagem", "Send message", "Enviar mensaje"},
		{commandChatRetryID, "retry", "Tentar mensagem novamente", "Retry message", "Reintentar mensaje"},
		{commandChatCancelID, "cancel", "Cancelar resposta", "Cancel response", "Cancelar respuesta"},
	} {
		d, h := commandWorkspaceChatOpenRegistration()
		d.ID = item.id
		d.HandlerRoute = "contextual/chat/" + item.route
		h.Route = d.HandlerRoute
		d.Persistence.Result = commandcatalog.PersistenceNever
		d.Presentation = &commandcatalog.Presentation{Version: "chat-actions-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: item.pt, Description: item.pt + " na conversa da aba ativa", Category: "Chat"},
			"en":    {Name: item.en, Description: item.en + " in the active tab's conversation", Category: "Chat"},
			"es":    {Name: item.es, Description: item.es + " en la conversación de la pestaña activa", Category: "Chat"},
		}}
		registrations = append(registrations, commandcatalog.Registration{Definition: d, Handler: h})
	}
	return registrations
}

func (a *App) captureChatAction(ctx context.Context, p *commandProductRuntime, id string) (workspace.CommandSnapshot, uint64, error) {
	var snapshot workspace.CommandSnapshot
	var generation uint64
	if a.chatCtrl == nil || a.streamMgr == nil || (id == commandChatRetryID && a.chatInteractor == nil) {
		return snapshot, 0, commandexecution.ErrDenied
	}
	err := p.workspaceMgr.WithCommandSnapshot(ctx, func(current workspace.CommandSnapshot) error {
		switch current.Tab.Type {
		case workspace.TabTypeChat, workspace.TabTypeEditor, workspace.TabTypeTerminal, workspace.TabTypeTasklist:
		default:
			return commandexecution.ErrDenied
		}
		if current.WorkspaceID != p.workspaceID || current.Tab.ConversationID == "" {
			return commandexecution.ErrDenied
		}
		if _, err := database.GetConversationInfoWithContext(database.WithUserID(ctx, p.principal.UserID), current.Tab.ConversationID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errChatConversationUnavailable
			}
			return err
		}
		snapshot = current
		return nil
	})
	if err == nil && id == commandChatCancelID {
		generation, _ = a.streamMgr.CurrentGeneration(snapshot.Tab.ConversationID)
	}
	return snapshot, generation, err
}

// The payload is held only on this synchronous call stack. The original bind
// supplies next with stripped params; neither content nor params enter a run/ledger.
func (a *App) commitChatSubmission(turnCtx context.Context, metadata *llm.ChatCommandMetadata, conversationID, retryID string, next func(context.Context) (string, error)) (string, error) {
	if turnCtx == nil || turnCtx.Err() != nil || metadata == nil || next == nil {
		return "", commandexecution.ErrDenied
	}
	p, run, err := a.commandUIRun(metadata.Ticket)
	if err != nil {
		return "", err
	}
	id := commandChatSendID
	if retryID != "" {
		id = commandChatRetryID
	}
	owner, err := database.RequireUserID(turnCtx)
	if err != nil || owner != p.principal.UserID || run.reservation.CommandID != id {
		return "", commandexecution.ErrDenied
	}
	ctx, cancel := context.WithTimeout(turnCtx, 35*time.Second)
	defer cancel()
	var result string
	err = p.ui.Commit(ctx, p.uiOwner(), metadata.Ticket, metadata.HandoffID, func(commitCtx context.Context) error {
		return a.admitChatAction(commitCtx, p, run, func(expected workspace.CommandSnapshot) error {
			if conversationID != expected.Tab.ConversationID {
				return commandexecution.ErrDenied
			}
			if retryID != "" {
				if a.chatInteractor == nil {
					return commandexecution.ErrDenied
				}
				if _, err := a.chatInteractor.GetRetryableUserMessage(turnCtx, conversationID, retryID); err != nil {
					return commandexecution.ErrDenied
				}
			}
			if commitCtx.Err() != nil || turnCtx.Err() != nil {
				return commandexecution.ErrStale
			}
			// Acceptance is the command effect. The existing pipeline owns the turn
			// thereafter, using the Wails session lifetime, never the broker deadline.
			var submitErr error
			result, submitErr = next(turnCtx)
			if submitErr != nil {
				return commandui.ErrOutcomeUnknown
			} // The pipeline may already have persisted. Never persist its raw error.
			return nil
		})
	})
	return result, err
}

func (a *App) admitChatAction(ctx context.Context, p *commandProductRuntime, run *commandUIRun, apply func(workspace.CommandSnapshot) error) error {
	current, err := a.authenticatedCommandProduct()
	if err != nil || current != p {
		return commandexecution.ErrDenied
	}
	p.mu.Lock()
	admission, expected := run.admission, run.snapshot
	p.mu.Unlock()
	if admission == nil || expected.Tab.ConversationID == "" {
		return commandexecution.ErrDenied
	}
	var admittedCtx context.Context
	release, err := p.epochs.AdmitExecution(ctx, admission.epoch, func(ctx context.Context) error {
		if run.sourceValid != nil && !run.sourceValid() {
			return commandexecution.ErrStale
		}
		versions, err := p.host.Snapshot(ctx, p.principal)
		if err != nil || versions != admission.versions || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
			return commandexecution.ErrStale
		}
		return nil
	}, func(runCtx context.Context) error { admittedCtx = runCtx; return nil })
	if err != nil {
		return err
	}
	defer release()
	a.authMu.RLock()
	valid := a.chatCtrl == run.chatController && a.streamMgr == run.chatStreamMgr && a.workspaceMgr == p.workspaceMgr && a.currentAuthUser != nil && a.currentAuthUser.UserID == p.principal.UserID && a.currentAuthUser.SessionID == p.principal.SessionID
	a.authMu.RUnlock()
	if !valid {
		return commandexecution.ErrDenied
	}
	if err := p.workspaceMgr.WithCommandSnapshot(admittedCtx, func(current workspace.CommandSnapshot) error {
		if current != expected {
			return commandexecution.ErrStale
		}
		_, err := database.GetConversationInfoWithContext(database.WithUserID(admittedCtx, p.principal.UserID), expected.Tab.ConversationID)
		return err
	}); err != nil {
		return err
	}
	// Never hold a workspace read lock through SendMessage: PrepareContext reads
	// that manager too. The submission is pinned to expected, not recaptured.
	if admittedCtx.Err() != nil {
		return commandexecution.ErrStale
	}
	return apply(expected)
}

func (a *App) commitChatCancel(ticket, handoffID string) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if run.reservation.CommandID != commandChatCancelID {
		return commandexecution.ErrDenied
	}
	ctx, cancel := context.WithTimeout(a.commandBridgeContext(), 35*time.Second)
	defer cancel()
	return p.ui.Commit(ctx, p.uiOwner(), ticket, handoffID, func(ctx context.Context) error {
		return a.admitChatAction(ctx, p, run, func(expected workspace.CommandSnapshot) error {
			if a.streamMgr == nil || a.streamMgr != run.chatStreamMgr || !a.streamMgr.CancelIfCurrent(expected.Tab.ConversationID, run.chatGeneration) {
				return commandexecution.ErrStale
			}
			return nil
		})
	})
}
