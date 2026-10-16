package tasklist

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
)

type commandMutationTestStore struct {
	TaskListRepository
	target      *database.TaskList
	fingerprint string
	commits     int
	lastRequest database.TaskListCommandMutationRequest
}

func (s *commandMutationTestStore) ReadCommandTarget(context.Context, string) (*database.TaskList, string, error) {
	return s.target, s.fingerprint, nil
}

func (s *commandMutationTestStore) CommitCommandMutation(_ context.Context, request database.TaskListCommandMutationRequest) (*database.TaskList, error) {
	s.lastRequest = request
	s.commits++
	return s.target, nil
}

type commandMutationTestEmitter struct {
	released bool
	events   []string
}

func (e *commandMutationTestEmitter) Emit(name string, _ any) {
	if !e.released {
		panic("evento emitido antes da liberação do guard")
	}
	e.events = append(e.events, name)
}

func TestCommandMutationCommitGuardedPersistsBeforeEvents(t *testing.T) {
	target := &database.TaskList{UUIDModel: database.UUIDModel{ID: "list-1"}, Title: "antes"}
	store := &commandMutationTestStore{target: target, fingerprint: "fp-1"}
	emitter := &commandMutationTestEmitter{}
	svc := NewService(ServiceConfig{Store: store, Emitter: emitter})
	ctx := database.WithUserID(context.Background(), "user-1")

	mutation, err := svc.PrepareCommandMutation(ctx, CommandMutationUpdate, target.ID, "depois", "", store.fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	inside := false
	result, err := mutation.CommitGuarded(ctx, func(commit func() error) error {
		inside = true
		if err := commit(); err != nil {
			return err
		}
		emitter.released = true
		return nil
	})
	if err != nil || result != target {
		t.Fatalf("commit: result=%+v err=%v", result, err)
	}
	if !inside || store.commits != 1 {
		t.Fatalf("guard/commit não executados uma vez: inside=%v commits=%d", inside, store.commits)
	}
	if len(emitter.events) != 1 || emitter.events[0] != "taskList:updated" {
		t.Fatalf("eventos após guard inesperados: %v", emitter.events)
	}
}

func TestPrepareCommandMutationRejectsStaleAndCommitIsOneShot(t *testing.T) {
	target := &database.TaskList{UUIDModel: database.UUIDModel{ID: "list-1"}}
	store := &commandMutationTestStore{target: target, fingerprint: "current"}
	svc := NewService(ServiceConfig{Store: store, Emitter: &commandMutationTestEmitter{released: true}})
	ctx := database.WithUserID(context.Background(), "user-1")
	if _, err := svc.PrepareCommandMutation(ctx, CommandMutationUpdate, target.ID, "novo", "", "old"); !errors.Is(err, ErrCommandMutationStale) {
		t.Fatalf("fingerprint stale aceito: %v", err)
	}

	mutation, err := svc.PrepareCommandMutation(ctx, CommandMutationUpdate, target.ID, "novo", "", "current")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.CommitGuarded(ctx, func(commit func() error) error { return commit() }); err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.CommitGuarded(ctx, func(commit func() error) error { return commit() }); !errors.Is(err, ErrCommandMutationConsumed) {
		t.Fatalf("replay aceito: %v", err)
	}
}

func TestPrepareCreateSealsEmptyFingerprint(t *testing.T) {
	store := &commandMutationTestStore{target: &database.TaskList{UUIDModel: database.UUIDModel{ID: "new"}}, fingerprint: "unused"}
	svc := NewService(ServiceConfig{Store: store, Emitter: &commandMutationTestEmitter{released: true}})
	ctx := database.WithUserID(context.Background(), "user-1")
	if _, err := svc.PrepareCommandMutation(ctx, CommandMutationCreate, "", "nova", "descrição", "não-vazio"); !errors.Is(err, ErrCommandMutationStale) {
		t.Fatalf("create com fingerprint não foi recusado: %v", err)
	}
	mutation, err := svc.PrepareCommandMutation(ctx, CommandMutationCreate, "", "nova", "descrição", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.CommitGuarded(ctx, func(commit func() error) error { return commit() }); err != nil {
		t.Fatal(err)
	}
	if store.commits != 1 {
		t.Fatalf("create não foi persistido uma vez: %d", store.commits)
	}
}

func TestPrepareCommandMutationRejectsBlankTitleForCreateAndUpdate(t *testing.T) {
	target := &database.TaskList{UUIDModel: database.UUIDModel{ID: "list-1"}}
	store := &commandMutationTestStore{target: target, fingerprint: "current"}
	svc := NewService(ServiceConfig{Store: store})
	ctx := database.WithUserID(context.Background(), "user-1")

	for _, operation := range []string{CommandMutationCreate, CommandMutationUpdate, CommandMutationClone} {
		id, fingerprint := "", ""
		if operation != CommandMutationCreate {
			id, fingerprint = target.ID, store.fingerprint
		}
		if _, err := svc.PrepareCommandMutation(ctx, operation, id, " \t\n ", "", fingerprint); !errors.Is(err, ErrCommandMutationInvalidTitle) {
			t.Fatalf("%s aceitou título em branco: %v", operation, err)
		}
	}
}

func TestClearCommandMutationSealsOperationAndEmitsAfterCommit(t *testing.T) {
	target := &database.TaskList{UUIDModel: database.UUIDModel{ID: "list-1"}, Title: "Lista", Slug: "lista"}
	store := &commandMutationTestStore{target: target, fingerprint: "clear-fp"}
	emitter := &commandMutationTestEmitter{}
	svc := NewService(ServiceConfig{Store: store, Emitter: emitter})
	ctx := database.WithUserID(context.Background(), "user-1")

	mutation, err := svc.PrepareCommandMutation(ctx, CommandMutationClear, target.ID, "", "", store.fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	result, err := mutation.CommitGuarded(ctx, func(commit func() error) error {
		if err := commit(); err != nil {
			return err
		}
		emitter.released = true
		return nil
	})
	if err != nil || result != target {
		t.Fatalf("clear commit: result=%+v err=%v", result, err)
	}
	if store.lastRequest.Operation != CommandMutationClear || store.commits != 1 {
		t.Fatalf("operação de clear não selada: request=%+v commits=%d", store.lastRequest, store.commits)
	}
	if len(emitter.events) != 1 || emitter.events[0] != "taskList:cleared" {
		t.Fatalf("evento de clear inesperado: %v", emitter.events)
	}
}

func TestClearCommandMutationRejectsStaleReplayCancellationAndOwnerChange(t *testing.T) {
	target := &database.TaskList{UUIDModel: database.UUIDModel{ID: "list-1"}}
	store := &commandMutationTestStore{target: target, fingerprint: "current"}
	svc := NewService(ServiceConfig{Store: store, Emitter: &commandMutationTestEmitter{released: true}})
	ctx := database.WithUserID(context.Background(), "owner-1")

	if _, err := svc.PrepareCommandMutation(ctx, CommandMutationClear, target.ID, "", "", "stale"); !errors.Is(err, ErrCommandMutationStale) {
		t.Fatalf("clear stale aceito: %v", err)
	}

	mutation, err := svc.PrepareCommandMutation(ctx, CommandMutationClear, target.ID, "", "", store.fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	otherOwner := database.WithUserID(context.Background(), "owner-2")
	called := false
	if _, err := mutation.CommitGuarded(otherOwner, func(func() error) error {
		called = true
		return nil
	}); !errors.Is(err, ErrCommandMutationOwner) || called {
		t.Fatalf("owner incorreto aceito: err=%v called=%v", err, called)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := mutation.CommitGuarded(cancelled, func(func() error) error {
		t.Fatal("cancelamento não deveria entrar no guard")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento não recusado: %v", err)
	}
	if store.commits != 0 {
		t.Fatalf("cancelamento persistiu clear: %d commits", store.commits)
	}
	if _, err := mutation.CommitGuarded(ctx, func(commit func() error) error { return commit() }); !errors.Is(err, ErrCommandMutationConsumed) {
		t.Fatalf("replay após cancelamento aceito: %v", err)
	}

	replay, err := svc.PrepareCommandMutation(ctx, CommandMutationClear, target.ID, "", "", store.fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := replay.CommitGuarded(ctx, func(commit func() error) error { return commit() }); err != nil {
		t.Fatalf("clear inicial falhou: %v", err)
	}
	if _, err := replay.CommitGuarded(ctx, func(commit func() error) error { return commit() }); !errors.Is(err, ErrCommandMutationConsumed) {
		t.Fatalf("replay aceito: %v", err)
	}
}
