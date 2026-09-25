package commandjobevents

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
)

func preparePurgeFixture(t *testing.T) (*Store, time.Time, []Fact) {
	t.Helper()
	store := testStore(t)
	clock := time.Now().UTC().Truncate(time.Microsecond)
	store.configure(func() time.Time { return clock }, time.Minute, time.Hour, 2)
	if _, err := store.EnsureReplayPolicyEpoch(context.Background(), ProducerType, clock.Add(-3*time.Hour), time.Hour); err != nil {
		t.Fatalf("seed replay policy epoch: %v", err)
	}
	if err := store.DB().Exec(`CREATE TABLE command_layer_activation_state (
		activation_id TEXT, user_id TEXT, source_type TEXT, source_event_id TEXT,
		source_correlation_id TEXT, state TEXT, expires_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("criar tabela de claims: %v", err)
	}
	if err := store.DB().Exec(`CREATE TABLE command_job_activation_leases (
		activation_id TEXT, user_id TEXT, run_id TEXT, expires_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("criar tabela de leases: %v", err)
	}
	facts := make([]Fact, 0, 4)
	for i := 0; i < 4; i++ {
		fact := testFact(t, clock.Add(-2*time.Hour))
		insertTestFact(t, store, fact)
		if err := store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Updates(map[string]any{
			"delivery_state": DeliveryDelivered,
		}).Error; err != nil {
			t.Fatalf("marcar fato entregue: %v", err)
		}
		facts = append(facts, fact)
	}
	return store, clock, facts
}

func TestPurgeExpiredProtegeClaimLeaseVivaEProcessaLotes(t *testing.T) {
	store, clock, facts := preparePurgeFixture(t)
	protected := facts[0]
	if err := store.DB().Exec(`INSERT INTO command_layer_activation_state
		(activation_id,user_id,source_type,source_event_id,source_correlation_id,state,expires_at)
		VALUES (?,?,?,?,?,?,?)`, testUUIDv7(t), protected.UserID, "job", protected.SourceEventID, protected.RunID, "active", clock.Add(time.Hour)).Error; err != nil {
		t.Fatalf("inserir claim ativa: %v", err)
	}
	var activationID string
	if err := store.DB().Raw("SELECT activation_id FROM command_layer_activation_state WHERE source_event_id = ?", protected.SourceEventID).Scan(&activationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DB().Exec(`INSERT INTO command_job_activation_leases
		(activation_id,user_id,run_id,expires_at) VALUES (?,?,?,?)`, activationID, protected.UserID, protected.RunID, clock.Add(time.Hour)).Error; err != nil {
		t.Fatalf("inserir lease ativa: %v", err)
	}

	processed, more, err := store.PurgeExpired(context.Background(), 2)
	if err != nil || processed != 2 || !more {
		t.Fatalf("primeiro lote=(%d,%v,%v), esperado (2,true,nil)", processed, more, err)
	}
	processed, more, err = store.PurgeExpired(context.Background(), 2)
	if err != nil || processed != 1 || more {
		t.Fatalf("segundo lote=(%d,%v,%v), esperado (1,false,nil)", processed, more, err)
	}
	if _, err := store.Get(context.Background(), protected.SourceEventID); err != nil {
		t.Fatalf("fonte com claim+lease viva foi removida: %v", err)
	}
	if err := store.DB().Exec("DELETE FROM command_job_activation_leases WHERE activation_id = ?", activationID).Error; err != nil {
		t.Fatal(err)
	}
	if processed, more, err := store.PurgeExpired(context.Background(), 2); err != nil || processed != 1 || more {
		t.Fatalf("purga após expirar lease=(%d,%v,%v), esperado (1,false,nil)", processed, more, err)
	}
	if _, err := store.Get(context.Background(), protected.SourceEventID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("fonte sem lease viva não foi purgada: %v", err)
	}
}

func TestPurgeExpiredFalhaFechadoComNowZero(t *testing.T) {
	store := testStore(t)
	store.configure(func() time.Time { return time.Time{} }, time.Minute, time.Hour, 2)
	processed, more, err := store.PurgeExpired(context.Background(), 1)
	if !errors.Is(err, ErrInvalidFact) || processed != 0 || more {
		t.Fatalf("now zero=(%d,%v,%v), esperado (0,false,ErrInvalidFact)", processed, more, err)
	}
}

func TestPurgeExpiredRollbackZeraContadoresEPreservaFonte(t *testing.T) {
	store, _, facts := preparePurgeFixture(t)
	if err := store.DB().Exec("CREATE TRIGGER purge_test_abort BEFORE DELETE ON command_job_activation_outbox BEGIN SELECT RAISE(ABORT, 'purge rollback'); END").Error; err != nil {
		t.Fatalf("criar trigger de rollback: %v", err)
	}
	processed, more, err := store.PurgeExpired(context.Background(), 2)
	if err == nil || processed != 0 || more {
		t.Fatalf("falha de TX=(%d,%v,%v), esperado contadores zerados", processed, more, err)
	}
	for _, fact := range facts {
		if _, err := store.Get(context.Background(), fact.SourceEventID); err != nil {
			t.Fatalf("rollback removeu fonte %s: %v", fact.SourceEventID, err)
		}
	}
}
