package workspace

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrCommandBindConversationNilContext      = errors.New("workspace command bind conversation: context nil")
	ErrCommandBindConversationStale           = errors.New("workspace command bind conversation: snapshot stale")
	ErrCommandBindConversationInvalidID       = errors.New("workspace command bind conversation: conversation id invalid")
	ErrCommandBindConversationUnsupportedType = errors.New("workspace command bind conversation: unsupported tab type")
	ErrCommandBindConversationConflict        = errors.New("workspace command bind conversation: tab already has another conversation")
)

// BindConversationForCommand vincula uma conversa já existente a uma aba de
// superfície contextual. O workspace apenas persiste o UUID recebido; a
// existência/autorização da conversa pertence ao chamador de domínio superior.
func (m *Manager) BindConversationForCommand(
	ctx context.Context,
	expected CommandSnapshot,
	conversationID string,
) (*Workspace, error) {
	if m == nil {
		return nil, fmt.Errorf("no workspace manager")
	}
	if ctx == nil {
		return nil, ErrCommandBindConversationNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !isValidUUIDv7(conversationID) {
		return nil, ErrCommandBindConversationInvalidID
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.active == nil {
		return nil, fmt.Errorf("no active workspace")
	}
	current, err := m.commandSnapshotLocked()
	if err != nil {
		return nil, err
	}
	if current.WorkspaceID != expected.WorkspaceID ||
		current.Version != expected.Version ||
		current.ActiveTabID != expected.ActiveTabID {
		return nil, ErrCommandBindConversationStale
	}

	tab := m.active.FindTab(expected.ActiveTabID)
	if tab == nil {
		return nil, fmt.Errorf("tab not found: %s", expected.ActiveTabID)
	}
	if tab.Type == TabTypeChat {
		// Aba de chat já vinculada é somente no-op; não se troca a conversa
		// implicitamente por uma ação contextual de outra superfície.
		if tab.ConversationID != conversationID {
			if tab.ConversationID != "" {
				return nil, ErrCommandBindConversationConflict
			}
			return nil, ErrCommandBindConversationUnsupportedType
		}
	} else if tab.Type != TabTypeEditor && tab.Type != TabTypeTerminal && tab.Type != TabTypeTasklist {
		return nil, ErrCommandBindConversationUnsupportedType
	}
	if tab.ConversationID == conversationID {
		return m.cloneActiveSnapshotLocked(m.active), nil
	}
	if tab.ConversationID != "" {
		return nil, ErrCommandBindConversationConflict
	}

	previousActive := m.active
	previousEpoch := m.commandEpoch
	m.active = cloneWorkspace(previousActive)
	boundTab := m.active.FindTab(expected.ActiveTabID)
	if boundTab == nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, fmt.Errorf("tab not found: %s", expected.ActiveTabID)
	}
	boundTab.ConversationID = conversationID
	m.commandMutationEpochLocked()
	if err := ctx.Err(); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}
	if err := m.saveWorkspaceForCommand(ctx, m.active, m.activePath); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}
	return m.cloneActiveSnapshotLocked(m.active), nil
}
