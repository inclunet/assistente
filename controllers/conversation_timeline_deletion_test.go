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

func TestConversationDeletionCoordinatesAtomicBatchBeforeEffects(t *testing.T) {
	for _, mode := range []string{"single", "batch", "clear-all", "rollback", "foreign", "busy"} {
		t.Run(mode, func(t *testing.T) {
			restore := database.SetDB(database.DB())
			if err := database.InitPath(filepath.Join(t.TempDir(), "delete-timeline.db")); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close(); restore() })
			db := database.DB()
			for _, user := range []string{"owner", "other"} {
				if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: user}, Username: user}).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx := database.WithUserID(context.Background(), "owner")
			reader := chat.NewDBTimelineReader(db)
			projection := chat.NewLiveProjection(reader)
			var ids []string
			var subscriptions []*chat.TimelineSubscription
			for _, user := range []string{"owner", "owner", "other"} {
				userCtx := database.WithUserID(context.Background(), user)
				conversation, err := database.CreateConversationWithContext(userCtx, "histórico", "")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := database.CreateMessageWithContext(userCtx, database.MessageOptions{ConversationID: conversation.ID, Role: "user", Content: "preservar ou remover"}); err != nil {
					t.Fatal(err)
				}
				sub, _, err := projection.SubscribeTimeline(userCtx, commandTimelineSession{userCtx}, "surface", database.MessageWindowQuery{ConversationID: conversation.ID, Anchor: "end", Direction: "before", Limit: 20}, reader)
				if err != nil {
					t.Fatal(err)
				}
				defer sub.Close()
				ids = append(ids, conversation.ID)
				subscriptions = append(subscriptions, sub)
			}
			failure := errors.New("exclusão recusada")
			if mode == "rollback" {
				if err := db.Callback().Delete().Before("gorm:delete").Register("reject_delete", func(tx *gorm.DB) {
					if tx.Statement.Table == "conversations" {
						_ = tx.AddError(failure)
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			maintenance, prepared := false, false
			coordinated, effects := 0, 0
			checkEffect := func() {
				effects++
				if _, err := subscriptions[0].Refresh(ctx); !errors.Is(err, chat.ErrTimelineSubscription) {
					t.Error("efeito antes da invalidação", err)
				}
			}
			controller := NewConversationsController(ConversationsControllerConfig{
				WithMaintenance: func(ctx context.Context, fn func() error) error {
					return database.WithSQLiteMaintenance(ctx, func() error { maintenance = true; defer func() { maintenance = false }(); return fn() })
				},
				PrepareBatchDelete: func(context.Context, []string) (func(bool), error) {
					if mode == "busy" {
						return nil, failure
					}
					prepared = true
					return func(committed bool) {
						if committed {
							checkEffect()
						}
						prepared = false
					}, nil
				},
				DeleteTimeline: func(ctx context.Context, ids []string, write func() ([]string, error)) ([]string, error) {
					coordinated++
					if !maintenance || !prepared {
						t.Fatal("ordem runtime/manutenção/projeção incorreta")
					}
					return projection.ApplyConversationDeletion(ctx, ids, write)
				},
				ResetScopedState: func(context.Context, string) { checkEffect() },
				Emitter:          commandTimelineEmitter{emit: func(string) { checkEffect() }},
			})
			var deleted []string
			var err error
			switch mode {
			case "single":
				err = controller.DeleteConversation(ctx, ids[0])
				if err == nil {
					deleted = ids[:1]
				}
			case "clear-all":
				deleted, err = controller.ClearConversations(ctx)
			case "foreign":
				deleted, err = controller.DeleteConversations(ctx, []string{ids[0], ids[2]})
			default:
				deleted, err = controller.DeleteConversations(ctx, []string{ids[1], ids[0], ids[1]})
			}
			failed := mode == "rollback" || mode == "foreign" || mode == "busy"
			if failed != (err != nil) {
				t.Fatalf("erro inesperado: %v", err)
			}
			if failed && (len(deleted) != 0 || effects != 0) {
				t.Fatal("falha produziu efeitos", deleted, effects)
			}
			if !failed && (coordinated != 1 || effects != 1+2*len(deleted)) {
				t.Fatal("coordenação/efeitos", coordinated, effects)
			}
			if (mode == "foreign" || mode == "busy") && coordinated != 0 {
				t.Fatal("recusa passou pela coordenação")
			}
			if prepared {
				t.Fatal("preparo não foi liberado")
			}
			for i, id := range ids {
				removed := !failed && i < 2 && (mode != "single" || i == 0)
				userCtx := ctx
				if i == 2 {
					userCtx = database.WithUserID(context.Background(), "other")
				}
				snapshot, readErr := subscriptions[i].Refresh(userCtx)
				if removed {
					if !errors.Is(readErr, chat.ErrTimelineSubscription) {
						t.Fatal("inscrição não invalidada", readErr)
					}
				} else if readErr != nil || len(snapshot.Items) != 1 {
					t.Fatal("conversa preservada perdeu conteúdo", readErr)
				}
				var conversations, messages int64
				db.Model(&database.Conversation{}).Where("id=?", id).Count(&conversations)
				db.Model(&database.ChatMessage{}).Where("conversation_id=?", id).Count(&messages)
				if removed && (conversations != 0 || messages != 0) || !removed && (conversations != 1 || messages != 1) {
					t.Fatal("atomicidade/isolamento", conversations, messages)
				}
			}
		})
	}
}
