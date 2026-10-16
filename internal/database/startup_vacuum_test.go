package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openStartupVacuumDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "startup.db")
	gdb, err := gorm.Open(sqlite.Open(sqliteDSN(path)), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gdb, path
}

func TestStartupAutoVacuumPreservesModes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup []string
		want  int
	}{
		{"new", nil, autoVacuumIncremental},
		{"legacy", []string{"CREATE TABLE probe (value INTEGER)"}, autoVacuumNone},
		{"full", []string{"PRAGMA auto_vacuum=FULL", "CREATE TABLE probe (value INTEGER)"}, autoVacuumIncremental},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gdb, _ := openStartupVacuumDB(t)
			for _, sql := range tc.setup {
				if err := gdb.Exec(sql).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := configureInitialAutoVacuum(gdb); err != nil {
				t.Fatal(err)
			}
			// Materializa o banco novo para provar que o modo persiste após criar tabelas.
			if err := gdb.Exec("CREATE TABLE IF NOT EXISTS probe (value INTEGER)").Error; err != nil {
				t.Fatal(err)
			}
			var mode int
			if err := gdb.Raw("PRAGMA auto_vacuum").Scan(&mode).Error; err != nil {
				t.Fatal(err)
			}
			if mode != tc.want {
				t.Fatalf("mode=%d, want %d", mode, tc.want)
			}
		})
	}
}

func TestStartupAutoVacuumAlreadyIncrementalDoesNotWrite(t *testing.T) {
	gdb, path := openStartupVacuumDB(t)
	for _, sql := range []string{
		"PRAGMA auto_vacuum=INCREMENTAL", "PRAGMA journal_mode=WAL",
		"CREATE TABLE probe (value INTEGER)", "INSERT INTO probe VALUES (42)",
		"PRAGMA wal_checkpoint(TRUNCATE)", "PRAGMA query_only=ON",
	} {
		if err := gdb.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	// query_only faz o teste falhar se o startup tentar qualquer escrita de metadado.
	if err := configureInitialAutoVacuum(gdb); err != nil {
		t.Fatalf("banco incremental exigiu escrita: %v", err)
	}
	info, err := os.Stat(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("startup acrescentou %d bytes ao WAL", info.Size())
	}
	var value int
	if err := gdb.Raw("SELECT value FROM probe").Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	if value != 42 {
		t.Fatalf("conteúdo alterado: %d", value)
	}
}

func TestStartupAutoVacuumReturnsReadFailure(t *testing.T) {
	gdb, _ := openStartupVacuumDB(t)
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := configureInitialAutoVacuum(gdb); err == nil {
		t.Fatal("banco indisponível foi aceito")
	}
}
