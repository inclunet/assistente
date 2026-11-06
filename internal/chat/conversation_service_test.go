package chat

import (
	"assistente/internal/database"
	"testing"
	"time"
)

// A cronologia precisa ser provada na leitura usada pelo produto, não numa
// árvore em memória sem consumidores. A mesma prova cobre raiz e thread.
func TestCanonicalWindowOrdersRootAndThreadByCreatedAt(t *testing.T) {
	for _, thread := range []bool{false, true} {
		name := "root"
		if thread {
			name = "thread"
		}
		t.Run(name, func(t *testing.T) {
			f := newSnapshotFixture(t)
			conv, err := database.CreateConversationWithContext(f.ctx, "chronology", "")
			if err != nil {
				t.Fatal(err)
			}
			var parentID *string
			if thread {
				parent, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: conv.ID, Role: "user", Content: "parent"})
				if err != nil {
					t.Fatal(err)
				}
				parentID = &parent.ID
			}
			base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			messages := []Message{
				{UUIDModel: database.UUIDModel{ID: "zz-earlier", CreatedAt: base}, ConversationID: conv.ID, ParentID: parentID, Role: "assistant", Content: "first"},
				{UUIDModel: database.UUIDModel{ID: "aa-later", CreatedAt: base.Add(time.Second)}, ConversationID: conv.ID, ParentID: parentID, Role: "assistant", Content: "second"},
			}
			for i := len(messages) - 1; i >= 0; i-- {
				if err := f.db.Create(&messages[i]).Error; err != nil {
					t.Fatal(err)
				}
			}
			window, err := f.reader.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: conv.ID, ParentID: parentID, Anchor: "end", Direction: "before", Limit: 20})
			if err != nil {
				t.Fatal(err)
			}
			if len(window.Items) != 2 {
				t.Fatal(window.Items)
			}
			for i, want := range messages {
				item := canonicalItem(window.Items[i])
				if item.RepresentativeMessageID == nil || *item.RepresentativeMessageID != want.ID || !sameScope(item.ScopeParentID, parentID) {
					t.Fatalf("ordem/escopo incorretos: %+v", item)
				}
			}
			if thread {
				root, err := f.reader.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: conv.ID, Anchor: "end", Direction: "before", Limit: 20})
				if err != nil {
					t.Fatal(err)
				}
				if len(root.Items) != 1 || canonicalItem(root.Items[0]).Metadata.ChildCount != 2 {
					t.Fatal("contagem de filhos perdida", root.Items)
				}
			}
		})
	}
}
