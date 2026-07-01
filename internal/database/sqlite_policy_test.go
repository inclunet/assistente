package database

import (
	"context"
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
