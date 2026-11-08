package dbfixture

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"assistente/internal/database"
	"gorm.io/gorm"
)

type schemaRow struct{ Type, Name, TblName, SQL string }
type migrationRow struct {
	Version int
	Name    string
}
type signature struct {
	Schema         []schemaRow
	Migrations     []migrationRow
	Pragmas        map[string]string
	MaxConnections int
}

func inspect(t *testing.T, db *gorm.DB) signature {
	t.Helper()
	result := signature{Pragmas: map[string]string{}}
	if err := db.Raw("SELECT type, name, tbl_name, coalesce(sql, '') AS sql FROM sqlite_master ORDER BY type, name").Scan(&result.Schema).Error; err != nil {
		t.Fatal(err)
	}
	// GORM itera constraints nomeadas em ordem não determinística. Preserva
	// todo o SQL (incluindo CHECKs), normalizando apenas a ordem dessas cláusulas.
	for i, row := range result.Schema {
		if row.Type != "table" {
			continue
		}
		parts := strings.Split(row.SQL, ",CONSTRAINT ")
		if len(parts) > 1 {
			parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], ")")
			sort.Strings(parts[1:])
			result.Schema[i].SQL = strings.Join(parts, ",CONSTRAINT ") + ")"
		}
	}
	if err := db.Raw("SELECT version, name FROM schema_migrations ORDER BY version").Scan(&result.Migrations).Error; err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"user_version", "journal_mode", "synchronous", "foreign_keys", "busy_timeout", "auto_vacuum", "integrity_check"} {
		var value string
		if err := db.Raw("PRAGMA " + name).Scan(&value).Error; err != nil {
			t.Fatal(err)
		}
		result.Pragmas[name] = value
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	result.MaxConnections = conn.Stats().MaxOpenConnections
	return result
}

func TestClonePreservesCanonicalSchemaAndConnectionPolicy(t *testing.T) {
	restore := database.SetDB(nil)
	defer restore()
	if err := database.InitPath(filepath.Join(t.TempDir(), "canonical.db")); err != nil {
		t.Fatal(err)
	}
	canonical := database.DB()
	conn, err := canonical.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("fechar banco canônico: %v", err)
		}
	}()
	expected := inspect(t, canonical)
	t.Run("clone", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "clone.db")
		Open(t, path)
		if got := inspect(t, database.DB()); !reflect.DeepEqual(got, expected) {
			t.Fatal("schema/política divergiu do bootstrap real")
		}
		stats, err := database.DatabaseStatsSnapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if stats.Path != path {
			t.Fatalf("caminho ativo=%q, esperado=%q", stats.Path, path)
		}
	})
	if database.DB() != canonical {
		t.Fatal("cleanup não restaurou o banco anterior")
	}
	if err := conn.Ping(); err != nil {
		t.Fatalf("cleanup fechou o banco anterior: %v", err)
	}
}

func TestClonesIsolateRowsSchemaAndCloseOwnedConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outer.db")
	Open(t, path)
	outer := database.DB()
	if err := outer.Exec("CREATE TABLE private_marker (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := outer.Exec("INSERT INTO private_marker VALUES (1)").Error; err != nil {
		t.Fatal(err)
	}
	if err := outer.Create(&database.User{UUIDModel: database.UUIDModel{ID: "private-user"}, Username: "private-user"}).Error; err != nil {
		t.Fatal(err)
	}
	var closed *sql.DB
	for _, name := range []string{"first", "second"} {
		t.Run(name, func(t *testing.T) {
			Open(t, filepath.Join(t.TempDir(), "copy.db"))
			copyDB := database.DB()
			var err error
			closed, err = copyDB.DB()
			if err != nil {
				t.Fatal(err)
			}
			if copyDB.Migrator().HasTable("private_marker") {
				t.Fatal("DDL escapou para outra cópia")
			}
			var users int64
			if err := copyDB.Table("users").Count(&users).Error; err != nil {
				t.Fatal(err)
			}
			if users != 0 {
				t.Fatalf("dados escaparam para outra cópia: %d", users)
			}
			if err := copyDB.Exec("CREATE TABLE private_marker (id INTEGER PRIMARY KEY)").Error; err != nil {
				t.Fatal(err)
			}
		})
		if err := closed.Ping(); err == nil {
			t.Fatal("cleanup deixou a conexão da cópia aberta")
		}
		if database.DB() != outer {
			t.Fatal("cleanup não restaurou o singleton externo")
		}
	}
	var count int
	if err := outer.Raw("SELECT count(*) FROM private_marker").Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("mutação da cópia alterou o banco externo")
	}
	stats, err := database.DatabaseStatsSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Path != path {
		t.Fatal("cleanup não restaurou o caminho externo")
	}
}

func TestImageFailureRestoresPreviousDatabase(t *testing.T) {
	Open(t, filepath.Join(t.TempDir(), "previous.db"))
	previous := database.DB()
	if _, err := buildImage("relative.db"); err == nil {
		t.Fatal("aceitou caminho inválido")
	}
	if database.DB() != previous {
		t.Fatal("falha não restaurou o singleton")
	}
	conn, err := previous.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Ping(); err != nil {
		t.Fatalf("falha fechou banco anterior: %v", err)
	}
}
