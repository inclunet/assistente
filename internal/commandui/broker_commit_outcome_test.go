package commandui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandledger"
)

func TestBrokerCommitOutcomeUnknownPreservesJoinedErrorAndRejectsReplay(t *testing.T) {
	b, owner, reservation, handoff, handle := commitFixture(t, time.Second)
	defer b.Close()

	partialErr := errors.New("persistência posterior falhou")
	applyErr := fmt.Errorf("efeito parcial: %w", errors.Join(ErrOutcomeUnknown, partialErr))
	var calls atomic.Int32

	err := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
		calls.Add(1)
		return applyErr
	})
	if !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, partialErr) {
		t.Fatalf("erro do commit não preservou sentinel e causa: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("callback executado %d vezes", calls.Load())
	}
	if got := awaitOutcome(t, handle); got.Status != commandledger.OutcomeUnknown {
		t.Fatalf("outcome: %s", got.Status)
	}

	if replayErr := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
		calls.Add(1)
		return nil
	}); !errors.Is(replayErr, ErrNotFound) {
		t.Fatalf("replay após outcome desconhecido: %v", replayErr)
	}
	if cancelErr := b.Cancel(owner, reservation.Ticket); !errors.Is(cancelErr, ErrNotFound) {
		t.Fatalf("cancel após outcome desconhecido: %v", cancelErr)
	}
	if calls.Load() != 1 {
		t.Fatalf("callback reexecutado após terminalização: %d", calls.Load())
	}
}

func TestBrokerCommitOutcomeUnknownWrappedSentinelIsNotFailed(t *testing.T) {
	b, owner, reservation, handoff, handle := commitFixture(t, time.Second)
	defer b.Close()

	err := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
		return fmt.Errorf("commit incerto: %w", ErrOutcomeUnknown)
	})
	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("sentinel não preservado: %v", err)
	}
	if got := awaitOutcome(t, handle); got.Status != commandledger.OutcomeUnknown {
		t.Fatalf("outcome: %s", got.Status)
	}
}

func TestBrokerCommitCancelledRecordsCancelledAndDoesNotReplay(t *testing.T) {
	b, owner, reservation, handoff, handle := commitFixture(t, time.Second)
	defer b.Close()

	err := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
		return ErrCancelled
	})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("erro de cancelamento: %v", err)
	}
	if got := awaitOutcome(t, handle); got.Status != commandledger.Cancelled {
		t.Fatalf("outcome do cancelamento: %s", got.Status)
	}
	if replayErr := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
		t.Fatal("callback executado no replay")
		return nil
	}); !errors.Is(replayErr, ErrNotFound) {
		t.Fatalf("replay após cancelamento: %v", replayErr)
	}
}

func TestBrokerCommitPanicTerminalizaComoOutcomeUnknownSemExporPanic(t *testing.T) {
	b, owner, reservation, handoff, handle := commitFixture(t, time.Second)
	defer b.Close()

	const secret = "conteúdo privado do callback"
	err := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
		panic(secret)
	})
	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("panic não virou outcome desconhecido: %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("panic vazou no erro retornado: %v", err)
	}
	if got := awaitOutcome(t, handle); got.Status != commandledger.OutcomeUnknown {
		t.Fatalf("outcome do panic: %s", got.Status)
	}
	if replayErr := b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
		t.Fatal("callback executado no replay após panic")
		return nil
	}); !errors.Is(replayErr, ErrNotFound) {
		t.Fatalf("replay após panic: %v", replayErr)
	}
}
