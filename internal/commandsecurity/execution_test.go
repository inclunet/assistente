package commandsecurity

import (
	"context"
	"errors"
	"testing"
)

func executionWatchCount(service *EpochService) int {
	service.watchesMu.Lock()
	defer service.watchesMu.Unlock()
	return len(service.watches)
}

func assertExecutionDone(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("contexto de execução não foi cancelado")
	}
}

func executionSnapshot(t *testing.T, service *EpochService) EpochSnapshot {
	t.Helper()
	snapshot, err := service.Capture(context.Background(), testEpochID(t), testEpochID(t))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestAdmitExecutionInscreveAntesDoHandoffEReleaseEIdempotente(t *testing.T) {
	service := newEpochServiceForTest(t)
	snapshot := executionSnapshot(t, service)
	var runCtx context.Context
	release, err := service.AdmitExecution(context.Background(), snapshot, func(context.Context) error {
		return nil
	}, func(ctx context.Context) error {
		runCtx = ctx
		if got := executionWatchCount(service); got != 1 {
			t.Fatalf("handoff observou %d watchers, want 1", got)
		}
		if service.gate.mu.TryLock() {
			service.gate.mu.Unlock()
			t.Fatal("handoff não manteve o gate")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if release == nil || runCtx == nil {
		t.Fatal("admissão bem-sucedida deveria retornar release e contexto")
	}
	if got := executionWatchCount(service); got != 1 {
		t.Fatalf("watchers após handoff = %d, want 1", got)
	}

	release()
	release()
	if got := executionWatchCount(service); got != 0 {
		t.Fatalf("watchers após release idempotente = %d, want 0", got)
	}
	assertExecutionDone(t, runCtx)
}

func TestAdmitExecutionInvalidateSessionCancelaSomenteSessao(t *testing.T) {
	service := newEpochServiceForTest(t)
	userA, sessionA := testEpochID(t), testEpochID(t)
	userB, sessionB := testEpochID(t), testEpochID(t)
	snapshotA, err := service.Capture(context.Background(), userA, sessionA)
	if err != nil {
		t.Fatal(err)
	}
	snapshotB, err := service.Capture(context.Background(), userB, sessionB)
	if err != nil {
		t.Fatal(err)
	}
	var ctxA, ctxB context.Context
	releaseA, err := service.AdmitExecution(context.Background(), snapshotA, func(context.Context) error { return nil }, func(ctx context.Context) error { ctxA = ctx; return nil })
	if err != nil {
		t.Fatal(err)
	}
	releaseB, err := service.AdmitExecution(context.Background(), snapshotB, func(context.Context) error { return nil }, func(ctx context.Context) error { ctxB = ctx; return nil })
	if err != nil {
		releaseA()
		t.Fatal(err)
	}
	defer releaseB()

	if err := service.InvalidateSession(context.Background(), userA, sessionA); err != nil {
		t.Fatal(err)
	}
	assertExecutionDone(t, ctxA)
	select {
	case <-ctxB.Done():
		t.Fatal("invalidação da sessão A cancelou a sessão B")
	default:
	}
	if got := executionWatchCount(service); got != 1 {
		t.Fatalf("watchers após invalidação da sessão A = %d, want 1", got)
	}
	releaseA()
}

func TestAdmitExecutionMutationsGlobaisCancelamTodosECallbackVeCtxCancelado(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*EpochService, context.Context, string, string, func() error) error
	}{
		{name: "principal", mutate: func(s *EpochService, ctx context.Context, user, session string, action func() error) error {
			return s.MutatePrincipal(ctx, user, session, action)
		}},
		{name: "security", mutate: func(s *EpochService, ctx context.Context, _, _ string, action func() error) error {
			return s.MutateSecurity(ctx, action)
		}},
		{name: "transition", mutate: func(s *EpochService, ctx context.Context, _, _ string, action func() error) error {
			finish, err := s.BeginTransition(ctx)
			if err != nil {
				return err
			}
			defer finish()
			return action()
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := newEpochServiceForTest(t)
			user, session := testEpochID(t), testEpochID(t)
			snapshot, err := service.Capture(context.Background(), user, session)
			if err != nil {
				t.Fatal(err)
			}
			var runCtx context.Context
			release, err := service.AdmitExecution(context.Background(), snapshot, func(context.Context) error { return nil }, func(ctx context.Context) error { runCtx = ctx; return nil })
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			callbackObservedCancellation := false
			if err := test.mutate(service, context.Background(), user, session, func() error {
				select {
				case <-runCtx.Done():
					callbackObservedCancellation = true
				default:
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !callbackObservedCancellation {
				t.Fatal("callback da mutação não observou o contexto já cancelado")
			}
			assertExecutionDone(t, runCtx)
			if got := executionWatchCount(service); got != 0 {
				t.Fatalf("watchers após mutação global = %d, want 0", got)
			}
		})
	}
}

func TestAdmitExecutionBeginTransitionCancelaTodos(t *testing.T) {
	service := newEpochServiceForTest(t)
	snapshot := executionSnapshot(t, service)
	var runCtx context.Context
	release, err := service.AdmitExecution(context.Background(), snapshot, func(context.Context) error { return nil }, func(ctx context.Context) error { runCtx = ctx; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	finish, err := service.BeginTransition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	assertExecutionDone(t, runCtx)
	if got := executionWatchCount(service); got != 0 {
		t.Fatalf("watchers durante transição = %d, want 0", got)
	}
}

func TestAdmitExecutionStaleNaoFazHandoffNemWatch(t *testing.T) {
	service := newEpochServiceForTest(t)
	snapshot := executionSnapshot(t, service)
	if err := service.InvalidateSession(context.Background(), snapshot.UserID, snapshot.SessionID); err != nil {
		t.Fatal(err)
	}
	handoff := false
	release, err := service.AdmitExecution(context.Background(), snapshot, func(context.Context) error {
		t.Fatal("revalidação executada para snapshot stale")
		return nil
	}, func(context.Context) error {
		handoff = true
		return nil
	})
	if release != nil || !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("admissão stale = (release=%v, err=%v)", release != nil, err)
	}
	if handoff || executionWatchCount(service) != 0 {
		t.Fatalf("snapshot stale fez handoff/watch: handoff=%v watchers=%d", handoff, executionWatchCount(service))
	}
}

func TestAdmitExecutionParentCancelAndHandoffFailureLimpam(t *testing.T) {
	t.Run("pai", func(t *testing.T) {
		service := newEpochServiceForTest(t)
		snapshot := executionSnapshot(t, service)
		ctx, cancel := context.WithCancel(context.Background())
		var runCtx context.Context
		release, err := service.AdmitExecution(ctx, snapshot, func(context.Context) error { return nil }, func(child context.Context) error { runCtx = child; return nil })
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		assertExecutionDone(t, runCtx)
		release()
		if got := executionWatchCount(service); got != 0 {
			t.Fatalf("watchers após cancelamento do pai/release = %d, want 0", got)
		}
	})

	t.Run("erro", func(t *testing.T) {
		service := newEpochServiceForTest(t)
		snapshot := executionSnapshot(t, service)
		want := errors.New("handoff falhou")
		release, err := service.AdmitExecution(context.Background(), snapshot, func(context.Context) error { return nil }, func(context.Context) error { return want })
		if release != nil || !errors.Is(err, want) {
			t.Fatalf("handoff com erro = (release=%v, err=%v)", release != nil, err)
		}
		if got := executionWatchCount(service); got != 0 {
			t.Fatalf("watchers após erro de handoff = %d, want 0", got)
		}
	})

	t.Run("panico", func(t *testing.T) {
		service := newEpochServiceForTest(t)
		snapshot := executionSnapshot(t, service)
		panicked := false
		func() {
			defer func() { panicked = recover() != nil }()
			_, _ = service.AdmitExecution(context.Background(), snapshot, func(context.Context) error { return nil }, func(context.Context) error { panic("handoff") })
		}()
		if !panicked {
			t.Fatal("panic do handoff não foi propagado")
		}
		if got := executionWatchCount(service); got != 0 {
			t.Fatalf("watchers após panic de handoff = %d, want 0", got)
		}
	})
}
func TestMutateUserConfigurationCancelaSomenteWatchesDoUsuario(t *testing.T) {
	service := newEpochServiceForTest(t)
	userA, userB := testEpochID(t), testEpochID(t)
	sessionA1, sessionA2, sessionB := testEpochID(t), testEpochID(t), testEpochID(t)
	snapshotA1, err := service.Capture(context.Background(), userA, sessionA1)
	if err != nil {
		t.Fatal(err)
	}
	snapshotA2, err := service.Capture(context.Background(), userA, sessionA2)
	if err != nil {
		t.Fatal(err)
	}
	snapshotB, err := service.Capture(context.Background(), userB, sessionB)
	if err != nil {
		t.Fatal(err)
	}

	var runA1, runA2, runB context.Context
	releaseA1, err := service.AdmitExecution(context.Background(), snapshotA1, func(context.Context) error { return nil }, func(ctx context.Context) error {
		runA1 = ctx
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseA1()
	releaseA2, err := service.AdmitExecution(context.Background(), snapshotA2, func(context.Context) error { return nil }, func(ctx context.Context) error {
		runA2 = ctx
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseA2()
	releaseB, err := service.AdmitExecution(context.Background(), snapshotB, func(context.Context) error { return nil }, func(ctx context.Context) error {
		runB = ctx
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseB()

	callbackObservedA1Cancellation := false
	callbackObservedA2Cancellation := false
	callbackObservedBCancellation := false
	if err := service.MutateUserConfiguration(context.Background(), userA, func() error {
		select {
		case <-runA1.Done():
			callbackObservedA1Cancellation = true
		default:
		}
		select {
		case <-runA2.Done():
			callbackObservedA2Cancellation = true
		default:
		}
		select {
		case <-runB.Done():
			callbackObservedBCancellation = true
		default:
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !callbackObservedA1Cancellation || !callbackObservedA2Cancellation {
		t.Fatalf("callback não observou os contextos de A cancelados: A1=%v A2=%v", callbackObservedA1Cancellation, callbackObservedA2Cancellation)
	}
	if callbackObservedBCancellation {
		t.Fatal("callback observou indevidamente o contexto de B cancelado")
	}
	assertExecutionDone(t, runA1)
	assertExecutionDone(t, runA2)
	select {
	case <-runB.Done():
		t.Fatal("configuração de A cancelou o contexto de B")
	default:
	}
	if got := executionWatchCount(service); got != 1 {
		t.Fatalf("watchers após configuração de A = %d, want 1 de B", got)
	}

	handoffB := false
	releaseFreshB, err := service.AdmitExecution(context.Background(), snapshotB, func(context.Context) error { return nil }, func(context.Context) error {
		handoffB = true
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot B após alteração de A foi recusado: %v", err)
	}
	if !handoffB || releaseFreshB == nil {
		t.Fatal("snapshot B deveria completar admissão normalmente")
	}
	releaseFreshB()
}
