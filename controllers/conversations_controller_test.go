package controllers

import (
	"context"
	"errors"
	"reflect"
	"testing"

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
		PrepareBatchDelete: func(_ context.Context, ids []string) (func(), error) {
			order = append(order, "preparar")
			return func() { order = append(order, "liberar") }, nil
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
		"validar", "preparar", "excluir",
		"reset:conv-1", "evento:conversation:deleted",
		"reset:conv-2", "evento:conversation:deleted",
		"liberar",
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
		PrepareBatchDelete: func(_ context.Context, ids []string) (func(), error) {
			order = append(order, "preparar")
			return func() { order = append(order, "liberar") }, nil
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
	if want := []string{"validar", "preparar", "excluir", "liberar"}; !reflect.DeepEqual(order, want) {
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
		PrepareBatchDelete: func(_ context.Context, ids []string) (func(), error) {
			called = true
			return func() {}, nil
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
