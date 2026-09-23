package database

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openCommandMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "command-migration.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter conexão: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func commandMigrationStampCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = 21 AND name = ?", "command_storage_initial").Scan(&count).Error; err != nil {
		t.Fatalf("ler v21: %v", err)
	}
	return count
}

func TestApplyCommandStorageMigrationStampsV21AfterCallback(t *testing.T) {
	db := openCommandMigrationTestDB(t)
	calls := 0
	err := ApplyCommandStorageMigration(context.Background(), db, func(tx *gorm.DB) error {
		calls++
		return tx.Exec("CREATE TABLE command_callback_effect (value TEXT NOT NULL); INSERT INTO command_callback_effect (value) VALUES ('applied')").Error
	})
	if err != nil {
		t.Fatalf("aplicar v21: %v", err)
	}
	if calls != 1 {
		t.Fatalf("callback chamado %d vezes, esperado 1", calls)
	}
	if got := commandMigrationStampCount(t, db); got != 1 {
		t.Fatalf("v21 não registrada após callback: %d", got)
	}
	var value string
	if err := db.Raw("SELECT value FROM command_callback_effect").Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	if value != "applied" {
		t.Fatalf("efeito do callback não persistido: %q", value)
	}
}

func TestApplyCommandStorageMigrationCallbackErrorRollsBackAndDoesNotStamp(t *testing.T) {
	db := openCommandMigrationTestDB(t)
	wantErr := errors.New("falha no bootstrap composto")
	err := ApplyCommandStorageMigration(context.Background(), db, func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE TABLE command_callback_effect (value TEXT NOT NULL)").Error; err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("erro do callback não propagado: %v", err)
	}
	var tables int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('schema_migrations', 'command_callback_effect')").Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatalf("rollback deixou tabelas da migração: %d", tables)
	}
}

func TestApplyCommandStorageMigrationIsNoopAfterExplicitCompletion(t *testing.T) {
	db := openCommandMigrationTestDB(t)
	calls := 0
	apply := func(*gorm.DB) error {
		calls++
		return nil
	}
	if err := ApplyCommandStorageMigration(context.Background(), db, apply); err != nil {
		t.Fatalf("primeira aplicação: %v", err)
	}
	if err := ApplyCommandStorageMigration(context.Background(), db, apply); err != nil {
		t.Fatalf("segunda aplicação: %v", err)
	}
	if calls != 1 {
		t.Fatalf("v21 reaplicada: callback chamado %d vezes", calls)
	}
	if got := commandMigrationStampCount(t, db); got != 1 {
		t.Fatalf("quantidade de carimbos v21 inesperada: %d", got)
	}
}

func TestApplyCommandStorageMigrationRejectsConflictingV21Name(t *testing.T) {
	for _, conflictingName := range []string{"other_storage", ""} {
		t.Run("nome="+conflictingName, func(t *testing.T) {
			db := openCommandMigrationTestDB(t)
			if err := ensureSchemaMigrationsTable(db); err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("INSERT INTO schema_migrations (version, name, applied_at) VALUES (21, ?, CURRENT_TIMESTAMP)", conflictingName).Error; err != nil {
				t.Fatal(err)
			}
			calls := 0
			err := ApplyCommandStorageMigration(context.Background(), db, func(*gorm.DB) error {
				calls++
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), "incompatível") {
				t.Fatalf("colisão de nome v21 não rejeitada: %v", err)
			}
			if calls != 0 {
				t.Fatal("callback executado apesar da colisão de nome")
			}
			var name string
			if err := db.Raw("SELECT name FROM schema_migrations WHERE version = 21").Scan(&name).Error; err != nil {
				t.Fatal(err)
			}
			if name != conflictingName {
				t.Fatalf("colisão alterou o carimbo existente: %q", name)
			}
		})
	}
}

func TestApplyCommandStorageMigrationHonorsCanceledContext(t *testing.T) {
	db := openCommandMigrationTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	if err := ApplyCommandStorageMigration(ctx, db, func(*gorm.DB) error {
		calls++
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado retornou %v", err)
	}
	if calls != 0 {
		t.Fatal("callback executado com contexto cancelado")
	}
	var tables int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'").Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("contexto cancelado criou tabela de migrações")
	}
}

func TestApplyCommandInstanceMigrationStampsV29AfterCallback(t *testing.T) {
	db := openCommandMigrationTestDB(t)
	calls := 0
	err := ApplyCommandInstanceMigration(context.Background(), db, func(tx *gorm.DB) error {
		calls++
		return tx.Exec("CREATE TABLE command_process_generations (startup_id TEXT PRIMARY KEY, file_identity TEXT NOT NULL, created_at DATETIME NOT NULL)").Error
	})
	if err != nil {
		t.Fatalf("aplicar v29: %v", err)
	}
	if calls != 1 {
		t.Fatalf("callback v29 chamado %d vezes, esperado 1", calls)
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = 29 AND name = ?", "command_process_generations").Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("v29 não registrada: %d", count)
	}
}
