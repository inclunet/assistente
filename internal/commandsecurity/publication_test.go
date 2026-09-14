package commandsecurity

import (
	"context"
	"errors"
	"math"
	"testing"
)

func publicationSnapshot(t *testing.T, service *EpochService, user, session string) EpochSnapshot {
	t.Helper()
	snapshot, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestPublishAuthenticatedConfigurationUsesExclusiveGateAndPreservesEpochs(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	snapshot := publicationSnapshot(t, service, user, session)

	revalidated, published := false, false
	err := service.PublishAuthenticatedConfiguration(context.Background(), snapshot,
		func(context.Context) error {
			revalidated = true
			assertEpochGateExclusive(t, service.gate)
			return nil
		}, func() error {
			published = true
			assertEpochGateExclusive(t, service.gate)
			return nil
		})
	if err != nil || !revalidated || !published {
		t.Fatalf("publicação = %v, revalidated=%v published=%v", err, revalidated, published)
	}

	fresh, err := service.Capture(context.Background(), user, session)
	if err != nil {
		t.Fatal(err)
	}
	if fresh != snapshot {
		t.Fatalf("publicação alterou epochs: antigo=%#v novo=%#v", snapshot, fresh)
	}
}

func TestPublishAuthenticatedConfigurationRejectsStaleBeforeCallbacks(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	snapshot := publicationSnapshot(t, service, user, session)
	if err := service.InvalidateSession(context.Background(), user, session); err != nil {
		t.Fatal(err)
	}

	calls := 0
	err := service.PublishAuthenticatedConfiguration(context.Background(), snapshot,
		func(context.Context) error { calls++; return nil },
		func() error { calls++; return nil })
	if !errors.Is(err, ErrStaleEpoch) || calls != 0 {
		t.Fatalf("snapshot stale = %v, callbacks=%d", err, calls)
	}
}

func TestPublishAuthenticatedConfigurationRevalidationFailureDoesNotPublishOrCancel(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	snapshot := publicationSnapshot(t, service, user, session)
	var executionContext context.Context
	release, err := service.AdmitExecution(context.Background(), snapshot,
		func(context.Context) error { return nil },
		func(ctx context.Context) error { executionContext = ctx; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	want := errors.New("revalidação falhou")
	published := false
	err = service.PublishAuthenticatedConfiguration(context.Background(), snapshot,
		func(context.Context) error { return want },
		func() error { published = true; return nil })
	if !errors.Is(err, want) || published {
		t.Fatalf("falha de revalidação = %v, published=%v", err, published)
	}
	select {
	case <-executionContext.Done():
		t.Fatal("falha de revalidação cancelou execução")
	default:
	}
}

func TestPublishAuthenticatedConfigurationContextCancellationDoesNotPublish(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	snapshot := publicationSnapshot(t, service, user, session)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	published := false
	err := service.PublishAuthenticatedConfiguration(ctx, snapshot,
		func(context.Context) error {
			cancel()
			return nil
		}, func() error {
			published = true
			return nil
		})
	if !errors.Is(err, context.Canceled) || published {
		t.Fatalf("cancelamento durante revalidação = %v, published=%v", err, published)
	}
}

func TestPublishAuthenticatedConfigurationRejectsTransitionBeforeCallbacks(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	snapshot := publicationSnapshot(t, service, user, session)
	finish, err := service.BeginTransition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer finish()

	calls := 0
	err = service.PublishAuthenticatedConfiguration(context.Background(), snapshot,
		func(context.Context) error { calls++; return nil },
		func() error { calls++; return nil })
	if !errors.Is(err, ErrStaleEpoch) || calls != 0 {
		t.Fatalf("publicação durante transição = %v, callbacks=%d", err, calls)
	}
}

func TestPublishAuthenticatedConfigurationRejectsDisabledBeforeCallbacks(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	snapshot := publicationSnapshot(t, service, user, session)
	service.sequence = math.MaxUint64
	if _, err := service.BeginTransition(context.Background()); !errors.Is(err, ErrInvalidEpochInput) {
		t.Fatal(err)
	}

	calls := 0
	err := service.PublishAuthenticatedConfiguration(context.Background(), snapshot,
		func(context.Context) error { calls++; return nil },
		func() error { calls++; return nil })
	if !errors.Is(err, ErrStaleEpoch) || calls != 0 {
		t.Fatalf("publicação com serviço desabilitado = %v, callbacks=%d", err, calls)
	}
}

func TestSecurityGenerationOverflowDisablesServiceRejectsOldSnapshotAndCancelsExecutions(t *testing.T) {
	service := newEpochServiceForTest(t)
	user, session := testEpochID(t), testEpochID(t)
	snapshot := publicationSnapshot(t, service, user, session)
	var runContext context.Context
	release, err := service.AdmitExecution(context.Background(), snapshot,
		func(context.Context) error { return nil },
		func(ctx context.Context) error { runContext = ctx; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	service.sequence = math.MaxUint64
	actionCalled := false
	err = service.MutateSecurity(context.Background(), func() error {
		actionCalled = true
		return nil
	})
	if !errors.Is(err, ErrInvalidEpochInput) || actionCalled || !service.disabled {
		t.Fatalf("overflow de security = %v, actionCalled=%v disabled=%v", err, actionCalled, service.disabled)
	}
	select {
	case <-runContext.Done():
	default:
		t.Fatal("overflow de security não cancelou a execução")
	}

	calls := 0
	err = service.PublishAuthenticatedConfiguration(context.Background(), snapshot,
		func(context.Context) error { calls++; return nil },
		func() error { calls++; return nil })
	if !errors.Is(err, ErrStaleEpoch) || calls != 0 {
		t.Fatalf("snapshot antigo após overflow = %v, callbacks=%d", err, calls)
	}
}

func TestPublishAuthenticatedConfigurationCancelsOnlyTheSnapshotUser(t *testing.T) {
	service := newEpochServiceForTest(t)
	userA, userB := testEpochID(t), testEpochID(t)
	snapshotA1 := publicationSnapshot(t, service, userA, testEpochID(t))
	snapshotA2 := publicationSnapshot(t, service, userA, testEpochID(t))
	snapshotB := publicationSnapshot(t, service, userB, testEpochID(t))

	contexts := make([]context.Context, 0, 3)
	for _, snapshot := range []EpochSnapshot{snapshotA1, snapshotA2, snapshotB} {
		var runContext context.Context
		release, err := service.AdmitExecution(context.Background(), snapshot,
			func(context.Context) error { return nil },
			func(ctx context.Context) error { runContext = ctx; return nil })
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		contexts = append(contexts, runContext)
	}

	if err := service.PublishAuthenticatedConfiguration(context.Background(), snapshotA1,
		func(context.Context) error { return nil }, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		select {
		case <-contexts[i].Done():
		default:
			t.Fatalf("execução %d de A não foi cancelada", i)
		}
	}
	select {
	case <-contexts[2].Done():
		t.Fatal("publicação de A cancelou execução de B")
	default:
	}
}
