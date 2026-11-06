package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalJSONSizeMatchesWireEncoding(t *testing.T) {
	for _, text := range []string{"", "ação 😀 <>&\n\t\"\\", string([]byte{0xff, 0, 0xfe}), strings.Repeat("large text <>&", 1<<15)} {
		item := CanonicalItem{ItemID: "turn:opaque", ConversationID: "conversation", MessageIDs: []string{}, Segments: []CanonicalSegment{
			{SegmentID: "text", Kind: "text", Text: text},
			{SegmentID: "tools", Kind: "tool_invocations", Invocations: []TurnSegmentToolCall{{InvocationID: "invocation", ID: "call", InputPreview: text, OutputPreview: text}}},
		}, Metadata: CanonicalMetadata{ConclusionSegmentIDs: []string{"text"}}}
		for _, value := range []any{item, CanonicalBatch{ConversationID: "conversation", Operations: []CanonicalOperation{{Kind: "item_upsert", Item: &item}}}, CanonicalBatch{Operations: []CanonicalOperation{}}, CanonicalBatch{}} {
			payload, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if got := canonicalJSONSize(value); got != len(payload) {
				t.Fatalf("cota divergente para %T: %d != %d", value, got, len(payload))
			}
		}
	}
}
