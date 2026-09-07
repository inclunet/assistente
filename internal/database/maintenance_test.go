package database

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
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
