package chat

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"github.com/google/uuid"
)

func TestCanonicalWindowIncludesPreparingAndMissingRepresentativeWithoutMessages(t *testing.T) {
	f := newSnapshotFixture(t)
	q, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "nova"})
	if err != nil {
		t.Fatal(err)
	}
	repo := database.NewChatTurnRepository(f.db)
	a, err := repo.Begin(f.ctx, f.conversation, q.ID, "", database.TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1}
	w, err := f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Items) != 1 || w.Window.TotalCount != 4 || w.Window.StartIndex != 3 || w.Items[0].ItemID != "turn:"+q.ID || w.Items[0].Representative != nil || w.Items[0].State != database.TurnPreparing {
		t.Fatalf("turno vazio ausente: %+v", w)
	}
	pageQuery := query
	pageQuery.Anchor = ""
	pageQuery.AnchorItemID = "turn:" + q.ID
	pageQuery.Direction = "before"
	page, err := f.reader.ReadCanonicalWindow(f.ctx, pageQuery)
	if err != nil || len(page.Items) != 1 || page.Items[0].ItemID != "message:"+q.ID {
		t.Fatalf("paginação do item sem representante: %+v %v positions=%+v %+v", page, err, w.Items[0].Position, page.Items[0].Position)
	}
	pageQuery.Direction = "around"
	pageQuery.Limit = 3
	page, err = f.reader.ReadCanonicalWindow(f.ctx, pageQuery)
	if err != nil {
		t.Fatal(err)
	}
	var aroundIDs []string
	for _, item := range page.Items {
		aroundIDs = append(aroundIDs, item.ItemID)
	}
	if !reflect.DeepEqual(aroundIDs, []string{"turn:" + f.question, "message:" + q.ID, "turn:" + q.ID}) || page.Window.StartIndex != 1 {
		t.Fatalf("janela ao redor do item sem representante: %+v", page)
	}
	for _, anchorKind := range []string{"anchorItemId", "anchorMessageId"} {
		missingQuery := pageQuery
		missingQuery.AnchorItemID = ""
		if anchorKind == "anchorItemId" {
			missingQuery.AnchorItemID = "turn:inexistente"
		} else {
			missingQuery.AnchorMessageID = "inexistente"
		}
		_, err := f.reader.ReadCanonicalWindow(f.ctx, missingQuery)
		if err == nil || !strings.Contains(err.Error(), anchorKind+" inválido:") || !strings.Contains(err.Error(), "inexistente") {
			t.Fatalf("diagnóstico de %s: %v", anchorKind, err)
		}
	}
	before, err := f.reader.ReadWindow(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if before.Window.TotalCount != 3 || before.Items[0].ItemID != "message:"+q.ID {
		t.Fatal("contrato legado passou a anunciar nó que não consegue representar")
	}
	query.Anchor = "start"
	query.Direction = "after"
	query.Limit = 4
	w, err = f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, item := range w.Items {
		ids = append(ids, item.ItemID)
	}
	if !reflect.DeepEqual(ids, []string{"message:" + f.question, "turn:" + f.question, "message:" + q.ID, "turn:" + q.ID}) {
		t.Fatalf("ordem: %+v", ids)
	}
	if err := repo.Finish(f.ctx, f.conversation, q.ID, a.ExecutionID, database.TurnFailed, "persistence_error"); err != nil {
		t.Fatal(err)
	}
	w, err = f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil || w.Items[3].State != database.TurnFailed {
		t.Fatalf("falha sem representante perdida: %+v %v", w, err)
	}
	if err := database.DeleteMessageWithContext(f.ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	w, err = f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil || w.Window.TotalCount != 2 {
		t.Fatalf("exclusão não removeu item: %+v %v", w, err)
	}
}

func TestCanonicalWindowComposesActiveContentAndSharesTurnRecovery(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(nil)
	repo := database.NewChatTurnRepository(f.db)
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	execution := id.String()
	e, err := p.Admit(f.ctx, f.conversation, f.question, execution, func() (database.ChatTimelineVersion, error) {
		version, err := f.reader.ReadVersion(f.ctx, f.conversation)
		if err != nil {
			return version, err
		}
		_, err = repo.BeginExecution(f.ctx, f.conversation, f.question, "", database.TurnModeRetry, execution, "")
		return version, err
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, execution) })
	if err != nil {
		t.Fatal(err)
	}
	e.StartStream(0).Append("conteúdo ativo")
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1}
	w, err := p.ReadCanonicalWindow(f.ctx, query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w.Items[0], r.Item) || w.Items[0].Segments[0].Text != "conteúdo ativo" || w.Items[0].Durability != "volatile" {
		t.Fatalf("janela e recuperação divergiram: W=%+v R=%+v WS=%+v RS=%+v", w.Items[0], r.Item, w.Items[0].Segments, r.Item.Segments)
	}
	if _, err := p.ReadCanonicalWindow(database.WithUserID(context.Background(), "other"), query, f.reader); err == nil {
		t.Fatal("conteúdo ativo exposto sem autorização")
	}
}

func TestCanonicalWindowKeepsPreparingThreadInItsScope(t *testing.T) {
	f := newSnapshotFixture(t)
	question, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.question, Role: "user", Content: "thread"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.NewChatTurnRepository(f.db).Begin(f.ctx, f.conversation, question.ID, "", database.TurnModeNew); err != nil {
		t.Fatal(err)
	}
	query := database.MessageWindowQuery{ConversationID: f.conversation, ParentID: &f.question, Anchor: "end", Direction: "before", Limit: 10}
	w, err := f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if w.Window.TotalCount != 2 || len(w.Items) != 2 || w.Items[1].ItemID != "turn:"+question.ID || w.Items[1].ScopeParentID == nil || *w.Items[1].ScopeParentID != f.question {
		t.Fatalf("escopo perdido: %+v", w)
	}
	query.ParentID = nil
	w, err = f.reader.ReadCanonicalWindow(f.ctx, query)
	if err != nil || w.Window.TotalCount != 2 {
		t.Fatalf("thread vazou para raiz: %+v %v", w, err)
	}
	query.Anchor = ""
	query.AnchorItemID = "turn:" + question.ID
	if _, err = f.reader.ReadCanonicalWindow(f.ctx, query); err == nil {
		t.Fatal("âncora de outro escopo aceita")
	}
}

func TestCanonicalWindowPreservesHistoricalUnknownWithoutContent(t *testing.T) {
	f := newSnapshotFixture(t)
	if err := f.db.Model(&database.ChatTurn{}).Where("user_message_id=?", f.question).Update("state", database.TurnHistoricalUnknown).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteMessageWithContext(f.ctx, f.answer); err != nil {
		t.Fatal(err)
	}
	w, err := f.reader.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if w.Window.TotalCount != 2 || len(w.Items) != 1 || w.Items[0].State != database.TurnHistoricalUnknown || w.Items[0].Representative != nil {
		t.Fatalf("legado ganhou estado inferido: %+v", w)
	}
}

func BenchmarkCanonicalWindowTurnMembership(b *testing.B) {
	for _, count := range []int{100, 500, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			f := newSnapshotFixture(b)
			var messages []database.ChatMessage
			var turns []database.ChatTurn
			stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			for i := 0; i < count; i++ {
				id := fmt.Sprintf("question-%04d", i)
				messages = append(messages, database.ChatMessage{UUIDModel: database.UUIDModel{ID: id, CreatedAt: stamp.Add(time.Duration(i) * time.Second)}, ConversationID: f.conversation, Role: "user", Content: "question"})
				turns = append(turns, database.ChatTurn{UserMessageID: id, ConversationID: f.conversation, State: database.TurnHistoricalUnknown, Origin: "legacy"})
			}
			if err := f.db.CreateInBatches(messages, 100).Error; err != nil {
				b.Fatal(err)
			}
			if err := f.db.CreateInBatches(turns, 100).Error; err != nil {
				b.Fatal(err)
			}
			query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 240}
			for _, canonical := range []bool{false, true} {
				b.Run(fmt.Sprintf("canonical_%t", canonical), func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						var w *TimelineWindowSnapshot
						var err error
						if canonical {
							w, err = f.reader.ReadCanonicalWindow(f.ctx, query)
						} else {
							w, err = f.reader.ReadWindow(f.ctx, query)
						}
						if err != nil {
							b.Fatal(err)
						}
						b.ReportMetric(float64(len(w.Items)), "items")
					}
				})
			}
		})
	}
}
