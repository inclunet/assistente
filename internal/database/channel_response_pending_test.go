package database

import (
	"testing"
	"time"
)

func TestFindFirstAssistantMessageAfter_UsesTurnNotStaleAssistant(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()
	convID := createTestConversation(t, "canal")

	t0 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Minute)
	t2 := t0.Add(2 * time.Minute)
	t3 := t0.Add(3 * time.Minute)

	// Turno A (antigo): user em t0, assistant só em t3 (atrasada).
	userA := &ChatMessage{
		UUIDModel:      UUIDModel{CreatedAt: t0, UpdatedAt: t0},
		ConversationID: convID,
		Role:           "user",
		Content:        "pergunta A",
	}
	if err := db.Create(userA).Error; err != nil {
		t.Fatalf("create userA: %v", err)
	}
	turnA := userA.ID
	asstA := &ChatMessage{
		UUIDModel:      UUIDModel{CreatedAt: t3, UpdatedAt: t3},
		ConversationID: convID,
		Role:           "assistant",
		Content:        "resposta A",
		TurnID:         &turnA,
	}
	if err := db.Create(asstA).Error; err != nil {
		t.Fatalf("create asstA: %v", err)
	}

	// Turno B (pendência CreatedAt=t1): user em t1, ainda sem assistant.
	userB := &ChatMessage{
		UUIDModel:      UUIDModel{CreatedAt: t1, UpdatedAt: t1},
		ConversationID: convID,
		Role:           "user",
		Content:        "pergunta B",
	}
	if err := db.Create(userB).Error; err != nil {
		t.Fatalf("create userB: %v", err)
	}

	// Pending do turno B não deve capturar a assistant A (criada depois de t1).
	got, err := FindFirstAssistantMessageAfter(ctx, convID, t1)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != nil {
		t.Fatalf("esperava nil (turno B sem assistant); got id=%s content=%q", got.ID, got.Content)
	}

	turnB := userB.ID
	asstB := &ChatMessage{
		UUIDModel:      UUIDModel{CreatedAt: t2, UpdatedAt: t2},
		ConversationID: convID,
		Role:           "assistant",
		Content:        "resposta B",
		TurnID:         &turnB,
	}
	if err := db.Create(asstB).Error; err != nil {
		t.Fatalf("create asstB: %v", err)
	}

	got, err = FindFirstAssistantMessageAfter(ctx, convID, t1)
	if err != nil {
		t.Fatalf("find após asstB: %v", err)
	}
	if got == nil || got.Content != "resposta B" {
		t.Fatalf("esperava resposta B; got %+v", got)
	}
}

func TestFindFirstAssistantMessageAfter_NoUserYet(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()
	convID := createTestConversation(t, "vazio")

	got, err := FindFirstAssistantMessageAfter(ctx, convID, time.Now().UTC())
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != nil {
		t.Fatalf("sem user após after → nil; got %+v", got)
	}
}
