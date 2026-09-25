package controllers

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func clearControllerFixture(t *testing.T) (*gorm.DB, context.Context, string, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "clear.sqlite")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Conversation{}, &database.ChatMessage{}, &database.ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateMessageRevisions(db); err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previous)
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	ctx := database.WithUserID(context.Background(), "clear-owner")
	conversation := database.Conversation{UserID: "clear-owner", Title: "Clear", Summary: "summary", SummaryUpToMessageID: "old", SummarizingInProgress: true}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	message := database.ChatMessage{ConversationID: conversation.ID, Role: "user", Content: "original"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	invocation := database.ToolInvocation{UserID: "clear-owner", ToolCatalogID: "tool", OriginType: "chat", OriginID: message.ID, Status: "completed", QueuedAt: time.Now()}
	if err := db.Create(&invocation).Error; err != nil {
		t.Fatal(err)
	}
	return db, ctx, conversation.ID, message.ID
}

func assertClearContent(t *testing.T, db *gorm.DB, id string, cleared bool) {
	t.Helper()
	var messages, invocations int64
	if err := db.Model(&database.ChatMessage{}).Where("conversation_id = ?", id).Count(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.ToolInvocation{}).Count(&invocations).Error; err != nil {
		t.Fatal(err)
	}
	var conversation database.Conversation
	if err := db.First(&conversation, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if cleared {
		if messages != 0 || invocations != 0 || conversation.Summary != "" || conversation.SummaryUpToMessageID != "" || conversation.SummarizingInProgress {
			t.Fatalf("conteúdo não limpo: messages=%d invocations=%d summary=%q", messages, invocations, conversation.Summary)
		}
	} else if messages != 1 || invocations != 1 || conversation.Summary != "summary" || conversation.SummaryUpToMessageID != "old" || !conversation.SummarizingInProgress {
		t.Fatalf("conteúdo alterado sem commit: messages=%d invocations=%d summary=%q", messages, invocations, conversation.Summary)
	}
}

func TestClearConversationContentCommitBeforeEffects(t *testing.T) {
	for _, conditional := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "conditional"}[conditional], func(t *testing.T) {
			db, ctx, id, _ := clearControllerFixture(t)
			var order []string
			controller := NewConversationsController(ConversationsControllerConfig{
				Emitter: conversationRecordingEmitter{events: &order},
				PrepareBatchDelete: func(context.Context, []string) (func(bool), error) {
					order = append(order, "prepare")
					return func(committed bool) {
						if committed {
							t.Error("clear não deve criar tombstone")
						}
						order = append(order, "release")
					}, nil
				},
				ResetScopedState: func(context.Context, string) {
					assertClearContent(t, db, id, true)
					order = append(order, "reset")
				},
			})
			revision, err := database.ConversationContentSnapshotWithContext(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if conditional {
				err = controller.ClearConversationIfUnchanged(ctx, id, revision)
			} else {
				err = controller.ClearConversation(ctx, id)
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(order, []string{"prepare", "reset", "evento:conversation:cleared", "release"}) {
				t.Fatalf("ordem: %v", order)
			}
			assertClearContent(t, db, id, true)
		})
	}
}

func TestClearConversationContentRollbackNoEffects(t *testing.T) {
	for _, stage := range []string{"tool_invocations", "chat_messages", "conversations"} {
		t.Run(stage, func(t *testing.T) {
			db, ctx, id, _ := clearControllerFixture(t)
			var events []string
			controller := NewConversationsController(ConversationsControllerConfig{Emitter: conversationRecordingEmitter{events: &events}, ResetScopedState: func(context.Context, string) { events = append(events, "reset") }})
			revision, err := database.ConversationContentSnapshotWithContext(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			forced := errors.New("forced rollback")
			callback := func(tx *gorm.DB) {
				if tx.Statement.Table == stage {
					_ = tx.AddError(forced)
				}
			}
			if stage == "conversations" {
				err = db.Callback().Update().Before("gorm:update").Register("clear_failure", callback)
			} else {
				err = db.Callback().Delete().Before("gorm:delete").Register("clear_failure", callback)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := controller.ClearConversationIfUnchanged(ctx, id, revision); !errors.Is(err, forced) {
				t.Fatalf("error=%v", err)
			}
			if len(events) != 0 {
				t.Fatalf("efeitos após falha: %v", events)
			}
			assertClearContent(t, db, id, false)
		})
	}
}

func TestClearConversationContentOwnershipFailClosed(t *testing.T) {
	db, ctx, id, _ := clearControllerFixture(t)
	revision, err := database.ConversationContentSnapshotWithContext(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	controller := NewConversationsController(ConversationsControllerConfig{Emitter: conversationRecordingEmitter{events: &events}, ResetScopedState: func(context.Context, string) { events = append(events, "reset") }})
	for _, other := range []context.Context{context.Background(), database.WithUserID(context.Background(), "stranger")} {
		if _, err := database.ConversationContentSnapshotWithContext(other, id); err == nil {
			t.Fatal("snapshot aceitou usuário não autorizado")
		}
		if err := controller.ClearConversationIfUnchanged(other, id, revision); err == nil {
			t.Fatal("clear condicional aceitou usuário não autorizado")
		}
		if err := controller.ClearConversation(other, id); err == nil {
			t.Fatal("clear aceitou usuário não autorizado")
		}
	}
	assertClearContent(t, db, id, false)
	if len(events) != 0 {
		t.Fatal(events)
	}
}

func TestClearConversationContentRejectsStaleSnapshot(t *testing.T) {
	for _, mutation := range []string{"message", "edit", "summary", "invocation"} {
		t.Run(mutation, func(t *testing.T) {
			db, ctx, id, messageID := clearControllerFixture(t)
			revision, err := database.ConversationContentSnapshotWithContext(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "message":
				err = db.Create(&database.ChatMessage{ConversationID: id, Role: "user", Content: "new"}).Error
			case "edit":
				err = db.Model(&database.ChatMessage{}).Where("id = ?", messageID).UpdateColumn("content", "edited").Error
			case "summary":
				err = db.Model(&database.Conversation{}).Where("id = ?", id).UpdateColumn("summary", "new summary").Error
			case "invocation":
				err = db.Model(&database.ToolInvocation{}).Where("origin_id = ?", messageID).UpdateColumn("output", "new output").Error
			}
			if err != nil {
				t.Fatal(err)
			}
			var events []string
			controller := NewConversationsController(ConversationsControllerConfig{Emitter: conversationRecordingEmitter{events: &events}, ResetScopedState: func(context.Context, string) { events = append(events, "reset") }})
			if err := controller.ClearConversationIfUnchanged(ctx, id, revision); !errors.Is(err, database.ErrConversationContentChanged) {
				t.Fatalf("error=%v", err)
			}
			if len(events) != 0 {
				t.Fatal(events)
			}
			after, err := database.ConversationContentSnapshotWithContext(ctx, id)
			if err != nil || after == revision {
				t.Fatalf("snapshot não mudou: %v", err)
			}
		})
	}
}

func TestClearConversationContentRejectsActiveGeneration(t *testing.T) {
	db, ctx, id, _ := clearControllerFixture(t)
	manager := chat.NewStreamingManager(nil)
	release, ok := manager.ReserveConversation(id)
	if !ok {
		t.Fatal("reserve failed")
	}
	var events []string
	controller := NewConversationsController(ConversationsControllerConfig{
		PrepareBatchDelete: func(_ context.Context, ids []string) (func(bool), error) {
			return manager.PrepareConversationDeletion(ids)
		},
		Emitter: conversationRecordingEmitter{events: &events},
	})
	if err := controller.ClearConversation(ctx, id); !errors.Is(err, chat.ErrConversationActive) {
		t.Fatalf("error=%v", err)
	}
	assertClearContent(t, db, id, false)
	if len(events) != 0 {
		t.Fatal(events)
	}
	release()
	_, cancel := context.WithCancel(context.Background())
	manager.Register(id, cancel)
	if err := controller.ClearConversation(ctx, id); !errors.Is(err, chat.ErrConversationActive) {
		t.Fatalf("active stream error=%v", err)
	}
	manager.Unregister(id)
	cancel()
	if err := controller.ClearConversation(ctx, id); err != nil {
		t.Fatal(err)
	}
	release, ok = manager.ReserveConversation(id)
	if !ok {
		t.Fatal("clear deixou tombstone")
	}
	release()
}

func TestClearConversationContentStaleReleasesRuntimeGate(t *testing.T) {
	db, ctx, id, _ := clearControllerFixture(t)
	manager := chat.NewStreamingManager(nil)
	controller := NewConversationsController(ConversationsControllerConfig{
		PrepareBatchDelete: func(_ context.Context, ids []string) (func(bool), error) {
			return manager.PrepareConversationDeletion(ids)
		},
	})
	if err := controller.ClearConversationIfUnchanged(ctx, id, "stale"); !errors.Is(err, database.ErrConversationContentChanged) {
		t.Fatalf("error=%v", err)
	}
	assertClearContent(t, db, id, false)
	reserved := make(chan bool, 1)
	go func() {
		release, ok := manager.ReserveConversation(id)
		if ok {
			release()
		}
		reserved <- ok
	}()
	select {
	case ok := <-reserved:
		if !ok {
			t.Fatal("clear obsoleto deixou tombstone")
		}
	case <-time.After(time.Second):
		t.Fatal("clear obsoleto não liberou gate runtime")
	}
}

func TestClearConversationContentGuardRunsInsideLifecycle(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "reject"}[reject], func(t *testing.T) {
			db, ctx, id, _ := clearControllerFixture(t)
			revision, err := database.ConversationContentSnapshotWithContext(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			var order []string
			controller := NewConversationsController(ConversationsControllerConfig{
				PrepareBatchDelete: func(context.Context, []string) (func(bool), error) {
					order = append(order, "runtime")
					return func(bool) { order = append(order, "release") }, nil
				},
				ResetScopedState: func(context.Context, string) { order = append(order, "reset") },
				Emitter:          conversationRecordingEmitter{events: &order},
			})
			denied := errors.New("workspace changed")
			err = controller.ClearConversationIfUnchangedGuarded(ctx, id, revision, func(commit func() error) error {
				order = append(order, "guard")
				probe, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
				if err := database.WithConversationLifecycle(probe, func() error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("guard fora do lifecycle: %v", err)
				}
				if reject {
					return denied
				}
				if err := commit(); err != nil {
					return err
				}
				assertClearContent(t, db, id, true)
				order = append(order, "guard-release")
				return nil
			})
			if reject {
				if !errors.Is(err, denied) {
					t.Fatal(err)
				}
				assertClearContent(t, db, id, false)
				if !reflect.DeepEqual(order, []string{"runtime", "guard", "release"}) {
					t.Fatal(order)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(order, []string{"runtime", "guard", "reset", "evento:conversation:cleared", "guard-release", "release"}) {
					t.Fatal(order)
				}
			}
		})
	}
}
