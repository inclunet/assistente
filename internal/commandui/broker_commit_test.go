package commandui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
)

func commitFixture(t *testing.T, ttl time.Duration) (*Broker, Owner, Reservation, Handoff, commandexecution.ExecutionHandle) {
	t.Helper()
	b, err := New(2, ttl)
	if err != nil {
		t.Fatal(err)
	}
	owner := testOwner()
	reservation, err := b.Reserve(owner, "command.test")
	if err != nil {
		b.Close()
		t.Fatal(err)
	}
	handle, err := b.Start(context.Background(), owner, testInvocation(reservation, owner))
	if err != nil {
		b.Close()
		t.Fatal(err)
	}
	handoff, err := b.Take(context.Background(), owner, reservation.Ticket)
	if err != nil {
		b.Close()
		t.Fatal(err)
	}
	return b, owner, reservation, handoff, handle
}

func awaitCommitEntered(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Commit não entrou no callback")
	}
}

func TestBrokerCommitClaimOnceConcurrentAndReplay(t *testing.T) {
	b, owner, reservation, handoff, handle := commitFixture(t, time.Second)
	defer b.Close()
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
			calls.Add(1)
			close(entered)
			<-release
			return nil
		})
	}()
	awaitCommitEntered(t, entered)
	if err := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
		calls.Add(1)
		return nil
	}); !errors.Is(err, ErrAlreadyCommitting) {
		t.Fatalf("segunda claim durante commit: %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("primeiro commit: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("apply executado %d vezes", calls.Load())
	}
	if got := awaitOutcome(t, handle); got.Status != commandledger.Succeeded {
		t.Fatalf("outcome do commit: %s", got.Status)
	}
	if err := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replay após commit: %v", err)
	}
}

func TestBrokerCommitWrongHandoffNaoAplica(t *testing.T) {
	b, owner, reservation, handoff, _ := commitFixture(t, time.Second)
	defer b.Close()
	var calls atomic.Int32
	if err := b.Commit(context.Background(), owner, reservation.Ticket, "wrong-handoff", func(context.Context) error {
		calls.Add(1)
		return nil
	}); !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("handoff divergente: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("callback executado para handoff divergente")
	}
	if err := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("handoff correto após rejeição: %v", err)
	}
}

func TestBrokerCompleteECancelDuranteCommitNaoTerminalizam(t *testing.T) {
	b, owner, reservation, handoff, _ := commitFixture(t, time.Second)
	defer b.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	awaitCommitEntered(t, entered)
	if err := b.Complete(owner, reservation.Ticket, handoff.HandoffID, string(commandledger.Succeeded)); !errors.Is(err, ErrAlreadyCommitting) {
		t.Fatalf("Complete durante commit: %v", err)
	}
	if err := b.Cancel(owner, reservation.Ticket); !errors.Is(err, ErrAlreadyCommitting) {
		t.Fatalf("Cancel durante commit: %v", err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("commit após tentativas não terminais: %v", err)
	}
}

func TestBrokerCloseDuranteCommitPreservaResultadoReal(t *testing.T) {
	b, owner, reservation, handoff, handle := commitFixture(t, time.Second)
	entered := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	awaitCommitEntered(t, entered)
	b.Close()
	select {
	case <-time.After(30 * time.Millisecond):
	case <-release:
		t.Fatal("release fechada inesperadamente")
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("Commit após Close: %v", err)
	}
	if outcome := awaitOutcome(t, handle); outcome.Status != commandledger.Succeeded {
		t.Fatalf("outcome após Close durante commit: %s", outcome.Status)
	}
}

func TestBrokerExpiryDuranteCommitNaoTerminalizaEConservaResultado(t *testing.T) {
	b, owner, reservation, handoff, handle := commitFixture(t, 20*time.Millisecond)
	defer b.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	awaitCommitEntered(t, entered)
	select {
	case outcome := <-handle.Done:
		t.Fatalf("expiry terminalizou commit ativo: %s", outcome.Status)
	case <-time.After(60 * time.Millisecond):
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("Commit após expiry: %v", err)
	}
	if outcome := awaitOutcome(t, handle); outcome.Status != commandledger.Succeeded {
		t.Fatalf("outcome após expiry durante commit: %s", outcome.Status)
	}
}

func TestBrokerCommitPropagaErroRealDoApply(t *testing.T) {
	b, owner, reservation, handoff, handle := commitFixture(t, time.Second)
	defer b.Close()
	realErr := errors.New("persistência falhou")
	if err := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error { return realErr }); !errors.Is(err, realErr) {
		t.Fatalf("erro real do apply não propagado: %v", err)
	}
	if got := awaitOutcome(t, handle); got.Status != commandledger.Failed {
		t.Fatalf("outcome do erro real: %s", got.Status)
	}
}
