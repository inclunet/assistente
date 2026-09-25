package commandsecurity

import (
	"context"
	"errors"
	"math"
	"testing"
)

func assertEpochGateExclusive(t *testing.T, gate *DispatchGate) {
	t.Helper()
	if gate.mu.TryRLock() {
		gate.mu.RUnlock()
		t.Fatal("callback deveria executar sob o gate exclusivo")
	}
}

func assertEpochGateReleased(t *testing.T, gate *DispatchGate) {
	t.Helper()
	if !gate.mu.TryLock() {
		t.Fatal("gate deveria estar liberado após a mutação")
	}
	gate.mu.Unlock()
}

func assertEpochSnapshotStale(t *testing.T, service *EpochService, snapshot EpochSnapshot) {
	t.Helper()
	if err := service.Admit(context.Background(), snapshot, func(context.Context) error {
		t.Fatal("snapshot invalidado não deveria revalidar")
		return nil
	}, func() error {
		t.Fatal("snapshot invalidado não deveria fazer handoff")
		return nil
	}); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("snapshot deveria estar stale: %v", err)
	}
}

func TestEpochMutateSessionInvalidatesBeforeCallbackAndPreservesOtherSession(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, otherUser := testEpochID(t), testEpochID(t)
	session, otherSession := testEpochID(t), testEpochID(t)
	old, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Capture(context.Background(), otherUser, otherSession)
	if err != nil {
		t.Fatal(err)
	}

	called := false
	err = service.MutateSession(context.Background(), user, session, func() error {
		called = true
		assertEpochGateExclusive(t, service.gate)
		if _, ok := service.sessions[session]; ok {
			t.Fatal("sessão deveria ser invalidada antes do callback")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("MutateSession = %v, callback chamado=%v", err, called)
	}
	assertEpochGateReleased(t, service.gate)

	assertEpochSnapshotStale(t, service, old)
	if err := service.Admit(context.Background(), other, func(context.Context) error { return nil }, func() error { return nil }); err != nil {
		t.Fatalf("segunda sessão não deveria ser afetada: %v", err)
	}
	fresh, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.AuthGeneration == old.AuthGeneration || fresh.SecurityGeneration != old.SecurityGeneration {
		t.Fatalf("MutateSession não rotacionou somente a sessão: antigo=%#v novo=%#v", old, fresh)
	}
}

func TestEpochMutationsKeepInvalidationWhenCallbackReturnsError(t *testing.T) {
	want := errors.New("falha autoritativa")
	tests := []struct {
		name   string
		mutate func(*EpochService, context.Context, string, string, func() error) error
		both   bool
	}{
		{name: "session", mutate: (*EpochService).MutateSession},
		{name: "principal", mutate: (*EpochService).MutatePrincipal, both: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newEpochServiceForTest(t)
			user, session := testEpochID(t), testEpochID(t)
			old, err := service.Capture(context.Background(), user, session)
			if err != nil {
				t.Fatal(err)
			}
			err = tt.mutate(service, context.Background(), user, session, func() error {
				assertEpochGateExclusive(t, service.gate)
				return want
			})
			if !errors.Is(err, want) {
				t.Fatalf("erro da callback = %v, want %v", err, want)
			}
			assertEpochGateReleased(t, service.gate)
			assertEpochSnapshotStale(t, service, old)
			fresh, err := service.Capture(context.Background(), user, session)
			if err != nil {
				t.Fatal(err)
			}
			if fresh.AuthGeneration == old.AuthGeneration || (tt.both && fresh.SecurityGeneration == old.SecurityGeneration) {
				t.Fatalf("invalidação não persistiu: antigo=%#v novo=%#v", old, fresh)
			}
		})
	}
}

func TestEpochMutationsKeepInvalidationWhenCallbackPanicsAndReleaseGate(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	old, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "panic autoritativo" {
				t.Fatalf("panic = %v, want panic autoritativo", recovered)
			}
		}()
		_ = service.MutatePrincipal(context.Background(), user, session, func() error {
			assertEpochGateExclusive(t, service.gate)
			panic("panic autoritativo")
		})
	}()
	assertEpochGateReleased(t, service.gate)
	assertEpochSnapshotStale(t, service, old)
}

func TestEpochMutateSecurityChangesGlobalAndPreservesAuthGeneration(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	old, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	if err := service.MutateSecurity(context.Background(), func() error {
		called = true
		assertEpochGateExclusive(t, service.gate)
		if service.security == old.SecurityGeneration {
			t.Fatal("segurança deveria ser invalidada antes do callback")
		}
		return nil
	}); err != nil || !called {
		t.Fatalf("MutateSecurity = %v, callback chamado=%v", err, called)
	}
	assertEpochGateReleased(t, service.gate)
	fresh, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.AuthGeneration != old.AuthGeneration || fresh.SecurityGeneration == old.SecurityGeneration {
		t.Fatalf("MutateSecurity alterou gerações incorretamente: antigo=%#v novo=%#v", old, fresh)
	}
}

func TestEpochMutationsRejectNilCallbackAndCancelledContextWithoutEffects(t *testing.T) {
	tests := []struct {
		name string
		call func(*EpochService, context.Context, string, string, func() error) error
	}{
		{name: "session", call: (*EpochService).MutateSession},
		{name: "principal", call: (*EpochService).MutatePrincipal},
	}
	for _, tt := range tests {
		t.Run(tt.name+" nil", func(t *testing.T) {
			service := newEpochServiceForTest(t)
			user, session := testEpochID(t), testEpochID(t)
			old, err := service.Capture(context.Background(), user, session)
			if err != nil {
				t.Fatal(err)
			}
			if err := tt.call(service, context.Background(), user, session, nil); !errors.Is(err, ErrInvalidEpochInput) {
				t.Fatalf("callback nil = %v", err)
			}
			if current, err := service.Capture(context.Background(), user, session); err != nil || current != old {
				t.Fatalf("callback nil alterou estado: snapshot=%#v err=%v", current, err)
			}
		})
		t.Run(tt.name+" cancelado", func(t *testing.T) {
			service := newEpochServiceForTest(t)
			user, session := testEpochID(t), testEpochID(t)
			old, err := service.Capture(context.Background(), user, session)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			called := false
			if err := tt.call(service, ctx, user, session, func() error { called = true; return nil }); !errors.Is(err, context.Canceled) || called {
				t.Fatalf("contexto cancelado = %v, callback chamado=%v", err, called)
			}
			if current, err := service.Capture(context.Background(), user, session); err != nil || current != old {
				t.Fatalf("cancelamento alterou estado: snapshot=%#v err=%v", current, err)
			}
		})
	}
}

func TestEpochMutationsRejectInvalidOwnerAndGlobalOverflowAtomically(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, otherUser, session := testEpochID(t), testEpochID(t), testEpochID(t)
	old, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	if err := service.MutateSession(context.Background(), otherUser, session, func() error { called = true; return nil }); !errors.Is(err, ErrInvalidEpochInput) || called {
		t.Fatalf("owner divergente = %v, callback chamado=%v", err, called)
	}
	if current, err := service.Capture(context.Background(), user, session); err != nil || current != old {
		t.Fatalf("owner divergente alterou estado: snapshot=%#v err=%v", current, err)
	}

	service.sequence = math.MaxUint64
	called = false
	if err := service.MutatePrincipal(context.Background(), user, session, func() error { called = true; return nil }); !errors.Is(err, ErrInvalidEpochInput) || called {
		t.Fatalf("overflow principal = %v, callback chamado=%v", err, called)
	}
	if service.sequence != math.MaxUint64 || service.security != old.SecurityGeneration || service.sessions[session] != (sessionEpoch{user: user, generation: old.AuthGeneration}) {
		t.Fatalf("overflow principal avançou estado parcialmente: sequence=%d security=%q session=%#v", service.sequence, service.security, service.sessions[session])
	}
	// Contrato de overflow: o serviço fica desabilitado e snapshots antigos
	// falham fechado, mesmo que a geração armazenada não tenha avançado.
	if current, err := service.Capture(context.Background(), user, session); !errors.Is(err, ErrStaleEpoch) || current != (EpochSnapshot{}) {
		t.Fatalf("overflow principal não falhou fechado: snapshot=%#v err=%v", current, err)
	}
	if err := service.Admit(context.Background(), old, func(context.Context) error { t.Fatal("snapshot antigo admitido após overflow principal"); return nil }, func() error { t.Fatal("snapshot antigo fez handoff após overflow principal"); return nil }); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("Admit após overflow principal = %v", err)
	}

	global := newEpochServiceForTest(t)
	globalOld, err := global.Capture(context.Background(), user, testEpochID(t))
	if err != nil {
		t.Fatal(err)
	}
	global.sequence = math.MaxUint64
	called = false
	if err := global.MutateSecurity(context.Background(), func() error { called = true; return nil }); !errors.Is(err, ErrInvalidEpochInput) || called {
		t.Fatalf("overflow global = %v, callback chamado=%v", err, called)
	}
	if global.sequence != math.MaxUint64 || global.security != globalOld.SecurityGeneration || global.sessions[globalOld.SessionID] != (sessionEpoch{user: globalOld.UserID, generation: globalOld.AuthGeneration}) {
		t.Fatalf("overflow global avançou estado parcialmente: sequence=%d security=%q session=%#v", global.sequence, global.security, global.sessions[globalOld.SessionID])
	}
	if current, err := global.Capture(context.Background(), globalOld.UserID, globalOld.SessionID); !errors.Is(err, ErrStaleEpoch) || current != (EpochSnapshot{}) {
		t.Fatalf("overflow global não falhou fechado: snapshot=%#v err=%v", current, err)
	}
	if err := global.Admit(context.Background(), globalOld, func(context.Context) error { t.Fatal("snapshot antigo admitido após overflow global"); return nil }, func() error { t.Fatal("snapshot antigo fez handoff após overflow global"); return nil }); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("Admit após overflow global = %v", err)
	}
}
