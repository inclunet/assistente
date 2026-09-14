package commandsecurity

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"
)

func TestEpochAdmissionKeepsSameGateThroughRevalidationAndHandoff(t *testing.T) {
	gate := &DispatchGate{}
	service, err := NewEpochService(gate)
	if err != nil {
		t.Fatal(err)
	}
	user, session := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	snapshot, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	checkLock := func() {
		t.Helper()
		if gate.mu.TryLock() {
			gate.mu.Unlock()
			t.Fatal("admissão fora do gate compartilhado")
		}
	}
	checked, started := false, false
	err = service.Admit(context.Background(), snapshot, func(context.Context) error { checkLock(); checked = true; return nil }, func() error {
		checkLock()
		if !checked {
			t.Fatal("handoff antes de revalidar")
		}
		started = true
		return nil
	})
	if err != nil || !started {
		t.Fatal(err)
	}
	if !gate.mu.TryLock() {
		t.Fatal("gate não liberado após handoff")
	}
	gate.mu.Unlock()
}

func TestEpochInvalidationDoesNotResurrectAndOverflowFailsAtomically(t *testing.T) {
	service, err := NewEpochService(&DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	user, session := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	old, err := service.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}
	service.sequence = math.MaxUint64 // fixture single-thread; simula exaustão do contador.
	if err := service.InvalidatePrincipal(ctx, user, session); !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatal(err)
	}
	unchanged, err := service.Capture(ctx, user, session)
	if err != nil || unchanged != old {
		t.Fatal("invalidação parcialmente aplicada")
	}
	if err := service.InvalidateSession(ctx, user, session); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Capture(ctx, user, session); !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatal("contador reiniciou")
	}
	if err := service.Admit(ctx, old, func(context.Context) error { return nil }, func() error { t.Fatal("snapshot ressuscitado"); return nil }); !errors.Is(err, ErrStaleEpoch) {
		t.Fatal(err)
	}
}
