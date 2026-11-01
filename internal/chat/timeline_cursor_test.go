package chat

import (
	"assistente/internal/database"
	"context"
	"errors"
	"testing"
	"time"
)

func TestCanonicalCursorRoundTripAndInvalidation(t *testing.T) {
	f := newSnapshotFixture(t)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1}
	last, err := f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	query.Anchor = ""
	query.Cursor = last.BeforeCursor
	before, err := f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil || len(before.Items) != 1 || before.Items[0].ItemID != "message:"+f.question {
		t.Fatalf("página anterior: %+v %v", before, err)
	}
	query.Cursor = before.AfterCursor
	query.Direction = "after"
	after, err := f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil || len(after.Items) != 1 || after.Items[0].ItemID != last.Items[0].ItemID {
		t.Fatalf("página posterior: %+v %v", after, err)
	}
	// Alteração da chave exige resync, mesmo quando a identidade continua existindo.
	if err := f.db.Model(&database.ChatMessage{}).Where("id = ?", f.question).Update("created_at", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadCanonicalWindow(f.ctx, query); !errors.Is(err, database.ErrTimelineCursorStale) {
		t.Fatalf("âncora movida: %v", err)
	}
	if err := database.DeleteMessageWithContext(f.ctx, f.question); err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadCanonicalWindow(f.ctx, query); !errors.Is(err, database.ErrTimelineCursorStale) {
		t.Fatalf("âncora removida: %v", err)
	}
}

func TestCanonicalCursorDoesNotGrantAccessOrChangeScope(t *testing.T) {
	f := newSnapshotFixture(t)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1}
	last, err := f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	query.Anchor = ""
	query.Cursor = last.BeforeCursor
	if _, err := f.reader.ReadCanonicalWindow(context.Background(), query); err == nil {
		t.Fatal("cursor concedeu acesso")
	}
	if _, err := f.reader.ReadCanonicalWindow(database.WithUserID(context.Background(), "outro"), query); err == nil {
		t.Fatal("cursor mudou owner")
	}
	query.ParentID = &f.question
	if _, err := f.reader.ReadCanonicalWindow(f.ctx, query); !errors.Is(err, database.ErrTimelineCursorInvalid) {
		t.Fatalf("scope: %v", err)
	}
	query.ParentID = nil
	for _, cursor := range []string{"?", "e30", last.BeforeCursor + "="} {
		query.Cursor = cursor
		if _, err := f.reader.ReadCanonicalWindow(f.ctx, query); !errors.Is(err, database.ErrTimelineCursorInvalid) {
			t.Fatalf("cursor %q: %v", cursor, err)
		}
	}
	query.Cursor = last.BeforeCursor
	query.Anchor = "end"
	if _, err := f.reader.ReadCanonicalWindow(f.ctx, query); !errors.Is(err, database.ErrTimelineCursorInvalid) {
		t.Fatalf("âncoras simultâneas: %v", err)
	}
}
