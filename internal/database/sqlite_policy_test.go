package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openSQLitePolicyTestDB(t *testing.T, dsn string) (*gorm.DB, func()) {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	configureSQLitePool(sqlDB)
	if err := gdb.AutoMigrate(&User{}, &MemoryRecord{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if err := gdb.Create(&User{
		UUIDModel:    UUIDModel{ID: "user-1"},
		Username:     "user-1",
		PasswordHash: "test",
		Role:         UserRoleUser,
		IsActive:     true,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return gdb, func() { _ = sqlDB.Close() }
}

func TestSQLiteDSNAppliesBusyTimeout(t *testing.T) {
	path := t.TempDir() + "/policy.db"
	gdb, cleanup := openSQLitePolicyTestDB(t, sqliteDSN(path))
	defer cleanup()

	var busyTimeout int
	if err := gdb.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatalf("busy_timeout pragma: %v", err)
	}
	if busyTimeout != int(sqliteBusyTimeout.Milliseconds()) {
		t.Fatalf("busy_timeout = %d, want %d", busyTimeout, sqliteBusyTimeout.Milliseconds())
	}
}

func TestInitPathEnablesForeignKeysOnEveryApplicationConnection(t *testing.T) {
	previousDB, previousPath := db, dbPath
	var initializedDB *gorm.DB
	defer func() {
		if initializedDB != nil {
			if sqlDB, err := initializedDB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		db, dbPath = previousDB, previousPath
	}()

	path, err := filepath.Abs(filepath.Join(t.TempDir(), "application.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := InitPath(path); err != nil {
		initializedDB = db
		t.Fatalf("InitPath: %v", err)
	}
	initializedDB = db
	sqlDB, err := initializedDB.DB()
	if err != nil {
		t.Fatalf("pool SQL: %v", err)
	}

	connections := make([]*sql.Conn, 0, sqliteMaxOpenConns)
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()
	for index := 0; index < sqliteMaxOpenConns; index++ {
		connection, err := sqlDB.Conn(context.Background())
		if err != nil {
			t.Fatalf("conexão %d: %v", index, err)
		}
		connections = append(connections, connection)
		var foreignKeys int
		if err := connection.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
			t.Fatalf("ler foreign_keys na conexão %d: %v", index, err)
		}
		if foreignKeys != 1 {
			t.Fatalf("foreign_keys na conexão %d = %d, esperado 1", index, foreignKeys)
		}
	}

	now := time.Now().UTC()
	if _, err := connections[0].ExecContext(context.Background(), `
		INSERT INTO llm_models (id, provider_id, remote_id, display_name, created_at, updated_at)
		VALUES ('orphan-model', 'missing-provider', 'remote-model', '', ?, ?)
	`, now, now); err == nil {
		t.Fatal("InitPath aceitou modelo sem provedor pai")
	}
}

func TestSQLiteDSNHandlesReservedPathCharacters(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profile with spaces #hash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "policy.db")
	gdb, cleanup := openSQLitePolicyTestDB(t, sqliteDSN(path))
	defer cleanup()

	var busyTimeout int
	if err := gdb.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatalf("busy_timeout pragma: %v", err)
	}
	if busyTimeout != int(sqliteBusyTimeout.Milliseconds()) {
		t.Fatalf("busy_timeout = %d, want %d", busyTimeout, sqliteBusyTimeout.Milliseconds())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database was not created at escaped path %q: %v", path, err)
	}
}

func TestSQLiteDSNDoesNotEnableWALBeforeAutoVacuum(t *testing.T) {
	path := t.TempDir() + "/auto-vacuum.db"
	gdb, err := gorm.Open(sqlite.Open(sqliteDSN(path)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	if err := gdb.Exec("PRAGMA auto_vacuum=INCREMENTAL").Error; err != nil {
		t.Fatalf("set auto_vacuum: %v", err)
	}
	if err := gdb.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatalf("set wal: %v", err)
	}
	if err := gdb.AutoMigrate(&User{}, &MemoryRecord{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	var autoVacuumMode int
	if err := gdb.Raw("PRAGMA auto_vacuum").Scan(&autoVacuumMode).Error; err != nil {
		t.Fatalf("auto_vacuum pragma: %v", err)
	}
	if autoVacuumMode != 2 {
		t.Fatalf("auto_vacuum = %d, want incremental (2)", autoVacuumMode)
	}
}

func TestWithSQLiteBusyRetryWaitsForTransientWriterLock(t *testing.T) {
	path := t.TempDir() + "/retry.db"
	// Timeout baixo deixa o driver devolver SQLITE_BUSY rapidamente; o helper
	// central fica responsável pelo backoff curto.
	gdb, cleanup := openSQLitePolicyTestDB(t, "file:"+path+"?_pragma=busy_timeout(1)&_pragma=journal_mode(WAL)")
	defer cleanup()

	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("begin immediate: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, "ROLLBACK") }()

	released := make(chan struct{})
	go func() {
		time.Sleep(90 * time.Millisecond)
		_, _ = conn.ExecContext(ctx, "COMMIT")
		close(released)
	}()

	started := time.Now()
	err = WithSQLiteBusyRetry(ctx, "test.locked_write", func() error {
		return gdb.Create(&MemoryRecord{
			UserID:     "user-1",
			Content:    "retry after writer lock",
			LoadPolicy: MemoryLoadPolicyRetrievable,
			Kind:       MemoryKindHistoricalNote,
			Scope:      MemoryScopeUser,
		}).Error
	})
	if err != nil {
		t.Fatalf("retry locked write: %v", err)
	}
	<-released
	if elapsed := time.Since(started); elapsed < 50*time.Millisecond {
		t.Fatalf("write completed too quickly (%s), retry path was not exercised", elapsed)
	}
}

func TestWALReaderSucceedsDuringBackgroundWriter(t *testing.T) {
	path := t.TempDir() + "/wal-read.db"
	gdb, cleanup := openSQLitePolicyTestDB(t, sqliteDSN(path)+"&_pragma=journal_mode(WAL)")
	defer cleanup()

	if err := gdb.Create(&MemoryRecord{
		UserID:     "user-1",
		Content:    "committed",
		LoadPolicy: MemoryLoadPolicyRetrievable,
		Kind:       MemoryKindHistoricalNote,
		Scope:      MemoryScopeUser,
	}).Error; err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("begin immediate: %v", err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, "ROLLBACK") }()
	if _, err := conn.ExecContext(ctx, `INSERT INTO memory_records (id, user_id, content, load_policy, kind, scope, created_at, updated_at) VALUES ('pending', 'user-1', 'uncommitted', 'retrievable', 'historical_note', 'user', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert uncommitted: %v", err)
	}

	var count int64
	if err := WithSQLiteBusyRetry(ctx, "test.wal_reader", func() error {
		return gdb.Model(&MemoryRecord{}).Where("user_id = ?", "user-1").Count(&count).Error
	}); err != nil {
		t.Fatalf("read during writer: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want committed snapshot count 1", count)
	}
}

func TestImmediateTransactionRollsBackAfterContextCancellation(t *testing.T) {
	gdb, cleanup := openSQLitePolicyTestDB(t, sqliteDSN(t.TempDir()+"/cancel-rollback.db"))
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())

	err := WithSQLiteImmediateTransaction(ctx, gdb, "test.cancel_rollback", func(tx *gorm.DB) error {
		if err := tx.Create(&MemoryRecord{
			UserID:     "user-1",
			Content:    "não deve confirmar",
			LoadPolicy: MemoryLoadPolicyRetrievable,
			Kind:       MemoryKindHistoricalNote,
			Scope:      MemoryScopeUser,
		}).Error; err != nil {
			return err
		}
		cancel()
		return ctx.Err()
	})
	if err == nil {
		t.Fatal("esperava cancelamento da transação")
	}

	if err := WithSQLiteImmediateTransaction(context.Background(), gdb, "test.after_cancel", func(tx *gorm.DB) error {
		return tx.Create(&MemoryRecord{
			UserID:     "user-1",
			Content:    "writer posterior",
			LoadPolicy: MemoryLoadPolicyRetrievable,
			Kind:       MemoryKindHistoricalNote,
			Scope:      MemoryScopeUser,
		}).Error
	}); err != nil {
		t.Fatalf("writer lock permaneceu após rollback cancelado: %v", err)
	}
	var cancelledRows int64
	if err := gdb.Model(&MemoryRecord{}).Where("content = ?", "não deve confirmar").Count(&cancelledRows).Error; err != nil {
		t.Fatal(err)
	}
	if cancelledRows != 0 {
		t.Fatalf("rollback não removeu escrita cancelada: %d", cancelledRows)
	}
}

func TestImmediateTransactionOnceUsesSavepointInsideOuterTransaction(t *testing.T) {
	gdb, cleanup := openSQLitePolicyTestDB(t, sqliteDSN(t.TempDir()+"/savepoint-once.db"))
	defer cleanup()
	if err := gdb.Exec("CREATE TABLE once_savepoint_rows (value TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	wantErr := fmt.Errorf("callback failure")
	if err := gdb.Transaction(func(outer *gorm.DB) error {
		if err := WithSQLiteImmediateTransactionOnce(context.Background(), time.Time{}, outer, "test.savepoint_once", func(tx *gorm.DB) error {
			if err := tx.Exec("INSERT INTO once_savepoint_rows(value) VALUES (?)", "inner").Error; err != nil {
				return err
			}
			return wantErr
		}); !errors.Is(err, wantErr) {
			return fmt.Errorf("erro do savepoint: %w", err)
		}
		return outer.Exec("INSERT INTO once_savepoint_rows(value) VALUES (?)", "outer").Error
	}); err != nil {
		t.Fatalf("transação externa: %v", err)
	}
	var values []string
	if err := gdb.Raw("SELECT value FROM once_savepoint_rows ORDER BY value").Scan(&values).Error; err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0] != "outer" {
		t.Fatalf("savepoint não preservou apenas a escrita externa: %v", values)
	}
}

func TestImmediateTransactionOncePoolWaitHonorsAcquisitionDeadline(t *testing.T) {
	gdb, cleanup := openSQLitePolicyTestDB(t, sqliteDSN(t.TempDir()+"/once-pool.db"))
	defer cleanup()
	root, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	root.SetMaxOpenConns(1)
	conn, err := root.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	called := false
	err = WithSQLiteImmediateTransactionOnce(context.Background(), time.Now().Add(30*time.Millisecond), gdb, "test.pool_wait", func(*gorm.DB) error { called = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) || called {
		t.Fatalf("aquisição bloqueada iniciou callback: called=%v err=%v", called, err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := WithSQLiteImmediateTransactionOnce(context.Background(), time.Now().Add(time.Second), gdb, "test.pool_recovered", func(*gorm.DB) error { called = true; return nil }); err != nil || !called {
		t.Fatalf("pool não recuperou: called=%v err=%v", called, err)
	}
}

func TestImmediateTransactionOnceCallbackSavepointUsesPinnedConnectionAndRoot(t *testing.T) {
	gdb, cleanup := openSQLitePolicyTestDB(t, sqliteDSN(t.TempDir()+"/once-wrapper-savepoint.db"))
	defer cleanup()
	if err := gdb.Exec("CREATE TABLE once_wrapper_rows (value TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	root, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("rollback nested savepoint")
	callbackCalls := 0
	err = WithSQLiteImmediateTransactionOnce(context.Background(), time.Now().Add(time.Second), gdb, "test.once_wrapper_savepoint", func(tx *gorm.DB) error {
		callbackCalls++
		gotRoot, err := tx.DB()
		if err != nil {
			return err
		}
		if gotRoot != root {
			return errors.New("GetDBConn não retornou o pool raiz")
		}
		if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
			return errors.New("conexão BEGIN IMMEDIATE não foi reconhecida como transação")
		}
		if err := tx.Transaction(func(nested *gorm.DB) error {
			if err := nested.Exec("INSERT INTO once_wrapper_rows(value) VALUES (?)", "rolled-back").Error; err != nil {
				return err
			}
			return wantErr
		}); !errors.Is(err, wantErr) {
			return fmt.Errorf("nested rollback retornou %v", err)
		}
		return tx.Transaction(func(nested *gorm.DB) error {
			return nested.Exec("INSERT INTO once_wrapper_rows(value) VALUES (?)", "committed").Error
		})
	})
	if err != nil {
		t.Fatalf("transação wrapper: %v", err)
	}
	if callbackCalls != 1 {
		t.Fatalf("callback chamado %d vezes", callbackCalls)
	}
	var values []string
	if err := gdb.Raw("SELECT value FROM once_wrapper_rows ORDER BY value").Scan(&values).Error; err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0] != "committed" {
		t.Fatalf("nested savepoint não preservou commit/rollback esperado: %v", values)
	}
}
