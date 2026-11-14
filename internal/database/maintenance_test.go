package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

type maintenanceBlob struct {
	ID   uint `gorm:"primaryKey"`
	Data string
}

// setupMaintenanceDB cria um banco SQLite em arquivo com muitas linhas grandes
// já deletadas (gerando freelist) e o instala como o banco do pacote. Retorna o
// caminho. incremental define se o banco nasce em auto_vacuum=INCREMENTAL.
func setupMaintenanceDB(t *testing.T, incremental bool) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	gdb, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, sErr := gdb.DB()
	if sErr != nil {
		t.Fatalf("sql db: %v", sErr)
	}
	// Uma única conexão garante que o PRAGMA auto_vacuum esteja em vigor na
	// conexão que cria a primeira tabela (quando o modo é fixado no arquivo).
	sqlDB.SetMaxOpenConns(1)
	// auto_vacuum deve ser definido ANTES de qualquer tabela existir (e antes do
	// journal_mode=WAL, para não escrever páginas que fixem o modo none).
	if incremental {
		gdb.Exec("PRAGMA auto_vacuum=INCREMENTAL")
	}
	gdb.Exec("PRAGMA journal_mode=WAL")
	if err := gdb.AutoMigrate(&maintenanceBlob{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	big := strings.Repeat("x", 4096)
	for i := 0; i < 3000; i++ {
		if err := gdb.Create(&maintenanceBlob{Data: big}).Error; err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	// Deleta tudo: páginas vão para a freelist mas o arquivo não encolhe.
	if err := gdb.Exec("DELETE FROM maintenance_blobs").Error; err != nil {
		t.Fatalf("delete: %v", err)
	}

	prevDB, prevPath := db, dbPath
	db = gdb
	dbPath = path
	t.Cleanup(func() {
		// Fecha a conexão para liberar o handle do arquivo (Windows) antes do
		// RemoveAll do TempDir.
		_ = sqlDB.Close()
		db = prevDB
		dbPath = prevPath
	})
	return path
}

func TestCompactFullVacuumReclaimsAndConvertsLegacyDB(t *testing.T) {
	setupMaintenanceDB(t, false)
	ctx := context.Background()

	before, err := DatabaseStatsSnapshot(ctx)
	if err != nil {
		t.Fatalf("stats antes: %v", err)
	}
	if before.AutoVacuumMode != "none" {
		t.Fatalf("banco legado deveria estar em auto_vacuum=none, got %q", before.AutoVacuumMode)
	}
	if before.FreeBytes <= 0 {
		t.Fatalf("esperava freelist > 0 após DELETE, got %d", before.FreeBytes)
	}

	res, err := Compact(ctx, true, 0)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if res.Mode != "full" {
		t.Fatalf("mode = %q, want full", res.Mode)
	}

	after, err := DatabaseStatsSnapshot(ctx)
	if err != nil {
		t.Fatalf("stats depois: %v", err)
	}
	if after.AutoVacuumMode != "incremental" {
		t.Fatalf("VACUUM completo deveria converter para incremental, got %q", after.AutoVacuumMode)
	}
	if after.FreeBytes >= before.FreeBytes {
		t.Fatalf("freelist não reduziu: antes=%d depois=%d", before.FreeBytes, after.FreeBytes)
	}
	// Em WAL o tamanho do arquivo principal pode oscilar com o checkpoint; o que
	// importa é o tamanho total (principal + -wal) ter encolhido.
	if after.TotalSizeBytes >= before.TotalSizeBytes {
		t.Fatalf("banco não encolheu: antes=%d depois=%d", before.TotalSizeBytes, after.TotalSizeBytes)
	}
	if res.ReclaimedBytes <= 0 {
		t.Fatalf("ReclaimedBytes = %d, want > 0", res.ReclaimedBytes)
	}
}

func TestCompactSkipsFullVacuumBelowThreshold(t *testing.T) {
	setupMaintenanceDB(t, false)
	ctx := context.Background()
	// Limiar gigante: nenhuma freelist plausível dispara o VACUUM completo.
	res, err := Compact(ctx, false, 1000000000000)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if res.Mode != "skipped" {
		t.Fatalf("mode = %q, want skipped (abaixo do limiar)", res.Mode)
	}
	if !res.WALCheckpointed {
		t.Fatalf("checkpoint do WAL deveria rodar mesmo quando o VACUUM é pulado")
	}
}

func TestCompactIncrementalMode(t *testing.T) {
	setupMaintenanceDB(t, true)
	ctx := context.Background()

	before, err := DatabaseStatsSnapshot(ctx)
	if err != nil {
		t.Fatalf("stats antes: %v", err)
	}
	if before.AutoVacuumMode != "incremental" {
		t.Fatalf("banco deveria nascer incremental, got %q", before.AutoVacuumMode)
	}

	res, err := Compact(ctx, false, 0)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if res.Mode != "incremental" {
		t.Fatalf("mode = %q, want incremental", res.Mode)
	}

	after, err := DatabaseStatsSnapshot(ctx)
	if err != nil {
		t.Fatalf("stats depois: %v", err)
	}
	if after.FreeBytes >= before.FreeBytes {
		t.Fatalf("incremental_vacuum não reduziu a freelist: antes=%d depois=%d", before.FreeBytes, after.FreeBytes)
	}
}

func TestCompactForceRunsFullVacuumForIncrementalDB(t *testing.T) {
	setupMaintenanceDB(t, true)
	ctx := context.Background()

	before, err := DatabaseStatsSnapshot(ctx)
	if err != nil {
		t.Fatalf("stats antes: %v", err)
	}
	if before.AutoVacuumMode != "incremental" {
		t.Fatalf("banco deveria nascer incremental, got %q", before.AutoVacuumMode)
	}
	if before.FreeBytes <= 0 {
		t.Fatalf("esperava freelist > 0 após DELETE, got %d", before.FreeBytes)
	}

	res, err := Compact(ctx, true, 0)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if res.Mode != "full" {
		t.Fatalf("mode = %q, want full", res.Mode)
	}

	after, err := DatabaseStatsSnapshot(ctx)
	if err != nil {
		t.Fatalf("stats depois: %v", err)
	}
	if after.AutoVacuumMode != "incremental" {
		t.Fatalf("VACUUM forçado deve preservar incremental, got %q", after.AutoVacuumMode)
	}
	if after.FreeBytes >= before.FreeBytes {
		t.Fatalf("VACUUM forçado não reduziu a freelist: antes=%d depois=%d", before.FreeBytes, after.FreeBytes)
	}
	if after.TotalSizeBytes >= before.TotalSizeBytes {
		t.Fatalf("VACUUM forçado não encolheu o banco: antes=%d depois=%d", before.TotalSizeBytes, after.TotalSizeBytes)
	}
	if res.ReclaimedBytes <= 0 {
		t.Fatalf("ReclaimedBytes = %d, want > 0", res.ReclaimedBytes)
	}
}

// Usa SQLite real: cada passagem libera o writer e devolve apenas um lote.
func TestCompactIncrementalBatchesAllowWritesAndFinish(t *testing.T) {
	path := setupMaintenanceDB(t, true)
	ctx := context.Background()
	if err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("PRAGMA wal_autocheckpoint=0").Error; err != nil {
		t.Fatal(err)
	}
	before, err := DatabaseStatsSnapshot(ctx)
	if err != nil || before.FreelistCount <= incrementalVacuumBatchPages {
		t.Fatalf("freelist inicial=%d err=%v", before.FreelistCount, err)
	}
	first, err := Compact(ctx, false, 0)
	if err != nil || !first.HasMore() {
		t.Fatalf("primeiro lote=%+v more=%v err=%v", first, first.HasMore(), err)
	}
	after, err := DatabaseStatsSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reclaimed := before.FreelistCount - after.FreelistCount
	if reclaimed <= 0 || reclaimed > incrementalVacuumBatchPages {
		t.Fatalf("lote removeu %d páginas", reclaimed)
	}
	if after.FileSizeBytes >= before.FileSizeBytes {
		t.Fatalf("checkpoint após lote não reduziu arquivo: antes=%d depois=%d", before.FileSizeBytes, after.FileSizeBytes)
	}
	// Uma gravação real usa outra conexão entre passagens, como o chat.
	writer, err := sql.Open("sqlite", SQLiteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	if _, err := writer.ExecContext(ctx, "INSERT INTO maintenance_blobs(data) VALUES (?)", "chat"); err != nil {
		t.Fatalf("gravação entre lotes: %v", err)
	}
	more := first.HasMore()
	for i := 0; more && i < 100; i++ {
		result, err := Compact(ctx, false, 0)
		if err != nil {
			t.Fatalf("lote %d: %v", i, err)
		}
		more = result.HasMore()
	}
	if more {
		t.Fatal("compactação não terminou")
	}
	var count int64
	if err := db.Model(&maintenanceBlob{}).Where("data = ?", "chat").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("gravação perdida: count=%d err=%v", count, err)
	}
}

func TestCompactFinalCheckpointDoesNotWaitForReader(t *testing.T) {
	path := setupMaintenanceDB(t, true)
	ctx := context.Background()
	for {
		stats, err := DatabaseStatsSnapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if stats.FreelistCount <= incrementalVacuumBatchPages {
			break
		}
		if _, err := Compact(ctx, false, 0); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := sql.Open("sqlite", SQLiteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	tx, err := reader.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM maintenance_blobs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO maintenance_blobs(data) VALUES ('writer-before')").Error; err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := Compact(bounded, false, 0)
	if err != nil || !result.HasMore() || !result.CheckpointBusy() {
		t.Fatalf("checkpoint ocupado não sinalizou continuação: result=%+v err=%v", result, err)
	}
	if err := db.WithContext(bounded).Exec("INSERT INTO maintenance_blobs(data) VALUES ('writer-after')").Error; err != nil {
		t.Fatalf("writer após checkpoint: %v", err)
	}
	for i := 0; i < 2; i++ {
		waiting, waitErr := Compact(bounded, false, 0)
		if waitErr != nil || !waiting.HasMore() || !waiting.CheckpointBusy() || waiting.Mode != "noop" {
			t.Fatalf("espera sem progresso %d: result=%+v err=%v", i, waiting, waitErr)
		}
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	result, err = Compact(bounded, false, 0)
	if err != nil || result.HasMore() || result.CheckpointBusy() {
		t.Fatalf("continuação após leitor: result=%+v err=%v", result, err)
	}
	stats, err := DatabaseStatsSnapshot(bounded)
	if err != nil || stats.WALSizeBytes != 0 {
		t.Fatalf("WAL após continuação: bytes=%d err=%v", stats.WALSizeBytes, err)
	}
}
