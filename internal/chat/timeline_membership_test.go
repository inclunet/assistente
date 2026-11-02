package chat

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMembershipMatchesCanonicalPaginationWithoutPayload(t *testing.T) {
	f := newSnapshotFixture(t)
	child, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.question, Role: "user", Content: "filha"})
	if err != nil {
		t.Fatal(err)
	}
	queries := []database.MessageWindowQuery{
		{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1},
		{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1},
		{ConversationID: f.conversation, AnchorItemID: "message:" + f.question, Direction: "around", Limit: 2},
		{ConversationID: f.conversation, ParentID: &f.question, Limit: 10},
		{ConversationID: f.conversation, Limit: 0},
	}
	first, err := f.reader.ReadCanonicalWindow(f.ctx, queries[0])
	if err != nil {
		t.Fatal(err)
	}
	queries = append(queries, database.MessageWindowQuery{ConversationID: f.conversation, Cursor: first.AfterCursor, Direction: "after", Limit: 1})
	for _, query := range queries {
		full, err := f.reader.ReadCanonicalWindow(f.ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		payloads := 0
		if err := f.db.Callback().Query().After("gorm:query").Register("membership_payload", func(tx *gorm.DB) {
			sql := tx.Statement.SQL.String()
			if strings.Contains(sql, "selected_payload") || strings.Contains(sql, " AS assistant_message_id") {
				payloads++
			}
		}); err != nil {
			t.Fatal(err)
		}
		light, err := f.reader.ReadWindowMembership(f.ctx, query)
		if removeErr := f.db.Callback().Query().Remove("membership_payload"); removeErr != nil {
			t.Fatal(removeErr)
		}
		if err != nil {
			t.Fatal(err)
		}
		rows := light.Selection
		if payloads != 0 || len(rows.Messages) != 0 {
			t.Fatal("participação carregou conteúdo", payloads)
		}
		if light.Version != full.Version || rows.TotalCount != full.Window.TotalCount || rows.StartIndex != full.Window.StartIndex || rows.EndIndex != full.Window.EndIndex || rows.HasBefore != full.Window.HasBefore || rows.HasAfter != full.Window.HasAfter || len(rows.Items) != len(full.Items) {
			t.Fatal("metadados divergentes")
		}
		for i, row := range rows.Items {
			prefix := "message:"
			if row.Kind == database.MessageWindowItemKindTurn {
				prefix = "turn:"
			}
			if prefix+row.ID != full.Items[i].ItemID || row.OriginalIndex != rows.StartIndex+i {
				t.Fatal("ordem divergente")
			}
		}
	}
	// Conteúdo pesado do escopo e do ledger não impede contar/ordenar identidades.
	large := strings.Repeat("x", canonicalSourceBytes+1)
	if err := f.db.Model(&database.ChatMessage{}).Where("id IN ?", []string{f.question, f.answer, child.ID}).Updates(map[string]any{"content": large, "audio": large}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Update("output_preview", large).Error; err != nil {
		t.Fatal(err)
	}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Limit: 10}
	if _, err := f.reader.ReadCanonicalWindow(f.ctx, query); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("fixture não excedeu conteúdo", err)
	}
	light, err := f.reader.ReadWindowMembership(f.ctx, query)
	if err != nil || light.Selection.TotalCount != 2 || len(light.Selection.Messages) != 0 {
		t.Fatal("participação dependeu de conteúdo", err)
	}
}

func TestMembershipKeepsVersionAndSelectionInOneSnapshot(t *testing.T) {
	f := newSnapshotFixture(t)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Limit: 10}
	before, err := f.reader.ReadWindowMembership(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := gorm.Open(sqlite.Open(f.path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := writer.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	var once sync.Once
	var writeErr error
	if err := f.db.Callback().Query().After("gorm:query").Register("membership_commit", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "chat_timeline_versions") {
			once.Do(func() {
				writeErr = writer.Create(&database.ChatMessage{ConversationID: f.conversation, Role: "user", Content: "nova"}).Error
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	current, err := f.reader.ReadWindowMembership(f.ctx, query)
	if err != nil || writeErr != nil {
		t.Fatal(err, writeErr)
	}
	if !reflect.DeepEqual(before, current) {
		t.Fatal("snapshot misturou versões")
	}
	after, err := f.reader.ReadWindowMembership(f.ctx, query)
	if err != nil || after.Version == before.Version || after.Selection.TotalCount != before.Selection.TotalCount+1 {
		t.Fatal("snapshot seguinte não avançou", err)
	}
}

func TestMembershipRejectsScopeCursorAndIdentityOverflow(t *testing.T) {
	f := newSnapshotFixture(t)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Limit: 1}
	for _, ctx := range []context.Context{context.Background(), database.WithUserID(context.Background(), "outro")} {
		if _, err := f.reader.ReadWindowMembership(ctx, query); err == nil {
			t.Fatal("owner ignorado")
		}
	}
	cancelled, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := f.reader.ReadWindowMembership(cancelled, query); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	child, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.question, Role: "user", Content: "filha"})
	if err != nil {
		t.Fatal(err)
	}
	query.ParentID = &child.ID
	if _, err := f.reader.ReadWindowMembership(f.ctx, query); err == nil {
		t.Fatal("pai não raiz aceito")
	}
	query.ParentID = nil
	query.Cursor = "invalid"
	if _, err := f.reader.ReadWindowMembership(f.ctx, query); !errors.Is(err, database.ErrTimelineCursorInvalid) {
		t.Fatal(err)
	}
	query.Cursor = ""
	full, err := f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	query.Cursor = full.AfterCursor
	if err := database.DeleteMessageWithContext(f.ctx, f.question); err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadWindowMembership(f.ctx, query); !errors.Is(err, database.ErrTimelineCursorStale) {
		t.Fatal("âncora removida aceita", err)
	}
	query.Cursor = ""
	huge := database.ChatMessage{UUIDModel: database.UUIDModel{ID: strings.Repeat("i", 3*1024*1024)}, ConversationID: f.conversation, Role: "user", Content: "curto"}
	if err := f.db.Create(&huge).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadWindowMembership(f.ctx, query); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("identidade excessiva aceita", err)
	}
}
