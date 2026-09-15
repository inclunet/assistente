package commandsecurity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDrainSealsOnlyIssuedGenerationsAfterAllParticipants(t *testing.T) {
	gate := &DispatchGate{}
	core, err := NewEpochService(gate)
	if err != nil {
		t.Fatal(err)
	}
	first := core.security
	finish, err := core.BeginTransition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	finish()
	second := core.security
	blocked := true
	failure := errors.New("finalization pending")
	var calls int
	if err := core.RegisterExecutorDrain(context.Background(), func(ctx context.Context) error {
		// Uma aquisição do mesmo gate aqui só termina se a espera estiver fora dele.
		if err := gate.WithMutation(ctx, func() error { calls++; return nil }); err != nil {
			return err
		}
		if blocked {
			return failure
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if proof, err := core.CloseAndDrain(context.Background()); !errors.Is(err, failure) || proof.Includes(first) {
		t.Fatalf("prova prematura: %+v %v", proof, err)
	}
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("novo executor=%v", err)
	}
	blocked = false
	proof, err := core.CloseAndDrain(context.Background())
	if err != nil || !proof.Includes(first) || !proof.Includes(second) || proof.Includes("foreign") {
		t.Fatalf("proof=%+v err=%v", proof, err)
	}
	if calls != 2 {
		t.Fatalf("drains=%d", calls)
	}
	if _, err := core.BeginTransition(context.Background()); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("core reaberto=%v", err)
	}
}

func TestClosedCoreRejectsMutationCallbacksAndWatches(t *testing.T) {
	core := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	ctx := context.Background()
	snapshot, err := core.Capture(ctx, user, session)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.CloseAndDrain(ctx); err != nil {
		t.Fatal(err)
	}
	action := func() error { t.Error("callback após fechamento"); return nil }
	validate := func(context.Context) error { t.Error("validação após fechamento"); return nil }
	for name, call := range map[string]func() error{
		"session":   func() error { return core.MutateSession(ctx, user, session, action) },
		"principal": func() error { return core.MutatePrincipal(ctx, user, session, action) },
		"security":  func() error { return core.MutateSecurity(ctx, action) },
		"config":    func() error { return core.MutateUserConfiguration(ctx, user, action) },
		"context": func() error {
			return core.MutateContext(ctx, ContextPrincipal{UserID: user, Type: "local_session", ID: session}, action)
		},
		"admit":       func() error { return core.Admit(ctx, snapshot, validate, action) },
		"mutation":    func() error { return core.AdmitMutation(ctx, snapshot, validate, action) },
		"publication": func() error { return core.PublishAuthenticatedConfiguration(ctx, snapshot, validate, action) },
	} {
		if err := call(); !errors.Is(err, ErrStaleEpoch) {
			t.Errorf("%s=%v", name, err)
		}
	}
	if _, _, err := core.WatchEpoch(ctx, snapshot); !errors.Is(err, ErrStaleEpoch) {
		t.Fatal(err)
	}
}

func TestDrainPanicAndReentryNeverSealAndDoNotSkipOtherExecutors(t *testing.T) {
	core := newEpochServiceForTest(t)
	panics := true
	calls := 0
	if err := core.RegisterExecutorDrain(context.Background(), func(ctx context.Context) error {
		if proof, err := core.CloseAndDrain(ctx); !errors.Is(err, ErrDrainInProgress) || proof.Includes(core.security) {
			t.Fatalf("reentrada=%v", err)
		}
		if panics {
			panic("test")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if proof, err := core.CloseAndDrain(context.Background()); !errors.Is(err, ErrDrainFailed) || proof.Includes(core.security) {
		t.Fatalf("panic produziu proof=%v", err)
	}
	panics = false
	if _, err := core.CloseAndDrain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("executor pulado: %d", calls)
	}
}

func TestCancelledDrainStillClosesAfterWaitingForGate(t *testing.T) {
	core := newEpochServiceForTest(t)
	core.gate.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := core.CloseAndDrain(ctx); result <- err }()
	cancel()
	core.gate.mu.Unlock()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("drain=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("drain não retornou")
	}
	if _, err := core.Capture(context.Background(), testEpochID(t), testEpochID(t)); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("admissão permaneceu aberta=%v", err)
	}
}
