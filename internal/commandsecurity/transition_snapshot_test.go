package commandsecurity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBeginTransitionFromSnapshotClaimsBeforeCancellingAllWatches(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session, foreignUser, foreignSession := testEpochID(t), testEpochID(t), testEpochID(t), testEpochID(t)
	snapshot, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := service.Capture(context.Background(), foreignUser, foreignSession)
	if err != nil {
		t.Fatal(err)
	}
	watched, release, err := service.WatchEpoch(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	foreignWatched, foreignRelease, err := service.WatchEpoch(context.Background(), foreign)
	if err != nil {
		t.Fatal(err)
	}
	defer foreignRelease()

	claimed := false
	finish, err := service.BeginTransitionFromSnapshot(context.Background(), snapshot,
		func(context.Context) error { return nil },
		func(context.Context) error {
			claimed = true
			select {
			case <-watched.Done():
				t.Fatal("watch da operação foi cancelado antes da claim")
			default:
			}
			select {
			case <-foreignWatched.Done():
				t.Fatal("watch estrangeiro foi cancelado antes da claim")
			default:
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("claim não foi executada")
	}
	if _, err := service.Capture(context.Background(), user, session); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("Capture durante transição = %v, want ErrStaleEpoch", err)
	}
	select {
	case <-watched.Done():
	case <-time.After(time.Second):
		t.Fatal("watch da operação não foi cancelado")
	}
	select {
	case <-foreignWatched.Done():
	case <-time.After(time.Second):
		t.Fatal("watch estrangeiro não foi cancelado")
	}
	finish()
	finish()
	if _, err := service.Capture(context.Background(), user, session); err != nil {
		t.Fatalf("Capture após finish = %v", err)
	}
}

func TestBeginTransitionFromSnapshotRefusesStaleRevalidationAndCancelledContexts(t *testing.T) {
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
	if finish, err := service.BeginTransitionFromSnapshot(context.Background(), snapshot,
		func(context.Context) error { called = true; return nil },
		func(context.Context) error { called = true; return nil }); finish != nil || !errors.Is(err, ErrStaleEpoch) || called {
		t.Fatalf("snapshot revogado aceito: finish=%v err=%v called=%v", finish != nil, err, called)
	}

	fresh, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	called = false
	revalidationErr := errors.New("host recusou")
	if finish, err := service.BeginTransitionFromSnapshot(context.Background(), fresh,
		func(context.Context) error { called = true; return revalidationErr },
		func(context.Context) error { called = true; return nil }); finish != nil || !errors.Is(err, revalidationErr) || !called {
		t.Fatalf("revalidação não falhou fechado: finish=%v err=%v called=%v", finish != nil, err, called)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	called = false
	if finish, err := service.BeginTransitionFromSnapshot(cancelled, fresh,
		func(context.Context) error { called = true; return nil },
		func(context.Context) error { called = true; return nil }); finish != nil || !errors.Is(err, context.Canceled) || called {
		t.Fatalf("contexto cancelado aceito: finish=%v err=%v called=%v", finish != nil, err, called)
	}
}

func TestBeginTransitionFromSnapshotRejectsExistingTransitionAndClosing(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	snapshot, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := service.BeginTransition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer outer()
	called := false
	if finish, err := service.BeginTransitionFromSnapshot(context.Background(), snapshot,
		func(context.Context) error { called = true; return nil },
		func(context.Context) error { called = true; return nil }); finish != nil || !errors.Is(err, ErrStaleEpoch) || called {
		t.Fatalf("transição aninhada aceita: finish=%v err=%v called=%v", finish != nil, err, called)
	}

	outer()
	service.closing = true
	if finish, err := service.BeginTransitionFromSnapshot(context.Background(), snapshot,
		func(context.Context) error { called = true; return nil },
		func(context.Context) error { called = true; return nil }); finish != nil || !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("core closing aceito: finish=%v err=%v", finish != nil, err)
	}
}
