package commandexecution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandledger"
	"gorm.io/gorm"
)

func TestServiceConcurrentReplaysObserveRunningWithoutNewHandoff(t *testing.T) {
	f := newExecutionFixture(t)
	sqlDB, err := f.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1) // Serializa SQLite, não o fluxo concorrente do serviço.
	f.service.config.ExecutionTimeout = 10 * time.Second
	started := make(chan struct{})
	done := make(chan Outcome, 1)
	f.startHook = func(context.Context, Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: "concurrent", Done: done, Cancel: func() {}}, nil
	}
	request := f.request(t, "workspace.read")
	result := make(chan error, 1)
	go func() {
		r, e := f.service.Execute(context.Background(), f.pair.AccessToken, request)
		if e == nil && r.Status != commandledger.Succeeded {
			e = errors.New("primeira execução não concluiu")
		}
		result <- e
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("Start não iniciou")
	}
	var group sync.WaitGroup
	replays := make(chan error, 6)
	for range 6 {
		group.Add(1)
		go func() {
			defer group.Done()
			r, e := f.service.Execute(context.Background(), f.pair.AccessToken, request)
			if e == nil && r.Status != commandledger.Running {
				e = errors.New("replay não retornou running")
			}
			replays <- e
		}()
	}
	group.Wait()
	close(replays)
	for err := range replays {
		if err != nil {
			t.Error(err)
		}
	}
	done <- Outcome{Status: commandledger.Succeeded}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("primeira execução não concluiu")
	}
	if f.startCalls.Load() != 1 {
		t.Fatal("replay repetiu Start")
	}
}

func TestServiceFailedQueueCommitNeverStartsOrOverwritesLedger(t *testing.T) {
	f := newExecutionFixture(t)
	request := f.request(t, "workspace.read")
	sentinel := errors.New("fixture commit failure")
	if err := f.db.Callback().Update().Before("gorm:update").Register("fixture:fail-queue", func(tx *gorm.DB) {
		if tx.Statement.Table == "command_idempotency_keys" {
			tx.AddError(sentinel)
		}
	}); err != nil {
		t.Fatal(err)
	}
	record, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
	if !errors.Is(err, sentinel) || record != (commandledger.Record{}) {
		t.Fatalf("%+v %v", record, err)
	}
	if f.startCalls.Load() != 0 {
		t.Fatal("Start após falha de persistência")
	}
	if f.record(t, request).Status != commandledger.Evaluating {
		t.Fatal("CAS falho alterou ledger")
	}
	if err := f.db.Callback().Update().Remove("fixture:fail-queue"); err != nil {
		t.Fatal(err)
	}
	replayed, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
	if err != nil || replayed.Status != commandledger.Evaluating || f.startCalls.Load() != 0 {
		t.Fatal("replay retomou reserva abandonada", replayed, err)
	}
}

func TestServicePanicBeforeStartPersistsFailed(t *testing.T) {
	for _, at := range []int{1, 2} {
		t.Run(string(rune('0'+at)), func(t *testing.T) {
			f := newExecutionFixture(t)
			f.authorizeHook = func(call int) error {
				if call == at {
					panic("fixture policy panic")
				}
				return nil
			}
			request := f.request(t, "workspace.read")
			record, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
			if err != nil || record.Status != commandledger.Failed || f.startCalls.Load() != 0 {
				t.Fatalf("%+v %v", record, err)
			}
		})
	}
}
