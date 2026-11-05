package chat

import (
	"reflect"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestPresentationSegmentsSurviveConfirmationTerminalAndLiveCollection(t *testing.T) {
	p, ctx, execution := liveFixture(t)
	if _, err := execution.AssignResponse(func() (string, error) { return "response", nil }); err != nil {
		t.Fatal(err)
	}
	stream := execution.StartStream(0)
	stream.Append("[link](https://example.com)")
	stream.Cut()
	stream.Reasoning("pensamento", false)
	stream.Append(" continuação")
	before, _ := p.Snapshot(ctx, "conversation", "question")
	text := "[link](https://example.com) continuação"
	if !stream.Seal(text, "pensamento") {
		t.Fatal("seal rejected")
	}
	sealed, _ := p.Snapshot(ctx, "conversation", "question")
	if !reflect.DeepEqual(before.Streams[0].Segments, sealed.Streams[0].Segments) {
		t.Fatal("seal moved published segments")
	}
	if err := execution.CommitContent(0, text, "pensamento", func() (string, error) { return "response", nil }); err != nil {
		t.Fatal(err)
	}
	item := compositionItem(t, []Message{{UUIDModel: database.UUIDModel{ID: "response"}, Content: text, Reasoning: "pensamento"}}, nil)
	item.ExecutionID = "execution"
	c, release := execution.lockStorage()
	c.windows = map[string]*subscribedWindow{"window": {itemIDs: map[string]bool{item.ItemID: true}}}
	confirmed, err := projectLiveItemLocked(c, item)
	if err != nil {
		release()
		t.Fatal(err)
	}
	turn := c.turns["question"]
	turn.finished, turn.volatile, turn.finishedAt = true, false, time.Now().Add(-2*projectionRetention)
	terminal, err := projectLiveItemLocked(c, item)
	release()
	if err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	c.mu.Lock()
	p.pruneLocked(projectionKey{"user", "conversation"}, c, time.Now())
	if len(c.turns) != 0 {
		t.Error("live state was not collected")
	}
	refreshed, err := projectLiveItemLocked(c, item)
	c.mu.Unlock()
	p.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range []*TimelineItem{confirmed, terminal, refreshed} {
		if len(result.Segments) != len(before.Streams[0].Segments) {
			t.Fatalf("boundaries changed: %+v", result.Segments)
		}
		for index, segment := range result.Segments {
			original := before.Streams[0].Segments[index]
			if segment.SegmentID != original.SegmentID || segment.Text != original.Text || segment.Kind != original.Kind {
				t.Fatalf("presentation changed: %+v -> %+v", original, segment)
			}
		}
	}
}

func TestPresentationSegmentsDoNotOverwriteEditOrCrossIncarnation(t *testing.T) {
	message := Message{UUIDModel: database.UUIDModel{ID: "response"}, Content: "ab"}
	item := compositionItem(t, []Message{message}, nil)
	live := &LiveTurnSnapshot{ExecutionID: "execution", Streams: []LiveStreamSnapshot{{Confirmed: true, PersistedMessageID: "response", Segments: []ResponseSegment{{SegmentID: "first", Kind: "text", Text: "a"}, {SegmentID: "second", Kind: "text", Text: "b"}}}}}
	c := newProjectionConversation()
	if err := c.rememberPresentationBoundaries(item, live); err != nil {
		t.Fatal(err)
	}
	if len(c.applyPresentationBoundaries(item).Segments) != 2 {
		t.Fatal("confirmed boundaries lost")
	}
	message.Content = "edited"
	edited := compositionItem(t, []Message{message}, nil)
	if got := c.applyPresentationBoundaries(edited); !reflect.DeepEqual(got.Segments, edited.Segments) {
		t.Fatal("old buffer replaced persisted edit")
	}
	c.incarnation = "new"
	if got := c.applyPresentationBoundaries(item); !reflect.DeepEqual(got.Segments, item.Segments) {
		t.Fatal("boundaries crossed incarnation")
	}
}

func TestSealDifferentTerminalContentReplacesSegmentIdentity(t *testing.T) {
	p, ctx, execution := liveFixture(t)
	stream := execution.StartStream(0)
	stream.Append("partial")
	before, _ := p.Snapshot(ctx, "conversation", "question")
	stream.Seal("different final", "")
	after, _ := p.Snapshot(ctx, "conversation", "question")
	if before.Streams[0].Segments[0].SegmentID == after.Streams[0].Segments[0].SegmentID {
		t.Fatal("explicit replacement reused presentation identity")
	}
}

func TestPresentationSegmentsRollbackRestoresEvictedBoundaries(t *testing.T) {
	message := Message{UUIDModel: database.UUIDModel{ID: "response"}, Content: "ab"}
	item := compositionItem(t, []Message{message}, nil)
	live := &LiveTurnSnapshot{Streams: []LiveStreamSnapshot{{Confirmed: true, PersistedMessageID: "response", Segments: []ResponseSegment{{SegmentID: "first", Kind: "text", Text: "a"}, {SegmentID: "second", Kind: "text", Text: "b"}}}}}
	c := newProjectionConversation()
	if err := c.rememberPresentationBoundaries(item, live); err != nil {
		t.Fatal(err)
	}
	before := c.applyPresentationBoundaries(item).Segments
	finish := c.beginCanonicalPublication()
	delete(c.presentationBoundaries, item.ItemID)
	finish("conversation", false)
	if got := c.applyPresentationBoundaries(item).Segments; !reflect.DeepEqual(got, before) {
		t.Fatal("rollback lost published boundaries", got)
	}
}

func TestPresentationConclusionIncludesEveryTextCutOfTheLastSource(t *testing.T) {
	item := &TimelineItem{Segments: []ResponseSegment{
		{SourceMessageID: "intermediate", SegmentID: "old", Kind: "text", Text: "old"},
		{SourceMessageID: "answer", SegmentID: "first", Kind: "text", Text: "hello "},
		{SegmentID: "tool", Kind: "tool_invocations"},
		{SourceMessageID: "answer", SegmentID: "last", Kind: "text", Text: "world"},
	}}
	got := canonicalItem(item).Metadata.ConclusionSegmentIDs
	if !reflect.DeepEqual(got, []string{"first", "last"}) {
		t.Fatal("incomplete conclusion", got)
	}
}

func TestCanonicalThinkingTracksPhaseNotAccumulatedReasoning(t *testing.T) {
	p, ctx, execution := liveFixture(t)
	stream := execution.StartStream(0)
	stream.Reasoning("prefixo herdado", true)
	snap, _ := p.Snapshot(ctx, "conversation", "question")
	if snap.Thinking {
		t.Fatal("prefixo iniciou thinking")
	}
	stream.Reasoning("novo", false, true)
	snap, _ = p.Snapshot(ctx, "conversation", "question")
	if !snap.Thinking {
		t.Fatal("thinking não publicado")
	}
	stream.Reasoning("prefixo herdado novo", true, false)
	snap, _ = p.Snapshot(ctx, "conversation", "question")
	if snap.Thinking || len(snap.Streams[0].Segments) == 0 {
		t.Fatal("fim deve preservar conteúdo sem atividade")
	}
	stream.Append("resposta")
	newer := execution.StartStream(0)
	newer.Reasoning("atual", false, true)
	if stream.StopThinking() || stream.Reasoning("obsoleto", false, false) {
		t.Fatal("callback obsoleto aceito")
	}
	snap, _ = p.Snapshot(ctx, "conversation", "question")
	if !snap.Thinking {
		t.Fatal("callback obsoleto alterou fase atual")
	}
	newer.StopThinking()
	snap, _ = p.Snapshot(ctx, "conversation", "question")
	if snap.Thinking {
		t.Fatal("interrupção manteve fase ativa")
	}
}
