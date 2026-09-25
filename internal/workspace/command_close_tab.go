package workspace

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrCommandCloseTabNilContext          = errors.New("workspace command close tab: context nil")
	ErrCommandCloseTabInvalidReplacementID = errors.New("workspace command close tab: replacement tab id invalid")
)

// removeTabLocked remove uma aba e mantém a política legada de escolha da
// sucessora. Não salva nem avança o epoch; o chamador deve concluir a
// mutação, ou fazer rollback, sob o mesmo lock.
func (m *Manager) removeTabLocked(tabID string) error {
	idx := -1
	for i, tab := range m.active.Tabs.Items {
		if tab.ID == tabID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("tab not found: %s", tabID)
	}

	m.active.Tabs.Items = append(m.active.Tabs.Items[:idx], m.active.Tabs.Items[idx+1:]...)
	if m.active.Tabs.Active == tabID {
		if len(m.active.Tabs.Items) > 0 {
			nextIdx := idx
			if nextIdx >= len(m.active.Tabs.Items) {
				nextIdx = len(m.active.Tabs.Items) - 1
			}
			m.active.Tabs.Active = m.active.Tabs.Items[nextIdx].ID
		} else {
			m.active.Tabs.Active = ""
		}
	}
	for i := range m.active.Tabs.Items {
		m.active.Tabs.Items[i].Position = i
	}
	return nil
}

// CloseActiveTabForCommand fecha exclusivamente a aba ativa autorizada pelo
// snapshot. Quando ela é a última, instala uma aba de chat vazia usando o ID
// de aba UUIDv7 já criado pelo App, na mesma transação de persistência. A
// operação não cria nem atribui uma conversa.
func (m *Manager) CloseActiveTabForCommand(
	ctx context.Context,
	expected CommandSnapshot,
	replacementChatID string,
) (*Workspace, error) {
	if m == nil {
		return nil, fmt.Errorf("no workspace manager")
	}
	if ctx == nil {
		return nil, ErrCommandCloseTabNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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
	if current.WorkspaceID != expected.WorkspaceID || current.Version != expected.Version || current.ActiveTabID != expected.ActiveTabID {
		return nil, ErrCommandCreateTabStale
	}
	if m.active.FindTab(expected.ActiveTabID) == nil {
		return nil, fmt.Errorf("tab not found: %s", expected.ActiveTabID)
	}

	previousActive := m.active
	previousEpoch := m.commandEpoch
	m.active = cloneWorkspace(previousActive)
	if err := m.removeTabLocked(expected.ActiveTabID); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}
	if len(m.active.Tabs.Items) == 0 {
		if !isValidUUIDv7(replacementChatID) || replacementChatID == expected.ActiveTabID {
			m.active = previousActive
			m.commandEpoch = previousEpoch
			return nil, ErrCommandCloseTabInvalidReplacementID
		}
		replacement := Tab{
			ID:       replacementChatID,
			Type:     TabTypeChat,
			Position: 0,
		}
		m.active.Tabs.Items = append(m.active.Tabs.Items, replacement)
		m.active.Tabs.Active = replacement.ID
	}
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
