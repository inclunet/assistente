package chat

import (
	"testing"

	"assistente/internal/database"
)

func TestTimelineSnapshotIncludesEmptyScopeAndOwnsParent(t *testing.T) {
	f := newSnapshotFixture(t)
	empty, err := database.CreateConversationWithContext(f.ctx, "empty", "")
	if err != nil {
		t.Fatal(err)
	}
	p := NewLiveProjection(f.reader)
	for _, thread := range []bool{false, true} {
		name := "root"
		query := database.MessageWindowQuery{ConversationID: empty.ID, Anchor: "end", Direction: "before", Limit: 10}
		if thread {
			name = "thread"
			parent := f.question
			query.ConversationID, query.ParentID = f.conversation, &parent
		}
		t.Run(name, func(t *testing.T) {
			sub, snapshot, err := p.SubscribeTimeline(f.ctx, requestTimelineSession{}, name, query, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			if snapshot.ConversationID != query.ConversationID || len(snapshot.Items) != 0 || len(snapshot.OrderedItemIDs) != 0 {
				t.Fatalf("janela vazia sem identidade: %+v", snapshot)
			}
			if !thread {
				if snapshot.ScopeParentID != nil {
					t.Fatal("raiz não tem parent")
				}
				return
			}
			if snapshot.ScopeParentID == nil || *snapshot.ScopeParentID != f.question {
				t.Fatalf("escopo incorreto: %+v", snapshot)
			}
			// Nem o pedido original nem a resposta podem alterar o escopo guardado.
			*query.ParentID = "changed-query"
			if *snapshot.ScopeParentID != f.question {
				t.Fatal("snapshot compartilha parent do pedido")
			}
			*snapshot.ScopeParentID = "changed-response"
			fresh, err := sub.Refresh(f.ctx, snapshot.WindowRevision, nil)
			if err != nil {
				t.Fatal(err)
			}
			if fresh.ConversationID != f.conversation || fresh.ScopeParentID == nil || *fresh.ScopeParentID != f.question || len(fresh.Items) != 0 {
				t.Fatalf("mutação externa mudou inscrição: %+v", fresh)
			}
		})
	}
}
