package database

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupTaskNoteBusyRetryDB abre um banco em arquivo com WAL e busy_timeout
// nativo curto (1ms), permitindo segurar um writer lock numa conexão paralela
// e observar o SQLITE_BUSY sendo retentado pelo caminho de upsert de nota.
func setupTaskNoteBusyRetryDB(t *testing.T) *gorm.DB {
	t.Helper()
	previous := DB()
	path := t.TempDir() + "/task-note-busy.db"
	testDB, err := gorm.Open(sqlite.Open("file:"+path+"?_pragma=busy_timeout(1)&_pragma=journal_mode(WAL)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	// >1 conexão: uma segura o writer lock, outra roda o upsert.
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(2)
	if err := testDB.AutoMigrate(&TaskList{}, &Task{}, &TaskNote{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	SetDB(testDB)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		SetDB(previous)
	})
	return testDB
}

// TestUpsertTaskNoteByExternal_RetentaSobWriterLock garante que a criação de
// uma nota externa nova sobrevive a um lock de escrita transitório (SQLITE_BUSY)
// em vez de falhar de imediato — o que, no fan-out de sync de tickets, virava
// "job attempt failed" no assistente.log.
func TestUpsertTaskNoteByExternal_RetentaSobWriterLock(t *testing.T) {
	testDB := setupTaskNoteBusyRetryDB(t)
	const userID = "user-busy"
	if err := testDB.Create(&TaskList{UUIDModel: UUIDModel{ID: "list-busy"}, UserID: userID, Title: "list"}).Error; err != nil {
		t.Fatalf("seed list: %v", err)
	}
	if err := testDB.Create(&Task{UUIDModel: UUIDModel{ID: "task-busy"}, TaskListID: "list-busy", Title: "task", StatusID: 1}).Error; err != nil {
		t.Fatalf("seed task: %v", err)
	}

	// Segura o writer lock numa conexão dedicada.
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	lockConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("lock conn: %v", err)
	}
	defer func() { _ = lockConn.Close() }()
	if _, err := lockConn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("begin writer lock: %v", err)
	}
	defer func() { _, _ = lockConn.ExecContext(context.Background(), "ROLLBACK") }()

	busyObserved := make(chan struct{})
	var busyOnce sync.Once
	var busyAttempts atomic.Int32
	if err := testDB.Callback().Create().After("gorm:create").Register("test:observe_busy_create", func(tx *gorm.DB) {
		if IsSQLiteBusyError(tx.Error) {
			busyAttempts.Add(1)
			busyOnce.Do(func() { close(busyObserved) })
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}

	ctx := WithUserID(context.Background(), userID)
	noteType := TaskNoteCustomer
	result := make(chan error, 1)
	go func() {
		_, _, upErr := UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
			TaskID:         "task-busy",
			Type:           &noteType,
			Content:        "conteúdo",
			ExternalSource: "jira",
			ExternalID:     "EXT-1",
		})
		result <- upErr
	}()

	select {
	case <-busyObserved:
	case <-time.After(3 * time.Second):
		t.Fatal("upsert não encontrou o writer lock (retry não exercitado)")
	}

	if _, err := lockConn.ExecContext(context.Background(), "COMMIT"); err != nil {
		t.Fatalf("release writer lock: %v", err)
	}

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("upsert após lock transitório: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("upsert não concluiu após liberar o writer lock")
	}

	if busyAttempts.Load() == 0 {
		t.Fatal("retry não foi exercitado (nenhum SQLITE_BUSY observado)")
	}

	var persisted TaskNote
	if err := testDB.Where("external_source = ? AND external_id = ?", "jira", "EXT-1").First(&persisted).Error; err != nil {
		t.Fatalf("nota não persistida: %v", err)
	}
	if persisted.TaskID != "task-busy" {
		t.Fatalf("task_id = %q, want task-busy", persisted.TaskID)
	}
}
