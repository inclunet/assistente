package history

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSearchConversations_Name(t *testing.T) {
	tool := NewSearchConversations(nil)
	if tool.Name() != "search_conversations" {
		t.Errorf("expected 'search_conversations', got '%s'", tool.Name())
	}
}

func TestSearchConversations_Description(t *testing.T) {
	tool := NewSearchConversations(nil)
	desc := tool.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
}

func TestSearchConversations_Parameters(t *testing.T) {
	tool := NewSearchConversations(nil)
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
	tool := NewSearchConversations(nil)
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
	tool := NewSearchConversations(nil)
	args := `{invalid`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("invalid args should return error result")
	}
}
