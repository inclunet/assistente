package controllers

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestConversationClearCoordinatesBeforeEffects(t *testing.T) {
	for _, mode := range []string{"plain", "unchanged", "guarded", "stale", "guard-rejected", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			restore := database.SetDB(database.DB())
			if err := database.InitPath(filepath.Join(t.TempDir(), "clear-timeline.db")); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close(); restore() })
			db := database.DB()
			if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: "owner"}, Username: "owner"}).Error; err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), "owner")
			conv, err := database.CreateConversationWithContext(ctx, "limpeza", "")
			if err != nil {
				t.Fatal(err)
			}
			question, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, Role: "user", Content: "pergunta"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, TurnID: &question.ID, Role: "assistant", Content: "resposta"}); err != nil {
				t.Fatal(err)
			}
			if err := database.NewChatTurnRepository(db).ReconcileImportedConversationInTransaction(ctx, conv.ID); err != nil {
				t.Fatal(err)
			}
			catalog := database.ToolCatalog{Name: "clear-tool", Origin: "builtin"}
			if err := db.Create(&catalog).Error; err != nil {
				t.Fatal(err)
			}
			invocation := database.ToolInvocation{UserID: "owner", ToolCatalogID: catalog.ID, OriginType: "chat", ConversationID: &conv.ID, TurnID: &question.ID, ToolCallID: "clear-call", Status: "succeeded", OutputPreview: "resultado"}
			if err := db.Create(&invocation).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&database.Conversation{}).Where("id=?", conv.ID).Update("summary", "resumo").Error; err != nil {
				t.Fatal(err)
			}
			r := &commandTimelineReader{DBTimelineReader: chat.NewDBTimelineReader(db)}
			p := chat.NewLiveProjection(r)
			sub, before, err := p.SubscribeTimeline(ctx, commandTimelineSession{ctx}, "clear", database.MessageWindowQuery{ConversationID: conv.ID, Anchor: "end", Direction: "before", Limit: 20}, r)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			revision, err := database.ConversationContentSnapshotWithContext(ctx, conv.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "stale" {
				if err := database.UpdateMessageTextWithContext(ctx, question.ID, "alterada"); err != nil {
					t.Fatal(err)
				}
			}
			failure := errors.New("limpeza recusada")
			if mode == "rollback" {
				if err := db.Callback().Delete().Before("gorm:delete").Register("reject_clear", func(tx *gorm.DB) {
					if tx.Statement.Table == "chat_messages" {
						_ = tx.AddError(failure)
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			maintenance, guarded := false, false
			coordinated, effects := 0, 0
			checkEffect := func() {
				effects++
				if r.reads != 1 || r.selected == nil || len(r.selected.Items) != 0 {
					t.Error("efeito ocorreu antes da reconciliação")
				}
			}
			c := NewConversationsController(ConversationsControllerConfig{
				WithMaintenance: func(ctx context.Context, fn func() error) error {
					return database.WithSQLiteMaintenance(ctx, func() error { maintenance = true; defer func() { maintenance = false }(); return fn() })
				},
				MutateTimeline: func(ctx context.Context, id string, known []string, write func() ([]string, error)) error {
					coordinated++
					if !maintenance || known != nil || (mode == "guarded" && !guarded) {
						t.Fatal("ordem guard/manutenção/projeção ou escopo alterados")
					}
					return p.ApplyStorageMutation(ctx, id, known, write)
				},
				ResetScopedState: func(context.Context, string) { checkEffect() },
				Emitter: commandTimelineEmitter{emit: func(name string) {
					if name != "conversation:cleared" {
						t.Error("evento inesperado", name)
					}
					checkEffect()
				}},
			})
			switch mode {
			case "plain", "rollback":
				err = c.ClearConversation(ctx, conv.ID)
			case "unchanged", "stale":
				err = c.ClearConversationIfUnchanged(ctx, conv.ID, revision)
			default:
				err = c.ClearConversationIfUnchangedGuarded(ctx, conv.ID, revision, func(commit func() error) error {
					if mode == "guard-rejected" {
						return failure
					}
					guarded = true
					defer func() { guarded = false }()
					return commit()
				})
			}
			cleared := mode == "plain" || mode == "unchanged" || mode == "guarded"
			if cleared {
				if err != nil || effects != 2 || coordinated != 1 {
					t.Fatal("limpeza não concluiu", err, effects, coordinated)
				}
			} else {
				expected := failure
				if mode == "stale" {
					expected = database.ErrConversationContentChanged
				}
				if !errors.Is(err, expected) || effects != 0 {
					t.Fatal("recusa perdida ou emitiu efeitos", err)
				}
				if mode == "guard-rejected" && coordinated != 0 {
					t.Fatal("guard recusado alcançou projetor")
				}
			}
			var stored database.Conversation
			if err := db.First(&stored, "id=?", conv.ID).Error; err != nil {
				t.Fatal("limpeza removeu conversa", err)
			}
			var messages, calls int64
			if err := db.Model(&database.ChatMessage{}).Where("conversation_id=?", conv.ID).Count(&messages).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&database.ToolInvocation{}).Where("id=?", invocation.ID).Count(&calls).Error; err != nil {
				t.Fatal(err)
			}
			after, err := sub.Refresh(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if cleared {
				if messages != 0 || calls != 0 || stored.Summary != "" || len(after.Items) != 0 || after.TotalCount != 0 {
					t.Fatal("conteúdo sobreviveu à limpeza")
				}
			} else if messages != 2 || calls != 1 || stored.Summary != "resumo" || len(after.Items) != len(before.Items) {
				t.Fatal("recusa alterou conteúdo confirmado")
			}
		})
	}
}

func TestConversationClearPreservesOpaqueConversationIdentity(t *testing.T) {
	for _, mode := range []string{"plain", "unchanged", "guarded"} {
		t.Run(mode, func(t *testing.T) {
			restore := database.SetDB(database.DB())
			if err := database.InitPath(filepath.Join(t.TempDir(), "opaque-clear.db")); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close(); restore() })
			db := database.DB()
			if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: "owner"}, Username: "owner"}).Error; err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), "owner")
			catalog := database.ToolCatalog{Name: "opaque-tool", Origin: "builtin"}
			if err := db.Create(&catalog).Error; err != nil {
				t.Fatal(err)
			}
			ids := []string{"conversation", " conversation "}
			for _, id := range ids {
				conv := database.Conversation{UUIDModel: database.UUIDModel{ID: id}, UserID: "owner", Title: id, Summary: "resumo " + id}
				if err := db.Create(&conv).Error; err != nil {
					t.Fatal(err)
				}
				q, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: id, Role: "user", Content: "pergunta " + id})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: id, TurnID: &q.ID, Role: "assistant", Content: "resposta " + id}); err != nil {
					t.Fatal(err)
				}
				if err := database.NewChatTurnRepository(db).ReconcileImportedConversationInTransaction(ctx, id); err != nil {
					t.Fatal(err)
				}
				inv := database.ToolInvocation{UserID: "owner", ToolCatalogID: catalog.ID, OriginType: "chat", ConversationID: &id, TurnID: &q.ID, ToolCallID: "call " + id, Status: "succeeded", OutputPreview: "resultado " + id}
				if err := db.Create(&inv).Error; err != nil {
					t.Fatal(err)
				}
			}
			r := chat.NewDBTimelineReader(db)
			p := chat.NewLiveProjection(r)
			subs := make([]*chat.TimelineSubscription, 0, 2)
			for _, id := range ids {
				sub, _, err := p.SubscribeTimeline(ctx, commandTimelineSession{ctx}, id, database.MessageWindowQuery{ConversationID: id, Anchor: "end", Direction: "before", Limit: 20}, r)
				if err != nil {
					t.Fatal(err)
				}
				defer sub.Close()
				subs = append(subs, sub)
			}
			target := ids[1]
			revision, err := database.ConversationContentSnapshotWithContext(ctx, target)
			if err != nil {
				t.Fatal(err)
			}
			c := NewConversationsController(ConversationsControllerConfig{MutateTimeline: func(ctx context.Context, id string, known []string, write func() ([]string, error)) error {
				if id != target {
					t.Fatal("barreira usa outra identidade")
				}
				return p.ApplyStorageMutation(ctx, id, known, write)
			}})
			switch mode {
			case "plain":
				err = c.ClearConversation(ctx, target)
			case "unchanged":
				err = c.ClearConversationIfUnchanged(ctx, target, revision)
			case "guarded":
				err = c.ClearConversationIfUnchangedGuarded(ctx, target, revision, func(commit func() error) error { return commit() })
			}
			if err != nil {
				t.Fatal(err)
			}
			for i, id := range ids {
				var messages, calls int64
				if err := db.Model(&database.ChatMessage{}).Where("conversation_id=?", id).Count(&messages).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Model(&database.ToolInvocation{}).Where("conversation_id=?", id).Count(&calls).Error; err != nil {
					t.Fatal(err)
				}
				var conv database.Conversation
				if err := db.First(&conv, "id=?", id).Error; err != nil {
					t.Fatal(err)
				}
				snapshot, err := subs[i].Refresh(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if id == target {
					if messages != 0 || calls != 0 || conv.Summary != "" || len(snapshot.Items) != 0 {
						t.Fatal("conversa opaca não foi limpa integralmente", messages, calls, len(snapshot.Items))
					}
				} else if messages != 2 || calls != 1 || conv.Summary != "resumo "+id || len(snapshot.Items) != 2 {
					t.Fatal("limpeza atingiu conversa vizinha", messages, calls, len(snapshot.Items))
				}
			}
		})
	}
}
