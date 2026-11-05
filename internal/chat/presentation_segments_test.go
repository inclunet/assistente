package chat

import (
	"errors"
	"github.com/google/uuid"
	"maps"
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

func TestPresentationSegmentsSurviveRealSubscriptionRefreshAfterCommit(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	sub, initial, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "identity-refresh"}, "identity-refresh", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	repo := database.NewChatTurnRepository(f.db)
	id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
	if err != nil {
		t.Fatal(err)
	}
	stream := e.StartStream(0)
	stream.Append("[link](https://example.com)")
	stream.Cut()
	stream.Reasoning("pensamento", false)
	stream.Append(" continuação")
	before, _ := publicationItem(t, e)
	deadline := time.Now().Add(3 * time.Second)
	for len(before.Segments) != 3 {
		if time.Now().After(deadline) {
			t.Fatal("stream not published")
		}
		time.Sleep(time.Millisecond)
		before, _ = publicationItem(t, e)
	}
	text := "[link](https://example.com) continuação"
	if err := e.CommitContent(0, text, "pensamento", func() (string, error) {
		return id, repo.WritePartial(f.ctx, e.executionID, id, text, "pensamento", false)
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.CommitResponse(database.TurnCompleted, "completed", text, "pensamento", func() (database.ChatTimelineVersion, error) {
		return repo.FinishResponseWithVersion(f.ctx, f.conversation, f.question, e.executionID, database.TurnCompleted, "completed", id, nil)
	}); err != nil {
		t.Fatal(err)
	}
	c := e.conversation
	p.mu.Lock()
	c.mu.Lock()
	c.turns[e.turnID].finishedAt = time.Now().Add(-2 * projectionRetention)
	p.pruneLocked(e.key, c, time.Now())
	remaining := len(c.turns)
	c.mu.Unlock()
	p.mu.Unlock()
	if remaining != 0 {
		t.Fatal("live not collected")
	}
	refreshed, err := sub.Refresh(f.ctx, initial.WindowRevision, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range refreshed.Items {
		if item.ItemID == before.ItemID {
			if !reflect.DeepEqual(item.Segments, before.Segments) {
				t.Fatalf("refresh changed identity or cuts: before=%+v after=%+v", before.Segments, item.Segments)
			}
			return
		}
	}
	t.Fatal("response absent after Refresh")
}

func TestSnapshotRetainsAllResponseBoundariesUntilWindowCommit(t *testing.T) {
	for _, budgetFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "refresh", true: "budget-rollback"}[budgetFailure], func(t *testing.T) {
			f, p, first, _ := automaticFixture(t)
			repo := database.NewChatTurnRepository(f.db)
			finish := func(e *ExecutionProjection) {
				id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, e.turnID, e.executionID) })
				if err != nil {
					t.Fatal(err)
				}
				stream := e.StartStream(0)
				stream.Append("[link](https://example.com)")
				stream.Cut()
				stream.Reasoning("pensamento", false)
				stream.Append(" fim")
				text := "[link](https://example.com) fim"
				if err := e.CommitContent(0, text, "pensamento", func() (string, error) {
					return id, repo.WritePartial(f.ctx, e.executionID, id, text, "pensamento", false)
				}); err != nil {
					t.Fatal(err)
				}
				if err := e.CommitResponse(database.TurnCompleted, "completed", text, "pensamento", func() (database.ChatTimelineVersion, error) {
					return repo.FinishResponseWithVersion(f.ctx, f.conversation, e.turnID, e.executionID, database.TurnCompleted, "completed", id, nil)
				}); err != nil {
					t.Fatal(err)
				}
			}
			finish(first)
			question, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "segunda"})
			if err != nil {
				t.Fatal(err)
			}
			eid, err := uuid.NewV7()
			if err != nil {
				t.Fatal(err)
			}
			second, err := p.Admit(f.ctx, f.conversation, question.ID, eid.String(), func() (database.ChatTimelineVersion, error) {
				_, err := repo.BeginExecution(f.ctx, f.conversation, question.ID, "", database.TurnModeNew, eid.String(), "")
				if err != nil {
					return database.ChatTimelineVersion{}, err
				}
				return f.reader.ReadVersion(f.ctx, f.conversation)
			})
			if err != nil {
				t.Fatal(err)
			}
			finish(second)
			c := first.conversation
			var checkpoint map[string]presentationBoundaries
			if budgetFailure {
				held, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "held"}, "held", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
				if err != nil {
					t.Fatal(err)
				}
				defer held.Close()
				c.mu.Lock()
				// Reserve the existing window's descriptor budget so one new response fits,
				// but two cannot. This tests accounting without allocating multi-MiB text.
				c.presentationBoundaries = map[string]presentationBoundaries{"message:" + f.question: {incarnation: c.incarnation, bytes: presentationBoundaryBudget - 900}}
				checkpoint = maps.Clone(c.presentationBoundaries)
				c.mu.Unlock()
			}
			sub, opened, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "two"}, "two", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
			if budgetFailure {
				if sub != nil {
					sub.Close()
				}
				if !errors.Is(err, ErrTimelineSourceLimit) {
					t.Fatal("aggregate budget not enforced", err)
				}
				c.mu.Lock()
				defer c.mu.Unlock()
				if !reflect.DeepEqual(checkpoint, c.presentationBoundaries) || len(c.preparingPresentation) != 0 {
					t.Fatal("failed snapshot changed descriptor checkpoint")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			expected := map[string][]CanonicalSegment{}
			for _, item := range opened.Items {
				if item.Kind == "response" {
					expected[item.ItemID] = item.Segments
				}
			}
			if len(expected) != 2 {
				t.Fatal("expected two responses", len(expected))
			}
			p.mu.Lock()
			c.mu.Lock()
			for _, turn := range c.turns {
				turn.finishedAt = time.Now().Add(-2 * projectionRetention)
			}
			p.pruneLocked(first.key, c, time.Now())
			remaining := len(c.turns)
			c.mu.Unlock()
			p.mu.Unlock()
			if remaining != 0 {
				t.Fatal("live state not collected")
			}
			refreshed, err := sub.Refresh(f.ctx, opened.WindowRevision, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range refreshed.Items {
				if segments, ok := expected[item.ItemID]; ok {
					if !reflect.DeepEqual(segments, item.Segments) {
						t.Fatalf("response identity lost: %s before=%+v after=%+v", item.ItemID, segments, item.Segments)
					}
					delete(expected, item.ItemID)
				}
			}
			if len(expected) != 0 {
				t.Fatal("response disappeared")
			}
		})
	}
}

func TestThinkingCompletionDoesNotInvalidateConfirmedBytes(t *testing.T) {
	p, ctx, e := liveFixture(t)
	stream := e.StartStream(0)
	stream.Reasoning("a", false, true)
	stream.Cut()
	stream.Reasoning("b", false, true)
	if err := e.CommitContent(0, "", "ab", func() (string, error) { return "response", nil }); err != nil {
		t.Fatal(err)
	}
	before, _ := p.Snapshot(ctx, "conversation", "question")
	c := e.conversation
	c.mu.Lock()
	version := c.turns[e.turnID].contentVersion
	c.mu.Unlock()
	if !stream.Reasoning("ab", true, false) {
		t.Fatal("completion refused")
	}
	after, _ := p.Snapshot(ctx, "conversation", "question")
	c.mu.Lock()
	defer c.mu.Unlock()
	if after.Thinking || !after.Streams[0].Confirmed || c.turns[e.turnID].contentVersion != version || !reflect.DeepEqual(before.Streams[0].Segments, after.Streams[0].Segments) {
		t.Fatal("phase update changed confirmed bytes")
	}
}
