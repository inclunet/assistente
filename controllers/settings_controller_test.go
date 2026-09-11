package controllers

import (
	"context"
	"testing"

	"assistente/internal/database"
)

func TestSettingsControllerClearMessagesDelegatesCanonicalPipeline(t *testing.T) {
	events := make([]string, 0, 1)
	emitter := conversationRecordingEmitter{events: &events}
	called := false
	controller := NewSettingsController(SettingsControllerConfig{
		Emitter: emitter,
		ClearMessages: func(ctx context.Context) error {
			called = true
			if userID, ok := database.UserIDFromContext(ctx); !ok || userID != "user-1" {
				t.Fatalf("contexto sem usuário: %q %t", userID, ok)
			}
			return nil
		},
	})

	if err := controller.ClearMessages(database.WithUserID(context.Background(), "user-1")); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("pipeline canônico não foi chamado")
	}
	if len(events) != 1 || events[0] != "evento:messages:cleared" {
		t.Fatalf("eventos=%v", events)
	}
}

func TestSettingsControllerClearMessagesFailsClosedWithoutPipeline(t *testing.T) {
	controller := NewSettingsController(SettingsControllerConfig{})
	if err := controller.ClearMessages(context.Background()); err == nil {
		t.Fatal("clear sem pipeline canônico foi aceito")
	}
}
