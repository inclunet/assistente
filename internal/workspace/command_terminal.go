package workspace

import "context"

// WithTerminalCommandTabSnapshot pins the active terminal tab and its
// optional session binding together. The callback must not reenter the
// workspace manager. An unbound tab returns an empty sessionID.
func (m *Manager) WithTerminalCommandTabSnapshot(ctx context.Context, fn func(CommandSnapshot, string) error) error {
	if m == nil {
		return ErrCommandSnapshotNilManager
	}
	if ctx == nil {
		return ErrCommandSnapshotNilContext
	}
	if fn == nil {
		return ErrCommandSnapshotNilCallback
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := m.commandSnapshotLocked()
	if err != nil {
		return err
	}
	if snapshot.Tab.Type != TabTypeTerminal {
		return ErrCommandSnapshotInvalidData
	}
	for _, tab := range m.active.Tabs.Items {
		if tab.ID == snapshot.ActiveTabID {
			id := ""
			if raw, ok := tab.State["sessionId"]; ok {
				var valid bool
				id, valid = raw.(string)
				if !valid || !canonicalSnapshotIdentifier(id) {
					return ErrCommandSnapshotInvalidData
				}
			}
			return fn(snapshot, id)
		}
	}
	return ErrCommandSnapshotActiveTabUnavailable
}

// WithTerminalCommandSnapshot pins the active tab and its terminal binding
// together. The callback must not reenter the workspace manager.
func (m *Manager) WithTerminalCommandSnapshot(ctx context.Context, fn func(CommandSnapshot, string) error) error {
	return m.WithTerminalCommandTabSnapshot(ctx, func(snapshot CommandSnapshot, id string) error {
		if id == "" {
			return ErrCommandSnapshotInvalidData
		}
		return fn(snapshot, id)
	})
}
