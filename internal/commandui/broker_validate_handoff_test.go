package commandui

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBrokerValidateHandoffReadOnlyAndExactErrors(t *testing.T) {
	t.Run("untaken", func(t *testing.T) {
		b, owner, reservation, _ := validateHandoffReservation(t, time.Second)
		defer b.Close()
		if err := b.ValidateHandoff(owner, reservation.Ticket, "handoff"); !errors.Is(err, ErrNotStarted) {
			t.Fatalf("erro = %v", err)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		b, owner, _, handoff := validateHandoffFixture(t, time.Second)
		defer b.Close()
		if err := b.ValidateHandoff(owner, handoff.Ticket, "wrong-handoff"); !errors.Is(err, ErrOwnerMismatch) {
			t.Fatalf("erro = %v", err)
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		b, owner, _, handoff := validateHandoffFixture(t, time.Second)
		defer b.Close()
		other := testOwner()
		if err := b.ValidateHandoff(other, handoff.Ticket, handoff.HandoffID); !errors.Is(err, ErrOwnerMismatch) {
			t.Fatalf("erro = %v", err)
		}
		_ = owner
	})

	t.Run("expired", func(t *testing.T) {
		b, owner, reservation, _ := validateHandoffReservation(t, time.Millisecond)
		defer b.Close()
		time.Sleep(10 * time.Millisecond)
		if err := b.ValidateHandoff(owner, reservation.Ticket, "handoff"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("erro = %v", err)
		}
	})

	t.Run("cancelled start context", func(t *testing.T) {
		b, owner, reservation, _ := validateHandoffReservation(t, time.Second)
		defer b.Close()
		startCtx, cancel := context.WithCancel(context.Background())
		if _, err := b.Start(startCtx, owner, testInvocation(reservation, owner)); err != nil {
			t.Fatal(err)
		}
		cancel()
		if err := b.ValidateHandoff(owner, reservation.Ticket, "not-taken"); !errors.Is(err, ErrNotStarted) {
			t.Fatalf("untaken deve ser avaliado antes do contexto: %v", err)
		}
	})
}

func TestBrokerValidateHandoffDoesNotClaimAndRejectsCommittingOrReplay(t *testing.T) {
	b, owner, reservation, handoff, _ := commitFixture(t, time.Second)
	defer b.Close()

	entered := make(chan struct{})
	release := make(chan struct{})
	commitDone := make(chan error, 1)
	go func() {
		commitDone <- b.Commit(context.Background(), owner, reservation.Ticket, handoff.HandoffID, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	awaitCommitEntered(t, entered)
	if err := b.ValidateHandoff(owner, reservation.Ticket, handoff.HandoffID); !errors.Is(err, ErrAlreadyCommitting) {
		t.Fatalf("commit em andamento: %v", err)
	}
	close(release)
	if err := <-commitDone; err != nil {
		t.Fatal(err)
	}
	if err := b.ValidateHandoff(owner, reservation.Ticket, handoff.HandoffID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replay após commit: %v", err)
	}
}

func validateHandoffReservation(t *testing.T, ttl time.Duration) (*Broker, Owner, Reservation, Handoff) {
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
	return b, owner, reservation, Handoff{Ticket: reservation.Ticket, HandoffID: "handoff"}
}

func validateHandoffFixture(t *testing.T, ttl time.Duration) (*Broker, Owner, Reservation, Handoff) {
	t.Helper()
	b, owner, reservation, _ := validateHandoffReservation(t, ttl)
	if _, err := b.Start(context.Background(), owner, testInvocation(reservation, owner)); err != nil {
		b.Close()
		t.Fatal(err)
	}
	handoff, err := b.Take(context.Background(), owner, reservation.Ticket)
	if err != nil {
		b.Close()
		t.Fatal(err)
	}
	return b, owner, reservation, handoff
}
