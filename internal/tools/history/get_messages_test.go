package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
)

type fakeMessageReader struct {
	messages  map[string]database.ChatMessage
	turns     map[string][]database.ChatMessage
	turnCalls map[string]int
}

func (f *fakeMessageReader) GetMessageWithContext(_ context.Context, id string) (*database.ChatMessage, error) {
	msg, ok := f.messages[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &msg, nil
}

func (f *fakeMessageReader) GetTurnMessagesWithContext(_ context.Context, turnID string) ([]database.ChatMessage, error) {
	if f.turnCalls != nil {
		f.turnCalls[turnID]++
	}
	return f.turns[turnID], nil
}

func TestGetMessagesContract(t *testing.T) {
	tool := NewGetMessages()
	if tool.Name() != "get_messages" {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	meta := tool.CatalogMetadata()
	if meta.Category != "history" || meta.Class != "read_context" || meta.Package != "history" || meta.Risk != "read" {
		t.Fatalf("unexpected catalog metadata: %+v", meta)
	}

	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("invalid schema: %v", err)
	}
	properties := schema["properties"].(map[string]any)
	ids := properties["ids"].(map[string]any)
	if ids["maxItems"] != float64(maxGetMessagesIDs) {
		t.Fatalf("maxItems = %v, want %d", ids["maxItems"], maxGetMessagesIDs)
	}
	if _, ok := properties["include_tool_results"]; !ok {
		t.Fatal("include_tool_results missing from schema")
	}
}

func TestGetMessagesDescriptionExplainsRehydrationScopeAndCost(t *testing.T) {
	tool := NewGetMessages()
	description := strings.ToLower(tool.Description())
	for _, concept := range []string{"known historical message ids", "search_conversations", "accessible to the current user", "whole conversation", "20 ids per call", "output cost"} {
		if !strings.Contains(description, concept) {
			t.Errorf("description should explain %q", concept)
		}
	}

	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("invalid parameter schema: %v", err)
	}
	if !strings.Contains(strings.ToLower(schema.Properties["ids"].Description), "separate calls") {
		t.Error("ids should explain batching beyond the per-call limit")
	}
	if !strings.Contains(strings.ToLower(schema.Properties["include_tool_results"].Description), "response size") {
		t.Error("include_tool_results should explain its output cost")
	}
}

func TestGetMessagesRejectsInvalidLimitsAndIDs(t *testing.T) {
	tool := NewGetMessages()
	cases := []json.RawMessage{
		json.RawMessage(`{}`),
		json.RawMessage(`{"ids":[]}`),
		json.RawMessage(`{"ids":[" "]}`),
		json.RawMessage(`{invalid`),
	}
	tooMany := make([]string, maxGetMessagesIDs+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("id-%d", i)
	}
	encoded, _ := json.Marshal(map[string]any{"ids": tooMany})
	cases = append(cases, encoded)

	for _, args := range cases {
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("Execute(%s): %v", args, err)
		}
		if !result.IsError {
			t.Errorf("Execute(%s) should fail", args)
		}
	}
}

func TestGetMessagesPreservesOrderAndDeduplicates(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	reader := &fakeMessageReader{messages: map[string]database.ChatMessage{
		"a": {UUIDModel: database.UUIDModel{ID: "a", CreatedAt: now}, ConversationID: "conv", Role: "user", Content: "A"},
		"b": {UUIDModel: database.UUIDModel{ID: "b", CreatedAt: now}, ConversationID: "conv", Role: "assistant", Content: "B"},
	}}
	tool := &GetMessagesTool{reader: reader}
	ctx := database.WithUserID(context.Background(), "user")
	result, err := tool.Execute(ctx, json.RawMessage(`{"ids":["b","a","b"]}`))
	if err != nil || result.IsError {
		t.Fatalf("unexpected result: %+v, err=%v", result, err)
	}
	var payload getMessagesPayload
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Messages) != 2 || payload.Messages[0].ID != "b" || payload.Messages[1].ID != "a" {
		t.Fatalf("unexpected messages: %+v", payload.Messages)
	}
	if !result.Structured {
		t.Fatal("JSON result must be marked Structured")
	}
}

func TestGetMessagesDoesNotReadLegacyTurnMessages(t *testing.T) {
	turnID := "user-1"
	reader := &fakeMessageReader{
		messages: map[string]database.ChatMessage{
			"user-1": {
				UUIDModel:      database.UUIDModel{ID: "user-1"},
				ConversationID: "conv",
				Role:           "user",
			},
			"assistant-1": {
				UUIDModel:      database.UUIDModel{ID: "assistant-1"},
				ConversationID: "conv",
				TurnID:         &turnID,
				Role:           "assistant",
			},
		},
		turns:     map[string][]database.ChatMessage{turnID: {}},
		turnCalls: map[string]int{},
	}
	tool := &GetMessagesTool{reader: reader}
	ctx := database.WithUserID(context.Background(), "user")

	result, err := tool.Execute(ctx, json.RawMessage(`{"ids":["user-1","assistant-1"],"include_tool_results":true}`))
	if err != nil || result.IsError {
		t.Fatalf("unexpected result: %+v, err=%v", result, err)
	}
	if reader.turnCalls[turnID] != 0 {
		t.Fatalf("leitura legada do turno ocorreu %d vezes", reader.turnCalls[turnID])
	}
}

func TestMessagePayloadNeverEmbedsToolProtocol(t *testing.T) {
	item, err := messagePayload(database.ChatMessage{Role: "assistant", Content: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if item.ToolCalls != nil || item.ToolCallID != "" {
		t.Fatalf("mensagem conversacional expôs protocolo técnico: %+v", item)
	}
}

func TestMessagePayloadPreservesTimestampPrecision(t *testing.T) {
	createdAt := time.Date(2026, 9, 4, 14, 32, 7, 123456789, time.FixedZone("UTC-3", -3*60*60))
	item, err := messagePayload(database.ChatMessage{
		UUIDModel: database.UUIDModel{CreatedAt: createdAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.CreatedAt != "2026-09-04T17:32:07.123456789Z" {
		t.Fatalf("created_at = %q, want timestamp UTC with full precision", item.CreatedAt)
	}
}

func TestMessagePayloadPreservaConteudoConversacional(t *testing.T) {
	item, err := messagePayload(database.ChatMessage{
		Role: "assistant", Content: "resposta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Role != "assistant" || item.Content != "resposta" || item.ToolCalls != nil {
		t.Fatalf("payload inesperado: %+v", item)
	}
}
