package history

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
		ToolCalls:      `[{"id":"call-1","type":"function","function":{"name":"web_fetch","arguments":"{}"}}]`,
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
	if _, ok := item["tool_calls"].([]any); !ok {
		t.Fatalf("tool_calls should be a JSON array: %#v", item["tool_calls"])
	}
	if _, exists := item["tool_call_id"]; exists {
		t.Fatal("empty tool_call_id should be omitted")
	}
}

func TestGetMessagesIntegrationIncludesToolResultsOnlyWhenRequested(t *testing.T) {
	setupConvInfoDB(t)
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
		ToolCalls:      `[{"id":"call-1","type":"function","function":{"name":"web_fetch","arguments":"{}"}}]`,
	})
	seedHistoryMessage(t, database.ChatMessage{
		ConversationID: convID,
		TurnID:         &turnID,
		Role:           "tool",
		Content:        "resultado integral da tool",
		ToolCallID:     "call-1",
	})

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
