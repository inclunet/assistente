package history

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
)

func seedHistoryMessage(t *testing.T, message database.ChatMessage) database.ChatMessage {
	t.Helper()
	if err := database.DB().Create(&message).Error; err != nil {
		t.Fatalf("failed to create message: %v", err)
	}
	return message
}

func decodeGetMessagesPayload(t *testing.T, content string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("invalid JSON payload: %v\n%s", err, content)
	}
	return payload
}

func TestGetMessagesIntegrationFullPayloadOmitsLargeBinaries(t *testing.T) {
	setupConvInfoDB(t)
	convID := seedConversation(t, "Payload completo")
	fullContent := strings.Repeat("conteúdo integral ", 700)
	msg := seedHistoryMessage(t, database.ChatMessage{
		ConversationID: convID,
		Role:           "assistant",
		Content:        fullContent,
		Reasoning:      "raciocínio-privado-marcador",
		Media:          "base64-midia-marcador",
		Audio:          "base64-audio-marcador",
		AudioMimeType:  "audio/mpeg",
	})

	args, _ := json.Marshal(map[string]any{"ids": []string{msg.ID}})
	result, err := NewGetMessages().Execute(itCtx(convID), args)
	if err != nil || result.IsError {
		t.Fatalf("unexpected result: %+v, err=%v", result, err)
	}
	if !strings.Contains(result.Content, fullContent) {
		t.Fatal("complete message content was not returned")
	}
	for _, forbidden := range []string{"base64-midia-marcador", "base64-audio-marcador", "raciocínio-privado-marcador", `"media"`, `"audio"`} {
		if strings.Contains(result.Content, forbidden) {
			t.Fatalf("payload exposed omitted field %q: %s", forbidden, result.Content)
		}
	}

	payload := decodeGetMessagesPayload(t, result.Content)
	messages := payload["messages"].([]any)
	item := messages[0].(map[string]any)
	if item["content"] != fullContent {
		t.Fatal("content differs from persisted value")
	}
	if _, exists := item["tool_calls"]; exists {
		t.Fatalf("mensagem conversacional expôs tool_calls: %#v", item)
	}
	if _, exists := item["tool_call_id"]; exists {
		t.Fatal("empty tool_call_id should be omitted")
	}
}

func TestGetMessagesIntegrationIncludesToolResultsOnlyWhenRequested(t *testing.T) {
	setupConvInfoDB(t)
	if err := database.DB().AutoMigrate(&database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	convID := seedConversation(t, "Resultados de tools")
	userMessage := seedHistoryMessage(t, database.ChatMessage{
		ConversationID: convID,
		Role:           "user",
		Content:        "consulte a fonte",
	})
	turnID := userMessage.ID
	seedHistoryMessage(t, database.ChatMessage{
		ConversationID: convID,
		TurnID:         &turnID,
		Role:           "assistant",
		Content:        "vou consultar",
	})
	catalog := database.ToolCatalog{Name: "web_fetch", DisplayName: "Web fetch", Origin: "builtin", AvailabilityStatus: "available"}
	if err := database.DB().Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Create(&database.ToolInvocation{
		UserID: itUserID, ToolCatalogID: catalog.ID, OriginType: "chat",
		OriginID: turnID, ConversationID: &convID, TurnID: &turnID,
		ToolCallID: "call-1", Status: "succeeded",
		Output: `{"content":"resultado integral da tool"}`, QueuedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	argsWithout, _ := json.Marshal(map[string]any{"ids": []string{userMessage.ID}})
	without, err := NewGetMessages().Execute(itCtx(convID), argsWithout)
	if err != nil || without.IsError {
		t.Fatalf("without tool results: %+v, err=%v", without, err)
	}
	if strings.Contains(without.Content, "resultado integral da tool") {
		t.Fatal("tool result should be omitted by default")
	}

	argsWith, _ := json.Marshal(map[string]any{
		"ids":                  []string{userMessage.ID},
		"include_tool_results": true,
	})
	with, err := NewGetMessages().Execute(itCtx(convID), argsWith)
	if err != nil || with.IsError {
		t.Fatalf("with tool results: %+v, err=%v", with, err)
	}
	payload := decodeGetMessagesPayload(t, with.Content)
	messages := payload["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("got %d messages, want requested user message + tool result", len(messages))
	}
	toolResult := messages[1].(map[string]any)
	if toolResult["role"] != "tool" || toolResult["tool_call_id"] != "call-1" || toolResult["content"] != "resultado integral da tool" {
		t.Fatalf("unexpected tool result payload: %#v", toolResult)
	}
}

func TestGetMessagesIntegrationUsaSomenteLedger(t *testing.T) {
	setupConvInfoDB(t)
	if err := database.DB().AutoMigrate(
		&database.ToolCatalog{},
		&database.ToolInvocation{},
		&database.ToolLedgerMigrationState{},
	); err != nil {
		t.Fatal(err)
	}
	convID := seedConversation(t, "Ledger canônico")
	userMessage := seedHistoryMessage(t, database.ChatMessage{
		ConversationID: convID,
		Role:           "user",
		Content:        "consulte",
	})
	turnID := userMessage.ID
	seedHistoryMessage(t, database.ChatMessage{
		ConversationID: convID,
		TurnID:         &turnID,
		Role:           "assistant",
	})
	catalog := database.ToolCatalog{Name: "canonical", DisplayName: "Canonical", Origin: "builtin", AvailabilityStatus: "available"}
	if err := database.DB().Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	completed := time.Now().UTC()
	if err := database.DB().Create(&database.ToolInvocation{
		UserID:         itUserID,
		ToolCatalogID:  catalog.ID,
		OriginType:     "chat",
		OriginID:       turnID,
		ConversationID: &convID,
		TurnID:         &turnID,
		ToolCallID:     "call-1",
		Status:         "succeeded",
		Output:         `{"content":"CANONICO"}`,
		QueuedAt:       completed.Add(-time.Second),
		CompletedAt:    &completed,
	}).Error; err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"ids": []string{userMessage.ID}, "include_tool_results": true})
	result, err := NewGetMessages().Execute(itCtx(convID), args)
	if err != nil || result.IsError {
		t.Fatalf("resultado inesperado: %+v err=%v", result, err)
	}
	if !strings.Contains(result.Content, "CANONICO") {
		t.Fatalf("get_messages não fez cutover para o ledger: %s", result.Content)
	}
}

func TestGetMessagesIntegrationCrossUserFailsClosed(t *testing.T) {
	setupConvInfoDB(t)
	ownConvID := seedConversation(t, "Conversa própria")
	own := seedHistoryMessage(t, database.ChatMessage{
		ConversationID: ownConvID,
		Role:           "user",
		Content:        "conteúdo próprio não deve sair parcialmente",
	})

	foreignConv := &database.Conversation{Title: "Conversa alheia", UserID: "other-user"}
	if err := database.DB().Create(foreignConv).Error; err != nil {
		t.Fatal(err)
	}
	foreign := seedHistoryMessage(t, database.ChatMessage{
		ConversationID: foreignConv.ID,
		Role:           "user",
		Content:        "segredo de outro usuário",
	})

	args, _ := json.Marshal(map[string]any{"ids": []string{own.ID, foreign.ID}})
	ctx := database.WithUserID(context.Background(), itUserID)
	result, err := NewGetMessages().Execute(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("cross-user read should fail: %s", result.Content)
	}
	if strings.Contains(result.Content, own.Content) || strings.Contains(result.Content, foreign.Content) {
		t.Fatalf("error must not return partial or foreign content: %s", result.Content)
	}
}
