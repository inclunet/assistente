package history

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSearchConversations_Name(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	if tool.Name() != "search_conversations" {
		t.Errorf("expected 'search_conversations', got '%s'", tool.Name())
	}
}

func TestSearchConversations_Description(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	desc := tool.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
}

func TestSearchConversations_Parameters(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() must return valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Error("schema must have type=object")
	}
	props := schema["properties"].(map[string]interface{})
	if _, ok := props["query"]; !ok {
		t.Error("schema must have 'query' property")
	}
	required := schema["required"].([]interface{})
	found := false
	for _, r := range required {
		if r == "query" {
			found = true
		}
	}
	if !found {
		t.Error("'query' should be required")
	}
}

func TestSearchConversations_EmptyQuery(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	args := `{"query": ""}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("empty query should return error result")
	}
}

func TestSearchConversations_InvalidArgs(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	args := `{invalid`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("invalid args should return error result")
	}
}

// TestSearchConversations_ProductionConstructorRejectsNilRepo garante que
// o construtor de produção não aceita repo == nil. Antes do B13 do AEP-0052
// passar nil silenciosamente caía no fallback database.* fail-open — agora
// é panic explícito no wiring, antes do agente ser registrado.
func TestSearchConversations_ProductionConstructorRejectsNilRepo(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewSearchConversations(nil) deveria entrar em panic")
		}
	}()
	_ = NewSearchConversations(nil)
}

// TestSearchConversations_FallbackRejectsContextWithoutUserID confirma que,
// mesmo no caminho de fallback (NewSearchConversationsForTest com repo nil),
// o tool rejeita ctx sem userID antes de chegar ao banco. Defesa em camadas
// para o caso (improvável) de um teste ou bug de wiring chamar Execute com
// ctx pelado em produção.
func TestSearchConversations_FallbackRejectsContextWithoutUserID(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	args := `{"query": "qualquer coisa"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("ctx sem userID no fallback deveria retornar IsError")
	}
}
