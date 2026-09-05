package history

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/tools/invocationctx"
)

func TestGetConversationInfo_Name(t *testing.T) {
	tool := NewGetConversationInfo()
	if tool.Name() != "get_conversation_info" {
		t.Errorf("expected 'get_conversation_info', got '%s'", tool.Name())
	}
}

func TestGetConversationInfo_Description(t *testing.T) {
	tool := NewGetConversationInfo()
	description := strings.ToLower(tool.Description())
	for _, concept := range []string{"metadata", "rolling summary", "search_conversations", "get_messages", "one conversation", "not paginated"} {
		if !strings.Contains(description, concept) {
			t.Errorf("description should explain %q", concept)
		}
	}
}

func TestGetConversationInfo_Parameters(t *testing.T) {
	tool := NewGetConversationInfo()
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() must return valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Error("schema must have type=object")
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("schema must have properties object")
	}
	for _, key := range []string{"conversation_id", "include_messages", "message_limit"} {
		if _, ok := props[key]; !ok {
			t.Errorf("schema must have '%s' property", key)
		}
	}
	messageLimit := props["message_limit"].(map[string]interface{})
	if !strings.Contains(strings.ToLower(messageLimit["description"].(string)), "not a pagination cursor") {
		t.Error("message_limit should distinguish recent-message caps from pagination")
	}
	if schema["additionalProperties"] != false {
		t.Error("schema should forbid additionalProperties")
	}
}

func TestGetConversationInfo_InvalidArgs(t *testing.T) {
	tool := NewGetConversationInfo()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("invalid args should return error result")
	}
}

// Sem conversation_id explícito e sem InvocationContext, a tool deve falhar
// graciosamente antes de tocar o banco.
func TestGetConversationInfo_NoConversationAvailable(t *testing.T) {
	tool := NewGetConversationInfo()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError when no conversation id is available")
	}
	if !strings.Contains(strings.ToLower(result.Content), "conversation id") {
		t.Errorf("expected message about missing conversation id, got: %s", result.Content)
	}
}

// InvocationContext presente mas com ConversationID vazio também não resolve
// e deve cair no mesmo erro (sem chamar o banco).
func TestGetConversationInfo_EmptyInvocationConversation(t *testing.T) {
	tool := NewGetConversationInfo()
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{ConversationID: "   "})
	result, err := tool.Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError when invocation context has blank conversation id")
	}
}
