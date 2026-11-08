package sqlite

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openQualificationDB(t *testing.T, dsn string, translate bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), TranslateError: translate})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return db
}

func qualificationDSN(t *testing.T) string {
	t.Helper()
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "histórico & #.db"))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(25)")
	u.RawQuery = q.Encode()
	return u.String()
}

type qualificationEntry struct {
	ID      string `gorm:"primaryKey"`
	Body    string
	Payload []byte
	Seen    *time.Time
	Discard string
}

func TestRebuildAndReopenPreserveStoredContent(t *testing.T) {
	dsn := qualificationDSN(t)
	db := openQualificationDB(t, dsn, false)
	if err := db.AutoMigrate(&qualificationEntry{}); err != nil {
		t.Fatal(err)
	}
	instant := time.Date(2026, 10, 30, 12, 34, 56, 123456789, time.FixedZone("offset", -3*3600))
	want := []qualificationEntry{
		{ID: "a", Body: "texto\x00áudio 漢字", Payload: []byte{0, 255, 1, 128}, Seen: &instant},
		{ID: "b", Body: "preservar NULL", Payload: nil, Seen: nil},
	}
	if err := db.Create(&want).Error; err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.Raw("SELECT CAST(seen AS TEXT) FROM qualification_entries WHERE id='a'").Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != "2026-10-30 12:34:56.123456789-03:00" {
		t.Fatalf("formato persistido mudou: %q", stored)
	}
	if err := db.Migrator().DropColumn(&qualificationEntry{}, "Discard"); err != nil {
		t.Fatal(err)
	}
	raw, _ := db.DB()
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	db = openQualificationDB(t, dsn, false)
	var got []qualificationEntry
	if err := db.Order("id").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("reconstrução perdeu registros: %d", len(got))
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Body != want[i].Body || !reflect.DeepEqual(got[i].Payload, want[i].Payload) {
			t.Fatalf("conteúdo alterado na linha %d: %#v", i, got[i])
		}
	}
	if got[0].Seen == nil || !got[0].Seen.Equal(instant) || got[1].Seen != nil {
		t.Fatal("instante ou NULL alterado na reconstrução/reabertura")
	}
}

func TestConstraintsTriggersAndRollback(t *testing.T) {
	db := openQualificationDB(t, qualificationDSN(t), true)
	for _, statement := range []string{
		"CREATE TABLE owners(id INTEGER PRIMARY KEY)",
		"CREATE TABLE records(id INTEGER PRIMARY KEY, owner_id INTEGER NOT NULL REFERENCES owners(id), name TEXT UNIQUE)",
		"CREATE TABLE audit(record_id INTEGER)",
		"CREATE TRIGGER record_audit AFTER INSERT ON records BEGIN INSERT INTO audit VALUES(NEW.id); END",
		"INSERT INTO owners VALUES(1)",
		"INSERT INTO records VALUES(1,1,'first')",
		"ALTER TABLE records ADD COLUMN label TEXT",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("INSERT INTO records VALUES(2,1,'first',NULL)").Error; !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("tradução de unicidade: %v", err)
	}
	if err := db.Exec("INSERT INTO records VALUES(2,9,'other',NULL)").Error; !errors.Is(err, gorm.ErrForeignKeyViolated) {
		t.Fatalf("FK desligada ou tradução alterada: %v", err)
	}
	abort := errors.New("rollback solicitado")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO records VALUES(2,1,'second',NULL)").Error; err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	var count int
	if err := db.Raw("SELECT COUNT(*) FROM audit").Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("trigger/rollback divergente: %d %v", count, err)
	}
}

func TestPreparedStatementKeepsSnapshotAndParameters(t *testing.T) {
	dsn := qualificationDSN(t)
	db := openQualificationDB(t, dsn, false)
	for _, statement := range []string{"PRAGMA journal_mode=WAL", "CREATE TABLE values_by_owner(owner TEXT PRIMARY KEY,value TEXT)", "INSERT INTO values_by_owner VALUES('a','old'),('b','private')"} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := db.DB()
	stmt, err := raw.PrepareContext(context.Background(), "SELECT value FROM values_by_owner WHERE owner=?")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stmt.Close() }()
	tx, err := raw.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	read := func(owner string) string {
		t.Helper()
		var value string
		if err := tx.StmtContext(context.Background(), stmt).QueryRowContext(context.Background(), owner).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if read("a") != "old" {
		t.Fatal("snapshot inicial incorreto")
	}
	writer := openQualificationDB(t, dsn, false)
	if err := writer.Exec("UPDATE values_by_owner SET value='new' WHERE owner='a'").Error; err != nil {
		t.Fatal(err)
	}
	if read("a") != "old" || read("b") != "private" {
		t.Fatal("statement reutilizou parâmetros ou perdeu o snapshot")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := stmt.QueryRowContext(context.Background(), "a").Scan(&value); err != nil || value != "new" {
		t.Fatalf("nova leitura reteve o snapshot anterior: %q %v", value, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := stmt.QueryRowContext(ctx, "a").Scan(&value); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado aceito: %v", err)
	}
}
