package chat

import (
	"assistente/internal/database"
	"context"
	"errors"
	"testing"
)

type retentionReceiptSession struct {
	armed bool
	calls int
}

func (s *retentionReceiptSession) AuthorizeTimeline(ctx context.Context) (context.Context, string, error) {
	if s.armed {
		s.calls++
		if s.calls > 1 {
			return nil, "", ErrTimelineSubscription
		}
	}
	return ctx, "receipt-session", nil
}

func TestRetentionReceiptReportsCommittedCheckpoint(t *testing.T) {
	f, _ := membershipFixture(t)
	p := NewLiveProjection(f.reader)
	sub, initial, err := p.SubscribeTimeline(f.ctx, requestTimelineSession{}, "receipt", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 10}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	receipt, err := sub.SetRetentionCheckpoint(f.ctx, TimelineRetentionRequest{Epoch: initial.Epoch, WindowRevision: initial.WindowRevision, ItemIDs: []string{"message:first"}})
	if err != nil {
		t.Fatal(err)
	}
	patches := membershipReplay(t, f, sub, initial)
	if receipt.WindowID != initial.WindowID || receipt.Epoch != initial.Epoch || len(patches) != 1 || receipt.WindowRevision != patches[0].WindowPatches[0].Revision {
		t.Fatal("receipt does not identify committed patch", receipt, patches)
	}
}

func TestRetentionReceiptDoesNotResurrectRevokedCapability(t *testing.T) {
	f, _ := membershipFixture(t)
	p := NewLiveProjection(f.reader)
	session := &retentionReceiptSession{}
	sub, initial, err := p.SubscribeTimeline(f.ctx, session, "receipt", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 10}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	session.armed = true
	receipt, err := sub.SetRetentionCheckpoint(f.ctx, TimelineRetentionRequest{Epoch: initial.Epoch, WindowRevision: initial.WindowRevision, ItemIDs: []string{"message:first"}})
	if !errors.Is(err, ErrTimelineSubscription) || receipt != nil {
		t.Fatal("revoked capability received checkpoint", receipt, err)
	}
	if sub.conversation.windows[sub.windowID] != nil {
		t.Fatal("revoked capability restored")
	}
}
