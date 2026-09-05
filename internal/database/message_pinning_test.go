package database

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestMessagePinningPersistsAndIsUserScoped(t *testing.T) {
	setupUserScopeTestDB(t)

	anaCtx := WithUserID(context.Background(), "user-ana")
	leoCtx := WithUserID(context.Background(), "user-leo")
	anaConversation, err := CreateConversationWithContext(anaCtx, "Ana", "")
	if err != nil {
		t.Fatalf("create ana conversation: %v", err)
	}
	leoConversation, err := CreateConversationWithContext(leoCtx, "Leo", "")
	if err != nil {
		t.Fatalf("create leo conversation: %v", err)
	}
	anaMessage, err := CreateMessageWithContext(anaCtx, MessageOptions{
		ConversationID: anaConversation.ID,
		Role:           "user",
		Content:        "Fixar isto",
	})
	if err != nil {
		t.Fatalf("create ana message: %v", err)
	}
	leoMessage, err := CreateMessageWithContext(leoCtx, MessageOptions{
		ConversationID: leoConversation.ID,
		Role:           "assistant",
		Content:        "Privada",
	})
	if err != nil {
		t.Fatalf("create leo message: %v", err)
	}

	pinned, err := ToggleMessagePinWithContext(anaCtx, anaMessage.ID)
	if err != nil {
		t.Fatalf("pin ana message: %v", err)
	}
	if !pinned.Pinned {
		t.Fatal("expected pinned=true")
	}
	reloaded, err := GetMessageWithContext(anaCtx, anaMessage.ID)
	if err != nil || !reloaded.Pinned {
		t.Fatalf("pinned state was not persisted: message=%+v err=%v", reloaded, err)
	}

	list, err := GetPinnedMessagesWithContext(anaCtx, anaConversation.ID)
	if err != nil {
		t.Fatalf("list ana pinned: %v", err)
	}
	if len(list) != 1 || list[0].ID != anaMessage.ID {
		t.Fatalf("unexpected pinned list: %+v", list)
	}
	if _, err := ToggleMessagePinWithContext(anaCtx, leoMessage.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user toggle error = %v, want record not found", err)
	}
	if _, err := GetPinnedMessagesWithContext(anaCtx, leoConversation.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user list error = %v, want record not found", err)
	}

	unpinned, err := ToggleMessagePinWithContext(anaCtx, anaMessage.ID)
	if err != nil {
		t.Fatalf("unpin ana message: %v", err)
	}
	if unpinned.Pinned {
		t.Fatal("expected pinned=false")
	}
}

func TestMessagePinningRequiresAuthenticatedUser(t *testing.T) {
	setupUserScopeTestDB(t)
	if _, err := ToggleMessagePinWithContext(context.Background(), "message"); !errors.Is(err, ErrUserScopeRequired) {
		t.Fatalf("toggle without user error = %v, want ErrUserScopeRequired", err)
	}
	if _, err := GetPinnedMessagesWithContext(context.Background(), "conversation"); !errors.Is(err, ErrUserScopeRequired) {
		t.Fatalf("list without user error = %v, want ErrUserScopeRequired", err)
	}
}
