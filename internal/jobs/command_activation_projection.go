package jobs

import (
	"context"

	"assistente/internal/commandjobactivation"
)

// ValidateCommandRuntimeProjection confirma a continuidade da projeção viva
// previamente admitida para runID. Ela é deliberadamente apenas memória: não
// autentica uma identidade nova, não consulta o ledger e não substitui a
// validação persistida de CommandRuntimeIdentity.
//
// O chamador deve manter o gate do executor enquanto usa esta confirmação.
func (m *Manager) ValidateCommandRuntimeProjection(ctx context.Context, runID string, expected commandjobactivation.RuntimeIdentity) error {
	if ctx == nil {
		return ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || runID == "" {
		return ErrCommandMaintenanceUnavailable
	}

	m.commandRuntimeMu.Lock()
	defer m.commandRuntimeMu.Unlock()
	if !m.commandRuntimeAccepting {
		return ErrCommandMaintenanceUnavailable
	}
	entry, ok := m.commandRuntime[runID]
	if !ok || entry.identity != expected || entry.watchCtx == nil || entry.watchCtx.Err() != nil {
		return ErrCommandMaintenanceUnavailable
	}
	return nil
}
