package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

// commitWorkspaceContextChat runs inside the authenticated command commit.
// Prepared editor/terminal/task context stays in the UI; no selection or text
// is accepted as authority here. Only the captured workspace snapshot is used.
func (a *App) commitWorkspaceContextChat(ctx context.Context, p *commandProductRuntime, expected workspace.CommandSnapshot) (*workspace.Workspace, error) {
	if ctx == nil || p == nil || p.workspaceMgr == nil {
		return nil, commandexecution.ErrDenied
	}
	var updated *workspace.Workspace
	err := database.WithConversationLifecycle(ctx, func() error {
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if a.workspaceMgr != p.workspaceMgr || a.commandProduct.Load() != p || a.currentAuthUser == nil ||
			a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
			return commandexecution.ErrStale
		}
		current, err := p.workspaceMgr.CommandSnapshot()
		if err != nil || current.WorkspaceID != expected.WorkspaceID || current.Version != expected.Version || current.ActiveTabID != expected.ActiveTabID {
			return commandexecution.ErrStale
		}
		updated, err = commitWorkspaceContextChatWithinLifecycle(ctx, p, expected)
		return err
	})
	return updated, err
}

func commitWorkspaceContextChatWithinLifecycle(ctx context.Context, p *commandProductRuntime, expected workspace.CommandSnapshot) (*workspace.Workspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch expected.Tab.Type {
	case workspace.TabTypeChat:
		// A chat panel already owns its conversation. This command only lets
		// the UI focus its input; it must never create another conversation.
		return nil, nil
	case workspace.TabTypeEditor, workspace.TabTypeTerminal, workspace.TabTypeTasklist:
	default:
		return nil, commandexecution.ErrDenied
	}
	db := database.DB()
	if db == nil {
		return nil, commandexecution.ErrDenied
	}
	repo := database.NewConversationRepository(db)
	userCtx := database.WithUserID(ctx, p.principal.UserID)
	if existing := expected.Tab.ConversationID; existing != "" {
		if _, err := uuid.Parse(existing); err != nil {
			return nil, fmt.Errorf("context chat: invalid existing conversation: %w", err)
		}
		if _, err := repo.GetConversationInfoWithContext(userCtx, existing); err != nil {
			return nil, err
		}
		// Revalidate after the database read without changing the binding.
		err := p.workspaceMgr.WithCommandSnapshot(ctx, func(current workspace.CommandSnapshot) error {
			if current.Version != expected.Version || current.WorkspaceID != expected.WorkspaceID || current.ActiveTabID != expected.ActiveTabID {
				return commandexecution.ErrStale
			}
			return nil
		})
		return nil, err
	}
	conversation, err := repo.CreateConversationWithContext(userCtx, "Chat", "")
	if err != nil {
		return nil, err
	}
	updated, err := p.workspaceMgr.BindConversationForCommand(ctx, expected, conversation.ID)
	if err == nil {
		return updated, nil
	}
	// Only this freshly created, unpublished conversation is compensated.
	// Keep the same repository and owner even if the command was cancelled.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(userCtx), 5*time.Second)
	defer cancel()
	if _, cleanupErr := repo.DeleteConversationsWithinLifecycleWithContext(cleanupCtx, []string{conversation.ID}); cleanupErr != nil {
		return nil, errors.Join(err, fmt.Errorf("context chat: compensate new conversation: %w", cleanupErr))
	}
	return nil, err
}
