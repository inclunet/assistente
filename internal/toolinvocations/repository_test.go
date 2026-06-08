package toolinvocations

import (
	"context"
	"encoding/json"
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
	if err := db.AutoMigrate(&database.User{}, &database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
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
	return NewDBRepository(db), database.WithUserID(context.Background(), "user-a"), database.WithUserID(context.Background(), "user-b")
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

func TestRepositoryCleanOld_DeletesChatAndDryRunsOnly(t *testing.T) {
	repo, userA, _ := setupRepositoryTest(t)
	fixedNow := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	repo.now = func() time.Time { return fixedNow }

	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatalf("resolve tool: %v", err)
	}

	old := fixedNow.Add(-48 * time.Hour)
	recent := fixedNow.Add(-2 * time.Hour)

	seed := func(id string, originType string, dryRun bool, queuedAt time.Time) {
		t.Helper()
		inv := &Invocation{
			ID:           id,
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

	deleted, err := repo.CleanOld(userA, 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanOld: %v", err)
	}
	// Espera deletar: chat-old, tool-catalog-old-dry, job-old-dry
	if deleted != 3 {
		t.Fatalf("deleted=%d, want 3", deleted)
	}

	remaining, err := repo.List(userA, Filter{Limit: 50})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	remainingIDs := map[string]struct{}{}
	for _, inv := range remaining {
		remainingIDs[inv.ID] = struct{}{}
	}
	for _, want := range []string{"chat-recent", "tool-catalog-old-real", "job-old-real"} {
		if _, ok := remainingIDs[want]; !ok {
			t.Fatalf("expected %s to remain, got %#v", want, remainingIDs)
		}
	}
}
