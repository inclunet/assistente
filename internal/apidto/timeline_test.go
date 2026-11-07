package apidto

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTimelineWireSnapshotPreservesIdentityAndPrecision(t *testing.T) {
	const sequence = "9007199254740993"
	snapshot := SubscribedTimelineSnapshot{
		ConversationID: "conversation", WindowID: "window", Epoch: "epoch",
		Sequence: sequence, WindowRevision: sequence, Mode: TimelineWindowFollowTail,
		Items:          []CanonicalItem{{ItemID: "message:one", Revision: sequence}},
		OrderedItemIDs: []string{"message:one"}, RetainedItemIDs: []string{},
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["sequence"] != sequence || wire["windowRevision"] != sequence || wire["conversationId"] != "conversation" || wire["mode"] != "followTail" {
		t.Fatalf("identidade ou precisão perdida: %s", raw)
	}
	if value, ok := wire["scopeParentId"]; !ok || value != nil {
		t.Fatalf("escopo raiz deve ser null explícito: %s", raw)
	}
	item := wire["items"].([]any)[0].(map[string]any)
	for _, key := range []string{"representativeMessageId"} {
		if value, ok := item[key]; !ok || value != nil {
			t.Fatalf("%s deve ser null explícito: %s", key, raw)
		}
	}
	for _, key := range []string{"conversationId", "scopeParentId", "messageIds", "replyToItemId", "kind", "turnId", "state", "durability"} {
		if _, exists := item[key]; exists {
			t.Fatalf("campo redundante %s: %s", key, raw)
		}
	}
	if item["revision"] != sequence {
		t.Fatalf("revisão perdeu precisão: %s", raw)
	}
	var restored SubscribedTimelineSnapshot
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, restored) {
		t.Fatal("round-trip alterou snapshot")
	}
}

func TestTimelineWireRequestsUseFlatLowerCamelContract(t *testing.T) {
	var navigation TimelineWindowNavigation
	err := json.Unmarshal([]byte(`{"mode":"followTail","epoch":"e","windowRevision":"9007199254740993","anchor":"end","direction":"before","limit":20}`), &navigation)
	if err != nil {
		t.Fatal(err)
	}
	if navigation.Mode != TimelineWindowFollowTail || navigation.Anchor != "end" || navigation.Direction != "before" || navigation.Limit != 20 {
		t.Fatalf("pedido não decodificado: %+v", navigation)
	}
	raw, err := json.Marshal(navigation)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["windowRevision"] != "9007199254740993" || wire["anchor"] != "end" {
		t.Fatalf("contrato flat alterado: %s", raw)
	}
	for key := range wire {
		if key[0] >= 'A' && key[0] <= 'Z' {
			t.Fatalf("campo não lowerCamel: %s", key)
		}
	}
	raw, err = json.Marshal(TimelineRetentionRequest{Epoch: "e", WindowRevision: "2", ItemIDs: []string{"message:one"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"epoch":"e","windowRevision":"2","itemIds":["message:one"]}` {
		t.Fatalf("retenção alterou contrato: %s", raw)
	}
}

func TestTimelineToolSummaryOmitsInternalProvenance(t *testing.T) {
	call := TurnSegmentToolCall{InvocationID: "ledger-id", ExecutionID: "private-execution", ExecutionDepth: 9, AssistantMessageID: "private-message", ID: "call", Name: "tool", Status: "succeeded", InputPreview: "resumo", ResultAvailability: "available"}
	raw, err := json.Marshal(call)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"invocationId": "ledger-id", "callId": "call", "name": "tool", "status": "succeeded", "inputPreview": "resumo", "hasDetails": false, "resultAvailability": "available"}
	if !reflect.DeepEqual(wire, want) {
		t.Fatalf("resumo expôs campos inesperados: %s", raw)
	}
}

func TestCanonicalMetadataOmitsDefaultsWithoutLosingValues(t *testing.T) {
	for _, metadata := range []CanonicalMetadata{
		{CreatedAt: "2026-01-01T00:00:00Z", Role: "assistant"},
		{CreatedAt: "2026-01-01T00:00:00Z", Role: "assistant", Source: "channel", ChildCount: 2, Pinned: true, Model: "model", PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7, CacheReadTokens: 5, CacheWriteTokens: 6, CacheMissTokens: 7},
	} {
		original := CanonicalItem{ItemID: "turn:q", Metadata: metadata}
		raw, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var decoded CanonicalItem
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatal("wire perdeu metadados", string(raw))
		}
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		if _, exists := wire["position"]; exists {
			t.Fatal("posição interna vazou")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(wire["metadata"], &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"timestamp", "threadParentMessageId"} {
			if _, exists := fields[key]; exists {
				t.Fatal("metadado redundante", key)
			}
		}
		if !metadata.Pinned {
			for _, key := range []string{"source", "childCount", "pinned", "model", "promptTokens", "completionTokens", "totalTokens", "cacheReadTokens", "cacheWriteTokens", "cacheMissTokens"} {
				if _, exists := fields[key]; exists {
					t.Fatal("default redundante", key)
				}
			}
		}
	}
}
