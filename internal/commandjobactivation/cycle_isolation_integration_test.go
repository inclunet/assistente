package commandjobactivation

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandjobevents"
	"gorm.io/gorm"
)

// O fixture compartilhado usa SQLite temporário, DispatchGate real e ports
// determinísticos. Isso prova a fronteira Consumer/store, mas não substitui a
// projeção do App nem um runtime ou banco externo real.
func TestRunCyclesRemainIsolatedByRunID(t *testing.T) {
	c, out, first, _, _ := fixture(t)
	if err := c.db.Exec("INSERT INTO job_runs(id, user_id, job_id) SELECT ?, user_id, job_id FROM job_runs WHERE id = ?", "opaque:run-2", first.RunID).Error; err != nil {
		t.Fatal(err)
	}

	second := first
	second.SourceEventID, _ = freshID()
	second.RunEventID = second.SourceEventID
	second.RunID = "opaque:run-2"
	if got := deliver(t, c, out, first); got.Applied != 1 {
		t.Fatalf("primeiro ciclo não aplicado: %+v", got)
	}
	if got := deliver(t, c, out, second); got.Applied != 1 {
		t.Fatalf("segundo ciclo não aplicado: %+v", got)
	}

	terminal := first
	terminal.SourceEventID, _ = freshID()
	terminal.RunEventID = terminal.SourceEventID
	terminal.Sequence = 2
	terminal.State = commandjobevents.StateCompleted
	terminal.OccurredAt = terminal.OccurredAt.Add(-time.Second)
	if got := deliver(t, c, out, terminal); got.Applied != 1 {
		t.Fatalf("terminal atrasado não aplicado: %+v", got)
	}

	var firstClaim, secondClaim commandactivation.Claim
	if err := c.db.Where("source_correlation_id = ?", first.RunID).Take(&firstClaim).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Where("source_correlation_id = ?", second.RunID).Take(&secondClaim).Error; err != nil {
		t.Fatal(err)
	}
	if firstClaim.State != commandactivation.StateDeactivated {
		t.Fatalf("terminal do primeiro run deixou claim=%q", firstClaim.State)
	}
	if secondClaim.State != commandactivation.StateActive {
		t.Fatalf("terminal do primeiro run fechou o segundo: claim=%q", secondClaim.State)
	}
	var leases int64
	if err := c.db.Model(&Lease{}).Where("run_id = ?", first.RunID).Count(&leases).Error; err != nil {
		t.Fatal(err)
	}
	if leases != 0 {
		t.Fatalf("lease do ciclo terminal permaneceu: %d", leases)
	}
	if err := c.db.Model(&Lease{}).Where("run_id = ?", second.RunID).Count(&leases).Error; err != nil {
		t.Fatal(err)
	}
	if leases != 1 {
		t.Fatalf("lease do segundo ciclo foi afetada: %d", leases)
	}
}

func TestRetryScheduledKeepsExistingRuntimeLease(t *testing.T) {
	c, out, first, _, _ := fixture(t)
	if got := deliver(t, c, out, first); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}
	var before Lease
	if err := c.db.Where("run_id = ?", first.RunID).Take(&before).Error; err != nil {
		t.Fatal(err)
	}

	retry := first
	retry.SourceEventID, _ = freshID()
	retry.RunEventID = retry.SourceEventID
	retry.Sequence = 2
	retry.State = commandjobevents.StateRetryScheduled
	if got := deliver(t, c, out, retry); got.Applied != 1 {
		t.Fatalf("retry_scheduled não aplicado: %+v", got)
	}

	var after Lease
	if err := c.db.Where("run_id = ?", first.RunID).Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.ID != before.ID || !after.ExpiresAt.Equal(before.ExpiresAt) || after.RuntimeGeneration != before.RuntimeGeneration {
		t.Fatalf("retry_scheduled substituiu lease: antes=%+v depois=%+v", before, after)
	}
	var claim commandactivation.Claim
	if err := c.db.Where("activation_id = ?", after.ActivationID).Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if claim.State != commandactivation.StateActive || claim.Sequence == nil || *claim.Sequence != 2 {
		t.Fatalf("claim após retry_scheduled=%+v", claim)
	}
}

func TestRejectedSourceOrRuntimeDoesNotKeepClaimOrLease(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Consumer, commandjobevents.Fact) error
	}{
		{
			name: "source ausente",
			setup: func(c *Consumer, fact commandjobevents.Fact) error {
				return c.db.Exec("DELETE FROM job_runs WHERE id = ?", fact.RunID).Error
			},
		},
		{
			name: "geração de runtime divergente",
			setup: func(c *Consumer, fact commandjobevents.Fact) error {
				c.ports.Runtime = func(context.Context, *gorm.DB, commandjobevents.Fact) (RuntimeIdentity, error) {
					return RuntimeIdentity{Generation: "runtime-external", UserID: fact.UserID, AuthContextType: "local_session", AuthContextID: "session", AuthGeneration: "1", SecurityGeneration: "1"}, nil
				}
				return nil
			},
		},
		{
			name: "runtime desconhecido",
			setup: func(c *Consumer, _ commandjobevents.Fact) error {
				c.ports.Runtime = func(context.Context, *gorm.DB, commandjobevents.Fact) (RuntimeIdentity, error) {
					return RuntimeIdentity{}, commandautomation.ErrNotFound
				}
				return nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, out, fact, _, _ := fixture(t)
			if got := deliver(t, c, out, fact); got.Applied != 1 {
				t.Fatalf("evento inicial não aplicado: %+v", got)
			}
			var lease Lease
			if err := c.db.Where("run_id = ?", fact.RunID).Take(&lease).Error; err != nil {
				t.Fatal(err)
			}
			if err := test.setup(c, fact); err != nil {
				t.Fatal(err)
			}

			_, done, err := c.ReconcileBatch(context.Background(), "", 10)
			if err != nil || !done {
				t.Fatalf("reconcile rejeitado=(done:%v, err:%v)", done, err)
			}
			var claim commandactivation.Claim
			if err := c.db.Where("activation_id = ?", lease.ActivationID).Take(&claim).Error; err != nil {
				t.Fatal(err)
			}
			if claim.State != commandactivation.StateInactive {
				t.Fatalf("claim rejeitada permaneceu ativa: %q", claim.State)
			}
			var count int64
			if err := c.db.Model(&Lease{}).Where("activation_id = ?", lease.ActivationID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("lease rejeitada permaneceu: %d", count)
			}
		})
	}
}

func TestOutboxRejectsExternalAndUnknownRootOrigins(t *testing.T) {
	for _, origin := range []string{"external_event", "unknown"} {
		t.Run(origin, func(t *testing.T) {
			c, out, fact, _, _ := fixture(t)
			fact.RootOriginType = origin
			err := c.db.Transaction(func(tx *gorm.DB) error {
				return out.InsertFactTx(tx, fact)
			})
			if !errors.Is(err, commandjobevents.ErrInvalidFact) {
				t.Fatalf("origem %q resultou em %v, esperado ErrInvalidFact", origin, err)
			}
			var count int64
			if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("origem rejeitada deixou %d registro(s) na outbox", count)
			}
		})
	}
}
