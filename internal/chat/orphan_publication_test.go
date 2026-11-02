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

func TestOrphanMaintenanceCoordinatesBoundedChildrenAndRollback(t *testing.T) {
	for _, limit := range []int{0, 1} {
		t.Run(map[int]string{0: "legacy", 1: "bounded"}[limit], func(t *testing.T) {
			f := newSnapshotFixture(t)
			p := NewLiveProjection(f.reader)
			var parent database.ToolInvocation
			if err := f.db.First(&parent, "id=?", f.invocation).Error; err != nil {
				t.Fatal(err)
			}
			// O resumo legado ainda projeta por turn_id, embora a origem removida
			// torne a linha elegível à limpeza de órfãos.
			if err := f.db.Model(&parent).Updates(map[string]any{"conversation_id": nil, "origin_id": "missing-origin"}).Error; err != nil {
				t.Fatal(err)
			}
			children := make([]database.ToolInvocation, 2)
			conversations := make([]string, 2)
			for i := range children {
				conv, turn := maintenanceRelatedConversation(t, f)
				conversations[i] = conv
				children[i] = database.ToolInvocation{UserID: parent.UserID, ToolCatalogID: parent.ToolCatalogID, OriginType: "chat", ConversationID: &conv, TurnID: &turn, ToolCallID: "child", ParentInvocationID: &parent.ID, Status: "succeeded", QueuedAt: time.Now(), OutputPreview: "preservado"}
				if err := f.db.Create(&children[i]).Error; err != nil {
					t.Fatal(err)
				}
			}
			sub, _, err := p.SubscribeTimeline(f.ctx, &fixtureTimelineSession{ctx: f.ctx, id: "orphans"}, "parent", database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			var scopes [][]string
			repo := toolinvocations.NewDBRepositoryWithMaintenanceCoordinator(f.db, func(ctx context.Context, ids []string, write func() error) error {
				scopes = append(scopes, append([]string(nil), ids...))
				return p.ApplyStorageBatchMutation(ctx, ids, write)
			})
			svc := toolinvocations.NewService(repo, nil)
			clean := func() (int, bool, error) {
				if limit > 0 {
					return svc.CleanOrphanChatBatch(f.ctx, limit)
				}
				n, err := svc.CleanOrphanChat(f.ctx)
				return n, false, err
			}
			if err := f.db.Callback().Update().Before("gorm:update").Register("orphan_rollback", func(tx *gorm.DB) {
				if tx.Statement.Table == "tool_invocations" {
					_ = tx.AddError(errors.New("rollback"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			if n, _, err := clean(); err == nil || n != 0 {
				t.Fatal("limpeza deveria reverter", n, err)
			}
			for _, child := range children {
				var stored database.ToolInvocation
				if err := f.db.First(&stored, "id=?", child.ID).Error; err != nil || stored.ParentInvocationID == nil {
					t.Fatal("rollback perdeu filho", stored, err)
				}
			}
			if err := f.db.Callback().Update().Remove("orphan_rollback"); err != nil {
				t.Fatal(err)
			}
			n, more, err := clean()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(scopes[0], scopes[1]) {
				t.Fatal("cursor avançou em rollback", scopes)
			}
			if limit > 0 {
				if n != 0 || !more {
					t.Fatal("detach não respeitou limite", n, more)
				}
				want := uniquePublicationIDs([]string{f.conversation, conversations[0]})
				if !reflect.DeepEqual(scopes[1], want) {
					t.Fatal("descoberta varreu filhos fora do lote", scopes[1], want)
				}
				var remaining database.ToolInvocation
				if err := f.db.First(&remaining, "id=?", children[1].ID).Error; err != nil || remaining.ParentInvocationID == nil {
					t.Fatal("segundo filho alterado fora do lote", remaining, err)
				}
				n, _, err = clean()
				if err != nil {
					t.Fatal(err)
				}
				want = uniquePublicationIDs([]string{f.conversation, conversations[1]})
				if !reflect.DeepEqual(scopes[2], want) {
					t.Fatal("retomada não selecionou próximo filho", scopes[2], want)
				}
			} else if !reflect.DeepEqual(scopes[1], uniquePublicationIDs(append([]string{f.conversation}, conversations...))) {
				t.Fatal("limpeza legada perdeu filho", scopes[1])
			}
			if n != 1 {
				t.Fatal("pai órfão não removido", n)
			}
			for _, child := range children {
				var stored database.ToolInvocation
				if err := f.db.First(&stored, "id=?", child.ID).Error; err != nil || stored.ParentInvocationID != nil {
					t.Fatal("filho não preservado/desvinculado", stored, err)
				}
			}
			sub.conversation.mu.Lock()
			item := sub.conversation.publication.items["turn:"+f.question]
			sub.conversation.mu.Unlock()
			if canonicalText(item) != "resposta antiga" {
				t.Fatal("limpeza removeu mensagem", item)
			}
			for _, segment := range item.Segments {
				if len(segment.Invocations) != 0 {
					t.Fatal("órfão permaneceu na base publicada")
				}
			}
		})
	}
}
