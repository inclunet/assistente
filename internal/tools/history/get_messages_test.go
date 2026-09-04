package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestGetMessagesExpandsEachTurnOnlyOnce(t *testing.T) {
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
	if reader.turnCalls[turnID] != 1 {
		t.Fatalf("turn queried %d times, want 1", reader.turnCalls[turnID])
	}
}

func TestMessagePayloadOmitsEmptyToolCallSentinels(t *testing.T) {
	for _, sentinel := range []string{"", " ", "[]", " null "} {
		item, err := messagePayload(database.ChatMessage{ToolCalls: sentinel})
		if err != nil {
			t.Fatalf("messagePayload(%q): %v", sentinel, err)
		}
		if item.ToolCalls != nil {
			t.Errorf("messagePayload(%q) returned tool_calls=%s, want omitted", sentinel, item.ToolCalls)
		}
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

func TestMessagePayloadNormalizesLegacySingleToolCall(t *testing.T) {
	item, err := messagePayload(database.ChatMessage{
		ToolCalls: `{"id":"legacy-call","type":"function","function":{"name":"search","arguments":"{}"}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	var calls []map[string]any
	if err := json.Unmarshal(item.ToolCalls, &calls); err != nil {
		t.Fatalf("normalized tool_calls is not an array: %v", err)
	}
	if len(calls) != 1 || calls[0]["id"] != "legacy-call" {
		t.Fatalf("unexpected normalized tool_calls: %#v", calls)
	}
}
