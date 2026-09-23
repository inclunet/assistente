package workspace

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrCommandEditorModeNilContext      = errors.New("workspace command editor mode: context nil")
	ErrCommandEditorModeStale           = errors.New("workspace command editor mode: snapshot stale")
	ErrCommandEditorModeInvalid         = errors.New("workspace command editor mode: invalid mode")
	ErrCommandEditorModeUnsupportedType = errors.New("workspace command editor mode: active tab is not an editor")
)

const (
	EditorModeMarkdown = "markdown"
	EditorModeRich     = "rich"
	EditorModeView     = "view"
)

// SetEditorModeForCommand altera somente o modo de apresentação da aba editor
// autorizada pelo snapshot. O snapshot é comparado sob o mesmo lock exclusivo
// da cópia e da persistência, portanto uma versão usada uma vez não pode ser
// reaplicada, inclusive quando o modo solicitado é o mesmo.
func (m *Manager) SetEditorModeForCommand(ctx context.Context, expected CommandSnapshot, mode string) (*Workspace, error) {
	if m == nil {
		return nil, fmt.Errorf("no workspace manager")
	}
	if ctx == nil {
		return nil, ErrCommandEditorModeNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validEditorMode(mode) {
		return nil, ErrCommandEditorModeInvalid
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
		return nil, ErrCommandEditorModeStale
	}
	if expected.Tab.Type != TabTypeEditor || current.Tab.Type != TabTypeEditor {
		return nil, ErrCommandEditorModeUnsupportedType
	}

	previousActive := m.active
	previousEpoch := m.commandEpoch
	updated := cloneWorkspace(previousActive)
	tab := updated.FindTab(expected.ActiveTabID)
	if tab == nil {
		return nil, ErrCommandEditorModeStale
	}
	if tab.Type != TabTypeEditor {
		return nil, ErrCommandEditorModeUnsupportedType
	}
	if tab.State == nil {
		tab.State = make(map[string]any)
	}
	tab.State["displayMode"] = mode
	m.active = updated
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

func validEditorMode(mode string) bool {
	switch mode {
	case EditorModeMarkdown, EditorModeRich, EditorModeView:
		return true
	default:
		return false
	}
}
