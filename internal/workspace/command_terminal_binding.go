package workspace

import (
	"context"
	"fmt"
)

// ErrCommandTerminalBindingStale indica que a aba terminal, seu workspace ou
// seu vínculo mudaram desde a captura do comando.
var ErrCommandTerminalBindingStale = fmt.Errorf("workspace command terminal binding: snapshot stale")

// BindTerminalSessionForCommand associa uma sessão recém-criada à aba
// terminal ativa capturada. A operação não cria, remove ou troca abas e
// preserva todos os campos de State além de sessionId.
func (m *Manager) BindTerminalSessionForCommand(
	ctx context.Context,
	expected CommandSnapshot,
	sessionID string,
	sessions TerminalSessionValidator,
) (*Workspace, error) {
	if m == nil {
		return nil, fmt.Errorf("no workspace manager")
	}
	if ctx == nil {
		return nil, ErrCommandCreateTabNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if expected.Tab.Type != TabTypeTerminal || !canonicalSnapshotIdentifier(sessionID) || sessions == nil || !sessions.Has(sessionID) {
		return nil, ErrCommandTerminalBindingStale
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
	if current != expected || current.Tab.Type != TabTypeTerminal {
		return nil, ErrCommandTerminalBindingStale
	}
	tab := m.active.FindTab(current.ActiveTabID)
	if tab == nil {
		return nil, ErrCommandTerminalBindingStale
	}
	if !sessions.Has(sessionID) {
		return nil, ErrCommandTerminalBindingStale
	}

	previousActive := m.active
	previousEpoch := m.commandEpoch
	m.active = cloneWorkspace(previousActive)
	bound := m.active.FindTab(current.ActiveTabID)
	if bound == nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, ErrCommandTerminalBindingStale
	}
	if bound.State == nil {
		bound.State = make(map[string]any)
	}
	bound.State["sessionId"] = sessionID
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

// UnbindTerminalSessionForCommand remove somente sessionId da aba terminal
// capturada. É usado depois da admissão de um fechamento e não altera foco,
// título ou qualquer outro dado da aba.
func (m *Manager) UnbindTerminalSessionForCommand(
	ctx context.Context,
	expected CommandSnapshot,
	sessionID string,
) (*Workspace, error) {
	if m == nil {
		return nil, fmt.Errorf("no workspace manager")
	}
	if ctx == nil {
		return nil, ErrCommandCreateTabNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if expected.Tab.Type != TabTypeTerminal || !canonicalSnapshotIdentifier(sessionID) {
		return nil, ErrCommandTerminalBindingStale
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
	if current != expected || current.Tab.Type != TabTypeTerminal {
		return nil, ErrCommandTerminalBindingStale
	}
	tab := m.active.FindTab(current.ActiveTabID)
	if tab == nil || tab.State == nil || tab.State["sessionId"] != sessionID {
		return nil, ErrCommandTerminalBindingStale
	}

	previousActive := m.active
	previousEpoch := m.commandEpoch
	m.active = cloneWorkspace(previousActive)
	unbound := m.active.FindTab(current.ActiveTabID)
	if unbound == nil || unbound.State == nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, ErrCommandTerminalBindingStale
	}
	delete(unbound.State, "sessionId")
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
