package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcontext"
	"assistente/internal/workspace"
)

func TestCommandFactBusRealRevalidaActiveTabMesmoSemNotify(t *testing.T) {
	app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
	bus, err := app.newCommandFactBus(principal)
	if err != nil {
		t.Fatalf("newCommandFactBus: %v", err)
	}
	scope := commandWorkspaceScope(principal, workspaceID)
	policy := commandFactPolicy("workspace", "active_tab")
	proof, err := bus.Capture(context.Background(), scope, policy)
	if err != nil {
		t.Fatalf("Capture active_tab: %v", err)
	}

	// A mudança real da fonte é suficiente: Notify é apenas um hint e pode
	// ser perdido sem tornar o snapshot capturado novamente válido.
	if err := manager.UpdateTab("tab-real", map[string]any{
		"state": map[string]any{"version": float64(2)},
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}
	if err := bus.Revalidate(context.Background(), scope, policy, proof, time.Now().UTC()); !errors.Is(err, commandcontext.ErrVersionMismatch) {
		t.Fatalf("Revalidate sem Notify = %v, want ErrVersionMismatch", err)
	}
}

func TestCommandFactBusRealNotificacaoPerdidaNaoEscondeMudanca(t *testing.T) {
	app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
	bus, err := app.newCommandFactBus(principal)
	if err != nil {
		t.Fatalf("newCommandFactBus: %v", err)
	}
	scope := commandWorkspaceScope(principal, workspaceID)
	policy := commandFactPolicy("workspace", "active_tab")
	proof, err := bus.Capture(context.Background(), scope, policy)
	if err != nil {
		t.Fatalf("Capture active_tab: %v", err)
	}
	changes, cancel, err := bus.Subscribe(scope)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	if err := manager.UpdateTab("tab-real", map[string]any{
		"state": map[string]any{"version": float64(3)},
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}
	provider, err := app.newCommandWorkspaceProvider(principal)
	if err != nil {
		t.Fatalf("newCommandWorkspaceProvider: %v", err)
	}
	owned, err := provider.Snapshot(context.Background(), scope, "active_tab")
	if err != nil {
		t.Fatalf("Snapshot atualizado: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := bus.Notify(context.Background(), scope, "workspace", "active_tab", owned); err != nil {
			t.Fatalf("Notify %d: %v", i, err)
		}
	}
	select {
	case change := <-changes:
		if change.ProviderID != "workspace" || change.Fact != "active_tab" {
			t.Fatalf("hint incorreto: %#v", change)
		}
	default:
		t.Fatal("Notify não publicou o primeiro hint")
	}
	select {
	case change := <-changes:
		t.Fatalf("segundo hint deveria ter sido descartado/coalescido: %#v", change)
	default:
	}
	if err := bus.Revalidate(context.Background(), scope, policy, proof, time.Now().UTC()); !errors.Is(err, commandcontext.ErrVersionMismatch) {
		t.Fatalf("Revalidate após perda do hint = %v, want ErrVersionMismatch", err)
	}
}

func TestCommandFactBusRealObservaProfileEAbaSemNotificacao(t *testing.T) {
	tests := []struct {
		name   string
		fact   string
		mutate func(*testing.T, *workspace.Manager)
	}{
		{name: "profile", fact: "profile", mutate: func(t *testing.T, m *workspace.Manager) {
			if err := m.SetProfile("profile-after"); err != nil {
				t.Fatalf("SetProfile: %v", err)
			}
		}},
		{name: "active tab", fact: "active_tab", mutate: func(t *testing.T, m *workspace.Manager) {
			if err := m.UpdateTab("tab-real", map[string]any{"state": map[string]any{"version": float64(4)}}); err != nil {
				t.Fatalf("UpdateTab: %v", err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
			bus, err := app.newCommandFactBus(principal)
			if err != nil {
				t.Fatalf("newCommandFactBus: %v", err)
			}
			scope := commandWorkspaceScope(principal, workspaceID)
			policy := commandFactPolicy("workspace", tc.fact)
			proof, err := bus.Capture(context.Background(), scope, policy)
			if err != nil {
				t.Fatalf("Capture %s: %v", tc.fact, err)
			}
			tc.mutate(t, manager)
			if err := bus.Revalidate(context.Background(), scope, policy, proof, time.Now().UTC()); !errors.Is(err, commandcontext.ErrVersionMismatch) {
				t.Fatalf("Revalidate %s = %v, want ErrVersionMismatch", tc.fact, err)
			}
		})
	}
}

func TestCommandFactBusRealRejeitaProvasStaleNasSequenciasABA(t *testing.T) {
	t.Run("active tab A-B-A", func(t *testing.T) {
		app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
		bus, err := app.newCommandFactBus(principal)
		if err != nil {
			t.Fatalf("newCommandFactBus: %v", err)
		}
		scope := commandWorkspaceScope(principal, workspaceID)
		policy := commandFactPolicy("workspace", "active_tab")
		oldProof, err := bus.Capture(context.Background(), scope, policy)
		if err != nil {
			t.Fatalf("Capture A: %v", err)
		}

		if err := manager.AddTab(workspace.Tab{
			ID:   "tab-b",
			Type: workspace.TabTypeEditor,
			State: map[string]any{
				"version":  float64(1),
				"filePath": "tab-b.txt",
			},
		}); err != nil {
			t.Fatalf("AddTab B: %v", err)
		}
		if err := manager.SetActiveTab("tab-real"); err != nil {
			t.Fatalf("SetActiveTab A: %v", err)
		}

		if err := bus.Revalidate(context.Background(), scope, policy, oldProof, time.Now().UTC()); !errors.Is(err, commandcontext.ErrVersionMismatch) {
			t.Fatalf("prova active_tab A após A-B-A = %v, want ErrVersionMismatch", err)
		}
		newProof, err := bus.Capture(context.Background(), scope, policy)
		if err != nil {
			t.Fatalf("Capture final A: %v", err)
		}
		if newProof.ContextVersion() == oldProof.ContextVersion() {
			t.Fatal("captura final A reutilizou a versão da prova antiga")
		}
		if err := bus.Revalidate(context.Background(), scope, policy, newProof, time.Now().UTC()); err != nil {
			t.Fatalf("nova prova active_tab A válida: %v", err)
		}
	})

	t.Run("profile A-B-A", func(t *testing.T) {
		app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
		bus, err := app.newCommandFactBus(principal)
		if err != nil {
			t.Fatalf("newCommandFactBus: %v", err)
		}
		scope := commandWorkspaceScope(principal, workspaceID)
		policy := commandFactPolicy("workspace", "profile")
		oldProof, err := bus.Capture(context.Background(), scope, policy)
		if err != nil {
			t.Fatalf("Capture profile A: %v", err)
		}

		if err := manager.SetProfile("profile-b"); err != nil {
			t.Fatalf("SetProfile B: %v", err)
		}
		if err := manager.SetProfile("workspace-profile"); err != nil {
			t.Fatalf("SetProfile A: %v", err)
		}

		if err := bus.Revalidate(context.Background(), scope, policy, oldProof, time.Now().UTC()); !errors.Is(err, commandcontext.ErrVersionMismatch) {
			t.Fatalf("prova profile A após A-B-A = %v, want ErrVersionMismatch", err)
		}
		newProof, err := bus.Capture(context.Background(), scope, policy)
		if err != nil {
			t.Fatalf("Capture final profile A: %v", err)
		}
		if newProof.ContextVersion() == oldProof.ContextVersion() {
			t.Fatal("captura final profile A reutilizou a versão da prova antiga")
		}
		if err := bus.Revalidate(context.Background(), scope, policy, newProof, time.Now().UTC()); err != nil {
			t.Fatalf("nova prova profile A válida: %v", err)
		}
	})
}

func TestCommandFactBusRealRecusaOwnershipWorkspaceEFonteAusente(t *testing.T) {
	t.Run("ownership", func(t *testing.T) {
		app, principal, _, workspaceID := newCommandWorkspaceProviderFixture(t)
		bus, err := app.newCommandFactBus(principal)
		if err != nil {
			t.Fatalf("newCommandFactBus: %v", err)
		}
		scope := commandWorkspaceScope(principal, workspaceID)
		policy := commandFactPolicy("workspace", "active_tab")
		proof, err := bus.Capture(context.Background(), scope, policy)
		if err != nil {
			t.Fatalf("Capture: %v", err)
		}
		app.setCurrentAuthUser(nil)
		if err := bus.Revalidate(context.Background(), scope, policy, proof, time.Now().UTC()); !errors.Is(err, commandcontext.ErrProviderUnavailable) {
			t.Fatalf("Revalidate após logout = %v, want ErrProviderUnavailable", err)
		}
	})

	t.Run("workspace alterado", func(t *testing.T) {
		app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
		bus, err := app.newCommandFactBus(principal)
		if err != nil {
			t.Fatalf("newCommandFactBus: %v", err)
		}
		scope := commandWorkspaceScope(principal, workspaceID)
		policy := commandFactPolicy("workspace", "workspace")
		proof, err := bus.Capture(context.Background(), scope, policy)
		if err != nil {
			t.Fatalf("Capture: %v", err)
		}
		other, err := manager.Create("outro")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := manager.Switch(other.ID); err != nil {
			t.Fatalf("Switch: %v", err)
		}
		if err := bus.Revalidate(context.Background(), scope, policy, proof, time.Now().UTC()); !errors.Is(err, commandcontext.ErrProviderUnavailable) {
			t.Fatalf("Revalidate após troca = %v, want ErrProviderUnavailable", err)
		}
	})

	t.Run("fonte ausente", func(t *testing.T) {
		app, principal, _, workspaceID := newCommandWorkspaceProviderFixture(t)
		bus, err := app.newCommandFactBus(principal)
		if err != nil {
			t.Fatalf("newCommandFactBus: %v", err)
		}
		_, err = bus.Capture(context.Background(), commandWorkspaceScope(principal, workspaceID), commandFactPolicy("active_tab", "active_tab"))
		if !errors.Is(err, commandcontext.ErrMissingSnapshot) {
			t.Fatalf("alias active_tab exposto ou erro incorreto: %v", err)
		}
	})
}
