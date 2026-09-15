package commandjobactivation

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandmaintenance"
	"gorm.io/gorm"
)

func TestMaintenanceAdaptersCoordinatorRealBlocksRetentionUntilOutboxDrained(t *testing.T) {
	c, out, fact, _, now := fixture(t)
	const total = 101
	for sequence := 1; sequence <= total; sequence++ {
		current := fact
		var err error
		current.SourceEventID, err = freshID()
		if err != nil {
			t.Fatal(err)
		}
		current.RunEventID = current.SourceEventID
		current.Sequence = sequence
		current.State = commandjobevents.StateQueued
		if err := c.db.Transaction(func(tx *gorm.DB) error { return out.InsertFactTx(tx, current) }); err != nil {
			t.Fatal(err)
		}
		if sequence == 1 {
			if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", current.SourceEventID).Updates(map[string]any{
				"delivery_state":   commandjobevents.DeliveryProcessing,
				"lease_owner":      "expired-host-capability",
				"lease_expires_at": (*now).Add(-time.Minute),
			}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}

	outbox, err := NewMaintenanceOutboxAdapter(c, "host-delivery-capability")
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := NewMaintenanceRecoveryAdapter(c)
	if err != nil {
		t.Fatal(err)
	}
	var retentionCalls, toolCalls, compactionCalls int
	coordinator, err := commandmaintenance.New(commandmaintenance.Ports{
		Outbox:       outbox,
		Decisions:    maintenanceAdapterRecovery{},
		Invocations:  maintenanceAdapterRecovery{},
		Claims:       recovery,
		Jobs:         maintenanceAdapterRetention{calls: &retentionCalls},
		Tools:        maintenanceAdapterTools{calls: &toolCalls},
		InvocationDB: maintenanceAdapterRetention{calls: &retentionCalls},
		Activations:  maintenanceAdapterRetention{calls: &retentionCalls},
		Compaction:   maintenanceAdapterCompaction{calls: &compactionCalls},
	})
	if err != nil {
		t.Fatal(err)
	}
	policy := commandmaintenance.Policy{
		InvocationRetention:   time.Hour,
		ActivationRetention:   time.Hour,
		LeaseDuration:         time.Minute,
		InvocationsPerUser:    1,
		InvocationsSystemKeep: 1,
		ActivationsPerUser:    1,
		BatchSize:             commandmaintenance.DefaultBatchSize,
	}

	first, err := coordinator.Run(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if first.OutboxRequeued != 1 || first.OutboxDrained || !first.MoreOutbox || first.Recovered != 1 || retentionCalls != 0 || toolCalls != 0 || compactionCalls != 0 {
		t.Fatalf("primeira passagem=%+v calls=(retention:%d tools:%d compact:%d), retenção deveria estar bloqueada", first, retentionCalls, toolCalls, compactionCalls)
	}

	second, err := coordinator.Run(context.Background(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if second.OutboxRequeued != 0 || !second.OutboxDrained || second.MoreOutbox || second.Recovered != 1 || second.MoreRecovery || !second.Compacted {
		t.Fatalf("segunda passagem=%+v, esperado drain/recovery completos", second)
	}
	if retentionCalls != 3 || toolCalls != 3 || compactionCalls != 1 {
		t.Fatalf("limpeza após drain=(retention:%d tools:%d compact:%d), esperado (3,3,1)", retentionCalls, toolCalls, compactionCalls)
	}

	var delivered int64
	if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("delivery_state = ?", commandjobevents.DeliveryDelivered).Count(&delivered).Error; err != nil {
		t.Fatal(err)
	}
	if delivered != total {
		t.Fatalf("outbox delivered=%d, want %d", delivered, total)
	}
}

func TestMaintenanceAdaptersRejectCanceledAndInvalidCalls(t *testing.T) {
	c, _, _, _, _ := fixture(t)
	outbox, err := NewMaintenanceOutboxAdapter(c, "host-capability")
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := NewMaintenanceRecoveryAdapter(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewMaintenanceOutboxAdapter(c, " "); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("owner vazio=%v, esperado ErrUnavailable", err)
	}
	if _, err := outbox.Drain(context.Background(), 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("limite zero no drain=%v, esperado ErrUnavailable", err)
	}
	if _, err := recovery.Recover(context.Background(), 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("limite zero na recuperação=%v, esperado ErrUnavailable", err)
	}
	recovery.mu.Lock()
	busyResult, busyErr := recovery.Recover(context.Background(), 1)
	recovery.mu.Unlock()
	if !errors.Is(busyErr, ErrUnavailable) || busyResult != (commandmaintenance.BatchResult{}) {
		t.Fatalf("recovery concorrente=(%+v,%v), esperado rejeição imediata", busyResult, busyErr)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := outbox.Drain(canceled, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("drain cancelado=%v, esperado context.Canceled", err)
	}
	if _, err := recovery.Recover(canceled, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("recovery cancelada=%v, esperado context.Canceled", err)
	}
}

func TestMaintenanceRecoveryAdapterRevisitsLeaseAfterCompletedCycle(t *testing.T) {
	t.Run("runtime revogado no ciclo seguinte", func(t *testing.T) {
		c, out, fact, _, _ := fixture(t)
		deliver(t, c, out, fact)
		adapter, err := NewMaintenanceRecoveryAdapter(c)
		if err != nil {
			t.Fatal(err)
		}
		first, err := adapter.Recover(context.Background(), 2)
		if err != nil || first.Processed != 1 || first.More {
			t.Fatalf("primeiro ciclo=%+v err=%v, esperado página concluída", first, err)
		}
		c.ports.Runtime = func(context.Context, *gorm.DB, commandjobevents.Fact) (RuntimeIdentity, error) {
			return RuntimeIdentity{Generation: "runtime-revoked", UserID: fact.UserID, AuthContextType: "local_session", AuthContextID: "session", AuthGeneration: "1", SecurityGeneration: "1"}, nil
		}
		second, err := adapter.Recover(context.Background(), 2)
		if err != nil || second.Processed != 1 || second.More {
			t.Fatalf("ciclo após mudança de runtime=%+v err=%v, lease não foi revisitada", second, err)
		}
		var claim commandactivation.Claim
		if err := c.db.Where("source_event_id = ?", fact.SourceEventID).Take(&claim).Error; err != nil {
			t.Fatal(err)
		}
		if claim.State != commandactivation.StateInactive || claim.TerminalReason == nil || *claim.TerminalReason != "source_unavailable" {
			t.Fatalf("claim não foi invalidada: %+v", claim)
		}
		var leases int64
		if err := c.db.Model(&Lease{}).Where("activation_id = ?", claim.ActivationID).Count(&leases).Error; err != nil {
			t.Fatal(err)
		}
		if leases != 0 {
			t.Fatalf("lease inválida permaneceu: %d", leases)
		}
	})

	t.Run("lease expirada no ciclo seguinte", func(t *testing.T) {
		c, out, fact, _, now := fixture(t)
		deliver(t, c, out, fact)
		adapter, err := NewMaintenanceRecoveryAdapter(c)
		if err != nil {
			t.Fatal(err)
		}
		if result, err := adapter.Recover(context.Background(), 2); err != nil || result.Processed != 1 || result.More {
			t.Fatalf("primeiro ciclo=%+v err=%v", result, err)
		}
		var lease Lease
		if err := c.db.Take(&lease).Error; err != nil {
			t.Fatal(err)
		}
		*now = lease.ExpiresAt
		result, err := adapter.Recover(context.Background(), 2)
		if err != nil || result.Processed != 1 || result.More {
			t.Fatalf("ciclo após expiração=%+v err=%v, lease não foi revisitada", result, err)
		}
		var claim commandactivation.Claim
		if err := c.db.Where("activation_id = ?", lease.ActivationID).Take(&claim).Error; err != nil {
			t.Fatal(err)
		}
		if claim.State != commandactivation.StateInactive {
			t.Fatalf("claim expirada não ficou inativa: %+v", claim)
		}
	})
}

type maintenanceAdapterRecovery struct{}

func (maintenanceAdapterRecovery) Recover(context.Context, int) (commandmaintenance.BatchResult, error) {
	return commandmaintenance.BatchResult{}, nil
}

type maintenanceAdapterRetention struct{ calls *int }

func (p maintenanceAdapterRetention) Retain(context.Context, commandmaintenance.Policy) (int64, error) {
	*p.calls++
	return 0, nil
}

type maintenanceAdapterTools struct{ calls *int }

func (p maintenanceAdapterTools) CleanOldDryRuns(context.Context, commandmaintenance.Policy) (int64, error) {
	*p.calls++
	return 0, nil
}
func (p maintenanceAdapterTools) CleanOrphanChat(context.Context, commandmaintenance.Policy) (int64, error) {
	*p.calls++
	return 0, nil
}
func (p maintenanceAdapterTools) CleanOldChat(context.Context, commandmaintenance.Policy) (int64, error) {
	*p.calls++
	return 0, nil
}

type maintenanceAdapterCompaction struct{ calls *int }

func (p maintenanceAdapterCompaction) Compact(context.Context, int64) error {
	*p.calls++
	return nil
}
