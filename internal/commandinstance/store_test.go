package commandinstance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func openInstanceTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir SQLite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter conexão: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func testStartupID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("gerar startup UUIDv7: %v", err)
	}
	return id.String()
}

func TestMigrateCreatesProcessGenerationSchemaAndIsIdempotent(t *testing.T) {
	db := openInstanceTestDB(t, filepath.Join(t.TempDir(), "commands.db"))
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrar novamente: %v", err)
	}
	if !db.Migrator().HasTable("command_process_generations") {
		t.Fatal("schema de gerações não criado")
	}
	if err := db.Exec(`INSERT INTO command_process_generations (startup_id, file_identity, created_at) VALUES ('not-a-uuid', 'identity-a', CURRENT_TIMESTAMP)`).Error; err == nil {
		t.Fatal("DDL aceitou startup que não é UUIDv7 canônico")
	}
	if err := db.Exec(`INSERT INTO command_process_generations (startup_id, file_identity, created_at) VALUES (?, '', CURRENT_TIMESTAMP)`, testStartupID(t)).Error; err == nil {
		t.Fatal("DDL aceitou identidade física vazia")
	}
}

func TestOpenRegistersStartupAndOnlyRecoversSamePhysicalIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.db")
	db := openInstanceTestDB(t, path)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	firstID := testStartupID(t)
	first, err := Open(context.Background(), db, firstID)
	if err != nil {
		t.Fatalf("abrir primeira lease: %v", err)
	}
	if !first.UsesDatabase(db) {
		t.Fatal("lease não reconheceu o banco recebido")
	}
	firstProof := first.RecoveryProof()
	if !firstProof.Valid() {
		t.Fatal("proof vazio do primeiro startup deveria ser válido")
	}
	if firstProof.Includes(firstID + ":1") {
		t.Fatal("proof não pode incluir a geração do startup atual")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("fechar primeira lease: %v", err)
	}
	if firstProof.Valid() {
		t.Fatal("proof deveria ser invalidado ao fechar a lease")
	}

	secondID := testStartupID(t)
	second, err := Open(context.Background(), db, secondID)
	if err != nil {
		t.Fatalf("abrir segunda lease: %v", err)
	}
	defer second.Close()
	proof := second.RecoveryProof()
	if !proof.Valid() || !proof.Includes(firstID+":0") || !proof.Includes(firstID+":18446744073709551615") {
		t.Fatal("prefixo antigo da mesma identidade não foi recuperado")
	}
	if !proof.UsesDatabase(db) {
		t.Fatal("proof não reconheceu a raiz SQL do banco")
	}
	if !proof.UsesDatabase(db.Session(&gorm.Session{})) {
		t.Fatal("wrapper GORM da mesma raiz SQL foi recusado")
	}
	foreignDB := openInstanceTestDB(t, filepath.Join(t.TempDir(), "foreign.db"))
	if proof.UsesDatabase(foreignDB) {
		t.Fatal("proof aceitou raiz SQL estrangeira")
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if second.UsesDatabase(tx) || proof.UsesDatabase(tx) {
		t.Fatal("lease/proof aceitaram wrapper transacional como banco raiz")
	}
	_ = tx.Rollback()
	for _, generation := range []string{
		secondID + ":1",
		firstID + ":01",
		firstID + ":18446744073709551616",
		testStartupID(t) + ":1",
		firstID + ":x",
	} {
		if proof.Includes(generation) {
			t.Fatalf("geração desconhecida/não canônica aceita: %q", generation)
		}
	}

	if err := second.Close(); err != nil {
		t.Fatalf("fechar segunda lease: %v", err)
	}
	duplicate, err := Open(context.Background(), db, secondID)
	if duplicate != nil {
		_ = duplicate.Close()
		t.Fatal("startup duplicado não deveria emitir lease")
	}
	if err == nil {
		t.Fatal("startup duplicado deveria falhar sem upsert")
	}
}

func TestOpenRefusesMissingSchemaWithoutDDL(t *testing.T) {
	db := openInstanceTestDB(t, filepath.Join(t.TempDir(), "commands.db"))
	_, err := Open(context.Background(), db, testStartupID(t))
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("erro esperado de schema, obtive %v", err)
	}
	if db.Migrator().HasTable("command_process_generations") {
		t.Fatal("Open criou DDL apesar de schema ausente")
	}
}

func TestOpenRejectsIncompatibleColumnTypesBeforeRegistering(t *testing.T) {
	for _, columns := range []string{
		"startup_id TEXT NOT NULL PRIMARY KEY, file_identity INTEGER NOT NULL, created_at DATETIME NOT NULL",
		"startup_id TEXT NOT NULL PRIMARY KEY, file_identity TEXT NOT NULL, created_at INTEGER NOT NULL",
	} {
		t.Run(columns, func(t *testing.T) {
			db := openInstanceTestDB(t, filepath.Join(t.TempDir(), "invalid.db"))
			if err := db.Exec("CREATE TABLE command_process_generations (" + columns + ")").Error; err != nil {
				t.Fatal(err)
			}
			if lease, err := Open(context.Background(), db, testStartupID(t)); !errors.Is(err, ErrSchema) {
				if lease != nil {
					_ = lease.Close()
				}
				t.Fatalf("schema incompatível aceito: %v", err)
			}
			var count int64
			if err := db.Table("command_process_generations").Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("registro apesar de schema inválido: %d %v", count, err)
			}
		})
	}
}

func TestOpenDoesNotRecoverCopiedDatabaseIdentity(t *testing.T) {
	dir := t.TempDir()
	originalPath := filepath.Join(dir, "original.db")
	db := openInstanceTestDB(t, originalPath)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrar original: %v", err)
	}
	first, err := Open(context.Background(), db, testStartupID(t))
	if err != nil {
		t.Fatalf("abrir original: %v", err)
	}
	oldID := first.startupID
	if err := first.Close(); err != nil {
		t.Fatalf("fechar original: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	// A cópia física é preparada fora do banco aberto, preservando o conteúdo
	// sem reutilizar sua identidade de arquivo.
	original, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "copy.db"), original, 0600); err != nil {
		t.Fatal(err)
	}
	copyDB := openInstanceTestDB(t, filepath.Join(dir, "copy.db"))
	second, err := Open(context.Background(), copyDB, testStartupID(t))
	if err != nil {
		t.Fatalf("abrir cópia: %v", err)
	}
	defer second.Close()
	if second.RecoveryProof().Includes(oldID + ":1") {
		t.Fatal("cópia física não pode recuperar prefixo da identidade original")
	}
}
