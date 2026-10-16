package commandconfig

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func ensureScopeContentionDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsnFor := func(path string) string {
		return "file:" + filepath.ToSlash(path) + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(25)"
	}

	path := filepath.Join(t.TempDir(), "ensure-scope.db")
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(dsnFor(path)), &gorm.Config{})
		if err != nil {
			t.Fatalf("abrir fixture SQLite: %v", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("obter pool SQLite: %v", err)
		}
		sqlDB.SetMaxOpenConns(4)
		sqlDB.SetMaxIdleConns(2)
		t.Cleanup(func() { _ = sqlDB.Close() })
		return db
	}
	storeDB, writerDB := open(), open()
	if err := Migrate(context.Background(), storeDB); err != nil {
		t.Fatalf("migrar fixture SQLite: %v", err)
	}
	return storeDB, writerDB
}

func lockEnsureScopeFixture(t *testing.T, db *gorm.DB) *sql.Conn {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter pool do writer: %v", err)
	}
	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("reservar conexão do writer: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("adquirir writer SQLite: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.ExecContext(context.Background(), "ROLLBACK") })
	return conn
}

func TestEnsureScopeExistingGlobalAndWorkspaceAvoidWriterLock(t *testing.T) {
	db, writer := ensureScopeContentionDB(t)
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	global := Scope{UserID: storeTestUUID7(t)}
	workspaceID := "workspace-contention"
	workspace := Scope{UserID: global.UserID, WorkspaceID: &workspaceID}
	if err := store.EnsureScope(context.Background(), workspace); err != nil {
		t.Fatalf("preparar global+workspace: %v", err)
	}
	lockEnsureScopeFixture(t, writer)

	for name, scope := range map[string]Scope{"global": global, "workspace": workspace} {
		if err := store.EnsureScope(context.Background(), scope); err != nil {
			t.Errorf("EnsureScope(%s) sob writer concorrente: %v", name, err)
		}
	}
}

func TestEnsureScopeMissingStillRequiresWriteUnderWriterLock(t *testing.T) {
	db, writer := ensureScopeContentionDB(t)
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	lockEnsureScopeFixture(t, writer)

	err = store.EnsureScope(context.Background(), Scope{UserID: storeTestUUID7(t)})
	if err == nil {
		t.Fatal("escopo ausente foi aceito sem conseguir gravar sob writer lock")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "busy") && !strings.Contains(strings.ToLower(err.Error()), "locked") {
		t.Fatalf("erro do escopo ausente sob writer lock = %v; esperado contenção SQLite", err)
	}
}

func TestEnsureScopeDoesNotAcceptCorruptedGenerations(t *testing.T) {
	db, _ := ensureScopeContentionDB(t)
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{UserID: storeTestUUID7(t)}
	if err := store.EnsureScope(context.Background(), scope); err != nil {
		t.Fatalf("preparar escopo global: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(context.Background(), "PRAGMA ignore_check_constraints = ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), "UPDATE command_config_generations SET id = 'corrupt' WHERE user_id = ?", scope.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), "PRAGMA ignore_check_constraints = OFF"); err != nil {
		t.Fatal(err)
	}
	err = store.EnsureScope(context.Background(), scope)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("EnsureScope aceitou/mascarou geração corrompida: %v", err)
	}
}
