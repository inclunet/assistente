package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/database"
)

func TestCanonicalPresentationKeepsMediaOutOfWindow(t *testing.T) {
	f := newSnapshotFixture(t)
	blob := strings.Repeat("private-media-payload", 1024)
	if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.answer).Updates(map[string]any{"media": blob, "audio": blob, "source": "test-source", "cache_read_tokens": 42}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.answer, Role: "user", Content: "thread"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.reader.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	item := canonicalItem(snapshot.Responses[f.question])
	metadata := item.Metadata
	if metadata.ThreadParentMessageID == nil || *metadata.ThreadParentMessageID != f.answer || metadata.ChildCount != 1 || metadata.CacheReadTokens != 42 || metadata.Source != "test-source" || metadata.Timestamp <= 0 || metadata.CreatedAt == "" || len(metadata.ConclusionSegmentIDs) == 0 {
		t.Fatalf("incomplete presentation: %+v", metadata)
	}
	wire, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "private-media-payload") {
		t.Fatal("window materialized media")
	}
	if _, err := f.reader.ReadCanonicalWindow(database.WithUserID(context.Background(), "other-user"), database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}); err == nil {
		t.Fatal("foreign window authorized")
	}
}

func TestCanonicalPresentationMetadataUpdatesArePublished(t *testing.T) {
	f := newSnapshotFixture(t)
	before, err := f.reader.ReadPublicationTurn(f.ctx, f.conversation, f.question)
	if err != nil {
		t.Fatal(err)
	}
	previous := canonicalItem(before.Response)
	if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.answer).Update("pinned", true).Error; err != nil {
		t.Fatal(err)
	}
	after, err := f.reader.ReadPublicationTurn(f.ctx, f.conversation, f.question)
	if err != nil {
		t.Fatal(err)
	}
	next := canonicalItem(after.Response)
	operations := canonicalDiff(&previous, next)
	if len(operations) != 1 || operations[0].Kind != "item_upsert" || !operations[0].Item.Metadata.Pinned {
		t.Fatalf("metadata update lost: %+v", operations)
	}
}

func TestCanonicalPresentationSerializesEmptyConclusionAsArray(t *testing.T) {
	for _, item := range []*TimelineItem{
		{Kind: "response", State: "preparing"},
		{Kind: "response", State: "completed", Segments: []ResponseSegment{{SegmentID: "tool", Kind: "tool_invocations"}}},
	} {
		wire, err := json.Marshal(canonicalItem(item))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(wire), `"conclusionSegmentIds":[]`) {
			t.Fatal("nullable conclusion on wire", string(wire))
		}
		var decoded CanonicalItem
		if err := json.Unmarshal(wire, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Metadata.ConclusionSegmentIDs == nil {
			t.Fatal("empty conclusion lost in JSON")
		}
	}
}

func TestCanonicalTerminalReasonSurvivesSnapshotWithPartialContent(t *testing.T) {
	for _, reason := range []string{"provider_error", "persistence_error", "limit_reached"} {
		t.Run(reason, func(t *testing.T) {
			f := newSnapshotFixture(t)
			state := database.TurnFailed
			if reason == "limit_reached" {
				state = database.TurnInterrupted
			}
			repo := database.NewChatTurnRepository(f.db)
			attempt, err := repo.Begin(f.ctx, f.conversation, f.question, "", database.TurnModeRetry)
			if err != nil {
				t.Fatal(err)
			}
			execution := attempt.ExecutionID
			response, err := repo.EnsureResponse(f.ctx, f.conversation, f.question, execution)
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.FinishResponse(f.ctx, f.conversation, f.question, execution, state, reason, response, &database.MessageOptions{Content: "resposta parcial"}); err != nil {
				t.Fatal(err)
			}

			snapshot, err := f.reader.ReadPublicationTurn(f.ctx, f.conversation, f.question)
			if err != nil {
				t.Fatal(err)
			}
			view := canonicalItem(snapshot.Response)
			if view.Metadata.TerminalReason != reason || view.State != state || len(view.Segments) == 0 {
				t.Fatalf("diagnóstico/conteúdo perdido: %+v", view)
			}
			live := &LiveTurnSnapshot{ItemID: snapshot.Response.ItemID, TurnID: f.question, ExecutionID: execution, State: state, Reason: reason, Finished: true}
			composed, err := composeResponse(snapshot.Response, live)
			if err != nil || canonicalItem(composed).Metadata.TerminalReason != reason {
				t.Fatalf("terminal live: %v %+v", err, composed)
			}
		})
	}
}
