package commandledger

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestStoreUsesDatabaseComparesTheUnderlyingSQLIdentity(t *testing.T) {
	open := func(name string) *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		return db
	}
	dbA, dbB := open("a.db"), open("b.db")
	store, err := New(dbA, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if !store.UsesDatabase(dbA) {
		t.Fatal("store não reconheceu a raiz SQL original")
	}
	rootSession := dbA.Session(&gorm.Session{NewDB: true})
	if !store.UsesDatabase(rootSession) {
		t.Fatal("store não reconheceu uma sessão da raiz SQL")
	}
	if store.UsesDatabase(dbB) {
		t.Fatal("store aceitou uma raiz SQL estrangeira")
	}
	if store.UsesDatabase(nil) || store.UsesDatabase(&gorm.DB{}) {
		t.Fatal("store aceitou DB nil/zero")
	}
	transaction := dbA.Begin()
	if transaction.Error != nil {
		t.Fatal(transaction.Error)
	}
	if store.UsesDatabase(transaction) {
		t.Fatal("store aceitou argumento transacional")
	}
	transactionalStore, err := New(transaction, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if transactionalStore.UsesDatabase(dbA) || transactionalStore.UsesDatabase(transaction) {
		t.Fatal("store transacional foi aceito")
	}
	if err := transaction.Rollback().Error; err != nil {
		t.Fatal(err)
	}
}
