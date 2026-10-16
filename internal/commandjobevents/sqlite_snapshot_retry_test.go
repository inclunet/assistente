package commandjobevents

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type snapshotRetryFixture struct {
	store  *Store
	writer *gorm.DB
	epoch  ReplayPolicyEpoch
	now    time.Time
}

func TestSQLiteBusyRetryRefreshesLeaseClockButPreservesExplicitPurgeTime(t *testing.T) {
	for _, operation := range []string{"claim", "requeue", "purge"} {
		t.Run(operation, func(t *testing.T) {
			f := newSnapshotRetryFixture(t)
			ctx := context.Background()
			now := f.now
			f.store.configure(func() time.Time { return now }, time.Second, time.Hour, 2)
			facts := []Fact{testFact(t, f.now.Add(-time.Minute)), testFact(t, f.now.Add(-time.Minute))}
			for i, fact := range facts {
				insertTestFact(t, f.store, fact)
				values := map[string]any{}
				if operation == "purge" {
					values["delivery_state"] = DeliveryDelivered
					values["source_replay_deadline"] = f.now.Add(time.Duration(i*2-1) * time.Second)
				} else if operation == "requeue" || i == 1 {
					values["delivery_state"] = DeliveryProcessing
					values["lease_owner"] = "previous-owner"
					values["lease_expires_at"] = f.now.Add(time.Duration(i*2-1) * time.Second)
				}
				if len(values) > 0 {
					if err := f.store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Updates(values).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			pool, err := f.writer.DB()
			if err != nil {
				t.Fatal(err)
			}
			conn, err := pool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
				t.Fatal(err)
			}
			var releaseOnce sync.Once
			var releaseErr error
			release := func() { releaseOnce.Do(func() { _, releaseErr = conn.ExecContext(ctx, "ROLLBACK") }) }
			defer release()
			busy := 0
			hook := "test:advance-clock-after-real-busy"
			afterWrite := func(tx *gorm.DB) {
				if database.IsSQLiteBusyError(tx.Error) {
					busy++
					// Só avança depois do erro SQLite real, antes da nova tentativa.
					now = f.now.Add(4 * time.Second)
					release()
				}
			}
			if operation == "purge" {
				if err := f.store.DB().Callback().Delete().After("gorm:delete").Register(hook, afterWrite); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = f.store.DB().Callback().Delete().Remove(hook) })
			} else {
				if err := f.store.DB().Callback().Update().After("gorm:update").Register(hook, afterWrite); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = f.store.DB().Callback().Update().Remove(hook) })
			}
			var count int
			var more bool
			switch operation {
			case "claim":
				var rows []ActivationOutbox
				rows, more, err = f.store.ClaimBatch(ctx, "fresh-owner", 2)
				count = len(rows)
				for _, row := range rows {
					if row.LeaseExpiresAt == nil || !row.LeaseExpiresAt.Equal(now.Add(time.Second)) {
						t.Errorf("lease retornada sem duração fresca: %+v now=%v", row.LeaseExpiresAt, now)
					}
				}
			case "requeue":
				count, more, err = f.store.RequeueExpiredLeases(ctx, 2)
			case "purge":
				count, more, err = f.store.PurgeExpiredAt(ctx, f.now, 2)
			}
			want := 2
			if operation == "purge" {
				want = 1
			}
			if err != nil || releaseErr != nil || busy != 1 || count != want || more {
				t.Fatalf("resultado count=%d want=%d more=%v err=%v busy=%d release=%v", count, want, more, err, busy, releaseErr)
			}
			for i, fact := range facts {
				row, err := f.store.Get(ctx, fact.SourceEventID)
				if operation == "purge" && i == 0 {
					if !errors.Is(err, gorm.ErrRecordNotFound) {
						t.Fatalf("fonte vencida não purgada: %v", err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				switch operation {
				case "claim":
					if row.Attempts != 1 || row.LeaseExpiresAt == nil || !row.LeaseExpiresAt.Equal(now.Add(time.Second)) {
						t.Fatalf("claim persistido inválido: %+v", row)
					}
					if err := f.store.Ack(ctx, fact.SourceEventID, "fresh-owner"); err != nil {
						t.Fatalf("lease já expirou após retry: %v", err)
					}
				case "requeue":
					if row.DeliveryState != DeliveryPending || row.LeaseOwner != nil || row.LeaseExpiresAt != nil {
						t.Fatalf("lease vencida durante espera não requeued: %+v", row)
					}
				case "purge":
					if !row.SourceReplayDeadline.Equal(f.now.Add(time.Second)) {
						t.Fatalf("deadline explícita alterada: %+v", row)
					}
				}
			}
		})
	}
}

func TestSQLiteBusyRetryExhaustionPreservesDataAndReturnsEmptyResults(t *testing.T) {
	for _, operation := range []string{"epoch", "claim", "requeue", "purge"} {
		t.Run(operation, func(t *testing.T) {
			f := newSnapshotRetryFixture(t)
			ctx := context.Background()
			fact := testFact(t, f.now.Add(-time.Minute))
			insertTestFact(t, f.store, fact)
			if operation == "requeue" {
				if err := f.store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Updates(map[string]any{
					"delivery_state": DeliveryProcessing, "lease_owner": "old-owner", "lease_expires_at": f.now.Add(-time.Second),
				}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if operation == "purge" {
				if err := f.store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Updates(map[string]any{
					"delivery_state": DeliveryDelivered, "source_replay_deadline": f.now.Add(-time.Second),
				}).Error; err != nil {
					t.Fatal(err)
				}
			}
			before, err := f.store.Get(ctx, fact.SourceEventID)
			if err != nil {
				t.Fatal(err)
			}
			pool, err := f.writer.DB()
			if err != nil {
				t.Fatal(err)
			}
			conn, err := pool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
				t.Fatal(err)
			}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { _, _ = conn.ExecContext(ctx, "ROLLBACK") }) }
			defer release()
			var count int
			var more bool
			var attempts atomic.Int32
			hook := "test:exhausted-retry"
			if err := f.store.DB().Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
				if tx.Statement.Table == (ActivationOutbox{}).TableName() || tx.Statement.Table == (ReplayPolicyEpoch{}).TableName() {
					attempts.Add(1)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.store.DB().Callback().Query().Remove(hook) })
			switch operation {
			case "epoch":
				var epoch ReplayPolicyEpoch
				epoch, err = f.store.EnsureReplayPolicyEpoch(ctx, ProducerType, f.now.Add(time.Minute), time.Hour)
				if epoch.ID != "" {
					count = 1
				}
			case "claim":
				var rows []ActivationOutbox
				rows, more, err = f.store.ClaimBatch(ctx, "blocked-owner", 1)
				count = len(rows)
			case "requeue":
				count, more, err = f.store.RequeueExpiredLeases(ctx, 1)
			case "purge":
				count, more, err = f.store.PurgeExpiredAt(ctx, f.now, 1)
			}
			if !database.IsSQLiteBusyError(err) || count != 0 || more {
				t.Fatalf("exhausted retry=(%d,%v,%v), expected empty busy result", count, more, err)
			}
			if attempts.Load() < 2 {
				t.Fatalf("persistent busy returned without retry: attempts=%d", attempts.Load())
			}
			release()
			after, err := f.store.Get(ctx, fact.SourceEventID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("exhausted retry changed outbox: before=%+v after=%+v err=%v", before, after, err)
			}
			var epochs int64
			if err := f.store.DB().Model(&ReplayPolicyEpoch{}).Count(&epochs).Error; err != nil || epochs != 1 {
				t.Fatalf("exhausted retry changed epochs: count=%d err=%v", epochs, err)
			}
		})
	}
}

func newSnapshotRetryFixture(t *testing.T) snapshotRetryFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "command-job-events-wal.db")
	open := func() *gorm.DB {
		dsn := "file:" + filepath.ToSlash(path) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(1)"
		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatalf("abrir SQLite temporário: %v", err)
		}
		pool, err := db.DB()
		if err != nil {
			t.Fatalf("obter pool SQLite: %v", err)
		}
		pool.SetMaxOpenConns(4)
		pool.SetMaxIdleConns(2)
		t.Cleanup(func() { _ = pool.Close() })
		return db
	}
	db, writer := open(), open()
	if err := db.AutoMigrate(Models()...); err != nil {
		t.Fatalf("migrar fixture outbox: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE command_layer_activation_state (
			activation_id TEXT, user_id TEXT, source_type TEXT, source_event_id TEXT,
			source_correlation_id TEXT, state TEXT, expires_at DATETIME
		)`,
		`CREATE TABLE command_job_activation_leases (
			activation_id TEXT, user_id TEXT, run_id TEXT, expires_at DATETIME
		)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("criar tabela auxiliar: %v", err)
		}
	}
	store := NewStore(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	epoch, err := store.EnsureReplayPolicyEpoch(context.Background(), ProducerType, now.Add(-time.Hour), time.Hour)
	if err != nil {
		t.Fatalf("criar epoch: %v", err)
	}
	store.configure(func() time.Time { return now }, time.Minute, time.Hour, 2)
	return snapshotRetryFixture{store: store, writer: writer, epoch: epoch, now: now}
}

// afterFirstRead pauses the transaction after its first relevant SELECT. The
// independent writer commits while that WAL snapshot remains open; resuming
// the operation forces its subsequent write to encounter BUSY_SNAPSHOT.
func (f snapshotRetryFixture) afterFirstRead(t *testing.T, match func(*gorm.DB) bool) (release func() error, unblock func(), reads *atomic.Int32) {
	t.Helper()
	selected := make(chan struct{})
	resume := make(chan struct{})
	reads = &atomic.Int32{}
	var first sync.Once
	name := fmt.Sprintf("test:snapshot-retry:%p", reads)
	callback := func(tx *gorm.DB) {
		if tx.Error != nil || !match(tx) {
			return
		}
		if reads.Add(1) == 1 {
			first.Do(func() { close(selected) })
			<-resume
		}
	}
	if err := f.store.DB().Callback().Query().After("gorm:query").Register(name, callback); err != nil {
		t.Fatalf("registrar barreira de leitura: %v", err)
	}
	rowName := name + ":row"
	if err := f.store.DB().Callback().Row().After("gorm:row").Register(rowName, callback); err != nil {
		t.Fatalf("registrar barreira de leitura row: %v", err)
	}
	var unblockOnce sync.Once
	unblock = func() { unblockOnce.Do(func() { close(resume) }) }
	t.Cleanup(func() {
		unblock()
		_ = f.store.DB().Callback().Query().Remove(name)
		_ = f.store.DB().Callback().Row().Remove(rowName)
	})
	return func() error {
		select {
		case <-selected:
		case <-time.After(5 * time.Second):
			unblock()
			return errors.New("transação não chegou ao SELECT instrumentado")
		}
		if err := f.writer.Exec("UPDATE command_event_replay_policy_epochs SET replay_horizon_seconds = replay_horizon_seconds + 1 WHERE id = ?", f.epoch.ID).Error; err != nil {
			unblock()
			return fmt.Errorf("writer concorrente não confirmou: %w", err)
		}
		unblock()
		return nil
	}, unblock, reads
}

func TestEnsureReplayPolicyEpochRetriesSQLiteBusySnapshot(t *testing.T) {
	f := newSnapshotRetryFixture(t)
	effectiveAt := f.now.Add(time.Minute)
	release, unblock, reads := f.afterFirstRead(t, func(tx *gorm.DB) bool {
		return tx.Statement.Table == (ReplayPolicyEpoch{}).TableName() && strings.Contains(strings.ToUpper(tx.Statement.SQL.String()), "COALESCE(MAX(GENERATION)")
	})
	done := make(chan error, 1)
	go func() {
		_, err := f.store.EnsureReplayPolicyEpoch(context.Background(), ProducerType, effectiveAt, time.Hour)
		done <- err
	}()
	joined := false
	t.Cleanup(func() {
		unblock()
		if !joined {
			<-done
		}
	})
	if err := release(); err != nil {
		t.Fatal(err)
	}
	resultErr := <-done
	joined = true
	if resultErr != nil {
		t.Fatalf("EnsureReplayPolicyEpoch não reiniciou a transação após BUSY_SNAPSHOT: %v", resultErr)
	}
	if got := reads.Load(); got < 2 {
		t.Fatalf("leituras da transação=%d; retry deveria abrir snapshot novo", got)
	}
}

func TestClaimBatchRetriesSQLiteBusySnapshot(t *testing.T) {
	f := newSnapshotRetryFixture(t)
	fact := testFact(t, f.now.Add(-time.Minute))
	insertTestFact(t, f.store, fact)
	release, unblock, reads := f.afterFirstRead(t, func(tx *gorm.DB) bool {
		return tx.Statement.Table == (ActivationOutbox{}).TableName() && strings.Contains(tx.Statement.SQL.String(), "delivery_state")
	})
	done := make(chan struct {
		rows []ActivationOutbox
		more bool
		err  error
	}, 1)
	go func() {
		rows, more, err := f.store.ClaimBatch(context.Background(), "owner-a", 1)
		done <- struct {
			rows []ActivationOutbox
			more bool
			err  error
		}{rows, more, err}
	}()
	joined := false
	t.Cleanup(func() {
		unblock()
		if !joined {
			<-done
		}
	})
	if err := release(); err != nil {
		t.Fatal(err)
	}
	result := <-done
	joined = true
	if result.err != nil || len(result.rows) != 1 || result.more {
		t.Fatalf("ClaimBatch após BUSY_SNAPSHOT=(%d,%v,%v)", len(result.rows), result.more, result.err)
	}
	if got := reads.Load(); got < 2 {
		t.Fatalf("leituras da transação=%d; retry deveria abrir snapshot novo", got)
	}
	row, err := f.store.Get(context.Background(), fact.SourceEventID)
	if err != nil || row.DeliveryState != DeliveryProcessing || row.Attempts != 1 {
		t.Fatalf("claim durável duplicado/perdido: state=%q attempts=%d err=%v", row.DeliveryState, row.Attempts, err)
	}
}

func TestRequeueExpiredLeasesRetriesSQLiteBusySnapshot(t *testing.T) {
	f := newSnapshotRetryFixture(t)
	fact := testFact(t, f.now.Add(-time.Minute))
	insertTestFact(t, f.store, fact)
	expired := f.now.Add(-time.Second)
	if err := f.store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Updates(map[string]any{
		"delivery_state": DeliveryProcessing, "lease_owner": "old-owner", "lease_expires_at": expired,
	}).Error; err != nil {
		t.Fatalf("preparar lease vencida: %v", err)
	}
	release, unblock, reads := f.afterFirstRead(t, func(tx *gorm.DB) bool {
		return tx.Statement.Table == (ActivationOutbox{}).TableName() && strings.Contains(tx.Statement.SQL.String(), "lease_expires_at")
	})
	done := make(chan struct {
		processed int
		more      bool
		err       error
	}, 1)
	go func() {
		processed, more, err := f.store.RequeueExpiredLeases(context.Background(), 1)
		done <- struct {
			processed int
			more      bool
			err       error
		}{processed, more, err}
	}()
	joined := false
	t.Cleanup(func() {
		unblock()
		if !joined {
			<-done
		}
	})
	if err := release(); err != nil {
		t.Fatal(err)
	}
	result := <-done
	joined = true
	if result.err != nil || result.processed != 1 || result.more {
		t.Fatalf("RequeueExpiredLeases após BUSY_SNAPSHOT=(%d,%v,%v)", result.processed, result.more, result.err)
	}
	if got := reads.Load(); got < 2 {
		t.Fatalf("leituras da transação=%d; retry deveria abrir snapshot novo", got)
	}
	row, err := f.store.Get(context.Background(), fact.SourceEventID)
	if err != nil || row.DeliveryState != DeliveryPending || row.LeaseOwner != nil || row.LeaseExpiresAt != nil {
		t.Fatalf("requeue não foi único/atômico: %+v err=%v", row, err)
	}
}

func TestRequeueExpiredLeasesErrorAfterWriteReturnsOnlyCommittedCount(t *testing.T) {
	f := newSnapshotRetryFixture(t)
	facts := []Fact{testFact(t, f.now.Add(-time.Minute)), testFact(t, f.now.Add(-time.Minute))}
	sort.Slice(facts, func(i, j int) bool { return facts[i].SourceEventID < facts[j].SourceEventID })
	expired := f.now.Add(-time.Second)
	for _, fact := range facts {
		insertTestFact(t, f.store, fact)
		if err := f.store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Updates(map[string]any{
			"delivery_state": DeliveryProcessing, "lease_owner": "old-owner", "lease_expires_at": expired,
		}).Error; err != nil {
			t.Fatalf("preparar lease vencida %s: %v", fact.SourceEventID, err)
		}
	}
	secondID := facts[1].SourceEventID
	trigger := fmt.Sprintf(`CREATE TRIGGER requeue_test_abort_second BEFORE UPDATE ON command_job_activation_outbox
		WHEN OLD.source_event_id = '%s'
		BEGIN SELECT RAISE(ABORT, 'requeue second row'); END`, secondID)
	if err := f.store.DB().Exec(trigger).Error; err != nil {
		t.Fatalf("criar trigger de falha: %v", err)
	}

	processed, more, err := f.store.RequeueExpiredLeases(context.Background(), 2)
	if err == nil || processed != 0 || more {
		t.Fatalf("lote revertido=(%d,%v,%v), esperado (0,false,erro)", processed, more, err)
	}
	for _, fact := range facts {
		row, getErr := f.store.Get(context.Background(), fact.SourceEventID)
		if getErr != nil || row.DeliveryState != DeliveryProcessing || row.LeaseOwner == nil || *row.LeaseOwner != "old-owner" || row.LeaseExpiresAt == nil || !row.LeaseExpiresAt.Equal(expired) {
			t.Fatalf("rollback parcial da linha %s: %+v err=%v", fact.SourceEventID, row, getErr)
		}
	}
}

func TestPurgeExpiredAtRetriesSQLiteBusySnapshot(t *testing.T) {
	f := newSnapshotRetryFixture(t)
	fact := testFact(t, f.now.Add(-30*time.Minute))
	insertTestFact(t, f.store, fact)
	if err := f.store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Updates(map[string]any{
		"delivery_state":         DeliveryDelivered,
		"source_replay_deadline": f.now.Add(-time.Second),
	}).Error; err != nil {
		t.Fatalf("preparar fonte terminal vencida: %v", err)
	}
	release, unblock, reads := f.afterFirstRead(t, func(tx *gorm.DB) bool {
		return tx.Statement.Table == (ActivationOutbox{}).TableName() && strings.Contains(tx.Statement.SQL.String(), "source_replay_deadline")
	})
	done := make(chan struct {
		processed int
		more      bool
		err       error
	}, 1)
	go func() {
		processed, more, err := f.store.PurgeExpiredAt(context.Background(), f.now, 1)
		done <- struct {
			processed int
			more      bool
			err       error
		}{processed, more, err}
	}()
	joined := false
	t.Cleanup(func() {
		unblock()
		if !joined {
			<-done
		}
	})
	if err := release(); err != nil {
		t.Fatal(err)
	}
	result := <-done
	joined = true
	if result.err != nil || result.processed != 1 || result.more {
		t.Fatalf("PurgeExpiredAt após BUSY_SNAPSHOT=(%d,%v,%v)", result.processed, result.more, result.err)
	}
	if got := reads.Load(); got < 2 {
		t.Fatalf("leituras da transação=%d; retry deveria abrir snapshot novo", got)
	}
	if _, err := f.store.Get(context.Background(), fact.SourceEventID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("purga não removeu a fonte exatamente uma vez: err=%v", err)
	}
}

func TestSQLiteBusyRetryCancellationReturnsNoUncommittedResults(t *testing.T) {
	for _, operation := range []string{"epoch", "claim", "requeue", "purge"} {
		t.Run(operation, func(t *testing.T) {
			f := newSnapshotRetryFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var match func(*gorm.DB) bool
			var invoke func() (int, bool, error)
			var sourceID string
			effectiveAt := f.now.Add(time.Minute)
			switch operation {
			case "epoch":
				match = func(tx *gorm.DB) bool {
					return tx.Statement.Table == (ReplayPolicyEpoch{}).TableName() && strings.Contains(strings.ToUpper(tx.Statement.SQL.String()), "COALESCE(MAX(GENERATION)")
				}
				invoke = func() (int, bool, error) {
					_, err := f.store.EnsureReplayPolicyEpoch(ctx, ProducerType, effectiveAt, time.Hour)
					return 0, false, err
				}
			case "claim", "requeue", "purge":
				fact := testFact(t, f.now.Add(-time.Minute))
				sourceID = fact.SourceEventID
				insertTestFact(t, f.store, fact)
				switch operation {
				case "claim":
					match = func(tx *gorm.DB) bool {
						return tx.Statement.Table == (ActivationOutbox{}).TableName() && strings.Contains(tx.Statement.SQL.String(), "delivery_state")
					}
					invoke = func() (int, bool, error) {
						rows, more, err := f.store.ClaimBatch(ctx, "cancel-owner", 1)
						return len(rows), more, err
					}
				case "requeue":
					expired := f.now.Add(-time.Second)
					if err := f.store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", sourceID).Updates(map[string]any{
						"delivery_state": DeliveryProcessing, "lease_owner": "old-owner", "lease_expires_at": expired,
					}).Error; err != nil {
						t.Fatal(err)
					}
					match = func(tx *gorm.DB) bool {
						return tx.Statement.Table == (ActivationOutbox{}).TableName() && strings.Contains(tx.Statement.SQL.String(), "lease_expires_at")
					}
					invoke = func() (int, bool, error) { return f.store.RequeueExpiredLeases(ctx, 1) }
				case "purge":
					if err := f.store.DB().Model(&ActivationOutbox{}).Where("source_event_id = ?", sourceID).Updates(map[string]any{
						"delivery_state": DeliveryDelivered, "source_replay_deadline": f.now.Add(-time.Second),
					}).Error; err != nil {
						t.Fatal(err)
					}
					match = func(tx *gorm.DB) bool {
						return tx.Statement.Table == (ActivationOutbox{}).TableName() && strings.Contains(tx.Statement.SQL.String(), "source_replay_deadline")
					}
					invoke = func() (int, bool, error) { return f.store.PurgeExpiredAt(ctx, f.now, 1) }
				}
			}
			release, unblock, reads := f.afterFirstRead(t, match)
			cancelOnBusy := func(tx *gorm.DB) {
				if tx.Error != nil && strings.Contains(strings.ToLower(tx.Error.Error()), "locked") {
					cancel()
				}
			}
			hook := fmt.Sprintf("test:cancel-busy:%s:%p", operation, &reads)
			callback := f.store.DB().Callback()
			switch operation {
			case "epoch":
				if err := callback.Create().After("gorm:create").Register(hook, cancelOnBusy); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = callback.Create().Remove(hook) })
			case "purge":
				if err := callback.Delete().After("gorm:delete").Register(hook, cancelOnBusy); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = callback.Delete().Remove(hook) })
			default:
				if err := callback.Update().After("gorm:update").Register(hook, cancelOnBusy); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = callback.Update().Remove(hook) })
			}
			type outcome struct {
				processed int
				more      bool
				err       error
			}
			done := make(chan outcome, 1)
			go func() { processed, more, err := invoke(); done <- outcome{processed, more, err} }()
			joined := false
			t.Cleanup(func() {
				unblock()
				if !joined {
					<-done
				}
			})
			if err := release(); err != nil {
				t.Fatal(err)
			}
			result := <-done
			joined = true
			if !errors.Is(result.err, context.Canceled) || result.processed != 0 || result.more {
				t.Fatalf("resultado cancelado=(%d,%v,%v), esperado (0,false,canceled)", result.processed, result.more, result.err)
			}
			if reads.Load() != 1 {
				t.Fatalf("retry não foi interrompido após a primeira leitura: %d", reads.Load())
			}
			if operation == "epoch" {
				var count int64
				if err := f.store.DB().Model(&ReplayPolicyEpoch{}).Count(&count).Error; err != nil || count != 1 {
					t.Fatalf("epoch não confirmado parcialmente: count=%d err=%v", count, err)
				}
				return
			}
			row, err := f.store.Get(context.Background(), sourceID)
			if err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "claim":
				if row.DeliveryState != DeliveryPending || row.Attempts != 0 {
					t.Fatalf("claim cancelado deixou write: %+v", row)
				}
			case "requeue":
				if row.DeliveryState != DeliveryProcessing || row.LeaseOwner == nil || *row.LeaseOwner != "old-owner" {
					t.Fatalf("requeue cancelado deixou write: %+v", row)
				}
			case "purge":
				if row.DeliveryState != DeliveryDelivered {
					t.Fatalf("purge cancelado removeu/alterou fonte: %+v", row)
				}
			}
		})
	}
}
