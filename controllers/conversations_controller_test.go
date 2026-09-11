package controllers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"assistente/internal/database"
)

type conversationRecordingEmitter struct {
	events *[]string
}

func (e conversationRecordingEmitter) Emit(event string, _ any) {
	*e.events = append(*e.events, "evento:"+event)
}

func TestNormalizeConversationPageRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		limit      int
		offset     int
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "preserva sem paginacao",
			limit:      0,
			offset:     0,
			wantLimit:  0,
			wantOffset: 0,
		},
		{
			name:       "normaliza offset antes de aplicar default",
			limit:      0,
			offset:     -1,
			wantLimit:  0,
			wantOffset: 0,
		},
		{
			name:       "usa limite default quando ha offset",
			limit:      0,
			offset:     1,
			wantLimit:  database.DefaultConversationPageLimit,
			wantOffset: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotLimit, gotOffset := normalizeConversationPageRequest(tt.limit, tt.offset)
			if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
				t.Fatalf("normalizeConversationPageRequest(%d, %d) = (%d, %d), want (%d, %d)",
					tt.limit, tt.offset, gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

func TestDeleteConversationsOrdenaPreflightPreparoCommitESideEffects(t *testing.T) {
	var order []string
	controller := NewConversationsController(ConversationsControllerConfig{
		Emitter: conversationRecordingEmitter{events: &order},
		ValidateBatchDelete: func(_ context.Context, ids []string) ([]string, error) {
			order = append(order, "validar")
			return []string{"conv-1", "conv-2"}, nil
		},
		PrepareBatchDelete: func(_ context.Context, ids []string) (func(bool), error) {
			order = append(order, "preparar")
			return func(committed bool) { order = append(order, fmt.Sprintf("finalizar:%t", committed)) }, nil
		},
		DeleteBatch: func(_ context.Context, ids []string) ([]string, error) {
			order = append(order, "excluir")
			return ids, nil
		},
		ResetScopedState: func(_ context.Context, id string) {
			order = append(order, "reset:"+id)
		},
	})

	deleted, err := controller.DeleteConversations(context.Background(), []string{" conv-1 ", "conv-2"})
	if err != nil {
		t.Fatalf("DeleteConversations: %v", err)
	}
	if want := []string{"conv-1", "conv-2"}; !reflect.DeepEqual(deleted, want) {
		t.Fatalf("IDs excluídos = %v, want %v", deleted, want)
	}
	wantOrder := []string{
		"validar", "preparar", "excluir", "finalizar:true",
		"reset:conv-1", "evento:conversation:deleted",
		"reset:conv-2", "evento:conversation:deleted",
	}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("ordem = %v, want %v", order, wantOrder)
	}
}

func TestDeleteConversationsLiberaPreparoSemSideEffectsQuandoCommitFalha(t *testing.T) {
	commitErr := errors.New("falha no commit")
	var order []string
	controller := NewConversationsController(ConversationsControllerConfig{
		Emitter: conversationRecordingEmitter{events: &order},
		ValidateBatchDelete: func(_ context.Context, ids []string) ([]string, error) {
			order = append(order, "validar")
			return ids, nil
		},
		PrepareBatchDelete: func(_ context.Context, ids []string) (func(bool), error) {
			order = append(order, "preparar")
			return func(committed bool) { order = append(order, fmt.Sprintf("finalizar:%t", committed)) }, nil
		},
		DeleteBatch: func(_ context.Context, ids []string) ([]string, error) {
			order = append(order, "excluir")
			return nil, commitErr
		},
		ResetScopedState: func(_ context.Context, id string) {
			order = append(order, "reset:"+id)
		},
	})

	_, err := controller.DeleteConversations(context.Background(), []string{"conv-1"})
	if !errors.Is(err, commitErr) {
		t.Fatalf("erro = %v, want %v", err, commitErr)
	}
	if want := []string{"validar", "preparar", "excluir", "finalizar:false"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("ordem = %v, want %v", order, want)
	}
}

func TestDeleteConversationsNaoPreparaQuandoPreflightFalha(t *testing.T) {
	preflightErr := errors.New("conversa não encontrada")
	called := false
	controller := NewConversationsController(ConversationsControllerConfig{
		ValidateBatchDelete: func(_ context.Context, ids []string) ([]string, error) {
			return nil, preflightErr
		},
		PrepareBatchDelete: func(_ context.Context, ids []string) (func(bool), error) {
			called = true
			return func(bool) {}, nil
		},
		DeleteBatch: func(_ context.Context, ids []string) ([]string, error) {
			called = true
			return ids, nil
		},
	})

	_, err := controller.DeleteConversations(context.Background(), []string{"conv-1"})
	if !errors.Is(err, preflightErr) {
		t.Fatalf("erro = %v, want %v", err, preflightErr)
	}
	if called {
		t.Fatal("preparo ou exclusão executado após falha no preflight")
	}
}

func TestClearConversationsRefazSnapshotAlteradoAntesDoCommit(t *testing.T) {
	snapshots := [][]database.Conversation{
		{{UUIDModel: database.UUIDModel{ID: "conv-1"}}},
		{{UUIDModel: database.UUIDModel{ID: "conv-1"}}, {UUIDModel: database.UUIDModel{ID: "conv-2"}}},
		{{UUIDModel: database.UUIDModel{ID: "conv-1"}}, {UUIDModel: database.UUIDModel{ID: "conv-2"}}},
		{{UUIDModel: database.UUIDModel{ID: "conv-1"}}, {UUIDModel: database.UUIDModel{ID: "conv-2"}}},
	}
	listCall := 0
	var finalized []bool
	var deletedBatches [][]string
	var events []string
	controller := NewConversationsController(ConversationsControllerConfig{
		Emitter: conversationRecordingEmitter{events: &events},
		ListConversations: func(context.Context) ([]database.Conversation, error) {
			result := snapshots[listCall]
			listCall++
			return result, nil
		},
		WithMaintenance: func(_ context.Context, fn func() error) error {
			return fn()
		},
		PrepareBatchDelete: func(_ context.Context, _ []string) (func(bool), error) {
			return func(committed bool) { finalized = append(finalized, committed) }, nil
		},
		DeleteWithinMaintenance: func(_ context.Context, ids []string) ([]string, error) {
			deletedBatches = append(deletedBatches, append([]string(nil), ids...))
			return ids, nil
		},
	})

	deleted, err := controller.ClearConversations(context.Background())
	if err != nil {
		t.Fatalf("ClearConversations: %v", err)
	}
	if want := []string{"conv-1", "conv-2"}; !reflect.DeepEqual(deleted, want) {
		t.Fatalf("IDs excluídos = %v, want %v", deleted, want)
	}
	if listCall != 4 || !reflect.DeepEqual(finalized, []bool{false, true}) {
		t.Fatalf("retry incorreto: listCall=%d finalized=%v", listCall, finalized)
	}
	if want := [][]string{{"conv-1", "conv-2"}}; !reflect.DeepEqual(deletedBatches, want) {
		t.Fatalf("batches mutados = %v, want %v", deletedBatches, want)
	}
	if len(events) != 2 {
		t.Fatalf("eventos pós-commit = %v", events)
	}
}

func TestClearConversationsPropagaErroSemEfeitosPosCommit(t *testing.T) {
	commitErr := errors.New("falha no commit")
	finalized := make([]bool, 0, 1)
	var events []string
	controller := NewConversationsController(ConversationsControllerConfig{
		Emitter: conversationRecordingEmitter{events: &events},
		ListConversations: func(context.Context) ([]database.Conversation, error) {
			return []database.Conversation{{UUIDModel: database.UUIDModel{ID: "conv-1"}}}, nil
		},
		WithMaintenance: func(_ context.Context, fn func() error) error {
			return fn()
		},
		PrepareBatchDelete: func(_ context.Context, _ []string) (func(bool), error) {
			return func(committed bool) { finalized = append(finalized, committed) }, nil
		},
		DeleteWithinMaintenance: func(context.Context, []string) ([]string, error) {
			return nil, commitErr
		},
	})

	if _, err := controller.ClearConversations(context.Background()); !errors.Is(err, commitErr) {
		t.Fatalf("erro = %v, want %v", err, commitErr)
	}
	if !reflect.DeepEqual(finalized, []bool{false}) || len(events) != 0 {
		t.Fatalf("efeitos em erro: finalized=%v events=%v", finalized, events)
	}
}

func TestDeleteConversationsMantemLifecycleGateDurantePosCommit(t *testing.T) {
	resetStarted := make(chan struct{})
	releaseReset := make(chan struct{})
	deleteDone := make(chan error, 1)
	controller := NewConversationsController(ConversationsControllerConfig{
		ValidateBatchDelete: func(_ context.Context, ids []string) ([]string, error) {
			return ids, nil
		},
		DeleteBatch: func(_ context.Context, ids []string) ([]string, error) {
			return ids, nil
		},
		ResetScopedState: func(context.Context, string) {
			close(resetStarted)
			<-releaseReset
		},
	})
	go func() {
		_, err := controller.DeleteConversations(context.Background(), []string{"conv-1"})
		deleteDone <- err
	}()
	<-resetStarted

	restoreAcquired := make(chan struct{})
	go func() {
		_ = database.WithConversationLifecycle(context.Background(), func() error {
			close(restoreAcquired)
			return nil
		})
	}()
	select {
	case <-restoreAcquired:
		t.Fatal("restauração atravessou efeitos pós-commit da exclusão")
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseReset)
	select {
	case err := <-deleteDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("exclusão não concluiu")
	}
	select {
	case <-restoreAcquired:
	case <-time.After(time.Second):
		t.Fatal("gate de ciclo de vida não foi liberado")
	}
}
