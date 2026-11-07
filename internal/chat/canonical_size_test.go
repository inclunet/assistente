package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalJSONSizeMatchesWireEncoding(t *testing.T) {
	for _, text := range []string{"", "ação 😀 <>&\n\t\"\\", string([]byte{0xff, 0, 0xfe}), strings.Repeat("large text <>&", 1<<15)} {
		item := CanonicalItem{ItemID: "turn:opaque", Segments: []CanonicalSegment{
			{SegmentID: "text", Kind: "text", Text: text, Conclusion: true},
			{SegmentID: "tools", Kind: "tool_invocations", Invocations: []TurnSegmentToolCall{{InvocationID: "invocation", ID: "call", InputPreview: text, OutputPreview: text}}},
		}}
		for _, value := range []any{item, &item, &CanonicalBatch{ConversationID: "conversation", Operations: []CanonicalOperation{{Kind: "item_upsert", Item: &item}}}, CanonicalBatch{Operations: []CanonicalOperation{}}, CanonicalBatch{}} {
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

func TestCanonicalStageCountsMaterializedPayloadAcrossSurfaces(t *testing.T) {
	item := CanonicalItem{ItemID: "turn:q", Segments: []CanonicalSegment{{SegmentID: "text", Kind: "text", Text: "início"}}}
	stage := &canonicalPublicationStage{}
	stage.append(nil, []CanonicalWindowPatch{
		{WindowID: "first", Items: []CanonicalItem{item}},
		{WindowID: "second", Items: []CanonicalItem{item}},
	})
	// JSON escapado e UTF-8 contam pelos bytes transmitidos, em cada superfície.
	item.Segments = []CanonicalSegment{{SegmentID: "text", Kind: "text", Text: "ação 😀 <>&\n\t\"\\"}}
	stage.materialize(item)
	payload, err := json.Marshal(CanonicalBatch{Operations: stage.operations, WindowPatches: stage.patches})
	if err != nil {
		t.Fatal(err)
	}
	if stage.overflow || stage.bytes != len(payload) || len(stage.patches) != 2 {
		t.Fatalf("cota difere do payload materializado: %d != %d", stage.bytes, len(payload))
	}
	for _, patch := range stage.patches {
		if patch.Items[0].Segments[0].Text != item.Segments[0].Text {
			t.Fatal("superfície não recebeu conteúdo materializado")
		}
	}
	item.Segments = []CanonicalSegment{{SegmentID: "text", Kind: "text", Text: strings.Repeat("x", canonicalReplayBytes/2)}}
	stage.materialize(item)
	if !stage.overflow || stage.patches != nil || stage.operations != nil {
		t.Fatal("payload repetido em duas superfícies excedeu cota sem descartar journal")
	}
}

func TestCanonicalCacheBudgetNeverUndercountsWireAndKeepsRecoveryExact(t *testing.T) {
	parent, turn, representative := "parent", "turn", "representative"
	for _, text := range []string{"", "ação 😀 <>&\n\t\"\\", string([]byte{0xff, 0, 0xfe}), strings.Repeat("large <>&", 1024)} {
		item := CanonicalItem{ItemID: text, ScopeParentID: &parent, TurnID: &turn,
			RepresentativeMessageID: &representative, Revision: "18446744073709551615",
			State: "completed", Durability: "volatile", ExecutionID: text,
			Metadata: CanonicalMetadata{CreatedAt: text, Role: "assistant", Model: text, Source: text,
				Pinned: true, Thinking: true, TerminalReason: text, TotalTokens: 2147483647},
			Segments: []CanonicalSegment{{SegmentID: text, Kind: "text", Text: text, Conclusion: true},
				{SegmentID: "tools", Kind: "tool_invocations", Invocations: []TurnSegmentToolCall{{InvocationID: text, ID: text, Name: text,
					InputPreview: text, OutputPreview: text, InputBytes: 2147483647, HasDetails: true, ResultAvailability: "available"}}}}}
		wire, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		if size := canonicalCacheSize(&item); size < len(wire) {
			t.Fatal("cache subestimou wire", size, len(wire))
		}
		item.RecoveryReference = &TimelineRecoveryReference{ProjectionID: text, Incarnation: text, TurnID: turn, ExecutionID: text}
		wire, err = json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		if size := canonicalCacheSize(&item); size != len(wire) {
			t.Fatal("recuperação perdeu tamanho exato", size, len(wire))
		}
	}
	oversized := CanonicalItem{ItemID: "large", Segments: []CanonicalSegment{{SegmentID: "text", Kind: "text", Text: strings.Repeat("x", canonicalItemCacheBytes)}}}
	if canonicalCacheSize(&oversized) <= canonicalItemCacheBytes {
		t.Fatal("estimador aceitou item excessivo")
	}
	cache := canonicalPublication{items: map[string]*CanonicalItem{}, itemSizes: map[string]int{}}
	cache.cacheItem(oversized)
	if len(cache.items) != 0 || cache.itemBytes != 0 {
		t.Fatal("cache reteve item que excede cota")
	}
}
