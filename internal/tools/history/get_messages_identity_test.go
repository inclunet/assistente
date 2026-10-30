package history

import (
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestGetMessagesPreservesOpaqueMessageTurnCallAndLedgerIDs(t *testing.T) {
	setupConvInfoDB(t)
	db := database.DB()
	if err := db.AutoMigrate(&database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	conv := seedConversation(t, "Identidades distintas")
	catalog := database.ToolCatalog{Name: "echo", DisplayName: "Echo", Origin: "builtin"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	for index, turn := range []string{"turn", " turn "} {
		seedHistoryMessage(t, database.ChatMessage{UUIDModel: database.UUIDModel{ID: turn}, ConversationID: conv, Role: "user"})
		assistantID := []string{"assistant", " assistant "}[index]
		seedHistoryMessage(t, database.ChatMessage{UUIDModel: database.UUIDModel{ID: assistantID}, ConversationID: conv, Role: "assistant", TurnID: &turn})
		for callIndex, call := range []string{"call", " call "} {
			id := []string{"inv", " inv "}[callIndex] + assistantID
			output, _ := json.Marshal(map[string]string{"content": id})
			inv := database.ToolInvocation{UUIDModel: database.UUIDModel{ID: id}, UserID: itUserID, ToolCatalogID: catalog.ID,
				OriginType: "chat", OriginID: turn, ConversationID: &conv, TurnID: &turn, ToolCallID: call,
				Status: "succeeded", Output: string(output), QueuedAt: time.Unix(int64(callIndex), 0)}
			if err := db.Create(&inv).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// A seleção da resposta exercita TurnID; a seleção da pergunta exercita o ID raiz.
	for _, ids := range [][]string{{" assistant "}, {" turn "}, {"assistant", " assistant ", "assistant"}} {
		args, _ := json.Marshal(getMessagesArgs{IDs: ids, IncludeToolResults: true})
		result, err := NewGetMessages().Execute(itCtx(conv), args)
		if err != nil || result.IsError {
			t.Fatalf("consulta falhou: %+v, %v", result, err)
		}
		var payload getMessagesPayload
		if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
			t.Fatal(err)
		}
		wantCount := 3
		if len(ids) > 1 {
			wantCount = 6
		}
		if len(payload.Messages) != wantCount || payload.Messages[0].ID != ids[0] {
			t.Fatalf("mensagens alteradas: %+v", payload.Messages)
		}
		seen := map[string]bool{}
		for _, item := range payload.Messages {
			if item.Role != "tool" {
				continue
			}
			if seen[item.ID] || item.Content != item.ID {
				t.Fatalf("invocação colidiu ou conteúdo mudou: %+v", item)
			}
			seen[item.ID] = true
			if len(ids) == 1 && item.ID != "inv assistant " && item.ID != " inv  assistant " {
				t.Fatalf("resultado de outro turno: %+v", item)
			}
			if item.ToolCallID != "call" && item.ToolCallID != " call " {
				t.Fatalf("chamada alterada: %+v", item)
			}
		}
	}
}
