package toolinvocations

import (
	"assistente/internal/database"
	"context"
	"fmt"
	"testing"
	"time"
)

func TestExecutionSelectionKeepsCallsDistinctAcrossContinuations(t *testing.T) {
	db := setupProjectionReadTest(t)
	ctx := database.WithUserID(context.Background(), "user-a")
	conv := database.Conversation{UUIDModel: database.UUIDModel{ID: "execution-conversation"}, UserID: "user-a"}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	question := database.ChatMessage{UUIDModel: database.UUIDModel{ID: "question"}, ConversationID: conv.ID, Role: "user"}
	if err := db.Create(&question).Error; err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "execution-tool", Origin: "builtin"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	repo := database.NewChatTurnRepository(db)
	first, err := repo.Begin(ctx, conv.ID, question.ID, "", database.TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	begin := func(id, mode string, base, baseResponse any) {
		t.Helper()
		if err := db.Exec("INSERT INTO chat_turn_attempts(execution_id,turn_id,mode,state,started_at,base_execution_id,base_response_message_id) VALUES (?,'question',?,'preparing',CURRENT_TIMESTAMP,?,?)", id, mode, base, baseResponse).Error; err != nil {
			t.Fatal(err)
		}
	}
	selectAndPopulate := func(id string, attempt int) {
		t.Helper()
		if err := db.Exec("UPDATE chat_turns SET current_execution_id=?,active_execution_id=?,response_message_id=NULL,state='preparing' WHERE user_message_id='question'", id, id).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Table("chat_messages").Create(map[string]any{"id": id + "-answer", "conversation_id": conv.ID, "turn_id": question.ID, "chat_execution_id": id, "role": "assistant", "content": id, "created_at": time.Now(), "updated_at": time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE chat_turn_attempts SET response_message_id=? WHERE execution_id=?", id+"-answer", id).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE chat_turns SET response_message_id=? WHERE user_message_id='question'", id+"-answer").Error; err != nil {
			t.Fatal(err)
		}
		for _, call := range []string{"same", " same "} {
			if err := db.Table("tool_invocations").Create(map[string]any{"id": id + call, "user_id": "user-a", "tool_catalog_id": catalog.ID, "origin_type": "chat", "origin_id": question.ID, "conversation_id": conv.ID, "turn_id": question.ID, "chat_execution_id": id, "tool_call_id": call, "attempt": attempt, "status": "succeeded", "queued_at": time.Now(), "metadata": fmt.Sprintf(`{"display":{"assistant_message_id":%q,"iteration":0}}`, id+"-answer"), "output": fmt.Sprintf(`{"content":%q}`, id+call)}).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := repo.Finish(ctx, conv.ID, question.ID, id, database.TurnCompleted, "completed"); err != nil {
			t.Fatal(err)
		}
	}
	selectAndPopulate(first.ExecutionID, 99)
	begin("retry", database.TurnModeRetry, nil, nil)
	selectAndPopulate("retry", 1)
	begin("continue", database.TurnModeContinue, "retry", "retry-answer")
	selectAndPopulate("continue", 1)
	summaries, err := LoadSummariesForTurnIDsWithUser(ctx, "user-a", []string{question.ID})
	if err != nil {
		t.Fatal(err)
	}
	displays, err := LoadChatToolInvocationDisplaysForTurnIDsWithUser(ctx, "user-a", []string{question.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries[question.ID]) != 4 || len(displays[question.ID]) != 4 {
		t.Fatalf("execuções/chamadas colidiram: %+v / %+v", summaries, displays)
	}
	for i, call := range summaries[question.ID] {
		want := "retry"
		depth := 1
		if i >= 2 {
			want = "continue"
			depth = 0
		}
		if call.ExecutionID != want || call.ExecutionDepth != depth || call.AssistantMessageID != "continue-answer" {
			t.Fatalf("seleção/âncora incorreta: %+v", call)
		}
	}
	for _, call := range displays[question.ID] {
		if call.ExecutionID != "retry" && call.ExecutionID != "continue" {
			t.Fatalf("retry antigo ressuscitou: %+v", call)
		}
		if call.AssistantMessageID != "continue-answer" || call.ModelResult != call.ExecutionID+call.ID {
			t.Fatalf("resultado perdeu proveniência: %+v", call)
		}
	}
	historic, err := LoadChatToolInvocationDisplaysForExecutionWithUser(ctx, "user-a", question.ID, &first.ExecutionID)
	if err != nil || len(historic) != 2 {
		t.Fatalf("histórico explícito perdeu ferramentas: %+v %v", historic, err)
	}
	for _, call := range historic {
		if call.ExecutionID != first.ExecutionID || call.AssistantMessageID != first.ExecutionID+"-answer" {
			t.Fatalf("histórico misturou execução atual: %+v", call)
		}
	}
	current := "continue"
	historic, err = LoadChatToolInvocationDisplaysForExecutionWithUser(ctx, "user-a", question.ID, &current)
	if err != nil || len(historic) != 4 {
		t.Fatalf("cadeia histórica divergiu: %+v %v", historic, err)
	}
	all, err := LoadDetailsWithUser(ctx, "user-a", []string{first.ExecutionID + "same"})
	if err != nil || len(all) != 1 {
		t.Fatalf("auditoria perdeu execução anterior: %+v %v", all, err)
	}
	other, err := LoadSummariesForTurnIDsWithUser(ctx, "user-b", []string{question.ID})
	if err != nil || len(other) != 0 {
		t.Fatalf("escopo: %+v %v", other, err)
	}
}
