package jobs

import (
	"context"
)

// CloseCommandMaintenance encerra somente a passagem de manutenção de
// comandos. O scheduler, as ferramentas e o cancelamento de jobs continuam
// pertencendo ao ciclo de vida geral do Manager.
func (m *Manager) CloseCommandMaintenance(ctx context.Context) error {
	if m == nil || ctx == nil {
		return ErrCommandMaintenanceUnavailable
	}

	m.mu.Lock()
	m.commandMaintenanceClosed = true
	retentionCancel := m.retentionCancel
	retentionDone := m.retentionDone
	m.mu.Unlock()

	if retentionCancel != nil {
		retentionCancel()
	}
	if retentionDone == nil {
		return nil
	}
	select {
	case <-retentionDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
