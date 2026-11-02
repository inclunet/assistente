package chat

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"gorm.io/gorm"
)

func maintenanceRelatedConversation(t *testing.T, f snapshotFixture) (string, string) {
	t.Helper()
	c, err := database.CreateConversationWithContext(f.ctx, "Relacionada", "")
	if err != nil {
		t.Fatal(err)
	}
	q, err := database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: c.ID, Role: "user", Content: "pergunta filha"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.CreateMessageWithContext(f.ctx, database.MessageOptions{ConversationID: c.ID, Role: "assistant", TurnID: &q.ID, Content: "resposta filha"}); err != nil {
		t.Fatal(err)
	}
	if err := database.NewChatTurnRepository(f.db).ReconcileImportedConversationInTransaction(f.ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	return c.ID, q.ID
}

func TestMaintenanceRetentionCoordinatesParentsAndChildrenAtomically(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[rollback], func(t *testing.T) {
			f := newSnapshotFixture(t)
			p := NewLiveProjection(f.reader)
			childConversation, childTurn := maintenanceRelatedConversation(t, f)
			var parent database.ToolInvocation
			if err := f.db.First(&parent, "id=?", f.invocation).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.db.Model(&parent).Update("queued_at", time.Now().Add(-48*time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			child := database.ToolInvocation{UserID: parent.UserID, ToolCatalogID: parent.ToolCatalogID, OriginType: "chat", ConversationID: &childConversation, TurnID: &childTurn, ToolCallID: "child", ParentInvocationID: &parent.ID, Status: "succeeded", QueuedAt: time.Now(), OutputPreview: "filho preservado"}
			if err := f.db.Create(&child).Error; err != nil {
				t.Fatal(err)
			}
			session := &fixtureTimelineSession{ctx: f.ctx, id: "maintenance"}
			first, before, err := p.SubscribeTimeline(f.ctx, session, "parent", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			second, _, err := p.SubscribeTimeline(f.ctx, session, "child", database.MessageWindowQuery{ConversationID: childConversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			if rollback {
				if err := f.db.Callback().Delete().Before("gorm:delete").Register("retention_rollback", func(tx *gorm.DB) {
					if tx.Statement.Table == "tool_invocations" {
						_ = tx.AddError(errors.New("rollback"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			var coordinated []string
			repo := toolinvocations.NewDBRepositoryWithMaintenanceCoordinator(f.db, func(ctx context.Context, ids []string, write func() error) error {
				coordinated = append([]string(nil), ids...)
				return p.ApplyStorageBatchMutation(ctx, ids, write)
			})
			svc := toolinvocations.NewService(repo, nil)
			deleted, err := svc.CleanOldChat(f.ctx, 24*time.Hour)
			if (err != nil) != rollback || deleted != map[bool]int{false: 1, true: 0}[rollback] {
				t.Fatal(deleted, err)
			}
			if !reflect.DeepEqual(coordinated, uniquePublicationIDs([]string{f.conversation, childConversation})) {
				t.Fatal("filho fora da barreira", coordinated)
			}
			if err := f.db.First(&child, "id=?", child.ID).Error; err != nil {
				t.Fatal(err)
			}
			if (child.ParentInvocationID != nil) != rollback {
				t.Fatal("detach não atômico", child.ParentInvocationID)
			}
			// A base deve estar atualizada antes de Refresh poder reler o banco.
			first.conversation.mu.Lock()
			item := first.conversation.publication.items["turn:"+f.question]
			first.conversation.mu.Unlock()
			calls := 0
			for _, segment := range item.Segments {
				calls += len(segment.Invocations)
			}
			if calls != map[bool]int{false: 0, true: 1}[rollback] || canonicalText(item) != "resposta antiga" {
				t.Fatal("projeção não reconciliada", item)
			}
			after, err := first.Refresh(f.ctx)
			if err != nil || after.WindowID != before.WindowID {
				t.Fatal(after, err)
			}
			if rollback && !reflect.DeepEqual(after.Items, before.Items) {
				t.Fatal("rollback mudou itens publicados")
			}
		})
	}
}

func TestMaintenanceRetentionRevalidatesNewAffectedConversation(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	second, turn := maintenanceRelatedConversation(t, f)
	var parent database.ToolInvocation
	if err := f.db.First(&parent, "id=?", f.invocation).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&parent).Update("queued_at", time.Now().Add(-48*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	attempts := 0
	repo := toolinvocations.NewDBRepositoryWithMaintenanceCoordinator(f.db, func(ctx context.Context, ids []string, write func() error) error {
		attempts++
		if attempts == 1 {
			if !reflect.DeepEqual(ids, []string{f.conversation}) {
				t.Fatal(ids)
			}
			late := database.ToolInvocation{UserID: parent.UserID, ToolCatalogID: parent.ToolCatalogID, OriginType: "chat", ConversationID: &second, TurnID: &turn, ToolCallID: "late", Status: "succeeded", QueuedAt: time.Now().Add(-48 * time.Hour)}
			if err := f.db.Create(&late).Error; err != nil {
				t.Fatal(err)
			}
		} else if !reflect.DeepEqual(ids, uniquePublicationIDs([]string{f.conversation, second})) {
			t.Fatal("retry não reservou nova conversa", ids)
		}
		return p.ApplyStorageBatchMutation(ctx, ids, write)
	})
	deleted, err := repo.CleanOldChat(f.ctx, 24*time.Hour)
	if err != nil || deleted != 2 || attempts != 2 {
		t.Fatal(deleted, attempts, err)
	}
}

func TestMaintenanceFirstSubscriberWaitsForRetentionCommit(t *testing.T) {
	f := newSnapshotFixture(t)
	p := NewLiveProjection(f.reader)
	if err := f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Update("queued_at", time.Now().Add(-48*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	entered, proceed := make(chan struct{}), make(chan struct{})
	cleanupRecoveryGate(t, proceed)
	repo := toolinvocations.NewDBRepositoryWithMaintenanceCoordinator(f.db, func(ctx context.Context, ids []string, write func() error) error {
		return p.ApplyStorageBatchMutation(ctx, ids, func() error { close(entered); <-proceed; return write() })
	})
	finished := make(chan error, 1)
	go func() { _, err := repo.CleanOldChat(f.ctx, 24*time.Hour); finished <- err }()
	<-entered
	type subscriptionResult struct {
		handle   *TimelineSubscription
		snapshot *SubscribedTimelineSnapshot
		err      error
	}
	subscribed := make(chan subscriptionResult, 1)
	go func() {
		h, s, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "first-maintenance"}, "first", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
		subscribed <- subscriptionResult{h, s, err}
	}()
	select {
	case r := <-subscribed:
		if r.handle != nil {
			r.handle.Close()
		}
		t.Fatal("primeira inscrição atravessou manutenção", r.err)
	case <-time.After(50 * time.Millisecond):
	}
	close(proceed)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	r := <-subscribed
	if r.err != nil {
		t.Fatal(r.err)
	}
	defer r.handle.Close()
	if len(r.snapshot.Items) != 2 {
		t.Fatal(r.snapshot)
	}
	for _, item := range r.snapshot.Items {
		for _, segment := range item.Segments {
			if len(segment.Invocations) != 0 {
				t.Fatal("inscrição recebeu ferramenta já removida")
			}
		}
	}
}

func TestMaintenanceRetentionIncludesLegacyTurnConversations(t *testing.T) {
	for _, divergent := range []bool{false, true} {
		t.Run(map[bool]string{false: "null", true: "divergent"}[divergent], func(t *testing.T) {
			f := newSnapshotFixture(t)
			p := NewLiveProjection(f.reader)
			other, _ := maintenanceRelatedConversation(t, f)
			var storedConversation *string
			wantIDs := []string{f.conversation}
			if divergent {
				storedConversation = &other
				wantIDs = uniquePublicationIDs(append(wantIDs, other))
			}
			if err := f.db.Model(&database.ToolInvocation{}).Where("id=?", f.invocation).Updates(map[string]any{"conversation_id": storedConversation, "origin_id": "unresolved-origin", "queued_at": time.Now().Add(-48 * time.Hour)}).Error; err != nil {
				t.Fatal(err)
			}
			sub, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "legacy-maintenance"}, "history", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			calls := func() int {
				sub.conversation.mu.Lock()
				defer sub.conversation.mu.Unlock()
				count := 0
				for _, s := range sub.conversation.publication.items["turn:"+f.question].Segments {
					count += len(s.Invocations)
				}
				return count
			}
			if calls() != 1 {
				t.Fatal("fixture não projetou a ferramenta legada")
			}
			repo := toolinvocations.NewDBRepositoryWithMaintenanceCoordinator(f.db, func(ctx context.Context, ids []string, write func() error) error {
				if !reflect.DeepEqual(ids, wantIDs) {
					t.Fatal("descoberta perdeu vínculo legado", ids, wantIDs)
				}
				return p.ApplyStorageBatchMutation(ctx, ids, write)
			})
			if count, err := repo.CleanOldChat(f.ctx, 24*time.Hour); err != nil || count != 1 {
				t.Fatal(count, err)
			}
			if calls() != 0 {
				t.Fatal("projeção ainda contém ferramenta removida")
			}
		})
	}
}
