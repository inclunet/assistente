package jobs

import (
	"assistente/internal/commandmaintenance"
)

// ConfigureCommandMaintenance resolve a dependência de montagem: os adapters
// legados precisam do Manager, mas o coordinator deve existir antes de Start.
// O host fornece as portas de comandos; jobs/tools/compactação são sempre os
// adapters reais deste Manager. Não é uma API de usuário nem habilita comandos.
// Montagem é única e somente enquanto frio; erro não altera o caminho legado.
func (m *Manager) ConfigureCommandMaintenance(ports commandmaintenance.Ports) error {
	if m == nil {
		return ErrCommandMaintenanceUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started || m.stopping != nil || m.retentionStop != nil || m.cfg.MaintenanceCoordinator != nil {
		return ErrCommandMaintenanceBusy
	}
	adapters, err := NewCommandMaintenanceAdapters(m)
	if err != nil {
		return err
	}
	ports.Jobs, ports.Tools, ports.Compaction = adapters.Jobs, adapters.Tools, adapters.Compaction
	coordinator, err := commandmaintenance.New(ports)
	if err != nil {
		return err
	}
	m.cfg.MaintenanceCoordinator = coordinator
	return nil
}
