package commandsecurity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func captureContextPrincipal(t *testing.T, service *EpochService, p ContextPrincipal) EpochSnapshot {
	t.Helper()
	snapshot, err := service.CaptureContextAuthenticated(context.Background(), func(context.Context) (ContextPrincipal, error) {
		return p, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestMutateContextGroupRevokesOnlyMatchingExternalTokensAndWatches(t *testing.T) {
	service, err := NewEpochService(&DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.Must(uuid.NewV7()).String()
	tokenA := ContextPrincipal{UserID: user, Type: "external_token", ID: "issuer/token-a", GroupID: "operators"}
	tokenB := ContextPrincipal{UserID: user, Type: "external_token", ID: "issuer/token-b", GroupID: "operators"}
	otherGroup := ContextPrincipal{UserID: user, Type: "external_token", ID: "issuer/token-c", GroupID: "auditors"}
	first, second, preserved := captureContextPrincipal(t, service, tokenA), captureContextPrincipal(t, service, tokenB), captureContextPrincipal(t, service, otherGroup)
	watchFirst, releaseFirst, err := service.WatchEpoch(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()
	watchSecond, releaseSecond, err := service.WatchEpoch(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	watchPreserved, releasePreserved, err := service.WatchEpoch(context.Background(), preserved)
	if err != nil {
		t.Fatal(err)
	}
	defer releasePreserved()

	called := false
	err = service.MutateContextGroup(context.Background(), ContextPrincipal{UserID: user, Type: "external_token", GroupID: "operators"}, func() error {
		called = true
		if service.gate.mu.TryLock() {
			service.gate.mu.Unlock()
			t.Error("callback sem gate exclusivo")
		}
		if watchFirst.Err() == nil || watchSecond.Err() == nil {
			t.Error("watches do grupo ainda ativos durante callback")
		}
		if watchPreserved.Err() != nil {
			t.Error("watch de outro grupo cancelado")
		}
		if _, ok := service.sessions[first.SessionID]; ok {
			t.Error("primeiro token ainda capturado durante callback")
		}
		if _, ok := service.sessions[second.SessionID]; ok {
			t.Error("segundo token ainda capturado durante callback")
		}
		if _, ok := service.sessions[preserved.SessionID]; !ok {
			t.Error("sessão de outro grupo removida")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("mutação não executada: called=%v err=%v", called, err)
	}
	for _, snapshot := range []EpochSnapshot{first, second} {
		if err := service.Admit(context.Background(), snapshot, func(context.Context) error { return nil }, func() error { return nil }); !errors.Is(err, ErrStaleEpoch) {
			t.Fatalf("snapshot revogado admitido: %v", err)
		}
	}
	if err := service.Admit(context.Background(), preserved, func(context.Context) error { return nil }, func() error { return nil }); err != nil {
		t.Fatalf("contexto de outro grupo invalidado: %v", err)
	}

	newFirst := captureContextPrincipal(t, service, tokenA)
	if newFirst.AuthGeneration == first.AuthGeneration {
		t.Fatal("recaptura reviveu a geração anterior")
	}
	if err := service.Admit(context.Background(), first, func(context.Context) error { return nil }, func() error { return nil }); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("snapshot antigo voltou a ser admitido após recaptura: %v", err)
	}
}

func TestCaptureContextRejectsGroupChangesAndInvalidGroups(t *testing.T) {
	for _, test := range []struct {
		name string
		p    ContextPrincipal
	}{
		{name: "non external", p: ContextPrincipal{Type: "job_service", ID: "job", GroupID: "g"}},
		{name: "over limit", p: ContextPrincipal{Type: "external_token", ID: "token", GroupID: string(make([]byte, 1025))}},
		{name: "invalid utf8", p: ContextPrincipal{Type: "external_token", ID: "token", GroupID: string([]byte{0xff})}},
		{name: "leading whitespace", p: ContextPrincipal{Type: "external_token", ID: "token", GroupID: " group"}},
		{name: "trailing whitespace", p: ContextPrincipal{Type: "external_token", ID: "token", GroupID: "group "}},
		{name: "nul", p: ContextPrincipal{Type: "external_token", ID: "token", GroupID: "group\x00suffix"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewEpochService(&DispatchGate{})
			if err != nil {
				t.Fatal(err)
			}
			test.p.UserID = uuid.Must(uuid.NewV7()).String()
			if _, err := service.CaptureContextAuthenticated(context.Background(), func(context.Context) (ContextPrincipal, error) { return test.p, nil }); !errors.Is(err, ErrInvalidEpochInput) {
				t.Fatalf("grupo inválido aceito: %v", err)
			}
			if len(service.sessions) != 0 {
				t.Fatal("grupo inválido persistido")
			}
		})
	}

	service, err := NewEpochService(&DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.Must(uuid.NewV7()).String()
	p := ContextPrincipal{UserID: user, Type: "external_token", ID: "issuer/token", GroupID: "first"}
	old := captureContextPrincipal(t, service, p)
	p.GroupID = "second"
	if _, err := service.CaptureContextAuthenticated(context.Background(), func(context.Context) (ContextPrincipal, error) { return p, nil }); !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatalf("mudança de grupo aceita para contexto existente: %v", err)
	}
	if current := service.sessions[old.SessionID]; current.group == "" {
		t.Fatal("grupo validado não armazenado na sessão")
	}
}

func TestMutateContextGroupFailureKeepsInvalidationAndCancellation(t *testing.T) {
	service, err := NewEpochService(&DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.Must(uuid.NewV7()).String()
	p := ContextPrincipal{UserID: user, Type: "external_token", ID: "issuer/token", GroupID: "team"}
	snapshot := captureContextPrincipal(t, service, p)
	watch, release, err := service.WatchEpoch(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	sentinel := errors.New("falha na mutação administrativa")
	err = service.MutateContextGroup(context.Background(), ContextPrincipal{UserID: user, Type: "external_token", GroupID: "team"}, func() error {
		if watch.Err() == nil {
			t.Fatal("watch não cancelado antes da ação")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("erro da ação perdido: %v", err)
	}
	if err := service.Admit(context.Background(), snapshot, func(context.Context) error { return nil }, func() error { return nil }); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("falha reverteu invalidação: %v", err)
	}
}

func TestMutateContextGroupSerializesConcurrentCapture(t *testing.T) {
	service, err := NewEpochService(&DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	user := uuid.Must(uuid.NewV7()).String()
	p := ContextPrincipal{UserID: user, Type: "external_token", ID: "issuer/token", GroupID: "team"}
	old := captureContextPrincipal(t, service, p)
	entered, continueMutation := make(chan struct{}), make(chan struct{})
	mutation := make(chan error, 1)
	go func() {
		mutation <- service.MutateContextGroup(ctx, ContextPrincipal{UserID: user, Type: "external_token", GroupID: "team"}, func() error {
			close(entered)
			select {
			case <-continueMutation:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	defer func() {
		select {
		case <-continueMutation:
		default:
			close(continueMutation)
		}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("mutação não entrou no gate")
	}
	if service.gate.mu.TryRLock() {
		service.gate.mu.RUnlock()
		t.Fatal("mutação de grupo não reteve o gate")
	}
	captured := make(chan struct {
		snapshot EpochSnapshot
		err      error
	}, 1)
	go func() {
		snapshot, err := service.CaptureContextAuthenticated(ctx, func(context.Context) (ContextPrincipal, error) { return p, nil })
		captured <- struct {
			snapshot EpochSnapshot
			err      error
		}{snapshot, err}
	}()
	select {
	case <-captured:
		t.Fatal("captura atravessou mutação de grupo")
	case <-time.After(30 * time.Millisecond):
	}
	close(continueMutation)
	select {
	case err := <-mutation:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("mutação não concluiu")
	}
	select {
	case result := <-captured:
		if result.err != nil || result.snapshot.AuthGeneration == old.AuthGeneration {
			t.Fatalf("captura concorrente não recebeu geração nova: %+v, %v", result.snapshot, result.err)
		}
	case <-ctx.Done():
		t.Fatal("captura não retomou após mutação")
	}
}

func TestMutateContextGroupRejectsInvalidInputs(t *testing.T) {
	service, err := NewEpochService(&DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.Must(uuid.NewV7()).String()
	for _, p := range []ContextPrincipal{
		{UserID: user, Type: "job_service", GroupID: "team"},
		{UserID: user, Type: "external_token"},
		{UserID: "invalid", Type: "external_token", GroupID: "team"},
		{UserID: user, Type: "external_token", GroupID: " team"},
		{UserID: user, Type: "external_token", GroupID: "team\x00suffix"},
		{UserID: user, Type: "external_token", GroupID: string(make([]byte, 1025))},
	} {
		if err := service.MutateContextGroup(context.Background(), p, func() error { return nil }); !errors.Is(err, ErrInvalidEpochInput) {
			t.Fatalf("entrada inválida aceita (%+v): %v", p, err)
		}
	}
}
