package chat

import (
	"testing"

	"assistente/internal/database"
)

func TestFirstTimelineSnapshotStartsDiffBaseWithoutUnreachableJournal(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	session := &fixtureTimelineSession{ctx: f.ctx, id: "session"}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}
	sub, first, err := p.SubscribeTimeline(f.ctx, session, "first", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	c := sub.conversation
	if first.Sequence != "1" || len(c.publication.batches) != 0 || len(c.publication.items) != len(first.Items) || c.publication.itemBytes == 0 {
		t.Fatal("snapshot inicial deve formar somente a base limitada de diff")
	}
	for _, item := range first.Items {
		if item.Revision != "1" || c.publication.itemSizes[item.ItemID] < canonicalJSONSize(&item) {
			t.Fatal("revisão ou cota inicial divergente")
		}
	}
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{"turn:" + f.question}, func() ([]string, error) {
		return nil, database.UpdateMessageTextWithContext(f.ctx, f.answer, "nova resposta")
	}); err != nil {
		t.Fatal(err)
	}
	// A mutação publica normalmente; a segunda superfície parte da mesma base.
	second, opened, err := p.SubscribeTimeline(f.ctx, session, "second", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if opened.Sequence != "2" || len(c.publication.batches) != 1 {
		t.Fatal("segunda abertura não publicou a mudança")
	}
	batches, ok, err := sub.Replay(f.ctx, first.Epoch, first.WindowRevision, 1, f.reader)
	if err != nil || !ok || len(batches) != 1 || batches[0].Sequence != "2" || len(batches[0].Operations) == 0 {
		t.Fatal("primeiro delta depois do snapshot não é reproduzível", batches, ok, err)
	}
}

func TestSnapshotInitializationPreservesExistingPublicationAndTransactions(t *testing.T) {
	item := &TimelineItem{ItemID: "turn:q", TurnID: "q", Segments: []ResponseSegment{{SegmentID: "text", Kind: "text", Text: "live"}}}
	c := &projectionConversation{epoch: "epoch"}
	c.publishCanonical("conversation", []*TimelineItem{item})
	if c.initializeCanonicalSnapshot([]*TimelineItem{item}) || len(c.publication.batches) != 1 || c.publication.sequence != 1 {
		t.Fatal("snapshot descartou publicação live anterior")
	}
	c = &projectionConversation{epoch: "epoch", publicationStage: &canonicalPublicationStage{}}
	if c.initializeCanonicalSnapshot([]*TimelineItem{item}) || len(c.publication.items) != 0 {
		t.Fatal("snapshot inicializou fora do estágio transacional")
	}
}

func TestCanonicalConclusionChangeReplacesSegmentInsteadOfAppending(t *testing.T) {
	before := CanonicalItem{ItemID: "turn:q", Segments: []CanonicalSegment{{SegmentID: "text", Kind: "text", Text: "parcial"}}}
	after := before
	after.Segments = []CanonicalSegment{{SegmentID: "text", Kind: "text", Text: "parcial final", Conclusion: true}}
	operations := canonicalDiff(&before, after)
	if len(operations) != 1 || operations[0].Kind != "segment_replace" || operations[0].Segment == nil || !operations[0].Segment.Conclusion {
		t.Fatal("mudança da conclusão foi perdida num append", operations)
	}
}
