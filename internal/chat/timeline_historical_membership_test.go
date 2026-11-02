package chat

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func historicalMessage(t *testing.T, f snapshotFixture, id string, day int, parent *string) {
	t.Helper()
	message := database.ChatMessage{UUIDModel: database.UUIDModel{ID: id, CreatedAt: time.Date(2020, 1, day, 0, 0, 0, 0, time.UTC)}, ConversationID: f.conversation, ParentID: parent, Role: "user", Content: strings.Repeat("x", 1024)}
	if err := f.db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
}

func historicalBounds(t *testing.T, f snapshotFixture, query database.MessageWindowQuery) (string, string) {
	t.Helper()
	selected, err := f.reader.ReadWindowMembership(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	rows := selected.Selection.Items
	if len(rows) == 0 {
		t.Fatal("fixture vazia")
	}
	return database.EncodeTimelineCursor(query.ConversationID, query.ParentID, rows[0]), database.EncodeTimelineCursor(query.ConversationID, query.ParentID, rows[len(rows)-1])
}

func TestHistoricalMembershipPreservesIntervalAcrossOutsideAndInsideWrites(t *testing.T) {
	for _, thread := range []bool{false, true} {
		t.Run(map[bool]string{false: "raiz", true: "thread"}[thread], func(t *testing.T) {
			f := newSnapshotFixture(t)
			var parent *string
			if thread {
				parent = &f.question
			}
			historicalMessage(t, f, "first", 10, parent)
			historicalMessage(t, f, "last", 20, parent)
			query := database.MessageWindowQuery{ConversationID: f.conversation, ParentID: parent, Anchor: "start", Direction: "after", Limit: 2}
			first, last := historicalBounds(t, f, query)
			query.Limit = 10
			read := func() *TimelineMembershipSnapshot {
				t.Helper()
				v, err := f.reader.ReadHistoricalMembership(f.ctx, query, first, last)
				if err != nil {
					t.Fatal(err)
				}
				return v
			}
			before := read()
			historicalMessage(t, f, "older", 1, parent)
			historicalMessage(t, f, "newer", 30, parent)
			outside := read()
			if outside.Selection.StartIndex != 1 || outside.Selection.EndIndex != 2 || outside.Selection.TotalCount != before.Selection.TotalCount+2 || !outside.Selection.HasBefore || !outside.Selection.HasAfter || len(outside.Selection.Items) != 2 || outside.Version == before.Version {
				t.Fatalf("janela deslocada: %+v", outside)
			}
			historicalMessage(t, f, "middle", 15, parent)
			inside := read()
			ids := []string{}
			for i, row := range inside.Selection.Items {
				ids = append(ids, row.ID)
				if row.OriginalIndex != i+1 {
					t.Fatal("índice incoerente")
				}
			}
			if !reflect.DeepEqual(ids, []string{"first", "middle", "last"}) || inside.Selection.EndIndex != 3 || len(inside.Selection.Messages) != 0 {
				t.Fatalf("participação errada: %+v", inside.Selection)
			}
			if err := database.DeleteMessageWithContext(f.ctx, "middle"); err != nil {
				t.Fatal(err)
			}
			removed := read()
			if len(removed.Selection.Items) != 2 || removed.Selection.EndIndex != 2 {
				t.Fatal("remoção interna não refletida")
			}
			// Conteúdo não participa da seleção nem de sua cota de identidades.
			if err := f.db.Model(&database.ChatMessage{}).Where("id=?", "first").Update("content", strings.Repeat("x", canonicalSourceBytes+1)).Error; err != nil {
				t.Fatal(err)
			}
			if len(read().Selection.Items) != 2 {
				t.Fatal("conteúdo alterou participação")
			}
			if err := database.DeleteMessageWithContext(f.ctx, "first"); err != nil {
				t.Fatal(err)
			}
			if _, err := f.reader.ReadHistoricalMembership(f.ctx, query, first, last); !errors.Is(err, database.ErrTimelineCursorStale) {
				t.Fatal("fronteira excluída foi substituída", err)
			}
		})
	}
}

func TestHistoricalMembershipRejectsInvalidScopeBoundsAndOverflow(t *testing.T) {
	f := newSnapshotFixture(t)
	historicalMessage(t, f, "first", 10, nil)
	historicalMessage(t, f, "last", 20, nil)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 2}
	first, last := historicalBounds(t, f, query)
	for _, bounds := range [][2]string{{"", last}, {first, ""}, {"invalid", last}, {last, first}} {
		if _, err := f.reader.ReadHistoricalMembership(f.ctx, query, bounds[0], bounds[1]); !errors.Is(err, database.ErrTimelineCursorInvalid) {
			t.Fatal("limite inválido aceito", err)
		}
	}
	for _, ctx := range []context.Context{context.Background(), database.WithUserID(context.Background(), "outro")} {
		if _, err := f.reader.ReadHistoricalMembership(ctx, query, first, last); err == nil {
			t.Fatal("owner ignorado")
		}
	}
	cancelled, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := f.reader.ReadHistoricalMembership(cancelled, query, first, last); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	query.ParentID = &f.question
	if _, err := f.reader.ReadHistoricalMembership(f.ctx, query, first, last); !errors.Is(err, database.ErrTimelineCursorInvalid) {
		t.Fatal("escopo ignorado", err)
	}
	query.ParentID = nil
	historicalMessage(t, f, "middle", 15, nil)
	if _, err := f.reader.ReadHistoricalMembership(f.ctx, query, first, last); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("intervalo truncado silenciosamente", err)
	}
	query.Limit = 10
	if err := f.db.Model(&database.ChatMessage{}).Where("id=?", "middle").Update("id", strings.Repeat("i", 3*1024*1024)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadHistoricalMembership(f.ctx, query, first, last); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("identidade excessiva aceita", err)
	}
}

func TestHistoricalMembershipEmptyWindowStaysEmpty(t *testing.T) {
	f := newSnapshotFixture(t)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Limit: 10}
	result, err := f.reader.ReadHistoricalMembership(f.ctx, query, "", "")
	if err != nil || len(result.Selection.Items) != 0 || result.Selection.TotalCount != 2 {
		t.Fatal("janela vazia navegou implicitamente", err)
	}
}

func TestHistoricalMembershipReadsOneCommit(t *testing.T) {
	f := newSnapshotFixture(t)
	historicalMessage(t, f, "first", 10, nil)
	historicalMessage(t, f, "last", 20, nil)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 2}
	first, last := historicalBounds(t, f, query)
	query.Limit = 10
	before, err := f.reader.ReadHistoricalMembership(f.ctx, query, first, last)
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
	if err := f.db.Callback().Query().After("gorm:query").Register("historical_commit", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "chat_timeline_versions") {
			once.Do(func() {
				writeErr = writer.Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: "middle", CreatedAt: time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)}, ConversationID: f.conversation, Role: "user", Content: "nova"}).Error
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.Callback().Query().Remove("historical_commit") })
	current, err := f.reader.ReadHistoricalMembership(f.ctx, query, first, last)
	if err != nil || writeErr != nil {
		t.Fatal(err, writeErr)
	}
	if !reflect.DeepEqual(before, current) {
		t.Fatal("misturou commits")
	}
	after, err := f.reader.ReadHistoricalMembership(f.ctx, query, first, last)
	if err != nil || after.Version == before.Version || len(after.Selection.Items) != 3 {
		t.Fatal("commit seguinte ausente", err)
	}
}

func TestHistoricalMembershipSingleBoundaryAndMovedAnchor(t *testing.T) {
	f := newSnapshotFixture(t)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Limit: 1}
	first, _ := historicalBounds(t, f, query)
	single, err := f.reader.ReadHistoricalMembership(f.ctx, query, first, first)
	if err != nil || len(single.Selection.Items) != 1 {
		t.Fatal("fronteira única recusada", err)
	}
	if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.question).Update("created_at", time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadHistoricalMembership(f.ctx, query, first, first); !errors.Is(err, database.ErrTimelineCursorStale) {
		t.Fatal("âncora movida aceita", err)
	}
}

func TestHistoricalMembershipRepositoryCancelsAfterAuthorization(t *testing.T) {
	f := newSnapshotFixture(t)
	query := database.MessageWindowQuery{ConversationID: f.conversation, Limit: 10}
	first, last := historicalBounds(t, f, query)
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	anchorReached := false
	if err := f.db.Callback().Row().Before("gorm:row").Register("historical_cancel", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "item_key=?") {
			anchorReached = true
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.Callback().Row().Remove("historical_cancel") })
	_, err := database.NewMessageRepository(f.db).SelectHistoricalTimelineWithContext(ctx, query, first, last)
	if !anchorReached || !errors.Is(err, context.Canceled) {
		t.Fatal("repositório ignorou cancelamento após autorizar", anchorReached, err)
	}
}
