package chat

import (
	"fmt"
	"reflect"
	"testing"

	"assistente/internal/database"
)

func TestTimelineResumePreservesRetainedWindowOutsideTail(t *testing.T) {
	f, p := membershipFixture(t)
	sub, initial := membershipSubscribe(t, f, p, "reading", 80)
	followTail(t, f, sub, initial, 80)
	retainWindow(t, f, sub, "message:first")
	for i := 0; i < 81; i++ {
		historicalMessage(t, f, fmt.Sprintf("new-%d", i), 30+i, nil)
	}
	membershipWrite(t, f, p, "newest", 120)
	w := sub.conversation.windows[sub.windowID]
	resume := &TimelineWindowResume{StorageIncarnation: initial.StorageIncarnation, Mode: w.mode, BeforeCursor: w.metadata.BeforeCursor, AfterCursor: w.metadata.AfterCursor, RetainedItemIDs: append([]string(nil), w.metadata.RetainedItemIDs...)}
	query := cloneWindowQuery(w.query)
	want := append([]string(nil), w.orderedIDs...)
	sub.Close()
	reopened, snapshot, err := p.SubscribeTimelineResuming(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "session"}, "replacement", query, f.reader, resume)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(snapshot.OrderedItemIDs, want) || !reflect.DeepEqual(snapshot.RetainedItemIDs, []string{"message:first"}) || snapshot.Mode != TimelineWindowFollowTail || !snapshot.RetentionBlocked {
		t.Fatal("reconexão abandonou janela retida", snapshot)
	}
	retainWindow(t, f, reopened)
	if current := reopened.conversation.windows[reopened.windowID]; current.metadata.RetentionBlocked || len(current.orderedIDs) != 80 || current.orderedIDs[len(current.orderedIDs)-1] != "message:newest" {
		t.Fatal("liberação não retomou cauda", current.orderedIDs)
	}
}

func TestTimelineResumeRevalidatesRetentionAndBounds(t *testing.T) {
	for _, scenario := range []string{"tail", "deleted", "foreign-scope", "foreign-conversation", "invalid-cursor", "import", "too-many"} {
		t.Run(scenario, func(t *testing.T) {
			f, p := membershipFixture(t)
			sub, initial := membershipSubscribe(t, f, p, "reading", 2)
			followTail(t, f, sub, initial, 2)
			retainWindow(t, f, sub, "message:first")
			w := sub.conversation.windows[sub.windowID]
			resume := &TimelineWindowResume{StorageIncarnation: initial.StorageIncarnation, Mode: w.mode, BeforeCursor: w.metadata.BeforeCursor, AfterCursor: w.metadata.AfterCursor, RetainedItemIDs: []string{"message:first"}}
			query := cloneWindowQuery(w.query)
			sub.Close()
			wantRetention := []string{"message:first"}
			switch scenario {
			case "deleted":
				if err := database.DeleteMessageWithContext(f.ctx, "first"); err != nil {
					t.Fatal(err)
				}
				wantRetention = []string{}
			case "foreign-scope":
				parent := "first"
				historicalMessage(t, f, "child", 15, &parent)
				resume.RetainedItemIDs = []string{"message:child"}
				wantRetention = []string{}
			case "foreign-conversation":
				other, err := database.CreateConversationWithContext(f.ctx, "other", "")
				if err != nil {
					t.Fatal(err)
				}
				message, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: other.ID, Role: "user", Content: "private"})
				if err != nil {
					t.Fatal(err)
				}
				resume.RetainedItemIDs = []string{"message:" + message.ID}
				wantRetention = []string{}
			case "invalid-cursor":
				historicalMessage(t, f, "new", 30, nil)
				resume.BeforeCursor = "invalid"
			case "import":
				if err := f.db.Exec("UPDATE chat_timeline_versions SET incarnation=lower(hex(randomblob(16))) WHERE conversation_id=?", f.conversation).Error; err != nil {
					t.Fatal(err)
				}
			case "too-many":
				resume.RetainedItemIDs = make([]string, database.MaxMessageWindowRows+1)
			}
			reopened, snapshot, err := p.SubscribeTimelineResuming(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "session"}, "replacement", query, f.reader, resume)
			if scenario == "invalid-cursor" || scenario == "too-many" || scenario == "import" {
				if err == nil {
					reopened.Close()
					t.Fatal("retomada inválida aceita")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if !reflect.DeepEqual(snapshot.RetainedItemIDs, wantRetention) || snapshot.RetentionBlocked {
				t.Fatal("retenção não revalidada", snapshot)
			}
			for _, item := range snapshot.Items {
				if item.Metadata.Role != "user" || item.ScopeParentID != nil || (item.ItemID != "message:first" && item.ItemID != "message:last") {
					t.Fatal("item fora do escopo", item)
				}
			}
		})
	}
}
