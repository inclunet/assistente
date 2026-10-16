package commandledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcontract"
	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestEnvelopeBusyWriterReleaseAndCancel(t *testing.T) {
	for _, cas := range []bool{false, true} {
		for _, cancelRun := range []bool{false, true} {
			name := "reserve"
			if cas {
				name = "cas"
			}
			if cancelRun {
				name += "_cancel"
			}
			t.Run(name, func(t *testing.T) {
				req, now := envelopeRequest(t)
				s, db := testStore(t, &now)
				owner := ownershipFromEnvelope(req.Envelope)
				if cas {
					if _, err := s.ReserveEnvelope(context.Background(), req); err != nil {
						t.Fatal(err)
					}
				}
				pool, err := db.DB()
				if err != nil {
					t.Fatal(err)
				}
				pool.SetMaxOpenConns(2)
				pool.SetMaxIdleConns(2)
				writer, err := pool.Conn(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := writer.Close(); err != nil {
						t.Error(err)
					}
				}()
				// Configure the other physical connection while the writer is pinned.
				if err := db.Exec("PRAGMA busy_timeout=0").Error; err != nil {
					t.Fatal(err)
				}
				if _, err := writer.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
					t.Fatal(err)
				}
				locked := true
				defer func() {
					if locked {
						if _, err := writer.ExecContext(context.Background(), "ROLLBACK"); err != nil {
							t.Error(err)
						}
					}
				}()
				busy := make(chan struct{}, 1)
				var ids []string
				observe := func(tx *gorm.DB) {
					if row, ok := tx.Statement.Dest.(*ledgerRow); ok {
						ids = append(ids, row.ID)
					}
					if database.IsSQLiteBusyError(tx.Error) {
						select {
						case busy <- struct{}{}:
						default:
						}
					}
				}
				if cas {
					if err := db.Callback().Update().After("gorm:update").Register("test:busy", observe); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := db.Callback().Create().After("gorm:create").Register("test:busy", observe); err != nil {
						t.Fatal(err)
					}
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					if cas {
						changed, err := s.CompareAndSwapEnvelope(ctx, owner, req.Envelope.InvocationID, Evaluating, Denied)
						if err == nil && !changed {
							err = errors.New("CAS não alterou")
						}
						done <- err
					} else {
						result, err := s.ReserveEnvelope(ctx, req)
						if err == nil && (!result.Created || result.Record.ArgumentsSummary != redactedDocument) {
							err = errors.New("reserva/redaction inválida")
						}
						done <- err
					}
				}()
				select {
				case <-busy:
				case <-time.After(5 * time.Second):
					t.Fatal("writer não provocou SQLITE_BUSY")
				}
				if cancelRun {
					cancel()
				} else {
					if _, err := writer.ExecContext(context.Background(), "ROLLBACK"); err != nil {
						t.Fatal(err)
					}
					locked = false
				}
				select {
				case err := <-done:
					if cancelRun {
						if !errors.Is(err, context.Canceled) {
							t.Fatalf("cancelamento: %v", err)
						}
					} else if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("retry não terminou")
				}
				if locked {
					if _, err := writer.ExecContext(context.Background(), "ROLLBACK"); err != nil {
						t.Fatal(err)
					}
					locked = false
				}
				if !cas && !cancelRun {
					if len(ids) < 2 {
						t.Fatalf("faltou retry: %v", ids)
					}
					for _, id := range ids {
						if id != ids[0] {
							t.Fatal("UUID mudou entre tentativas")
						}
					}
				}
				for _, table := range []string{"command_idempotency_keys", "command_invocations"} {
					var rows []struct{ Status Status }
					if err := db.Table(table).Select("status").Scan(&rows).Error; err != nil {
						t.Fatal(err)
					}
					if cancelRun && !cas {
						if len(rows) != 0 {
							t.Fatal("cancelamento deixou reserva")
						}
						continue
					}
					want := Evaluating
					if cas && !cancelRun {
						want = Denied
					}
					if len(rows) != 1 || rows[0].Status != want {
						t.Fatalf("%s: %+v want=%s", table, rows, want)
					}
				}
			})
		}
	}
}

func TestEnvelopeNestedTransactionDoesNotRetryBusy(t *testing.T) {
	req, now := envelopeRequest(t)
	_, db := testStore(t, &now)
	calls := 0
	if err := db.Callback().Create().Before("gorm:create").Register("test:busy_nested", func(tx *gorm.DB) {
		calls++
		_ = tx.AddError(errors.New("SQLITE_BUSY")) // Erro injetado no estado transacional do GORM.
	}); err != nil {
		t.Fatal(err)
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		s, err := New(tx, func() time.Time { return now })
		if err != nil {
			return err
		}
		_, err = s.ReserveEnvelope(context.Background(), req)
		return err
	})
	if !errors.Is(err, ErrInconsistent) || calls != 1 {
		t.Fatalf("nested err=%v attempts=%d", err, calls)
	}
}

// A falha é injetada depois do INSERT/UPDATE real, forçando rollback. O relógio
// avança no mesmo callback, antes do próximo retry: não depende de sleeps.
func TestEnvelopeRetryRefreshesClock(t *testing.T) {
	for _, mode := range []string{"direct", "event", "event_existing", "cas"} {
		t.Run(mode, func(t *testing.T) {
			req, now := envelopeRequest(t)
			deadline := req.ExpiresAt
			if mode == "event" || mode == "event_existing" {
				req.Envelope.SourceType = sourcePtr(commandcontract.SourceEvent)
				req.Envelope.SourceInstanceID = stringPtr(uuid.Must(uuid.NewV7()).String())
				req.Envelope.SourceEventID = stringPtr(uuid.Must(uuid.NewV7()).String())
				req.Envelope.ObserverType = stringPtr("event")
				req.Envelope.SourceOccurredAt = timePtr(now.Add(-time.Second))
				req.Envelope.SourceReplayDeadline = &deadline
				req.Envelope.SourceReplayPolicyGeneration = stringPtr("replay-1")
				req.Envelope.Provenance = rawPtr(`{"version":1,"_source":"fixture"}`)
			}
			s, db := testStore(t, &now)
			var original EnvelopeReservation
			if mode == "cas" || mode == "event_existing" {
				var err error
				original, err = s.ReserveEnvelope(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
			}
			attempts := 0
			inject := func(tx *gorm.DB) {
				if tx.Statement.Table != "command_idempotency_keys" {
					return
				}
				attempts++
				if attempts == 1 {
					now = deadline.Add(time.Second)
					_ = tx.AddError(errors.New("SQLITE_BUSY")) // Força rollback pelo estado do GORM.
				}
			}
			if mode == "cas" {
				if err := db.Callback().Update().After("gorm:update").Register("test:clock", inject); err != nil {
					t.Fatal(err)
				}
				ok, err := s.CompareAndSwapEnvelope(context.Background(), ownershipFromEnvelope(req.Envelope), req.Envelope.InvocationID, Evaluating, Denied)
				if err != nil || !ok || attempts != 2 {
					t.Fatalf("CAS ok=%v err=%v attempts=%d", ok, err, attempts)
				}
				var audit invocationRow
				if err := db.First(&audit).Error; err != nil {
					t.Fatal(err)
				}
				if audit.CompletedAt == nil || !audit.CompletedAt.Equal(now) {
					t.Fatalf("completed_at=%v want=%v", audit.CompletedAt, now)
				}
				return
			}
			if err := db.Callback().Create().After("gorm:create").Register("test:clock", inject); err != nil {
				t.Fatal(err)
			}
			got, err := s.ReserveEnvelope(context.Background(), req)
			if mode == "direct" {
				if !errors.Is(err, ErrExpired) || got.Created {
					t.Fatalf("reserva vencida: %+v %v", got, err)
				}
				for _, table := range []string{"command_idempotency_keys", "command_invocations"} {
					var count int64
					if err := db.Table(table).Count(&count).Error; err != nil || count != 0 {
						t.Fatalf("%s count=%d err=%v", table, count, err)
					}
				}
				return
			}
			if err != nil || attempts != 2 {
				t.Fatalf("evento err=%v attempts=%d", err, attempts)
			}
			if mode == "event_existing" {
				if got.Created || got.Record.ID != original.Record.ID || got.Record.Status != original.Record.Status {
					t.Fatalf("replay existente mudou: %+v", got)
				}
			} else {
				if !got.Created || got.Record.Status != RejectedStale || !got.Record.ExpiresAt.Equal(deadline) {
					t.Fatalf("evento expirado: %+v", got)
				}
				var audits int64
				if err := db.Table("command_invocations").Count(&audits).Error; err != nil || audits != 0 {
					t.Fatalf("audits=%d err=%v", audits, err)
				}
			}
		})
	}
}
