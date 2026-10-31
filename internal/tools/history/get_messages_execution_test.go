package history

import (
	"assistente/internal/database"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestGetMessagesHistoricalExecutionDoesNotHydrateCurrentTools(t *testing.T) {
	setupConvInfoDB(t)
	conv := seedConversation(t, "execuções")
	q := seedHistoryMessage(t, database.ChatMessage{ConversationID: conv, Role: "user", Content: "pergunta"})
	db := database.DB()
	turns := database.NewChatTurnRepository(db)
	first, err := turns.Begin(itCtx(conv), conv, q.ID, "", database.TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "history-execution", Origin: "builtin"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	for index, execution := range []string{first.ExecutionID, "retry"} {
		if index > 0 {
			if err := db.Exec("INSERT INTO chat_turn_attempts(execution_id,turn_id,mode,state,started_at) VALUES (?,?,'retry','preparing',CURRENT_TIMESTAMP)", execution, q.ID).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Exec("UPDATE chat_turns SET current_execution_id=?,active_execution_id=?,response_message_id=NULL,state='preparing' WHERE user_message_id=?", execution, execution, q.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Table("chat_messages").Create(map[string]any{"id": execution + "-answer", "conversation_id": conv, "turn_id": q.ID, "chat_execution_id": execution, "role": "assistant", "content": execution, "created_at": time.Now(), "updated_at": time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE chat_turn_attempts SET response_message_id=? WHERE execution_id=?", execution+"-answer", execution).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE chat_turns SET response_message_id=? WHERE user_message_id=?", execution+"-answer", q.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Table("tool_invocations").Create(map[string]any{"id": execution + "-call", "user_id": itUserID, "tool_catalog_id": catalog.ID, "origin_type": "chat", "origin_id": q.ID, "conversation_id": conv, "turn_id": q.ID, "chat_execution_id": execution, "tool_call_id": "same-call", "status": "succeeded", "queued_at": time.Now(), "output": fmt.Sprintf(`{"content":%q}`, execution+"-result")}).Error; err != nil {
			t.Fatal(err)
		}
		if err := turns.Finish(itCtx(conv), conv, q.ID, execution, database.TurnCompleted, "completed"); err != nil {
			t.Fatal(err)
		}
	}
	for _, execution := range []string{first.ExecutionID, "retry"} {
		args, _ := json.Marshal(map[string]any{"ids": []string{execution + "-answer"}, "include_tool_results": true})
		result, err := NewGetMessages().Execute(itCtx(conv), args)
		if err != nil || result.IsError {
			t.Fatalf("leitura: %+v %v", result, err)
		}
		var payload getMessagesPayload
		if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Messages) != 2 || payload.Messages[0].Content != execution || payload.Messages[1].Content != execution+"-result" {
			t.Fatalf("texto e ferramenta de execuções distintas: %+v", payload)
		}
	}
}
