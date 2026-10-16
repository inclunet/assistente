package app

import (
	"context"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/workspace"
)

const commandConversationClearID = "chat.conversation.clear"

func commandConversationClearRegistration() (commandcatalog.Definition, commandcatalog.HandlerContract) {
	d, h := commandWorkspaceChatOpenRegistration()
	d.ID = commandConversationClearID
	d.Effect, h.Effect = commandcatalog.Destructive, commandcatalog.Destructive
	d.Decision = commandcatalog.Interactive
	d.Risk = commandcatalog.RiskHigh
	d.HandlerRoute = "contextual/chat/conversation/clear"
	h.Route = d.HandlerRoute
	d.Persistence.Result = commandcatalog.PersistenceNever
	d.Presentation = &commandcatalog.Presentation{Version: "chat-conversation-clear-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Limpar conversa", Description: "Apaga o conteúdo da conversa vinculada à aba ativa após confirmação", Category: "Chat"},
		"en":    {Name: "Clear conversation", Description: "Deletes the content of the conversation linked to the active tab after confirmation", Category: "Chat"},
		"es":    {Name: "Limpiar conversación", Description: "Elimina el contenido de la conversación vinculada a la pestaña activa tras la confirmación", Category: "Chat"},
	}}
	return d, h
}

// The fingerprint is runtime-only. Neither content nor fingerprint is an argument.
func (a *App) captureConversationClear(ctx context.Context, p *commandProductRuntime) (workspace.CommandSnapshot, string, error) {
	var expected workspace.CommandSnapshot
	var fingerprint string
	err := a.withConversationClearTarget(ctx, p, func(snapshot workspace.CommandSnapshot) error {
		var err error
		fingerprint, err = database.ConversationContentSnapshotWithContext(database.WithUserID(ctx, p.principal.UserID), snapshot.Tab.ConversationID)
		expected = snapshot
		return err
	})
	return expected, fingerprint, err
}

// Readiness checks only the owned binding, never scans conversation history.
func (a *App) conversationClearReady(ctx context.Context, p *commandProductRuntime) error {
	return a.withConversationClearTarget(ctx, p, func(snapshot workspace.CommandSnapshot) error {
		_, err := database.GetConversationInfoWithContext(database.WithUserID(ctx, p.principal.UserID), snapshot.Tab.ConversationID)
		return err
	})
}

func (a *App) withConversationClearTarget(ctx context.Context, p *commandProductRuntime, inspect func(workspace.CommandSnapshot) error) error {
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.conversationsCtrl == nil {
		return commandexecution.ErrDenied
	}
	return p.workspaceMgr.WithCommandSnapshot(ctx, func(snapshot workspace.CommandSnapshot) error {
		switch snapshot.Tab.Type {
		case workspace.TabTypeChat, workspace.TabTypeEditor, workspace.TabTypeTerminal, workspace.TabTypeTasklist:
		default:
			return commandexecution.ErrDenied
		}
		if snapshot.WorkspaceID != p.workspaceID || snapshot.Tab.ConversationID == "" {
			return commandexecution.ErrDenied
		}
		return inspect(snapshot)
	})
}

func (a *App) commitConversationClear(ctx context.Context, p *commandProductRuntime, expected workspace.CommandSnapshot, fingerprint string) error {
	if fingerprint == "" || expected.Tab.ConversationID == "" {
		return commandexecution.ErrDenied
	}
	a.authMu.RLock()
	controller := a.conversationsCtrl
	a.authMu.RUnlock()
	if controller == nil {
		return commandexecution.ErrDenied
	}
	return controller.ClearConversationIfUnchangedGuarded(database.WithUserID(ctx, p.principal.UserID), expected.Tab.ConversationID, fingerprint, func(commit func() error) error {
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if a.conversationsCtrl != controller || a.workspaceMgr != p.workspaceMgr || a.commandProduct.Load() != p ||
			a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID ||
			a.currentAuthUser.SessionID != p.principal.SessionID {
			return commandexecution.ErrDenied
		}
		return p.workspaceMgr.WithCommandSnapshot(ctx, func(current workspace.CommandSnapshot) error {
			if current != expected {
				return commandexecution.ErrStale
			}
			return commit()
		})
	})
}
