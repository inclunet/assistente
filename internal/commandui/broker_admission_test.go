package commandui

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandledger"
)

func TestTakeValidatedRefusesBeforeConsumingHandoff(t *testing.T) {
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
	h, err := b.Start(context.Background(), owner, testInvocation(r, owner))
	if err != nil {
		t.Fatal(err)
	}
	denied := errors.New("configuration changed")
	if _, err := b.TakeValidated(context.Background(), owner, r.Ticket, func() error {
		// Reentrancy proves validation is outside the broker mutex.
		other, err := b.Reserve(owner, "command.other")
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Cancel(owner, other.Ticket); err != nil {
			t.Fatal(err)
		}
		return denied
	}); !errors.Is(err, denied) {
		t.Fatalf("Take: %v", err)
	}
	if outcome := awaitOutcome(t, h); outcome.Status != commandledger.Cancelled {
		t.Fatalf("no handoff delivered: %+v", outcome)
	}
}
