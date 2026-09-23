package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/commandui"
)

func waitCommandUIAdmission(t *testing.T, a *App, reservation commandui.Reservation) {
	t.Helper()
	p := a.commandProduct.Load()
	deadline := time.NewTimer(uiCommandTestTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		p.mu.Lock()
		run := p.uiRuns[reservation.Ticket]
		ready := run != nil && run.admission != nil
		p.mu.Unlock()
		if ready {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("executor não publicou admissão UI")
		case <-tick.C:
		}
	}
}

func TestUICommandRevalidatesPublishedConfigurationBeforeHandoff(t *testing.T) {
	for _, mutation := range []string{"rebuild", "active_layers"} {
		t.Run(mutation, func(t *testing.T) {
			a := readyCommandProduct(t)
			reservation := beginAuditedTabCreate(t, a)
			waitCommandUIAdmission(t, a, reservation)
			p := a.commandProduct.Load()
			var err error
			if mutation == "rebuild" {
				err = a.rebuildCommandLifecyclePersistedConfiguration(context.Background())
			} else {
				err = p.host.SetActiveLayers(context.Background(), p.principal.UserID, []string{"changed-layer"})
			}
			if err != nil {
				t.Fatal(err)
			}
			if handoff, err := a.TakeUICommand(reservation.Ticket); err == nil {
				t.Fatalf("configuração antiga liberou efeito visual: %+v", handoff)
			}
			result := getUIResultEventually(t, a, reservation.Ticket)
			// A mutação revoga o contexto já running. O executor conserva seu
			// contrato conservador: resultado invalidado é outcome_unknown,
			// mesmo que esta fachada tenha recusado entregar o efeito visual.
			if result.Status != "outcome_unknown" {
				t.Fatalf("contexto revogado não pode confirmar execução: %+v", result)
			}
			assertUICommandLedgerStatus(t, reservation.InvocationID, "outcome_unknown")
			// Mudança invalida a tentativa antiga, não impede toda execução futura.
			next := beginAuditedTabCreate(t, a)
			handoff := takeAuditedTabCreate(t, a, next.Ticket)
			if err := a.CommitWorkspaceTabCommand(next.Ticket, handoff.HandoffID); err != nil {
				t.Fatalf("CommitWorkspaceTabCommand create após %s: %v", mutation, err)
			}
			if result := getUIResultEventually(t, a, next.Ticket); result.Status != "succeeded" {
				t.Fatalf("nova tentativa: %+v", result)
			}
		})
	}
}

func TestUICommandAdmissionCannotSurviveVaultLockUnlock(t *testing.T) {
	a := readyCommandProduct(t)
	r := beginAuditedTabCreate(t, a)
	waitCommandUIAdmission(t, a, r)
	if err := a.commandHost.SetVaultUnlocked(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := a.commandHost.SetVaultUnlocked(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if handoff, err := a.TakeUICommand(r.Ticket); err == nil {
		t.Fatalf("handoff atravessou lock/unlock: %+v", handoff)
	}
}
