package commandui

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"github.com/google/uuid"
)

func testOwner() Owner {
	return Owner{UserID: uuid.NewString(), SessionID: uuid.NewString(), WorkspaceID: "ws-test"}
}

func testInvocation(r Reservation, owner Owner) commandexecution.Invocation {
	return commandexecution.Invocation{ID: r.InvocationID, CommandID: r.CommandID, Principal: auth.LocalSessionPrincipal{UserID: owner.UserID, SessionID: owner.SessionID}}
}

func awaitOutcome(t *testing.T, handle commandexecution.ExecutionHandle) commandexecution.Outcome {
	t.Helper()
	select {
	case outcome := <-handle.Done:
		return outcome
	case <-time.After(time.Second):
		t.Fatal("handle não recebeu outcome")
		return commandexecution.Outcome{}
	}
}

func TestBrokerReserveStartTakeCompleteAndJSON(t *testing.T) {
	b, err := New(2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	owner := testOwner()
	r, err := b.Reserve(owner, "command.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(r.Ticket); err != nil {
		t.Fatalf("ticket não é UUID: %v", err)
	}
	if _, err := uuid.Parse(r.InvocationID); err != nil {
		t.Fatalf("invocation não é UUID: %v", err)
	}
	handle, err := b.Start(context.Background(), owner, testInvocation(r, owner))
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := b.Take(context.Background(), owner, r.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if handoff.InvocationID != r.InvocationID || handoff.CommandID != r.CommandID || handoff.HandoffID == "" {
		t.Fatalf("handoff incompleto: %#v", handoff)
	}
	if err := b.Complete(owner, r.Ticket, handoff.HandoffID, string(commandledger.Succeeded)); err != nil {
		t.Fatal(err)
	}
	outcome := awaitOutcome(t, handle)
	if outcome.Status != commandledger.Succeeded || string(outcome.Result) != "{}" {
		t.Fatalf("outcome incorreto: %#v", outcome)
	}
	if err := b.Complete(owner, r.Ticket, handoff.HandoffID, string(commandledger.Failed)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replay de Complete: %v", err)
	}
}

func TestBrokerCrossOwnerAndInvocationMismatch(t *testing.T) {
	b, _ := New(2, time.Second)
	defer b.Close()
	owner, other := testOwner(), testOwner()
	r, _ := b.Reserve(owner, "command.test")
	if _, err := b.Start(context.Background(), other, testInvocation(r, other)); !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("Start cross-owner: %v", err)
	}
	bad := testInvocation(r, owner)
	bad.CommandID = "command.other"
	if _, err := b.Start(context.Background(), owner, bad); !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("Start command divergente: %v", err)
	}
	if _, err := b.Take(context.Background(), other, r.Ticket); !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("Take cross-owner: %v", err)
	}
	if err := b.Cancel(other, r.Ticket); !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("Cancel cross-owner: %v", err)
	}
}

func TestBrokerTakeBeforeStartBlocksAndUnblocks(t *testing.T) {
	b, _ := New(1, time.Second)
	defer b.Close()
	owner := testOwner()
	r, _ := b.Reserve(owner, "command.test")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	taken := make(chan Handoff, 1)
	errCh := make(chan error, 1)
	go func() { h, err := b.Take(ctx, owner, r.Ticket); taken <- h; errCh <- err }()
	select {
	case <-taken:
		t.Fatal("Take liberou antes de Start")
	case <-time.After(25 * time.Millisecond):
	}
	if _, err := b.Start(context.Background(), owner, testInvocation(r, owner)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Take não desbloqueou")
	}
}

func TestBrokerCancelBeforeTakeIsSafeAndUnblocks(t *testing.T) {
	b, _ := New(1, time.Second)
	defer b.Close()
	owner := testOwner()
	r, _ := b.Reserve(owner, "command.test")
	// A chamada já aguardando deve ser liberada pelo cancelamento. Depois de
	// removida, uma nova chamada não revive nem consulta tombstone.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	takenErr := make(chan error, 1)
	go func() {
		_, takeErr := b.Take(ctx, owner, r.Ticket)
		takenErr <- takeErr
	}()
	select {
	case err := <-takenErr:
		t.Fatalf("Take não deveria concluir antes do cancelamento: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if err := b.Cancel(owner, r.Ticket); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-takenErr:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("Take bloqueado após cancelamento: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Take bloqueado não foi liberado pelo cancelamento")
	}
}

func TestBrokerCancelAfterTakeIsOutcomeUnknown(t *testing.T) {
	b, _ := New(1, time.Second)
	defer b.Close()
	owner := testOwner()
	r, _ := b.Reserve(owner, "command.test")
	handle, _ := b.Start(context.Background(), owner, testInvocation(r, owner))
	handoff, err := b.Take(context.Background(), owner, r.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Cancel(owner, r.Ticket); err != nil {
		t.Fatal(err)
	}
	if got := awaitOutcome(t, handle); got.Status != commandledger.OutcomeUnknown {
		t.Fatalf("status após Take: %s", got.Status)
	}
	if err := b.Complete(owner, r.Ticket, handoff.HandoffID, string(commandledger.Succeeded)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Complete após cancelamento: %v", err)
	}
}

func TestBrokerExpiryAfterTakeSignalsUnknownAndRejectsLateComplete(t *testing.T) {
	b, _ := New(1, 20*time.Millisecond)
	defer b.Close()
	owner := testOwner()
	r, _ := b.Reserve(owner, "command.test")
	handle, err := b.Start(context.Background(), owner, testInvocation(r, owner))
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := b.Take(context.Background(), owner, r.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	outcome := awaitOutcome(t, handle)
	if outcome.Status != commandledger.OutcomeUnknown {
		t.Fatalf("expiração após Take: %s", outcome.Status)
	}
	if err := b.Complete(owner, r.Ticket, handoff.HandoffID, string(commandledger.Succeeded)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Complete tardio após expiração: %v", err)
	}
}

func TestBrokerStartContextCancelPreventsTakeAndComplete(t *testing.T) {
	b, _ := New(2, time.Second)
	defer b.Close()
	owner := testOwner()
	r, _ := b.Reserve(owner, "command.test")
	startCtx, cancel := context.WithCancel(context.Background())
	handle, err := b.Start(startCtx, owner, testInvocation(r, owner))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := b.Take(context.Background(), owner, r.Ticket); !errors.Is(err, ErrCancelled) {
		t.Fatalf("Take após cancelamento do executionCtx: %v", err)
	}
	if got := awaitOutcome(t, handle); got.Status != commandledger.Cancelled {
		t.Fatalf("status de handoff não consumido: %s", got.Status)
	}

	r, _ = b.Reserve(owner, "command.test")
	startCtx, cancel = context.WithCancel(context.Background())
	handle, _ = b.Start(startCtx, owner, testInvocation(r, owner))
	handoff, _ := b.Take(context.Background(), owner, r.Ticket)
	cancel()
	if err := b.Complete(owner, r.Ticket, handoff.HandoffID, string(commandledger.Succeeded)); !errors.Is(err, commandledger.ErrInconsistent) {
		t.Fatalf("Complete após cancelamento do executionCtx: %v", err)
	}
	if got := awaitOutcome(t, handle); got.Status != commandledger.OutcomeUnknown {
		t.Fatalf("status de Complete inconclusivo: %s", got.Status)
	}
}

func TestBrokerExpiryAndCloseRemainBounded(t *testing.T) {
	b, _ := New(2, 20*time.Millisecond)
	owner := testOwner()
	r, _ := b.Reserve(owner, "command.test")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := b.Take(ctx, owner, r.Ticket); !errors.Is(err, ErrExpired) {
		t.Fatalf("Take expirado: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := b.Reserve(owner, "command.test"); err != nil {
			t.Fatalf("reserva após expiração %d: %v", i, err)
		}
	}
	if _, err := b.Reserve(owner, "command.test"); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("limite não aplicado: %v", err)
	}
	b.Close()
	b.Close()
}

func TestBrokerCloseSignalsTakenAsUnknown(t *testing.T) {
	b, _ := New(1, time.Second)
	owner := testOwner()
	r, _ := b.Reserve(owner, "command.test")
	handle, _ := b.Start(context.Background(), owner, testInvocation(r, owner))
	if _, err := b.Take(context.Background(), owner, r.Ticket); err != nil {
		t.Fatal(err)
	}
	if err := b.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := b.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := awaitOutcome(t, handle); got.Status != commandledger.OutcomeUnknown {
		t.Fatalf("shutdown após Take: %s", got.Status)
	}
	if _, err := b.Start(context.Background(), owner, testInvocation(r, owner)); !errors.Is(err, ErrClosed) && !errors.Is(err, ErrNotFound) {
		t.Fatalf("Start após Close: %v", err)
	}
}
