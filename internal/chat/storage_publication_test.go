package chat

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"gorm.io/gorm"
)

type failingSelectionReader struct {
	*DBTimelineReader
	fail atomic.Bool
}

func TestStoragePublicationFirstSubscriptionWaitsForWriter(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	started, proceed := make(chan struct{}), make(chan struct{})
	cleanupRecoveryGate(t, proceed)
	written := make(chan error, 1)
	go func() {
		written <- p.ApplyStorageMutation(f.ctx, f.conversation, []string{"message:" + f.question}, func() ([]string, error) {
			close(started)
			<-proceed
			return nil, database.UpdateMessageTextWithContext(f.ctx, f.question, "após commit")
		})
	}()
	<-started
	type result struct {
		handle   *TimelineSubscription
		snapshot *SubscribedTimelineSnapshot
		err      error
	}
	subscribed := make(chan result, 1)
	go func() {
		h, s, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "first"}, "first", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
		subscribed <- result{h, s, err}
	}()
	select {
	case r := <-subscribed:
		if r.handle != nil {
			r.handle.Close()
		}
		t.Fatal("inscrição ultrapassou writer pendente", r.err)
	case <-time.After(50 * time.Millisecond):
	}
	close(proceed)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	r := <-subscribed
	if r.err != nil {
		t.Fatal(r.err)
	}
	defer r.handle.Close()
	if len(r.snapshot.Items) != 1 || canonicalText(r.snapshot.Items[0]) != "após commit" {
		t.Fatal("primeira inscrição recebeu estado anterior")
	}
}

func TestStoragePublicationBudgetQueriesAreBatched(t *testing.T) {
	f := newSnapshotFixture(t)
	messages := make([]database.ChatMessage, 80)
	for i := range messages {
		messages[i] = database.ChatMessage{ConversationID: f.conversation, Role: "user", Content: "curta"}
	}
	if err := f.db.CreateInBatches(&messages, 100).Error; err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(messages))
	for i, m := range messages {
		ids[i] = "message:" + m.ID
	}
	count := 0
	if err := f.db.Callback().Row().After("gorm:row").Register("selection_budget_count", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "SUM(1024") {
			count++
		}
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.reader.ReadSelection(f.ctx, f.conversation, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 80 || count != 2 {
		t.Fatalf("pré-leitura deixou de usar lote: itens=%d consultas=%d", len(snapshot.Items), count)
	}
}

func TestStoragePublicationRecoveryRetainsNewIdentityAfterReadFailure(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	r := &failingSelectionReader{DBTimelineReader: f.reader}
	p.reader = r
	r.fail.Store(true)
	var created string
	err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{}, func() ([]string, error) {
		message, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: "nova"})
		if err != nil {
			return nil, err
		}
		created = "message:" + message.ID
		return []string{created}, nil
	})
	if err != nil {
		t.Fatal("commit bem-sucedido virou falha", err)
	}
	r.fail.Store(false)
	if _, err := p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader); err != nil {
		t.Fatal(err)
	}
	c := e.conversation
	c.mu.Lock()
	defer c.mu.Unlock()
	if canonicalText(c.publication.items[created]) != "nova" || len(c.pendingStorageIDs) != 0 {
		t.Fatal("identidade confirmada perdida na recuperação")
	}
}

func (r *failingSelectionReader) ReadSelection(ctx context.Context, conversationID string, ids []string) (*TimelineSelectionSnapshot, error) {
	if r.fail.Load() {
		return nil, errors.New("selection unavailable")
	}
	return r.DBTimelineReader.ReadSelection(ctx, conversationID, ids)
}

func TestStoragePublicationSelectionUsesActualScopeAndOwnership(t *testing.T) {
	f := newSnapshotFixture(t)
	thread, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, ParentID: &f.question, Role: "user", Content: "thread"})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{"message:" + f.question, "turn:" + f.question, "message:" + thread.ID, "message:missing"}
	snapshot, err := f.reader.ReadSelection(f.ctx, f.conversation, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 3 {
		t.Fatalf("seleção: %+v", snapshot.Items)
	}
	found := false
	for _, item := range snapshot.Items {
		if item.ItemID == "message:"+thread.ID {
			found = item.ScopeParentID != nil && *item.ScopeParentID == f.question
		}
	}
	if !found {
		t.Fatal("escopo real ignorado")
	}
	other := database.WithUserID(context.Background(), "other-owner")
	if _, err := f.reader.ReadSelection(other, f.conversation, ids); err == nil {
		t.Fatal("seleção expôs outro owner")
	}
}

func TestStoragePublicationDeletionReconcilesEvenAfterPartialError(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	s := e.StartStream(0)
	s.Append("reter bytes")
	_, err := p.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	partial := errors.New("falha depois de confirmar exclusão")
	err = p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) {
		if err := database.DeleteMessageWithContext(f.ctx, f.question); err != nil {
			return nil, err
		}
		return nil, partial
	})
	if !errors.Is(err, partial) {
		t.Fatalf("resultado do writer alterado: %v", err)
	}
	c := e.conversation
	c.mu.Lock()
	if c.publicationErrorLocked() != nil || !c.turns[e.turnID].detached {
		t.Fatal("exclusão não reconciliada")
	}
	if _, exists := c.publication.items["turn:"+f.question]; exists {
		t.Fatal("resposta excluída permaneceu no cache")
	}
	removed := make(map[string]bool)
	for _, batch := range c.publication.batches {
		for _, op := range batch.Operations {
			if op.Kind == "item_remove" {
				removed[op.ItemID] = true
			}
		}
	}
	retained := snapshotLiveTurn(c, c.turns[e.turnID])
	c.mu.Unlock()
	if !removed["message:"+f.question] || !removed["turn:"+f.question] {
		t.Fatal("tombstones incompletos")
	}
	if testLiveSegment(retained.Streams[0], "text", 0).Text != "reter bytes" || s.Append("tardio") {
		t.Fatal("buffers desvinculados incorretos")
	}
}

func TestStoragePublicationSelectionDoesNotRemoveOtherPage(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	subscription, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "page"}, "history", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	err = p.ApplyStorageMutation(f.ctx, f.conversation, []string{"message:" + f.question}, func() ([]string, error) {
		return nil, database.UpdateMessageTextWithContext(f.ctx, f.question, "editada")
	})
	if err != nil {
		t.Fatal(err)
	}
	c, release := p.lockConversation(projectionKey{"snapshot-user", f.conversation}, false)
	defer release()
	if c.publicationErrorLocked() != nil {
		t.Fatal(c.publicationErrorLocked())
	}
	if _, ok := c.publication.items["turn:"+f.question]; !ok {
		t.Fatal("item fora da seleção tratado como exclusão")
	}
	if canonicalText(c.publication.items["message:"+f.question]) != "editada" {
		t.Fatal("edição não publicada")
	}
}

func TestStoragePublicationRecoveryRequiresRefreshForEachWindow(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	r := &failingSelectionReader{DBTimelineReader: f.reader}
	p.reader = r
	session := &fixtureTimelineSession{ctx: f.ctx, id: "storage"}
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}
	a, before, err := p.SubscribeTimeline(f.ctx, session, "a", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, _, err := p.SubscribeTimeline(f.ctx, session, "b", query, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	r.fail.Store(true)
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	e.StartStream(0).Append("durante falha")
	if batches, ok, err := a.Replay(f.ctx, before.Epoch, 0, f.reader); !errors.Is(err, ErrTimelinePublication) || ok || len(batches) != 0 {
		t.Fatal("replay não bloqueado", err)
	}
	r.fail.Store(false)
	after, err := a.Refresh(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Epoch == before.Epoch {
		t.Fatal("época não invalidada")
	}
	item, _ := publicationItem(t, e)
	if canonicalText(item) != "durante falha" {
		t.Fatal("recuperação perdeu buffers")
	}
	if _, ok, err := b.Replay(f.ctx, after.Epoch, 0, f.reader); err != nil || ok {
		t.Fatal("janela antiga aceitou época nova", err)
	}
	if _, err := b.Refresh(f.ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStoragePublicationLedgerUsesSharedProjection(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	e.StartStream(0).Append("texto vivo")
	svc := toolinvocations.NewServiceWithCoordinator(toolinvocations.NewDBRepository(f.db), nil, NewLedgerMutationCoordinator(p, f.db))
	inv, err := svc.Record(f.ctx, toolinvocations.RecordRequest{
		Origin: toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: f.question, ChatExecutionID: &e.executionID},
		Call:   tools.ToolCall{ID: "publication-call", Function: tools.FunctionCall{Name: "snapshot-tool", Arguments: "{}"}},
		Result: tools.ToolResult{Content: "resultado publicado"},
	})
	if err != nil {
		t.Fatal(err)
	}
	item, _ := publicationItem(t, e)
	found := false
	for _, segment := range item.Segments {
		for _, call := range segment.Invocations {
			if call.InvocationID == inv.ID {
				found = call.Status == toolinvocations.StatusSucceeded
			}
		}
	}
	if !found || canonicalText(item) != "texto vivo" {
		t.Fatalf("ledger e buffers não compostos: %+v", item)
	}
}
