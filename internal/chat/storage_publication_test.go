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
	fail           atomic.Bool
	afterSelection func()
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
	for _, mixed := range []bool{false, true} {
		count = 0
		selected := append([]string(nil), ids...)
		expected := 1
		if mixed {
			expected = 2
			for _, m := range messages {
				selected = append(selected, "turn:"+m.ID)
			}
		}
		snapshot, err := f.reader.ReadSelection(f.ctx, f.conversation, selected)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Items) != 80 || count != expected {
			t.Fatalf("pré-leitura deixou de usar lote: misto=%v itens=%d consultas=%d", mixed, len(snapshot.Items), count)
		}
	}
}

func TestStoragePublicationSelectionQueryPlanUsesSelectedIdentities(t *testing.T) {
	f := newSnapshotFixture(t)
	messages := make([]database.ChatMessage, 1000)
	for i := range messages {
		messages[i] = database.ChatMessage{ConversationID: f.conversation, Role: "user", Content: "fora da seleção"}
	}
	if err := f.db.CreateInBatches(&messages, 100).Error; err != nil {
		t.Fatal(err)
	}
	var query string
	var args []interface{}
	type capturedQuery struct {
		sql  string
		args []interface{}
	}
	var budgets []capturedQuery
	var payload capturedQuery
	if err := f.db.Callback().Query().After("gorm:query").Register("selection_payload_plan", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "selected_payload") {
			payload = capturedQuery{tx.Statement.SQL.String(), append([]interface{}(nil), tx.Statement.Vars...)}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Callback().Row().After("gorm:row").Register("selection_plan", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "WITH scoped AS") {
			query = tx.Statement.SQL.String()
			args = append([]interface{}(nil), tx.Statement.Vars...)
		}
		if strings.Contains(tx.Statement.SQL.String(), "SUM(1024") {
			budgets = append(budgets, capturedQuery{tx.Statement.SQL.String(), append([]interface{}(nil), tx.Statement.Vars...)})
		}
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.reader.ReadSelection(f.ctx, f.conversation, []string{"message:" + f.question, "turn:" + f.question})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 2 || query == "" {
		t.Fatal("seleção não exercitada")
	}
	var plan []struct{ Detail string }
	if err := f.db.Raw("EXPLAIN QUERY PLAN "+query, args...).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	messageLookup, turnLookup, sourceLookup := false, false, false
	for _, row := range plan {
		messageLookup = messageLookup || (strings.HasPrefix(row.Detail, "SEARCH m ") && strings.Contains(row.Detail, "(id=?)"))
		turnLookup = turnLookup || (strings.HasPrefix(row.Detail, "SEARCH t ") && strings.Contains(row.Detail, "user_message_id=?"))
		sourceLookup = sourceLookup || (strings.HasPrefix(row.Detail, "SEARCH chat_messages ") && strings.Contains(row.Detail, "turn_id=?"))
	}
	if !messageLookup || !turnLookup || !sourceLookup {
		t.Fatalf("seleção deixou de buscar identidades indexadas: %+v", plan)
	}
	if payload.sql == "" {
		t.Fatal("materialização não capturada")
	}
	plan = nil
	if err := f.db.Raw("EXPLAIN QUERY PLAN "+payload.sql, payload.args...).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	messageLookup, turnLookup = false, false
	for _, row := range plan {
		messageLookup = messageLookup || (strings.HasPrefix(row.Detail, "SEARCH chat_messages ") && strings.Contains(row.Detail, "(id=?)"))
		turnLookup = turnLookup || (strings.HasPrefix(row.Detail, "SEARCH chat_messages ") && strings.Contains(row.Detail, "turn_id=?"))
		if strings.HasPrefix(row.Detail, "SEARCH chat_messages ") && strings.Contains(row.Detail, "conversation_id=?") && !strings.Contains(row.Detail, "turn_id=?") {
			t.Fatalf("payload varre escopo: %+v", plan)
		}
	}
	if !messageLookup || !turnLookup {
		t.Fatalf("payload sem buscas indexadas: %+v", plan)
	}
	if len(budgets) != 2 {
		t.Fatal("preflight não capturado", len(budgets))
	}
	for i, budget := range budgets {
		plan = nil
		if err := f.db.Raw("EXPLAIN QUERY PLAN "+budget.sql, budget.args...).Scan(&plan).Error; err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			messageLookup, turnLookup = false, false
			for _, row := range plan {
				if strings.HasPrefix(row.Detail, "SEARCH chat_messages ") && !strings.Contains(row.Detail, "(id=?)") {
					t.Fatalf("preflight percorre mensagens fora da seleção: %+v", plan)
				}
				messageLookup = messageLookup || (strings.HasPrefix(row.Detail, "SEARCH chat_messages ") && strings.Contains(row.Detail, "(id=?)"))
				turnLookup = turnLookup || (strings.HasPrefix(row.Detail, "SEARCH m ") && strings.Contains(row.Detail, "turn_id=?"))
			}
			if !messageLookup || !turnLookup {
				t.Fatalf("preflight varre histórico: %+v", plan)
			}
		} else {
			ledgerLookup := false
			for _, row := range plan {
				ledgerLookup = ledgerLookup || (strings.HasPrefix(row.Detail, "SEARCH tool_invocations ") && strings.Contains(row.Detail, "turn_id=?"))
			}
			if !ledgerLookup {
				t.Fatalf("preflight percorre ledger fora da seleção: %+v", plan)
			}
		}
	}
}

func TestStoragePublicationLegacySelfLinkedQuestionIsNotResponseSource(t *testing.T) {
	f := newSnapshotFixture(t)
	if err := f.db.Model(&database.ChatMessage{}).Where("id=?", f.question).Updates(map[string]any{"turn_id": f.question, "content": strings.Repeat("x", canonicalSourceBytes+1)}).Error; err != nil {
		t.Fatal(err)
	}
	for _, publication := range []bool{false, true} {
		var snapshot *TimelineTurnSnapshot
		var err error
		if publication {
			snapshot, err = f.reader.ReadPublicationTurn(f.ctx, f.conversation, f.question)
		} else {
			snapshot, err = f.reader.ReadTurn(f.ctx, f.conversation, f.question)
		}
		if err != nil || snapshot.Response == nil || canonicalText(canonicalItem(snapshot.Response)) != "resposta antiga" {
			t.Fatal("pergunta legada entrou na resposta", publication, err)
		}
		for _, message := range snapshot.Response.messages {
			if message.Role == "user" {
				t.Fatal("materializou pergunta como fonte da resposta")
			}
		}
	}
	snapshot, err := f.reader.ReadSelection(f.ctx, f.conversation, []string{"turn:" + f.question})
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatal("orçamento contou pergunta fora da seleção", err)
	}
	if _, err := f.reader.ReadSelection(f.ctx, f.conversation, []string{"message:" + f.question}); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("pergunta selecionada escapou do orçamento", err)
	}
	p := NewLiveProjection(f.reader)
	sub, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "legacy-self"}, "response", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{"turn:" + f.question}, func() ([]string, error) {
		return nil, f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Update("output_preview", "novo resultado").Error
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sub.Refresh(f.ctx); err != nil {
		t.Fatal("pergunta fora da janela bloqueou Refresh", err)
	}
}

func TestStoragePublicationQuestionBudgetExcludesUnselectedLargeResponse(t *testing.T) {
	f := newSnapshotFixture(t)
	if err := database.UpdateMessageTextWithContext(f.ctx, f.answer, strings.Repeat("x", canonicalSourceBytes+1)); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Update("output_preview", strings.Repeat("y", canonicalSourceBytes+1)).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.reader.ReadSelection(f.ctx, f.conversation, []string{"message:" + f.question})
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatal("pergunta contou fontes fora da seleção", err)
	}
	if _, err := f.reader.ReadSelection(f.ctx, f.conversation, []string{"turn:" + f.question}); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("turno grande escapou do orçamento", err)
	}
	r := &failingSelectionReader{DBTimelineReader: f.reader}
	p := NewLiveProjection(r)
	sub, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "question-budget"}, "q", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	r.fail.Store(true)
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	r.fail.Store(false)
	if _, err := sub.Refresh(f.ctx); err != nil {
		t.Fatal("recuperação de pergunta bloqueada pela resposta fora da janela", err)
	}
}

func TestStoragePublicationBudgetDoesNotDoubleCountSelectedMessageAndTurn(t *testing.T) {
	f := newSnapshotFixture(t)
	tx := f.db.WithContext(f.ctx)
	turn, err := publicationSelectionReadSize(tx, f.conversation, nil, []string{f.question})
	if err != nil {
		t.Fatal(err)
	}
	both, err := publicationSelectionReadSize(tx, f.conversation, []string{f.answer}, []string{f.question})
	if err != nil {
		t.Fatal(err)
	}
	if both != turn {
		t.Fatalf("mensagem duplicada pelo turno: %d != %d", both, turn)
	}
}

func TestStoragePublicationOversizedTurnStillAllowsAuthorizedExplicitRecovery(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	sub, before, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "large-recovery"}, "history", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	repo := database.NewChatTurnRepository(f.db)
	id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
	if err != nil {
		t.Fatal(err)
	}
	e.StartStream(0).Append("bytes recuperáveis")
	large := strings.Repeat("x", canonicalSourceBytes+1)
	if err := repo.WritePartial(f.ctx, e.executionID, id, large, "", false); err != nil {
		t.Fatal(err)
	}
	svc := toolinvocations.NewServiceWithCoordinator(toolinvocations.NewDBRepository(f.db), nil, NewLedgerMutationCoordinator(p, f.db))
	if _, err := svc.Record(f.ctx, toolinvocations.RecordRequest{Origin: toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: f.question, ChatExecutionID: &e.executionID}, Call: tools.ToolCall{ID: "large-recovery", Function: tools.FunctionCall{Name: "snapshot-tool", Arguments: "{}"}}, Result: tools.ToolResult{Content: "confirmado"}}); err != nil {
		t.Fatal("falha auxiliar alterou commit", err)
	}
	c := e.conversation
	c.mu.Lock()
	sequence := c.publication.sequence
	epoch := c.epoch
	storageErr := c.storagePublicationErr
	c.mu.Unlock()
	if !errors.Is(storageErr, ErrTimelineSourceLimit) {
		t.Fatal("fixture não excedeu publicação", storageErr)
	}
	result, err := p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader)
	if err != nil {
		t.Fatal("orçamento auxiliar bloqueou recuperação explícita", err)
	}
	if result.Persisted.Response.Representative.Content != large || result.Active == nil || testLiveSegment(result.Active.Streams[0], "text", 0).Text != "bytes recuperáveis" {
		t.Fatal("recuperação perdeu persistência ou buffer")
	}
	c.mu.Lock()
	unchanged := c.publication.sequence == sequence && c.epoch == epoch && c.publicationErrorLocked() != nil
	c.mu.Unlock()
	if !unchanged {
		t.Fatal("recuperação acima da cota publicou ou liberou journal")
	}
	if _, ok, err := sub.Replay(f.ctx, before.Epoch, 0, f.reader); ok || !errors.Is(err, ErrTimelinePublication) {
		t.Fatal("replay liberado", err)
	}
	if _, err := sub.Refresh(f.ctx); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("snapshot automático ignorou cota", err)
	}
	denied := errors.New("acesso revogado")
	if result, err := p.ReadRecovery(f.ctx, f.conversation, f.question, recoveryReader{func() (*TimelineTurnSnapshot, error) { return nil, denied }}); result != nil || !errors.Is(err, denied) {
		t.Fatal("recuperação ignorou autorização", err)
	}
	if err := repo.WritePartial(f.ctx, e.executionID, id, "curto", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := sub.Refresh(f.ctx); err != nil {
		t.Fatal("publicação não recuperou após redução", err)
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
	snapshot, err := r.DBTimelineReader.ReadSelection(ctx, conversationID, ids)
	if err == nil && r.afterSelection != nil {
		r.afterSelection()
	}
	return snapshot, err
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

func TestStoragePublicationRecoveryParcelsIndependentLargeWindows(t *testing.T) {
	f := newSnapshotFixture(t)
	large := strings.Repeat("x", (4<<20)+4096)
	if err := database.UpdateMessageTextWithContext(f.ctx, f.question, large); err != nil {
		t.Fatal(err)
	}
	last, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: f.conversation, Role: "user", Content: large})
	if err != nil {
		t.Fatal(err)
	}
	r := &failingSelectionReader{DBTimelineReader: f.reader}
	p := NewLiveProjection(r)
	session := &fixtureTimelineSession{ctx: f.ctx, id: "large"}
	a, _, err := p.SubscribeTimeline(f.ctx, session, "a", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "start", Direction: "after", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, _, err := p.SubscribeTimeline(f.ctx, session, "b", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 1}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	r.fail.Store(true)
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	r.fail.Store(false)
	first, err := a.Refresh(f.ctx)
	if err != nil {
		t.Fatal("janelas válidas excederam cota agregada", err)
	}
	second, err := b.Refresh(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || len(second.Items) != 1 || first.Items[0].ItemID != "message:"+f.question || second.Items[0].ItemID != "message:"+last.ID || canonicalText(first.Items[0]) != large || canonicalText(second.Items[0]) != large {
		t.Fatal("recuperação perdeu itens das janelas")
	}
}

func TestStoragePublicationRecoveryRejectsVersionDriftBetweenParcels(t *testing.T) {
	f := newSnapshotFixture(t)
	messages := make([]database.ChatMessage, 70)
	for i := range messages {
		messages[i] = database.ChatMessage{ConversationID: f.conversation, Role: "user", Content: "estável"}
	}
	if err := f.db.CreateInBatches(&messages, 100).Error; err != nil {
		t.Fatal(err)
	}
	r := &failingSelectionReader{DBTimelineReader: f.reader}
	p := NewLiveProjection(r)
	a, before, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "drift"}, "a", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 100}, f.reader)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	r.fail.Store(true)
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	r.fail.Store(false)
	r.afterSelection = func() {
		r.afterSelection = nil
		if err := database.UpdateMessageTextWithContext(f.ctx, messages[69].ID, "novo commit"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Refresh(f.ctx); !errors.Is(err, ErrProjectionRecoveryRequired) {
		t.Fatal("mistura de versões aceita", err)
	}
	if _, ok, err := a.Replay(f.ctx, before.Epoch, 0, f.reader); ok || !errors.Is(err, ErrTimelinePublication) {
		t.Fatal("replay liberado após recuperação parcial", err)
	}
	after, err := a.Refresh(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != len(before.Items) {
		t.Fatal("parcela não lida virou exclusão")
	}
	found := false
	for _, item := range after.Items {
		if item.ItemID == "message:"+messages[69].ID {
			found = canonicalText(item) == "novo commit"
		}
	}
	if !found {
		t.Fatal("recuperação não incorporou último commit")
	}
}

func TestStoragePublicationTerminalRecoveryKeepsLateLedgerUpdate(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	r := &failingSelectionReader{DBTimelineReader: f.reader}
	p.reader = r
	svc := toolinvocations.NewServiceWithCoordinator(toolinvocations.NewDBRepository(f.db), nil, NewLedgerMutationCoordinator(p, f.db))
	inv, err := svc.Record(f.ctx, toolinvocations.RecordRequest{Origin: toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: f.question, ChatExecutionID: &e.executionID}, Call: tools.ToolCall{ID: "late", Function: tools.FunctionCall{Name: "snapshot-tool", Arguments: "{}"}}, Result: tools.ToolResult{Content: "anterior"}})
	if err != nil {
		t.Fatal(err)
	}
	repo := database.NewChatTurnRepository(f.db)
	id, err := e.AssignResponse(func() (string, error) { return repo.EnsureResponse(f.ctx, f.conversation, f.question, e.executionID) })
	if err != nil {
		t.Fatal(err)
	}
	r.fail.Store(true)
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, nil, func() ([]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if err := e.CommitResponse(database.TurnCompleted, "completed", "final", "", func() (database.ChatTimelineVersion, error) {
		return repo.FinishResponseWithVersion(f.ctx, f.conversation, f.question, e.executionID, database.TurnCompleted, "completed", id, &database.MessageOptions{Content: "final"})
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.ApplyStorageMutation(f.ctx, f.conversation, []string{"turn:" + f.question}, func() ([]string, error) {
		return nil, f.db.Model(&database.ToolInvocation{}).Where("id=?", inv.ID).Update("output_preview", "resultado tardio").Error
	}); err != nil {
		t.Fatal(err)
	}
	r.fail.Store(false)
	if _, err := p.ReadRecovery(f.ctx, f.conversation, f.question, f.reader); err != nil {
		t.Fatal(err)
	}
	item, _ := publicationItem(t, e)
	found := false
	for _, segment := range item.Segments {
		for _, call := range segment.Invocations {
			if call.InvocationID == inv.ID {
				found = call.OutputPreview == "resultado tardio"
			}
		}
	}
	if !found {
		t.Fatal("flush terminal sobrescreveu ledger recuperado", item)
	}
	e.conversation.mu.Lock()
	defer e.conversation.mu.Unlock()
	if e.conversation.turns[e.turnID].publicationBase != nil {
		t.Fatal("base concluída não liberada")
	}
}
