package toolinvocations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdlog "log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRepositoryTest(t *testing.T) (*DBRepository, context.Context, context.Context) {
	t.Helper()
	db := setupProjectionReadTest(t)
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

func TestRepositoryCreateNaoRegistraPayloadEmFalha(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	var logs bytes.Buffer
	dbWithLogs := repo.db.Session(&gorm.Session{Logger: logger.New(
		stdlog.New(&logs, "", 0),
		logger.Config{LogLevel: logger.Info, ParameterizedQueries: false},
	)})
	callbackName := "test:tool_invocation_payload_error"
	if err := dbWithLogs.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*database.ToolInvocation); ok {
			_ = tx.AddError(errors.New("falha sintética sem payload"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dbWithLogs.Callback().Create().Remove(callbackName); err != nil {
			t.Errorf("remove callback: %v", err)
		}
	})
	repo = NewDBRepository(dbWithLogs)
	invocation := Invocation{
		ToolCatalogID: "catalog-a",
		OriginType:    OriginJobRun,
		OriginID:      "run-safe-log",
		ToolCallID:    "call-safe-log",
		Input:         json.RawMessage(`{"token":"NAO-PODE-VAZAR"}`),
		QueuedAt:      time.Now().UTC(),
	}

	if err := repo.Create(userA, &invocation); err == nil {
		t.Fatal("falha sintética não propagada")
	}
	if output := logs.String(); strings.Contains(output, "NAO-PODE-VAZAR") {
		t.Fatalf("log expôs payload técnico: %s", output)
	}
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

func TestRepositoryDerivaVinculoChatEIncrementaTentativa(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		invocation := &Invocation{
			ToolCatalogID:  toolID,
			OriginType:     OriginChat,
			OriginID:       "conversation-a",
			ConversationID: "conv-b",
			TurnID:         "turn-forjado",
			ToolCallID:     "retry-call",
			Status:         StatusQueued,
			QueuedAt:       time.Now(),
		}
		if err := repo.Create(userA, invocation); err != nil {
			t.Fatal(err)
		}
		if invocation.ConversationID != "conv-a" || invocation.TurnID != "conversation-a" ||
			invocation.Attempt != attempt {
			t.Fatalf("tentativa/vínculo %d inválido: %+v", attempt, invocation)
		}
	}
}

func TestRepositoryCriaCatalogoArchivalIsoladoEIndisponivel(t *testing.T) {
	repo, userA, userB := setupRepositoryTest(t)
	toolA, err := repo.ResolveOrCreateArchivalToolCatalogID(userA, "missing_tool")
	if err != nil {
		t.Fatal(err)
	}
	again, err := repo.ResolveOrCreateArchivalToolCatalogID(userA, "missing_tool")
	if err != nil {
		t.Fatal(err)
	}
	toolB, err := repo.ResolveOrCreateArchivalToolCatalogID(userB, "missing_tool")
	if err != nil {
		t.Fatal(err)
	}
	if toolA != again || toolA == toolB {
		t.Fatalf("identidade archival inválida: a=%s again=%s b=%s", toolA, again, toolB)
	}
	var rows []database.ToolCatalog
	if err := database.DB().Where("id IN ?", []string{toolA, toolB}).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("catálogos=%d, esperado 2", len(rows))
	}
	for _, row := range rows {
		if row.Origin != ToolOriginArchival || row.AvailabilityStatus != tools.ToolAvailabilityUnavailable ||
			row.UserID == nil {
			t.Fatalf("catálogo archival executável ou sem owner: %+v", row)
		}
	}
}

func TestRepositoryNormalizesOriginTypeButPreservesOpaqueID(t *testing.T) {
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
	if err := repo.Create(userA, inv); err == nil {
		t.Fatal("ID opaco foi confundido com origem sem espaços")
	}
	inv.OriginID = "conversation-a"
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
	turnID := "turn-1"
	oldTime := time.Now().Add(-time.Minute)
	newTime := time.Now()
	rows := []database.ToolInvocation{
		{
			UserID:        "user-a",
			ToolCatalogID: toolID,
			OriginType:    OriginChat,
			OriginID:      "turn-1",
			TurnID:        &turnID,
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
			TurnID:        &turnID,
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
	turnID := "turn-running"
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
			TurnID:        &turnID,
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
			TurnID:        &turnID,
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

func TestRepositoryCleanOldChatDetachesRecentChildrenAtomicallyWithForeignKeys(t *testing.T) {
	path, err := filepath.Abs(filepath.Join(t.TempDir(), "clean-old-chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	restoreDB := database.SetDB(database.DB())
	var initializedDB *gorm.DB
	t.Cleanup(func() {
		if initializedDB != nil {
			if sqlDB, err := initializedDB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		restoreDB()
	})
	if err := database.InitPath(path); err != nil {
		initializedDB = database.DB()
		t.Fatalf("InitPath: %v", err)
	}
	db := database.DB()
	initializedDB = db
	var foreignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("InitPath foreign_keys=%d, want 1", foreignKeys)
	}
	if err := db.Create(&database.User{
		UUIDModel: database.UUIDModel{ID: "retention-owner"}, Username: "retention-owner", PasswordHash: "test",
		Role: database.UserRoleUser, IsActive: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	tool := database.ToolCatalog{
		UUIDModel:          database.UUIDModel{ID: "retention-tool"},
		Name:               "retention-tool",
		DisplayName:        "Retention Tool",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}
	if err := db.Create(&tool).Error; err != nil {
		t.Fatal(err)
	}
	fixedNow := time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC)
	old := fixedNow.Add(-48 * time.Hour)
	recent := fixedNow.Add(-2 * time.Hour)
	parent := database.ToolInvocation{
		UUIDModel: database.UUIDModel{ID: "parent-old"}, UserID: "retention-owner", ToolCatalogID: tool.ID,
		OriginType: OriginChat, Status: StatusQueued, QueuedAt: old,
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("criar invocação antiga: %v", err)
	}
	child := database.ToolInvocation{
		UUIDModel: database.UUIDModel{ID: "child-recent"}, UserID: "retention-owner", ToolCatalogID: tool.ID,
		OriginType: OriginChat, Status: StatusQueued, QueuedAt: recent, ParentInvocationID: &parent.ID,
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("criar invocação recente filha: %v", err)
	}
	repo := NewDBRepository(db)
	repo.now = func() time.Time { return fixedNow }
	if err := db.Exec(`CREATE TRIGGER reject_old_invocation_delete BEFORE DELETE ON tool_invocations
		WHEN OLD.id = 'parent-old' BEGIN SELECT RAISE(ABORT, 'reject old invocation delete'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CleanOldChat(database.WithUserID(context.Background(), "retention-owner"), 24*time.Hour); err == nil {
		t.Fatal("CleanOldChat deveria falhar com trigger de regressão")
	}
	var storedParent, storedChild database.ToolInvocation
	if err := db.First(&storedParent, "id = ?", parent.ID).Error; err != nil {
		t.Fatalf("falha parcial removeu invocação pai: %v", err)
	}
	if err := db.First(&storedChild, "id = ?", child.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedChild.ParentInvocationID == nil || *storedChild.ParentInvocationID != parent.ID {
		t.Fatalf("falha parcial desassociou filha recente: parent=%v", storedChild.ParentInvocationID)
	}
	if err := db.Exec("DROP TRIGGER reject_old_invocation_delete").Error; err != nil {
		t.Fatal(err)
	}
	deleted, err := repo.CleanOldChat(database.WithUserID(context.Background(), "retention-owner"), 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanOldChat: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("CleanOldChat deleted=%d, want 1", deleted)
	}
	if err := db.First(&storedChild, "id = ?", child.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedChild.ParentInvocationID != nil {
		t.Fatalf("filha recente continua vinculada ao pai removido: %v", *storedChild.ParentInvocationID)
	}
	if err := db.First(&storedParent, "id = ?", parent.ID).Error; err == nil {
		t.Fatal("invocação antiga permaneceu após limpeza")
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

func TestRepositoryCleanOrphanChatBatchLimitsDeletesAndResumes(t *testing.T) {
	repo, userA, userB := setupRepositoryTest(t)
	if err := database.DB().AutoMigrate(&database.ChatMessage{}); err != nil {
		t.Fatalf("automigrate chat_messages: %v", err)
	}
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve tool: %v", err)
	}
	seedOrphan := func(ctx context.Context, id string) {
		t.Helper()
		userID, err := database.RequireUserID(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := database.DB().WithContext(ctx).Create(&database.ToolInvocation{
			UUIDModel:     database.UUIDModel{ID: id},
			UserID:        userID,
			ToolCatalogID: toolID,
			OriginType:    OriginChat,
			OriginID:      "missing-" + id,
			ToolCallID:    "call-" + id,
			Status:        StatusSucceeded,
			QueuedAt:      time.Now(),
		}).Error; err != nil {
			t.Fatalf("seed orphan %s: %v", id, err)
		}
	}
	seedOrphan(userA, "batch-orphan-a")
	seedOrphan(userA, "batch-orphan-b")
	seedOrphan(userA, "batch-orphan-c")
	seedOrphan(userB, "other-user-orphan")

	deleted, more, err := repo.CleanOrphanChatBatch(userA, 2)
	if err != nil {
		t.Fatalf("first batch: %v", err)
	}
	if deleted != 2 || !more {
		t.Fatalf("first batch = (%d, %v), want (2, true)", deleted, more)
	}
	deleted, more, err = repo.CleanOrphanChatBatch(userA, 2)
	if err != nil {
		t.Fatalf("second batch: %v", err)
	}
	if deleted != 1 || more {
		t.Fatalf("second batch = (%d, %v), want (1, false)", deleted, more)
	}

	var remaining int64
	if err := database.DB().Model(&database.ToolInvocation{}).Where("id = ?", "other-user-orphan").Count(&remaining).Error; err != nil {
		t.Fatalf("count other user's row: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("other user's orphan rows remaining = %d, want 1", remaining)
	}
}

func TestRepositoryCleanOrphanChatBatchBoundsChildDetachment(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	if err := database.DB().AutoMigrate(&database.ChatMessage{}); err != nil {
		t.Fatalf("automigrate chat_messages: %v", err)
	}
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve tool: %v", err)
	}
	parent := database.ToolInvocation{
		UUIDModel:     database.UUIDModel{ID: "orphan-parent-batch"},
		UserID:        "user-a",
		ToolCatalogID: toolID,
		OriginType:    OriginChat,
		OriginID:      "missing-parent-message",
		ToolCallID:    "call-orphan-parent-batch",
		Status:        StatusSucceeded,
		QueuedAt:      time.Now(),
	}
	if err := database.DB().Create(&parent).Error; err != nil {
		t.Fatalf("create orphan parent: %v", err)
	}
	for i := 0; i < 3; i++ {
		parentID := parent.ID
		if err := database.DB().Create(&database.ToolInvocation{
			UUIDModel:          database.UUIDModel{ID: fmt.Sprintf("surviving-child-%d", i)},
			UserID:             "user-a",
			ToolCatalogID:      toolID,
			OriginType:         OriginJobRun,
			OriginID:           fmt.Sprintf("run-%d", i),
			ParentInvocationID: &parentID,
			ToolCallID:         fmt.Sprintf("child-call-%d", i),
			Status:             StatusSucceeded,
			QueuedAt:           time.Now(),
		}).Error; err != nil {
			t.Fatalf("create child %d: %v", i, err)
		}
	}

	for pass := 1; pass <= 3; pass++ {
		deleted, more, err := repo.CleanOrphanChatBatch(userA, 1)
		if err != nil {
			t.Fatalf("batch %d: %v", pass, err)
		}
		wantDeleted, wantMore := 0, true
		if pass == 3 {
			wantDeleted, wantMore = 1, false
		}
		if deleted != wantDeleted || more != wantMore {
			t.Fatalf("batch %d = (%d, %v), want (%d, %v)", pass, deleted, more, wantDeleted, wantMore)
		}
		var linkedChildren int64
		if err := database.DB().Model(&database.ToolInvocation{}).
			Where("parent_invocation_id = ?", parent.ID).Count(&linkedChildren).Error; err != nil {
			t.Fatalf("count linked children after batch %d: %v", pass, err)
		}
		wantLinked := int64(3 - pass)
		if linkedChildren != wantLinked {
			t.Fatalf("linked children after batch %d = %d, want %d", pass, linkedChildren, wantLinked)
		}
	}
	var survivingChildren int64
	if err := database.DB().Model(&database.ToolInvocation{}).
		Where("id LIKE ? AND parent_invocation_id IS NULL", "surviving-child-%").Count(&survivingChildren).Error; err != nil {
		t.Fatalf("count detached children: %v", err)
	}
	if survivingChildren != 3 {
		t.Fatalf("detached children = %d, want 3", survivingChildren)
	}
}

func TestRepositoryCleanOrphanChatDetachesSurvivingChildrenAtomicallyWithForeignKeys(t *testing.T) {
	path, err := filepath.Abs(filepath.Join(t.TempDir(), "clean-orphan-chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	restoreDB := database.SetDB(database.DB())
	var initializedDB *gorm.DB
	t.Cleanup(func() {
		if initializedDB != nil {
			if sqlDB, err := initializedDB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		restoreDB()
	})
	if err := database.InitPath(path); err != nil {
		initializedDB = database.DB()
		t.Fatalf("InitPath: %v", err)
	}
	db := database.DB()
	initializedDB = db
	if err := db.Create(&database.User{
		UUIDModel: database.UUIDModel{ID: "orphan-owner"}, Username: "orphan-owner", PasswordHash: "test",
		Role: database.UserRoleUser, IsActive: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	tool := database.ToolCatalog{
		UUIDModel: database.UUIDModel{ID: "orphan-tool"}, Name: "orphan-tool", DisplayName: "Orphan tool",
		Origin: tools.ToolOriginBuiltin, AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}
	if err := db.Create(&tool).Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), "orphan-owner")
	conversation, err := database.CreateConversationWithContext(ctx, "live", "")
	if err != nil {
		t.Fatal(err)
	}
	message, err := database.CreateMessageWithContext(ctx, database.MessageOptions{
		ConversationID: conversation.ID, Role: "assistant", Content: "still live",
	})
	if err != nil {
		t.Fatal(err)
	}
	child := database.ToolInvocation{
		UUIDModel: database.UUIDModel{ID: "orphan-child"}, UserID: "orphan-owner", ToolCatalogID: tool.ID,
		OriginType: OriginChat, OriginID: message.ID, ConversationID: &conversation.ID,
		Status: StatusSucceeded, QueuedAt: time.Now(),
	}
	parent := database.ToolInvocation{
		UUIDModel: database.UUIDModel{ID: "orphan-parent"}, UserID: "orphan-owner", ToolCatalogID: tool.ID,
		OriginType: OriginChat, OriginID: "missing-message", Status: StatusSucceeded, QueuedAt: time.Now(),
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	child.ParentInvocationID = &parent.ID
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewDBRepository(db)
	if err := db.Exec(`CREATE TRIGGER reject_orphan_parent_delete BEFORE DELETE ON tool_invocations
		WHEN OLD.id = 'orphan-parent' BEGIN SELECT RAISE(ABORT, 'reject orphan parent delete'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CleanOrphanChat(ctx); err == nil {
		t.Fatal("CleanOrphanChat deveria falhar com trigger de regressão")
	}
	var storedChild database.ToolInvocation
	if err := db.First(&storedChild, "id = ?", child.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedChild.ParentInvocationID == nil || *storedChild.ParentInvocationID != parent.ID {
		t.Fatalf("falha parcial desassociou a filha: parent=%v", storedChild.ParentInvocationID)
	}
	if err := db.Exec("DROP TRIGGER reject_orphan_parent_delete").Error; err != nil {
		t.Fatal(err)
	}
	deleted, err := repo.CleanOrphanChat(ctx)
	if err != nil {
		t.Fatalf("CleanOrphanChat: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("CleanOrphanChat deleted=%d, want 1", deleted)
	}
	if err := db.First(&storedChild, "id = ?", child.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedChild.ParentInvocationID != nil {
		t.Fatalf("filha viva continua vinculada ao pai órfão: %v", *storedChild.ParentInvocationID)
	}
}

// TestResolveToolCatalogID_UsaCacheComTTL prova que uma resolução positiva é
// servida do cache em memória (sem tocar o banco) dentro do TTL e volta a
// consultar o banco depois que a entrada expira. O efeito é observável: após a
// primeira resolução, a linha é apagada do banco; enquanto o TTL vale, a
// resolução ainda retorna o ID cacheado; passado o TTL, retorna "não encontrado".
func TestResolveToolCatalogID_UsaCacheComTTL(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	clock := time.Now()
	repo.now = func() time.Time { return clock }

	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve inicial: %v", err)
	}
	if toolID == "" {
		t.Fatal("ID vazio na resolução inicial")
	}

	// Remove a linha do catálogo: qualquer resolução subsequente que atinja o
	// banco falharia. Só o cache pode continuar respondendo.
	if err := database.DB().Where("name = ?", "echo").Delete(&database.ToolCatalog{}).Error; err != nil {
		t.Fatalf("delete catálogo: %v", err)
	}

	cached, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve dentro do TTL deveria vir do cache: %v", err)
	}
	if cached != toolID {
		t.Fatalf("cache retornou ID divergente: got=%s want=%s", cached, toolID)
	}

	// Avança o relógio além do TTL: a entrada expira e a resolução volta ao banco
	// (agora vazio), retornando "não encontrado".
	clock = clock.Add(toolCatalogResolveCacheTTL + time.Second)
	if _, err := repo.ResolveToolCatalogID(userA, "echo"); !errors.Is(err, ErrToolCatalogNotFound) {
		t.Fatalf("após expirar o TTL, esperado ErrToolCatalogNotFound, got %v", err)
	}
}

// TestResolveToolCatalogID_CacheIsoladoPorUsuario garante que o cache é chaveado
// por usuário: a entrada cacheada de um usuário nunca é servida a outro (AEP-0104).
func TestResolveToolCatalogID_CacheIsoladoPorUsuario(t *testing.T) {
	repo, userA, userB := setupRepositoryTest(t)
	clock := time.Now()
	repo.now = func() time.Time { return clock }

	ownerA := "user-a"
	ownerB := "user-b"
	seedRow := func(id, owner string) {
		if err := database.DB().Create(&database.ToolCatalog{
			UUIDModel:          database.UUIDModel{ID: id},
			UserID:             &owner,
			Name:               "scoped_tool",
			DisplayName:        "scoped_tool",
			Origin:             tools.ToolOriginMCPBridge,
			AvailabilityStatus: tools.ToolAvailabilityAvailable,
		}).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seedRow("scoped-a", ownerA)
	seedRow("scoped-b", ownerB)

	gotA, err := repo.ResolveToolCatalogID(userA, "scoped_tool")
	if err != nil || gotA != "scoped-a" {
		t.Fatalf("userA resolve=%s err=%v, esperado scoped-a", gotA, err)
	}
	gotB, err := repo.ResolveToolCatalogID(userB, "scoped_tool")
	if err != nil || gotB != "scoped-b" {
		t.Fatalf("userB resolve=%s err=%v, esperado scoped-b (não pode servir cache de A)", gotB, err)
	}

	// Remove ambas as linhas: dentro do TTL cada usuário só pode receber a SUA
	// própria entrada cacheada.
	if err := database.DB().Where("name = ?", "scoped_tool").Delete(&database.ToolCatalog{}).Error; err != nil {
		t.Fatalf("delete catálogos: %v", err)
	}
	if got, err := repo.ResolveToolCatalogID(userA, "scoped_tool"); err != nil || got != "scoped-a" {
		t.Fatalf("cache userA=%s err=%v, esperado scoped-a", got, err)
	}
	if got, err := repo.ResolveToolCatalogID(userB, "scoped_tool"); err != nil || got != "scoped-b" {
		t.Fatalf("cache userB=%s err=%v, esperado scoped-b", got, err)
	}
}

// TestResolveToolCatalogID_NaoCacheiaArchival garante que uma resolução que cai
// numa entrada archival (placeholder) não é fixada: assim que a tool real aparece
// no catálogo, a próxima resolução prefere a real (a ordenação desprioriza
// archival).
func TestResolveToolCatalogID_NaoCacheiaArchival(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)

	archID, err := repo.ResolveOrCreateArchivalToolCatalogID(userA, "arch_tool")
	if err != nil {
		t.Fatalf("cria archival: %v", err)
	}
	first, err := repo.ResolveToolCatalogID(userA, "arch_tool")
	if err != nil || first != archID {
		t.Fatalf("resolve archival=%s err=%v, esperado %s", first, err, archID)
	}

	// Surge a tool real (não-archival) com o mesmo nome. Se o archival tivesse
	// sido cacheado, a resolução continuaria retornando archID.
	owner := "user-a"
	if err := database.DB().Create(&database.ToolCatalog{
		UUIDModel:          database.UUIDModel{ID: "arch-real"},
		UserID:             &owner,
		Name:               "arch_tool",
		DisplayName:        "arch_tool",
		Origin:             tools.ToolOriginMCPBridge,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed real: %v", err)
	}

	got, err := repo.ResolveToolCatalogID(userA, "arch_tool")
	if err != nil {
		t.Fatalf("resolve após tool real: %v", err)
	}
	if got != "arch-real" {
		t.Fatalf("resolve=%s, esperado arch-real (archival não pode ter sido cacheado)", got)
	}
}
