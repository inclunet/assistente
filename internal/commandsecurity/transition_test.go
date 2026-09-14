package commandsecurity

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestBeginTransitionNestingAndIdempotentFinish(t *testing.T) {
	service := newEpochServiceForTest(t)
	ctx := context.Background()
	user, session := testEpochID(t), testEpochID(t)

	old, err := service.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}
	finishOuter, err := service.BeginTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	finishInner, err := service.BeginTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Capture(ctx, user, session); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("Capture durante transições aninhadas = %v, want ErrStaleEpoch", err)
	}
	finishInner()
	finishInner()
	if _, err := service.Capture(ctx, user, session); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("finish interno reabriu a barreira = %v", err)
	}

	finishOuter()
	finishOuter()
	fresh, err := service.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}
	if fresh == old {
		t.Fatalf("snapshot fresco não mudou após transição: antigo=%#v novo=%#v", old, fresh)
	}
	if fresh.AuthGeneration == old.AuthGeneration || fresh.SecurityGeneration == old.SecurityGeneration {
		t.Fatalf("transição não rotacionou as gerações: antigo=%#v novo=%#v", old, fresh)
	}
}

func TestBeginTransitionLeavesGateAvailableButRejectsEpochOperations(t *testing.T) {
	service := newEpochServiceForTest(t)
	ctx := context.Background()
	user, session := testEpochID(t), testEpochID(t)
	old, err := service.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}

	finish, err := service.BeginTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer finish()

	if !service.gate.mu.TryLock() {
		t.Fatal("gate permaneceu retido durante a transição")
	}
	service.gate.mu.Unlock()

	callbacks := 0
	if err := service.Admit(ctx, old, func(context.Context) error {
		callbacks++
		return nil
	}, func() error {
		callbacks++
		return nil
	}); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("Admit durante transição = %v, want ErrStaleEpoch", err)
	}
	if callbacks != 0 {
		t.Fatalf("callbacks executados para snapshot stale: %d", callbacks)
	}
}

func TestBeginTransitionInvalidatesOldSnapshotAndClearsSessions(t *testing.T) {
	service := newEpochServiceForTest(t)
	ctx := context.Background()
	user, session := testEpochID(t), testEpochID(t)
	old, err := service.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}

	finish, err := service.BeginTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	finish()

	called := false
	if err := service.Admit(ctx, old, func(context.Context) error {
		called = true
		return nil
	}, func() error {
		called = true
		return nil
	}); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("snapshot antigo após transição = %v, want ErrStaleEpoch", err)
	}
	if called {
		t.Fatal("snapshot antigo foi admitido")
	}

	fresh, err := service.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.AuthGeneration == old.AuthGeneration || fresh.SecurityGeneration == old.SecurityGeneration {
		t.Fatalf("sessão não foi limpa/segurança não avançou: antigo=%#v novo=%#v", old, fresh)
	}
}

func TestBeginTransitionCancelledBeforeStartHasNoEffect(t *testing.T) {
	service := newEpochServiceForTest(t)
	ctx := context.Background()
	user, session := testEpochID(t), testEpochID(t)
	old, err := service.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	finish, err := service.BeginTransition(cancelled)
	if finish != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("BeginTransition cancelado = (finish=%v, err=%v)", finish != nil, err)
	}
	fresh, err := service.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}
	if fresh != old {
		t.Fatalf("cancelamento antes do início alterou o snapshot: antigo=%#v novo=%#v", old, fresh)
	}
}

func TestBeginTransitionFinishUsesBackgroundAfterContextCancellation(t *testing.T) {
	service := newEpochServiceForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	finish, err := service.BeginTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	finish()

	if _, err := service.Capture(context.Background(), testEpochID(t), testEpochID(t)); err != nil {
		t.Fatalf("finish não encerrou a barreira após cancelamento do contexto: %v", err)
	}
}

func TestBeginTransitionOverflowFailsClosedAndRejectsOldPrincipal(t *testing.T) {
	service := newEpochServiceForTest(t)
	ctx := context.Background()
	user, session := testEpochID(t), testEpochID(t)
	old, err := service.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}
	service.sequence = math.MaxUint64

	if finish, err := service.BeginTransition(ctx); finish != nil || !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatalf("overflow em BeginTransition = (finish=%v, err=%v)", finish != nil, err)
	}
	if _, err := service.Capture(ctx, user, session); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("Capture após overflow = %v, want ErrStaleEpoch", err)
	}
	if err := service.Admit(ctx, old, func(context.Context) error {
		t.Fatal("snapshot principal antigo admitido após overflow")
		return nil
	}, func() error {
		t.Fatal("handoff do principal antigo após overflow")
		return nil
	}); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("Admit do principal antigo após overflow = %v, want ErrStaleEpoch", err)
	}
}
