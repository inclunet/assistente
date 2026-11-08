package database

import (
	"context"
	"path/filepath"
	"testing"

	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func TestCompactRestoresConnectionBusyTimeout(t *testing.T) {
	for _, mode := range []string{"full", "incremental", "skipped", "readonly-error"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "pool-policy.db")
			dsn := SQLiteDSN(path)
			gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			pool, err := gdb.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pool.Close() })
			pool.SetMaxOpenConns(1)
			pool.SetMaxIdleConns(1)
			if mode == "incremental" {
				if err := gdb.Exec("PRAGMA auto_vacuum=INCREMENTAL").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := gdb.Exec("CREATE TABLE evidence (value TEXT)").Error; err != nil {
				t.Fatal(err)
			}
			if err := gdb.Exec("INSERT INTO evidence VALUES ('preserved')").Error; err != nil {
				t.Fatal(err)
			}
			if mode == "readonly-error" {
				if err := pool.Close(); err != nil {
					t.Fatal(err)
				}
				gdb, err = gorm.Open(sqlite.Open(dsn+"&mode=ro"), &gorm.Config{})
				if err != nil {
					t.Fatal(err)
				}
				pool, err = gdb.DB()
				if err != nil {
					t.Fatal(err)
				}
				pool.SetMaxOpenConns(1)
				pool.SetMaxIdleConns(1)
			}
			previousDB, previousPath := db, dbPath
			db, dbPath = gdb, path
			t.Cleanup(func() { db, dbPath = previousDB, previousPath })
			// Valor não padrão prova restauração do estado da conexão, sem hardcode.
			const originalTimeout = 37
			if err := gdb.Exec("PRAGMA busy_timeout=37").Error; err != nil {
				t.Fatal(err)
			}
			force := mode == "full" || mode == "readonly-error"
			minFree := int64(0)
			if mode == "skipped" {
				minFree = 1 << 30
			}
			result, err := Compact(ctx, force, minFree)
			if mode == "readonly-error" {
				if err == nil {
					t.Fatal("read-only VACUUM should fail")
				}
			} else if err != nil || result.Mode != mode {
				t.Fatalf("compact mode=%s err=%v", result.Mode, err)
			}
			var timeout int
			if err := gdb.Raw("PRAGMA busy_timeout").Scan(&timeout).Error; err != nil {
				t.Fatal(err)
			}
			if timeout != originalTimeout {
				t.Fatalf("pooled busy_timeout=%d want %d", timeout, originalTimeout)
			}
			var value string
			if err := gdb.Raw("SELECT value FROM evidence").Scan(&value).Error; err != nil || value != "preserved" {
				t.Fatalf("data=%q err=%v", value, err)
			}
		})
	}
}
