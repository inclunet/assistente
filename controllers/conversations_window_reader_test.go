package controllers

import (
	"assistente/internal/chat"
	"assistente/internal/database"
	"context"
	"errors"
	"testing"
)

func TestConversationWindowUsesCoordinatedReader(t *testing.T) {
	ctx := database.WithUserID(context.Background(), "owner")
	calls := 0
	denied := errors.New("falha da leitura coordenada")
	controller := NewConversationsController(ConversationsControllerConfig{ReadTimelineWindow: func(received context.Context, q database.MessageWindowQuery) (*chat.TimelineWindowSnapshot, error) {
		calls++
		user, err := database.RequireUserID(received)
		if err != nil || user != "owner" || q.ConversationID != "conversation" || q.ParentID == nil || *q.ParentID != "parent" {
			t.Fatalf("contexto/escopo não encaminhado: %+v %v", q, err)
		}
		return nil, denied
	}})
	result, err := controller.GetConversationMessageWindow(ctx, chat.MessageWindowRequest{ConversationID: "conversation", Scope: chat.MessageWindowScopeThread, ThreadParentID: "parent", Limit: 20})
	if result != nil || !errors.Is(err, denied) || calls != 1 {
		t.Fatalf("leitura duplicada ou fallback após erro: %+v %v %d", result, err, calls)
	}
}

func TestConversationWindowRequiresExplicitReader(t *testing.T) {
	controller := NewConversationsController(ConversationsControllerConfig{})
	result, err := controller.GetConversationMessageWindow(context.Background(), chat.MessageWindowRequest{ConversationID: "conversation", Limit: 20})
	if result != nil || err == nil {
		t.Fatal("dependência omitida aceitou leitura direta")
	}
}
