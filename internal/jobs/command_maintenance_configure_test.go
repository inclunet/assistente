package jobs

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandmaintenance"
)

func TestReconfigureCommandMaintenancePreservesOldOnFailureAndRejectsWarmOrClosed(t *testing.T) {
	m, _, _, _ := newCommandMaintenanceTestManager(t)
	empty := commandMaintenanceNoopPort{}
	ports := commandmaintenance.Ports{Outbox: empty, Decisions: empty, Invocations: empty, Claims: empty, InvocationDB: empty, Activations: empty}
	if err := m.ReconfigureCommandMaintenance(ports); !errors.Is(err, ErrCommandMaintenanceBusy) {
		t.Fatalf("replace sem montagem: %v", err)
	}
	if err := m.ConfigureCommandMaintenance(ports); err != nil {
		t.Fatal(err)
	}
	previous := m.cfg.MaintenanceCoordinator
	if err := m.ReconfigureCommandMaintenance(commandmaintenance.Ports{}); err == nil || m.cfg.MaintenanceCoordinator != previous {
		t.Fatal("replace inválido alterou coordenador")
	}
	m.started = true
	if err := m.ReconfigureCommandMaintenance(ports); !errors.Is(err, ErrCommandMaintenanceBusy) {
		t.Fatalf("replace quente: %v", err)
	}
	m.started = false
	if err := m.ReconfigureCommandMaintenance(ports); err != nil || m.cfg.MaintenanceCoordinator == previous {
		t.Fatalf("replace frio: %v", err)
	}
	if err := m.CloseCommandMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.ReconfigureCommandMaintenance(ports); !errors.Is(err, ErrCommandMaintenanceUnavailable) {
		t.Fatalf("replace após fechamento: %v", err)
	}
}

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
