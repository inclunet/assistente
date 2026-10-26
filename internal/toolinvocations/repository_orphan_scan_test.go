package toolinvocations

import (
	"assistente/internal/database"
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"
	"testing"
	"time"
)

func TestOrphanScanBoundsHealthyPagesAndRevisitsEarlierIDs(t *testing.T) {
	repo, userA, userB := setupRepositoryTest(t)
	toolID, err := repo.ResolveToolCatalogID(userA, "echo")
	if err != nil {
		t.Fatal(err)
	}
	seed := func(user, id, origin string) {
		t.Helper()
		if err := repo.db.Create(&database.ToolInvocation{
			UUIDModel: database.UUIDModel{ID: id}, UserID: user, ToolCatalogID: toolID,
			OriginType: OriginChat, OriginID: origin, ToolCallID: id, Status: StatusSucceeded, QueuedAt: time.Now(),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		seed("user-a", fmt.Sprintf("page-%02d", i), "conversation-a")
	}
	seed("user-a", "page-05", "missing")
	seed("user-b", "other-orphan", "missing")
	for pass := 0; pass < 2; pass++ {
		deleted, more, err := repo.CleanOrphanChatBatch(userA, 2)
		if err != nil || deleted != 0 || !more {
			t.Fatalf("healthy pass %d: %d %v %v", pass, deleted, more, err)
		}
		want := fmt.Sprintf("page-%02d", pass*2+1)
		if repo.orphanScanAfter["user-a"] != want {
			t.Fatalf("cursor=%q want %q", repo.orphanScanAfter["user-a"], want)
		}
	}
	// Outra conta progride independentemente.
	if n, more, err := repo.CleanOrphanChatBatch(userB, 2); err != nil || n != 1 || more {
		t.Fatalf("other user: %d %v %v", n, more, err)
	}
	// Um ID inserido atrás do cursor será revisitado no próximo ciclo.
	seed("user-a", "earlier-orphan", "missing")
	if n, more, err := repo.CleanOrphanChatBatch(userA, 2); err != nil || n != 1 || more {
		t.Fatalf("last page: %d %v %v", n, more, err)
	}
	if repo.orphanScanAfter["user-a"] != "" {
		t.Fatal("cycle must reset")
	}
	if n, more, err := repo.CleanOrphanChatBatch(userA, 2); err != nil || n != 1 || !more {
		t.Fatalf("wrap: %d %v %v", n, more, err)
	}
	// Cancelamento anterior à transação não avança o cursor.
	cursor := repo.orphanScanAfter["user-a"]
	ctx, cancel := context.WithCancel(userA)
	cancel()
	if n, _, err := repo.CleanOrphanChatBatch(ctx, 2); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %d %v", n, err)
	}
	if repo.orphanScanAfter["user-a"] != cursor {
		t.Fatal("cursor advanced on failure")
	}
	// Recriar o repositório reinicia a verificação com segurança.
	restarted := NewDBRepository(repo.db)
	if n, more, err := restarted.CleanOrphanChatBatch(userA, 2); err != nil || n != 0 || !more {
		t.Fatalf("restart: %d %v %v", n, more, err)
	}
}

// Registra o SQL efetivamente executado pelo GORM para validar cada etapa do lote.
type orphanPlanLogger struct {
	logger.Interface
	statements []string
}

func (l *orphanPlanLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	if strings.Contains(sql, "tool_invocations") && (strings.HasPrefix(sql, "SELECT") || strings.HasPrefix(sql, "UPDATE") || strings.HasPrefix(sql, "DELETE")) {
		l.statements = append(l.statements, sql)
	}
}

func TestOrphanScanActualBatchPlansAndRollback(t *testing.T) {
	repo, ctx, _ := setupRepositoryTest(t)
	for _, sql := range []string{
		`CREATE INDEX idx_tool_invocations_orphan_scan ON tool_invocations(user_id,origin_type,id)`,
		`CREATE INDEX idx_tool_invocations_user_id_id ON tool_invocations(user_id,id)`,
		`CREATE INDEX idx_tool_invocations_parent_user ON tool_invocations(parent_invocation_id,user_id) WHERE parent_invocation_id IS NOT NULL`,
	} {
		if err := repo.db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	toolID, err := repo.ResolveToolCatalogID(ctx, "echo")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 130; i++ {
		inv := database.ToolInvocation{UUIDModel: database.UUIDModel{ID: fmt.Sprintf("orphan-%03d", i)}, UserID: "user-a", ToolCatalogID: toolID, OriginType: OriginChat, OriginID: "missing", ToolCallID: fmt.Sprint(i), Status: StatusSucceeded, QueuedAt: time.Now()}
		if err := repo.db.Create(&inv).Error; err != nil {
			t.Fatal(err)
		}
	}
	parent := "orphan-000"
	for i := 0; i < 129; i++ {
		inv := database.ToolInvocation{UUIDModel: database.UUIDModel{ID: fmt.Sprintf("child-%03d", i)}, UserID: "user-a", ToolCatalogID: toolID, OriginType: OriginJobRun, OriginID: "run", ParentInvocationID: &parent, ToolCallID: fmt.Sprint(i), Status: StatusSucceeded, QueuedAt: time.Now()}
		if err := repo.db.Create(&inv).Error; err != nil {
			t.Fatal(err)
		}
	}
	log := &orphanPlanLogger{Interface: logger.Default}
	repo.db = repo.db.Session(&gorm.Session{Logger: log})
	if n, more, err := repo.CleanOrphanChatBatch(ctx, 128); err != nil || n != 0 || !more {
		t.Fatalf("detach: %d %v %v", n, more, err)
	}
	// A falha ocorre após DELETE, dentro da transação: tudo deve voltar.
	failure := errors.New("injected after delete")
	callback := "test:orphan-rollback"
	if err := repo.db.Callback().Delete().After("gorm:delete").Register(callback, func(tx *gorm.DB) { _ = tx.AddError(failure) }); err != nil {
		t.Fatal(err)
	}
	n, _, err := repo.CleanOrphanChatBatch(ctx, 128)
	if removeErr := repo.db.Callback().Delete().Remove(callback); removeErr != nil {
		t.Fatal(removeErr)
	}
	if n != 0 || !errors.Is(err, failure) {
		t.Fatalf("rollback: %d %v", n, err)
	}
	if repo.orphanScanAfter["user-a"] != "" {
		t.Fatal("cursor advanced after rollback")
	}
	var linked int64
	if err := repo.db.Model(&database.ToolInvocation{}).Where("parent_invocation_id = ?", parent).Count(&linked).Error; err != nil || linked != 1 {
		t.Fatalf("rollback lost remaining child link: %d %v", linked, err)
	}

	var count int64
	if err := repo.db.Model(&database.ToolInvocation{}).Where("id LIKE ?", "orphan-%").Count(&count).Error; err != nil || count != 130 {
		t.Fatalf("rollback lost parents: %d %v", count, err)
	}
	if n, more, err := repo.CleanOrphanChatBatch(ctx, 128); err != nil || n != 128 || !more {
		t.Fatalf("retry: %d %v %v", n, more, err)
	}
	statements := append([]string(nil), log.statements...)
	checked := 0
	for _, sql := range statements {
		if strings.Contains(sql, "count(*)") {
			continue
		}
		var plan []struct{ Detail string }
		if err := repo.db.Raw("EXPLAIN QUERY PLAN " + sql).Scan(&plan).Error; err != nil {
			t.Fatal(err)
		}
		for _, step := range plan {
			if strings.Contains(step.Detail, "tool_invocations") && (strings.Contains(step.Detail, "SCAN ") || strings.HasSuffix(step.Detail, "(user_id=?)")) {
				t.Fatalf("unbounded plan: %s; SQL=%s", step.Detail, sql)
			}
			if strings.Contains(step.Detail, "TEMP B-TREE") {
				t.Fatalf("full sort: %s", step.Detail)
			}
		}
		checked++
	}
	if checked < 8 {
		t.Fatalf("missing production statements: %d", checked)
	}
}
