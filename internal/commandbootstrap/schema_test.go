package commandbootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func openBootstrapTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir banco de teste: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter conexão do banco de teste: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func bootstrapUUID7(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("gerar UUIDv7: %v", err)
	}
	return id.String()
}

func seedBootstrapLayer(t *testing.T, db *gorm.DB) string {
	t.Helper()
	id := bootstrapUUID7(t)
	userID := bootstrapUUID7(t)
	if err := db.Table("command_layers").Create(map[string]any{
		"id": id, "user_id": userID, "name": "fixture", "description": "dados I01",
		"enabled": true, "source": "test", "resolution_priority": 1,
		"created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z",
	}).Error; err != nil {
		t.Fatalf("semear layer: %v", err)
	}
	return id
}

func assertCommandStamp(t *testing.T, db *gorm.DB, want bool) {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = 21 AND name = ?", "command_storage_initial").Scan(&count).Error; err != nil {
		t.Fatalf("ler carimbo v20: %v", err)
	}
	if (count == 1) != want {
		t.Fatalf("carimbo v20 = %v, esperado %v", count == 1, want)
	}
}

func assertCommandTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{
		"command_layers", "command_bindings", "command_config_generations",
		"command_config_mutations", "command_decision_receipts", "command_decision_receipt_events",
		"command_idempotency_keys", "command_invocations", "command_key_versions",
	} {
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count).Error; err != nil {
			t.Fatalf("verificar tabela %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("tabela %s ausente", table)
		}
	}
}

func TestMigrateFreshReopenPreservesCommandStorageAndStamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.db")
	db := openBootstrapTestDB(t, path)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migração fresh: %v", err)
	}
	assertCommandTables(t, db)
	assertCommandStamp(t, db, true)
	layerID := seedBootstrapLayer(t, db)

	var keyCount int64
	if err := db.Exec("INSERT INTO command_key_versions (id, version, digest, active) VALUES (?, ?, ?, ?)", bootstrapUUID7(t), "fixture-v1", strings.Repeat("a", 64), 1).Error; err != nil {
		t.Fatalf("semear chave: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("fechar primeiro handle: %v", err)
	}

	reopened := openBootstrapTestDB(t, path)
	if err := Migrate(context.Background(), reopened); err != nil {
		t.Fatalf("migração no reopen: %v", err)
	}
	assertCommandTables(t, reopened)
	assertCommandStamp(t, reopened, true)
	if err := reopened.Raw("SELECT COUNT(*) FROM command_key_versions WHERE version = ?", "fixture-v1").Scan(&keyCount).Error; err != nil {
		t.Fatal(err)
	}
	if keyCount != 1 {
		t.Fatalf("chave persistida desapareceu no reopen: %d", keyCount)
	}
	var layerCount int64
	if err := reopened.Raw("SELECT COUNT(*) FROM command_layers WHERE id = ?", layerID).Scan(&layerCount).Error; err != nil {
		t.Fatal(err)
	}
	if layerCount != 1 {
		t.Fatal("dados de configuração desapareceram no reopen")
	}
}

func TestMigrateAdoptsTempfileLegacySchemaBeforeI01(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db := openBootstrapTestDB(t, path)
	ctx := context.Background()
	if err := commandconfig.Migrate(ctx, db); err != nil {
		t.Fatalf("schema legado de config: %v", err)
	}
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatalf("schema legado de decisão: %v", err)
	}
	if err := commandledger.Migrate(ctx, db); err != nil {
		t.Fatalf("schema legado de ledger: %v", err)
	}
	layerID := seedBootstrapLayer(t, db)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("fechar schema legado: %v", err)
	}

	reopened := openBootstrapTestDB(t, path)
	if err := Migrate(ctx, reopened); err != nil {
		t.Fatalf("adotar schema legado pré-I01: %v", err)
	}
	assertCommandTables(t, reopened)
	assertCommandStamp(t, reopened, true)
	var count int64
	if err := reopened.Raw("SELECT COUNT(*) FROM command_layers WHERE id = ?", layerID).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("adoção do schema legado alterou dados existentes")
	}
}

func TestMigrateRejectsUnknownCommandObjectWithoutChangingData(t *testing.T) {
	db := openBootstrapTestDB(t, filepath.Join(t.TempDir(), "unknown.db"))
	if err := db.Exec("CREATE TABLE command_future (id INTEGER PRIMARY KEY, value TEXT NOT NULL); INSERT INTO command_future (id, value) VALUES (1, 'preserve')").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); !errors.Is(err, ErrStorage) {
		t.Fatalf("objeto desconhecido aceito: %v", err)
	}
	assertCommandStampAbsent(t, db)
	var value string
	if err := db.Raw("SELECT value FROM command_future WHERE id = 1").Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	if value != "preserve" {
		t.Fatalf("dado do objeto desconhecido alterado: %q", value)
	}
}

func TestMigrateRejectsNamespaceCollisionWithoutChangingData(t *testing.T) {
	db := openBootstrapTestDB(t, filepath.Join(t.TempDir(), "collision.db"))
	if err := db.Exec("CREATE TABLE collision_source (value TEXT NOT NULL); INSERT INTO collision_source (value) VALUES ('preserve'); CREATE VIEW command_layers AS SELECT value FROM collision_source").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); !errors.Is(err, ErrStorage) {
		t.Fatalf("colisão de namespace aceita: %v", err)
	}
	assertCommandStampAbsent(t, db)
	var objectType string
	if err := db.Raw("SELECT type FROM sqlite_master WHERE name = 'command_layers'").Scan(&objectType).Error; err != nil {
		t.Fatal(err)
	}
	if objectType != "view" {
		t.Fatalf("objeto colidente foi alterado: %q", objectType)
	}
	var value string
	if err := db.Raw("SELECT value FROM command_layers").Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	if value != "preserve" {
		t.Fatalf("dado sob namespace colidente alterado: %q", value)
	}
}

func TestMigrateRejectsIncompatibleIndexWithoutChangingData(t *testing.T) {
	db := openBootstrapTestDB(t, filepath.Join(t.TempDir(), "drift.db"))
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("schema inicial: %v", err)
	}
	layerID := seedBootstrapLayer(t, db)
	if err := db.Exec("DROP INDEX ux_command_layers_user_global_name; CREATE INDEX ux_command_layers_user_global_name ON command_layers (name)").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); !errors.Is(err, ErrStorage) {
		t.Fatalf("índice incompatível aceito: %v", err)
	}
	assertCommandStamp(t, db, true)
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM command_layers WHERE id = ?", layerID).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("drift de índice alterou dados")
	}
}

func TestMigrateCanceledContextDoesNotCreateStorage(t *testing.T) {
	db := openBootstrapTestDB(t, filepath.Join(t.TempDir(), "canceled.db"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Migrate(ctx, db); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado retornou %v", err)
	}
	assertCommandStampAbsent(t, db)
}

func TestMigrateConcurrentConnectionsLeavesOneCompleteSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	first := openBootstrapTestDB(t, path)
	second := openBootstrapTestDB(t, path)
	start := make(chan struct{})
	results := make(chan error, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, db := range []*gorm.DB{first, second} {
		go func(db *gorm.DB) {
			<-start
			results <- Migrate(ctx, db)
		}(db)
	}
	close(start)

	successes := 0
	for i := 0; i < 2; i++ {
		var err error
		select {
		case err = <-results:
		case <-ctx.Done():
			t.Fatalf("migração concorrente não terminou no prazo: %v", ctx.Err())
		}
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrStorage) {
			t.Fatalf("erro inesperado na concorrência: %v", err)
		}
	}
	if successes == 0 {
		t.Fatal("nenhuma conexão concluiu a migração concorrente")
	}

	check := openBootstrapTestDB(t, path)
	if err := Migrate(context.Background(), check); err != nil {
		t.Fatalf("reparo/verificação após concorrência: %v", err)
	}
	assertCommandTables(t, check)
	assertCommandStamp(t, check, true)
}

func assertCommandStampAbsent(t *testing.T, db *gorm.DB) {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'").Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		var stamps int64
		if err := db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = 21").Scan(&stamps).Error; err != nil {
			t.Fatal(err)
		}
		if stamps != 0 {
			t.Fatalf("falha deixou carimbo v20: %d", stamps)
		}
	}
}
