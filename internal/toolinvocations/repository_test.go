package toolinvocations

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRepositoryTest(t *testing.T) (*DBRepository, context.Context, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{}, &database.Conversation{}, &database.ChatMessage{},
		&database.ToolCatalog{}, &database.ToolInvocation{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previous)
	})
	if err := db.Create(&database.ToolCatalog{
		Name:               "echo",
		DisplayName:        "echo",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}
	for _, row := range []struct {
		userID, conversationID, messageID string
	}{
		{userID: "user-a", conversationID: "conv-a", messageID: "conversation-a"},
		{userID: "user-b", conversationID: "conv-b", messageID: "conversation-b"},
	} {
		if err := db.Create(&database.Conversation{
			UUIDModel: database.UUIDModel{ID: row.conversationID},
			UserID:    row.userID,
			Title:     row.conversationID,
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&database.ChatMessage{
			UUIDModel:      database.UUIDModel{ID: row.messageID},
			ConversationID: row.conversationID,
			Role:           "assistant",
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, messageID := range []string{
		"turn-1", "turn-running", "turn-redact", "turn-status",
		"turn-cancel", "turn-timeout", "turn-rec", "turn-big",
	} {
		if err := db.Create(&database.ChatMessage{
			UUIDModel:      database.UUIDModel{ID: messageID},
			ConversationID: "conv-a",
			Role:           "assistant",
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return NewDBRepository(db), database.WithUserID(context.Background(), "user-a"), database.WithUserID(context.Background(), "user-b")
}

func TestCreateChatInvocationFailsClosedWithoutConversationTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	repo := NewDBRepository(db)
	inv := &Invocation{ToolCatalogID: "tool", OriginType: OriginChat, OriginID: "missing", Status: StatusQueued}
	if err := repo.Create(database.WithUserID(context.Background(), "user-a"), inv); err == nil {
		t.Fatal("invocação chat foi criada sem tabelas para validar posse")
	}
	var count int64
	if err := db.Model(&database.ToolInvocation{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invocações criadas sem validação: %d", count)
	}
}

func TestRepositoryCreatesAndListsScopedInvocations(t *testing.T) {
	repo, userA, userB := setupRepositoryTest(t)
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve tool: %v", err)
	}
	invA := &Invocation{
		ToolCatalogID: toolID,
		OriginType:    OriginChat,
		OriginID:      "conversation-a",
		ToolCallID:    "call-a",
		Status:        StatusQueued,
		Input:         json.RawMessage(`{"hello":"a"}`),
		QueuedAt:      time.Now(),
	}
	if err := repo.Create(userA, invA); err != nil {
		t.Fatalf("create user A: %v", err)
	}
	invB := *invA
	invB.ID = ""
	invB.OriginID = "conversation-b"
	invB.ToolCallID = "call-b"
	if err := repo.Create(userB, &invB); err != nil {
		t.Fatalf("create user B: %v", err)
	}

	gotA, err := repo.List(userA, Filter{OriginType: OriginChat})
	if err != nil {
		t.Fatalf("list user A: %v", err)
	}
	if len(gotA) != 1 || gotA[0].ToolCallID != "call-a" {
		t.Fatalf("unexpected user A invocations: %#v", gotA)
	}
}

func TestRepositoryNormalizesChatOriginBeforeOwnershipValidation(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatal(err)
	}
	inv := &Invocation{
		ToolCatalogID: toolID,
		OriginType:    " chat ",
		OriginID:      " conversation-a ",
		ToolCallID:    "normalized-chat",
		Status:        StatusQueued,
	}
	if err := repo.Create(userA, inv); err != nil {
		t.Fatalf("origem chat normalizada foi recusada: %v", err)
	}
	if inv.OriginType != OriginChat || inv.OriginID != "conversation-a" {
		t.Fatalf("origem não normalizada: type=%q id=%q", inv.OriginType, inv.OriginID)
	}
}

func TestRepositoryRejectsBlankChatOriginID(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatal(err)
	}
	inv := &Invocation{
		ToolCatalogID: toolID,
		OriginType:    " ",
		OriginID:      " ",
		ToolCallID:    "blank-origin",
		Status:        StatusQueued,
	}
	if err := repo.Create(userA, inv); !errors.Is(err, ErrChatOriginIDRequired) {
		t.Fatalf("erro=%v, esperado origin ID obrigatório", err)
	}
}

func TestCreateChatInvocationCannotRaceIntoDeletedConversation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "race.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&database.User{}, &database.Conversation{}, &database.ChatMessage{},
		&database.ToolCatalog{}, &database.ToolInvocation{},
	); err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previous) })
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := database.WithUserID(context.Background(), "user-a")
	if err := db.Create(&database.User{
		UUIDModel:    database.UUIDModel{ID: "user-a"},
		Username:     "user-a",
		PasswordHash: "test",
	}).Error; err != nil {
		t.Fatal(err)
	}
	conv := database.Conversation{UserID: "user-a", Title: "race"}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatal(err)
	}
	msg := database.ChatMessage{ConversationID: conv.ID, Role: "user", Content: "origem"}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{
		Name:               "echo-race",
		DisplayName:        "echo",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewDBRepository(db)
	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		errs <- repo.Create(ctx, &Invocation{
			ToolCatalogID: catalog.ID,
			OriginType:    OriginChat,
			OriginID:      msg.ID,
			ToolCallID:    "call-race",
		})
	}()
	go func() {
		<-start
		errs <- database.DeleteConversationWithContext(ctx, conv.ID)
	}()
	close(start)
	for range 2 {
		err := <-errs
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("corrida create/delete: %v", err)
		}
	}
	var count int64
	if err := db.Model(&database.ToolInvocation{}).Where("origin_id = ?", msg.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("tool invocation órfã criada após exclusão concorrente")
	}
}

func TestLoadChatToolInvocationDisplaysKeepsLatestRetryForCallID(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve tool: %v", err)
	}
	oldTime := time.Now().Add(-time.Minute)
	newTime := time.Now()
	rows := []database.ToolInvocation{
		{
			UserID:        "user-a",
			ToolCatalogID: toolID,
			OriginType:    OriginChat,
			OriginID:      "turn-1",
			ToolCallID:    "call-1",
			Status:        StatusFailed,
			Output:        `{"content":"OLD"}`,
			QueuedAt:      oldTime,
		},
		{
			UserID:        "user-a",
			ToolCatalogID: toolID,
			OriginType:    OriginChat,
			OriginID:      "turn-1",
			ToolCallID:    "call-1",
			Status:        StatusSucceeded,
			Output:        `{"content":"NEW"}`,
			QueuedAt:      newTime,
		},
	}
	if err := database.DB().Create(&rows).Error; err != nil {
		t.Fatalf("create invocations: %v", err)
	}

	displays, err := LoadChatToolInvocationDisplaysForTurnIDsWithUser(userA, "user-a", []string{"turn-1"})
	if err != nil {
		t.Fatalf("load displays: %v", err)
	}
	if got := displays["turn-1"]; len(got) != 1 || got[0].Result != "NEW" {
		t.Fatalf("expected latest retry result, got %+v", got)
	}
}

func TestLoadChatToolInvocationDisplaysIncludesQueuedAndRunning(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve tool: %v", err)
	}
	rows := []database.ToolInvocation{
		{
			UserID:        "user-a",
			ToolCatalogID: toolID,
			OriginType:    OriginChat,
			OriginID:      "turn-running",
			ToolCallID:    "call-queued",
			Status:        StatusQueued,
			Metadata:      `{"display":{"version":1,"type":"function","name":"echo","arguments":"{}","iteration":1}}`,
			QueuedAt:      time.Now().Add(-time.Second),
		},
		{
			UserID:        "user-a",
			ToolCatalogID: toolID,
			OriginType:    OriginChat,
			OriginID:      "turn-running",
			ToolCallID:    "call-running",
			Status:        StatusRunning,
			Metadata:      `{"display":{"version":1,"type":"function","name":"echo","arguments":"{}","iteration":2}}`,
			QueuedAt:      time.Now(),
		},
	}
	if err := database.DB().Create(&rows).Error; err != nil {
		t.Fatalf("create invocations: %v", err)
	}

	displays, err := LoadChatToolInvocationDisplaysForTurnIDsWithUser(userA, "user-a", []string{"turn-running"})
	if err != nil {
		t.Fatalf("load displays: %v", err)
	}
	got := displays["turn-running"]
	if len(got) != 2 {
		t.Fatalf("expected queued and running displays, got %+v", got)
	}
	if got[0].ID != "call-queued" || got[1].ID != "call-running" {
		t.Fatalf("unexpected display order/content: %+v", got)
	}
}

func TestRepositoryCompleteInvocation(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve tool: %v", err)
	}
	inv := &Invocation{
		ToolCatalogID: toolID,
		OriginType:    OriginToolCatalog,
		Status:        StatusQueued,
		QueuedAt:      time.Now(),
	}
	if err := repo.Create(userA, inv); err != nil {
		t.Fatalf("create: %v", err)
	}
	startedAt := time.Now()
	if err := repo.MarkRunning(userA, inv.ID, startedAt); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	completedAt := startedAt.Add(10 * time.Millisecond)
	inv.Status = StatusSucceeded
	inv.Output = json.RawMessage(`{"content":"ok"}`)
	inv.CompletedAt = &completedAt
	inv.DurationMs = 10
	if err := repo.Complete(userA, inv.ID, inv); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := repo.Get(userA, inv.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusSucceeded || got.DurationMs != 10 || string(got.Output) != `{"content":"ok"}` {
		t.Fatalf("unexpected completed invocation: %#v", got)
	}
}

func TestRepositoryRequiresUser(t *testing.T) {
	repo, _, _ := setupRepositoryTest(t)
	err := repo.Create(context.Background(), &Invocation{})
	if err != database.ErrUserScopeRequired {
		t.Fatalf("Create sem usuário: got %v, want ErrUserScopeRequired", err)
	}
}

// seedRetentionInvocations popula um conjunto comum de invocações para os
// testes de retenção (chat e dry-runs, com idades variadas).
func seedRetentionInvocations(t *testing.T, repo *DBRepository, userA context.Context, fixedNow time.Time) {
	t.Helper()
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve tool: %v", err)
	}
	old := fixedNow.Add(-48 * time.Hour)
	recent := fixedNow.Add(-2 * time.Hour)
	seed := func(id string, originType string, dryRun bool, queuedAt time.Time) {
		if originType == OriginChat {
			if err := database.DB().Create(&database.ChatMessage{
				UUIDModel:      database.UUIDModel{ID: "origin-" + id},
				ConversationID: "conv-a",
				Role:           "assistant",
			}).Error; err != nil {
				t.Fatalf("seed message %s: %v", id, err)
			}
		}
		inv := &Invocation{
			ID:            id,
			ToolCatalogID: toolID,
			OriginType:    originType,
			OriginID:      "origin-" + id,
			ToolCallID:    "call-" + id,
			Status:        StatusQueued,
			DryRun:        dryRun,
			QueuedAt:      queuedAt,
		}
		if err := repo.Create(userA, inv); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("chat-old", OriginChat, false, old)
	seed("chat-recent", OriginChat, false, recent)
	seed("tool-catalog-old-dry", OriginToolCatalog, true, old)
	seed("tool-catalog-old-real", OriginToolCatalog, false, old)
	seed("job-old-dry", OriginJobRun, true, old)
	seed("job-old-real", OriginJobRun, false, old)
}

func remainingIDSet(t *testing.T, repo *DBRepository, ctx context.Context) map[string]struct{} {
	t.Helper()
	remaining, err := repo.List(ctx, Filter{Limit: 50})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	ids := map[string]struct{}{}
	for _, inv := range remaining {
		ids[inv.ID] = struct{}{}
	}
	return ids
}

// CleanOldDryRuns só remove dry-runs operacionais (job_run/tool_catalog) por
// idade; nunca toca em chat nem em execuções reais (AEP-0074).
func TestRepositoryCleanOldDryRuns(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	fixedNow := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return fixedNow }
	seedRetentionInvocations(t, repo, userA, fixedNow)

	deleted, err := repo.CleanOldDryRuns(userA, 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanOldDryRuns: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted=%d, want 2 (tool-catalog-old-dry, job-old-dry)", deleted)
	}
	remaining := remainingIDSet(t, repo, userA)
	for _, want := range []string{"chat-old", "chat-recent", "tool-catalog-old-real", "job-old-real"} {
		if _, ok := remaining[want]; !ok {
			t.Fatalf("expected %s to remain, got %#v", want, remaining)
		}
	}
}

// CleanOldChat é o cap de idade OPCIONAL de chat: remove só invocações de chat
// mais antigas que maxAge, sem tocar em dados de jobs.
func TestRepositoryCleanOldChat(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	fixedNow := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return fixedNow }
	seedRetentionInvocations(t, repo, userA, fixedNow)

	deleted, err := repo.CleanOldChat(userA, 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanOldChat: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1 (chat-old)", deleted)
	}
	remaining := remainingIDSet(t, repo, userA)
	if _, ok := remaining["chat-old"]; ok {
		t.Fatalf("chat-old deveria ter sido removido, got %#v", remaining)
	}
	for _, want := range []string{"chat-recent", "tool-catalog-old-dry", "tool-catalog-old-real", "job-old-dry", "job-old-real"} {
		if _, ok := remaining[want]; !ok {
			t.Fatalf("expected %s to remain, got %#v", want, remaining)
		}
	}
}

// CleanOrphanChat remove invocações de chat cuja mensagem de origem não existe
// mais; preserva as que ainda têm mensagem viva (AEP-0074).
func TestRepositoryCleanOrphanChat(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	if err := database.DB().AutoMigrate(&database.ChatMessage{}); err != nil {
		t.Fatalf("automigrate chat_messages: %v", err)
	}
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve tool: %v", err)
	}
	// Mensagem viva: invocação ligada a ela deve sobreviver.
	liveMsg := database.ChatMessage{
		UUIDModel:      database.UUIDModel{ID: "msg-live"},
		ConversationID: "conv-a",
		Role:           "assistant",
	}
	if err := database.DB().Create(&liveMsg).Error; err != nil {
		t.Fatalf("create chat message: %v", err)
	}
	seed := func(id, originID string) {
		inv := &Invocation{
			ID:            id,
			ToolCatalogID: toolID,
			OriginType:    OriginChat,
			OriginID:      originID,
			ToolCallID:    "call-" + id,
			Status:        StatusQueued,
			QueuedAt:      time.Now(),
		}
		if err := repo.Create(userA, inv); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("chat-live", "msg-live")
	if err := database.DB().Create(&database.ToolInvocation{
		UUIDModel:     database.UUIDModel{ID: "chat-orphan"},
		UserID:        "user-a",
		ToolCatalogID: toolID,
		OriginType:    OriginChat,
		OriginID:      "msg-missing",
		ToolCallID:    "call-chat-orphan",
		Status:        StatusQueued,
		QueuedAt:      time.Now(),
	}).Error; err != nil {
		t.Fatalf("seed legado órfão: %v", err)
	}

	deleted, err := repo.CleanOrphanChat(userA)
	if err != nil {
		t.Fatalf("CleanOrphanChat: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1 (chat-orphan)", deleted)
	}
	remaining := remainingIDSet(t, repo, userA)
	if _, ok := remaining["chat-live"]; !ok {
		t.Fatalf("chat-live deveria sobreviver, got %#v", remaining)
	}
	if _, ok := remaining["chat-orphan"]; ok {
		t.Fatalf("chat-orphan deveria ter sido removido, got %#v", remaining)
	}
}
