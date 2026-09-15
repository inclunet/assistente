package commandsecurity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func testEpochID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("gerar UUIDv7: %v", err)
	}
	return id.String()
}

func newEpochServiceForTest(t *testing.T) *EpochService {
	t.Helper()
	service, err := NewEpochService(&DispatchGate{})
	if err != nil {
		t.Fatalf("criar EpochService: %v", err)
	}
	return service
}

func TestNewEpochServiceRejectsNilGateAndUsesUniqueUUIDv7Startup(t *testing.T) {
	if service, err := NewEpochService(nil); !errors.Is(err, ErrInvalidEpochInput) || service != nil {
		t.Fatalf("NewEpochService(nil) = (%v, %v), want (nil, ErrInvalidEpochInput)", service, err)
	}

	one := newEpochServiceForTest(t)
	two := newEpochServiceForTest(t)
	if one.startup == two.startup {
		t.Fatal("instâncias novas deveriam ter startup distinto")
	}
	for name, value := range map[string]string{"one": one.startup, "two": two.startup} {
		id, err := uuid.Parse(value)
		if err != nil || id.Version() != 7 || id.Variant() != uuid.RFC4122 || id.String() != value {
			t.Errorf("startup %s = %q não é UUIDv7 canônico", name, value)
		}
	}
}

func TestEpochCaptureIsStableAndRejectsCrossUserSession(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, otherUser, session := testEpochID(t), testEpochID(t), testEpochID(t)

	snapshot, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.UserID != user || snapshot.SessionID != session || snapshot.AuthGeneration == "" || snapshot.SecurityGeneration == "" {
		t.Fatalf("snapshot incompleto: %#v", snapshot)
	}
	if _, err := uuid.Parse(snapshot.SecurityGeneration[:36]); err != nil {
		t.Fatalf("security generation deveria começar por UUIDv7: %v", err)
	}

	repeated, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	if repeated != snapshot {
		t.Fatalf("captura repetida mudou snapshot: primeiro %#v, segundo %#v", snapshot, repeated)
	}
	if _, err := service.Capture(context.Background(), otherUser, session); !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatalf("usuário diferente na mesma sessão: %v, want ErrInvalidEpochInput", err)
	}
}

func TestEpochAdmitRevalidatesThenHandsOffAndPropagatesErrors(t *testing.T) {
	service := newEpochServiceForTest(t)
	snapshot, err := service.Capture(context.Background(), testEpochID(t), testEpochID(t))
	if err != nil {
		t.Fatal(err)
	}

	var revalidated, handedOff bool
	if err := service.Admit(context.Background(), snapshot, func(context.Context) error {
		revalidated = true
		return nil
	}, func() error {
		handedOff = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !revalidated || !handedOff {
		t.Fatalf("ordem/calls incorretos: revalidated=%v handedOff=%v", revalidated, handedOff)
	}

	want := errors.New("falha de revalidação")
	handedOff = false
	if err := service.Admit(context.Background(), snapshot, func(context.Context) error { return want }, func() error {
		handedOff = true
		return nil
	}); !errors.Is(err, want) {
		t.Fatalf("erro de revalidação = %v, want %v", err, want)
	}
	if handedOff {
		t.Fatal("handoff não deveria ocorrer quando revalidação falha")
	}
}

func TestEpochAdmitRejectsStaleWithoutCallbacks(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	snapshot, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.InvalidateSession(context.Background(), user, session); err != nil {
		t.Fatal(err)
	}

	called := false
	if err := service.Admit(context.Background(), snapshot, func(context.Context) error {
		called = true
		return nil
	}, func() error {
		called = true
		return nil
	}); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("admit de snapshot stale = %v, want ErrStaleEpoch", err)
	}
	if called {
		t.Fatal("snapshot stale não deveria chamar callbacks")
	}
}

func TestEpochInvalidationsHaveExpectedScope(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, otherUser := testEpochID(t), testEpochID(t)
	sessionOne, sessionTwo := testEpochID(t), testEpochID(t)
	one, err := service.Capture(context.Background(), user, sessionOne)
	if err != nil {
		t.Fatal(err)
	}
	two, err := service.Capture(context.Background(), otherUser, sessionTwo)
	if err != nil {
		t.Fatal(err)
	}

	if err := service.InvalidateSession(context.Background(), user, sessionOne); err != nil {
		t.Fatal(err)
	}
	fresh, err := service.Capture(context.Background(), user, sessionOne)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.AuthGeneration == one.AuthGeneration || fresh.SecurityGeneration != one.SecurityGeneration {
		t.Fatalf("invalidação de sessão: antigo %#v, novo %#v", one, fresh)
	}
	if err := service.Admit(context.Background(), two, func(context.Context) error { return nil }, func() error { return nil }); err != nil {
		t.Fatal("sessão independente deveria continuar válida:", err)
	}

	if err := service.InvalidateSecurity(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Admit(context.Background(), fresh, func(context.Context) error { return nil }, func() error { return nil }); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("snapshot antigo após invalidação de segurança = %v", err)
	}
	securityFresh, err := service.Capture(context.Background(), user, sessionOne)
	if err != nil {
		t.Fatal(err)
	}
	if securityFresh.AuthGeneration != fresh.AuthGeneration || securityFresh.SecurityGeneration == fresh.SecurityGeneration {
		t.Fatalf("invalidação de segurança alterou gerações incorretamente: %#v -> %#v", fresh, securityFresh)
	}

	principal, err := service.Capture(context.Background(), user, sessionOne)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.InvalidatePrincipal(context.Background(), user, sessionOne); err != nil {
		t.Fatal(err)
	}
	if replacement, err := service.Capture(context.Background(), user, sessionOne); err != nil || replacement.AuthGeneration == principal.AuthGeneration || replacement.SecurityGeneration == principal.SecurityGeneration {
		t.Fatalf("invalidação de principal não rotacionou ambas: replacement=%#v err=%v", replacement, err)
	}
}

func TestEpochRejectsInvalidAndCancelledBasics(t *testing.T) {
	service := newEpochServiceForTest(t)
	validUser, validSession := testEpochID(t), testEpochID(t)
	invalid := "00000000-0000-4000-8000-000000000000"
	if _, err := service.Capture(context.Background(), invalid, validSession); !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatalf("UUIDv7 não canônico deveria ser rejeitado: %v", err)
	}
	if _, err := service.Capture(nil, validUser, validSession); !errors.Is(err, errNilContext) { //nolint:staticcheck // Testa deliberadamente a recusa de contexto nil.
		t.Fatalf("contexto nil em Capture = %v", err)
	}

	snapshot, err := service.Capture(context.Background(), validUser, validSession)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Admit(context.Background(), snapshot, nil, func() error { return nil }); !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatalf("revalidate nil = %v", err)
	}
	if err := service.Admit(context.Background(), snapshot, func(context.Context) error { return nil }, nil); !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatalf("handoff nil = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if err := service.Admit(ctx, snapshot, func(context.Context) error { called = true; return nil }, func() error { called = true; return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("Admit cancelado = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("callbacks não deveriam executar com contexto cancelado")
	}
}
