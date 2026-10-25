package toolinvocations

import (
	"context"
	"path/filepath"
	"testing"

	"assistente/internal/tools"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestServiceLedgerContentionDoesNotExecuteAndRecoversOnce(t *testing.T) {
	fixture, user, _ := setupRepositoryTest(t)
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "ledger.db"))
	if err := fixture.db.Exec("VACUUM INTO ?", path).Error; err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(4)
	writer, err := pool.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer writer.ExecContext(context.Background(), "ROLLBACK")
	calls := 0
	registry := tools.NewRegistry()
	registry.MustRegister(countingTool{calls: &calls})
	repo := NewDBRepository(db)
	svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	req := ExecuteRequest{
		Call:   tools.ToolCall{ID: "blocked-call", Function: tools.FunctionCall{Name: "echo", Arguments: `{}`}},
		Origin: Origin{Type: OriginChat, ID: "turn-1"},
	}
	blocked := svc.Execute(user, req)
	if calls != 0 || blocked.Persisted || blocked.Execution.ErrorKind != tools.ErrorKindUnavailable || blocked.Execution.ErrorCode != "invocation_ledger_unavailable" {
		t.Fatalf("contenção deve impedir efeitos sem simular cancelamento: calls=%d result=%+v", calls, blocked)
	}
	if _, err := writer.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	// Nova tentativa explícita, após liberar o writer: uma execução e um registro.
	result := svc.Execute(user, req)
	if calls != 1 || !result.Persisted || result.Execution.Result.IsError {
		t.Fatalf("recuperação: calls=%d result=%+v", calls, result)
	}
	inv, err := repo.Get(user, result.Invocation.ID)
	if err != nil || inv.Status != StatusSucceeded || inv.Attempt != 1 || len(inv.Output) == 0 {
		t.Fatalf("registro após recuperação: %+v err=%v", inv, err)
	}
	var count int64
	if err := db.Table("tool_invocations").Where("tool_call_id = ?", req.Call.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("registros=%d err=%v", count, err)
	}
}
