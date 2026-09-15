package jobs

import (
	"errors"
	"testing"

	"assistente/internal/commandmaintenance"
)

func TestConfigureCommandMaintenanceIsColdOnlyAndUsesRealLegacyAdapters(t *testing.T) {
	m, _, _, _ := newCommandMaintenanceTestManager(t)
	if err := m.ConfigureCommandMaintenance(commandmaintenance.Ports{}); !errors.Is(err, commandmaintenance.ErrInvalid) {
		t.Fatalf("montagem incompleta=%v", err)
	}
	if m.cfg.MaintenanceCoordinator != nil {
		t.Fatal("falha publicou coordinator")
	}
	empty := commandMaintenanceNoopPort{}
	ports := commandmaintenance.Ports{Outbox: empty, Decisions: empty, Invocations: empty, Claims: empty, InvocationDB: empty, Activations: empty}
	if err := m.ConfigureCommandMaintenance(ports); err != nil {
		t.Fatal(err)
	}
	if m.cfg.MaintenanceCoordinator == nil {
		t.Fatal("montagem ausente")
	}
	if err := m.ConfigureCommandMaintenance(ports); !errors.Is(err, ErrCommandMaintenanceBusy) {
		t.Fatalf("remontagem=%v", err)
	}
	n, _, _, _ := newCommandMaintenanceTestManager(t)
	n.started = true
	if err := n.ConfigureCommandMaintenance(ports); !errors.Is(err, ErrCommandMaintenanceBusy) {
		t.Fatalf("montagem quente=%v", err)
	}
}
