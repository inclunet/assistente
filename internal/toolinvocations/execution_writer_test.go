package toolinvocations

import (
	"context"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestInvocationWriterScopesAttemptsAndAllowsLateResult(t *testing.T) {
	db := setupProjectionReadTest(t)
	ctx := database.WithUserID(context.Background(), "user-a")
	conv := database.Conversation{UUIDModel: database.UUIDModel{ID: "execution-writer"}, UserID: "user-a"}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	question := database.ChatMessage{UUIDModel: database.UUIDModel{ID: " question "}, ConversationID: conv.ID, Role: "user"}
	if err := db.Create(&question).Error; err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "writer-tool", Origin: "builtin"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	turns := database.NewChatTurnRepository(db)
	repo := NewDBRepository(db)
	first, err := turns.Begin(ctx, conv.ID, question.ID, "", database.TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	makeCall := func(execution string, call string) *Invocation {
		t.Helper()
		i := &Invocation{ToolCatalogID: catalog.ID, OriginType: OriginChat, OriginID: question.ID, ChatExecutionID: &execution, ToolCallID: call, Status: StatusRunning}
		if err := repo.Create(ctx, i); err != nil {
			t.Fatal(err)
		}
		return i
	}
	old := makeCall(first.ExecutionID, " call ")
	repeated := makeCall(first.ExecutionID, " call ")
	distinct := makeCall(first.ExecutionID, "call")
	if old.Attempt != 1 || repeated.Attempt != 2 || distinct.Attempt != 1 {
		t.Fatal("identidades de chamada colidiram")
	}
	if err := turns.Finish(ctx, conv.ID, question.ID, first.ExecutionID, database.TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	next, err := turns.Begin(ctx, conv.ID, question.ID, "", database.TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	current := makeCall(next.ExecutionID, " call ")
	if current.Attempt != 1 || current.ChatExecutionID == nil || *current.ChatExecutionID != next.ExecutionID {
		t.Fatalf("execuções misturadas: %+v", current)
	}
	if err := db.Model(&database.ToolInvocation{}).Where("id=?", old.ID).Updates(map[string]any{"status": StatusSucceeded, "completed_at": time.Now(), "output": "resultado tardio"}).Error; err != nil {
		t.Fatal(err)
	}
	stale := &Invocation{ToolCatalogID: catalog.ID, OriginType: OriginChat, OriginID: question.ID, ChatExecutionID: &first.ExecutionID, ToolCallID: "late-new"}
	if err := repo.Create(ctx, stale); err == nil {
		t.Fatal("nova chamada da execução encerrada aceita")
	}
	summaries, err := LoadSummariesForTurnIDsWithUser(ctx, "user-a", []string{question.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries[question.ID]) != 1 || summaries[question.ID][0].InvocationID != current.ID {
		t.Fatalf("resultado anterior visível: %+v", summaries)
	}
}
