package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCanonicalPublicationUsesStableIdentityAndSegmentDeltas(t *testing.T) {
	c := &projectionConversation{epoch: "epoch"}
	item := &TimelineItem{ItemID: "turn:q", TurnID: "q", ConversationID: "c", Kind: "response", State: "streaming", Durability: "volatile", Segments: []ResponseSegment{{SegmentID: "text:one", Kind: "text", Text: "a"}}}
	c.publishCanonical("c", []*TimelineItem{item})
	if c.publication.sequence != 1 || c.publication.items[item.ItemID].RepresentativeMessageID != nil {
		t.Fatal("primeiro item")
	}
	item.Segments[0].Text = "ab"
	c.publishCanonical("c", []*TimelineItem{item})
	batch := c.publication.batches[1]
	if len(batch.Operations) != 1 || batch.Operations[0].Kind != "segment_append" || batch.Operations[0].Text != "b" || batch.Sequence != "2" {
		t.Fatalf("delta: %+v", batch)
	}
	c.publishCanonical("c", []*TimelineItem{item})
	if c.publication.sequence != 2 {
		t.Fatal("leitura sem alteração gerou lote")
	}
	item.Segments[0].Text = "outra"
	c.publishCanonical("c", []*TimelineItem{item})
	if c.publication.batches[2].Operations[0].Kind != "segment_replace" {
		t.Fatal("substituição interpretada como append")
	}
	item.Segments[0].SegmentID = "text:new-attempt"
	c.publishCanonical("c", []*TimelineItem{item})
	if c.publication.batches[3].Operations[0].Kind != "item_upsert" || c.publication.batches[3].Operations[0].Item.ItemID != "turn:q" {
		t.Fatal("reset perdeu identidade")
	}
}

func TestCanonicalPublicationBoundsReplayAndResetsEpoch(t *testing.T) {
	c := &projectionConversation{epoch: "epoch"}
	item := &TimelineItem{ItemID: "message:m", ConversationID: "c", Kind: "message", Segments: []ResponseSegment{{SegmentID: "s", Kind: "text"}}}
	for i := 0; i < 300; i++ {
		item.Segments[0].Text += "a"
		c.publishCanonical("c", []*TimelineItem{item})
	}
	if len(c.publication.batches) != 256 || c.publication.sequence != 300 {
		t.Fatal("limite de lotes")
	}
	if _, ok := c.publication.replay("epoch", 0); ok {
		t.Fatal("replay de histórico removido")
	}
	if _, ok := c.publication.replay("epoch", 43); ok {
		t.Fatal("replay anterior ao limite retido")
	}
	if batches, ok := c.publication.replay("epoch", 44); !ok || len(batches) != 256 || batches[0].Sequence != "45" || batches[255].Sequence != "300" {
		t.Fatal("replay na fronteira retida")
	}
	item.Segments[0].Text = strings.Repeat("x", canonicalReplayBytes+1)
	c.publishCanonical("c", []*TimelineItem{item})
	if len(c.publication.batches) != 0 || c.publication.bytes != 0 || c.publication.sequence != 301 {
		t.Fatal("overflow")
	}
	if _, ok := c.publication.replay("epoch", 300); ok {
		t.Fatal("replay atravessou lote maior que o orçamento")
	}
	if batches, ok := c.publication.replay("epoch", 301); !ok || len(batches) != 0 {
		t.Fatal("fronteira atual após overflow")
	}
	c.epoch = "new"
	item.Segments[0].Text = "x"
	c.publishCanonical("c", []*TimelineItem{item})
	if c.publication.sequence != 1 || c.publication.batches[0].Epoch != "new" {
		t.Fatal("época antiga")
	}
}

func TestCanonicalItemOmitsPhysicalPayloadAndUsesNullableRepresentative(t *testing.T) {
	item := &TimelineItem{ItemID: "turn:q", TurnID: "q", ConversationID: "c", Kind: "response", State: "preparing"}
	data, err := json.Marshal(canonicalItem(item))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"representativeMessageId":null`) || !strings.Contains(string(data), `"messageIds":[]`) || !strings.Contains(string(data), `"segments":[]`) {
		t.Fatalf("nullable/arrays: %s", data)
	}
}

func TestCanonicalReplayReconstructsSnapshotAndDoesNotShareMemory(t *testing.T) {
	c := &projectionConversation{epoch: "e"}
	item := &TimelineItem{ItemID: "turn:q", TurnID: "q", ConversationID: "c", Kind: "response", Segments: []ResponseSegment{{SegmentID: "s", Kind: "text", Text: "a"}}}
	c.publishCanonical("c", []*TimelineItem{item})
	item.Segments[0].Text = "abc"
	c.publishCanonical("c", []*TimelineItem{item})
	item.Segments[0].Text = "novo"
	c.publishCanonical("c", []*TimelineItem{item})
	item.State = "completed"
	c.publishCanonical("c", []*TimelineItem{item})
	batches, ok := c.publication.replay("e", 0)
	if !ok {
		t.Fatal("replay indisponível")
	}
	var got CanonicalItem
	for _, batch := range batches {
		for _, op := range batch.Operations {
			switch op.Kind {
			case "item_upsert":
				got = *op.Item
			case "segment_append":
				for i := range got.Segments {
					if got.Segments[i].SegmentID == op.SegmentID {
						got.Segments[i].Text += op.Text
					}
				}
			case "segment_replace":
				for i := range got.Segments {
					if got.Segments[i].SegmentID == op.SegmentID {
						got.Segments[i] = *op.Segment
					}
				}
			default:
				t.Fatalf("operação desconhecida: %s", op.Kind)
			}
			got.Revision = op.Revision
		}
	}
	encoded, _ := json.Marshal(got)
	expected, _ := json.Marshal(c.publication.items[item.ItemID])
	if string(encoded) != string(expected) {
		t.Fatalf("replay=%s snapshot=%s", encoded, expected)
	}
	batches[0].Operations[0].Item.Segments[0].Text = "corrompido"
	again, ok := c.publication.replay("e", 0)
	if !ok || again[0].Operations[0].Item.Segments[0].Text != "a" {
		t.Fatal("replay mutável")
	}
	if _, ok := c.publication.replay("other", 0); ok {
		t.Fatal("época errada")
	}
	if _, ok := c.publication.replay("e", 99); ok {
		t.Fatal("sequência futura")
	}
}

func TestCanonicalPublicationEvictsOnlyDiffBase(t *testing.T) {
	c := &projectionConversation{epoch: "e"}
	for i := 0; i < canonicalItemCacheCount+1; i++ {
		item := &TimelineItem{ItemID: fmt.Sprint(i), ConversationID: "c", Kind: "message"}
		c.publishCanonical("c", []*TimelineItem{item})
	}
	if len(c.publication.items) != canonicalItemCacheCount {
		t.Fatal("cache sem limite")
	}
	c.publishCanonical("c", []*TimelineItem{{ItemID: "0", ConversationID: "c", Kind: "message"}})
	batch := c.publication.batches[len(c.publication.batches)-1]
	if batch.Operations[0].Kind != "item_upsert" {
		t.Fatal("base expulsa tratada como continuidade")
	}
}
