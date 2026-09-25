package commanddecision

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestConsumeForDatabaseAcceptsSameDatabaseRootAndSession(t *testing.T) {
	cases := []struct {
		name     string
		database func(*gorm.DB) *gorm.DB
	}{
		{name: "root", database: func(db *gorm.DB) *gorm.DB { return db }},
		{name: "session", database: func(db *gorm.DB) *gorm.DB { return db.Session(&gorm.Session{}) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, db, _, request := acceptedFixture(t)
			callbackCalls := 0

			err := store.ConsumeForDatabase(context.Background(), tc.database(db), request, func(tx *gorm.DB) error {
				callbackCalls++
				return tx.Create(&decisionEffectRow{ID: request.MutationID, Value: "applied"}).Error
			})
			if err != nil {
				t.Fatalf("consumir no mesmo banco: %v", err)
			}
			if callbackCalls != 1 {
				t.Fatalf("callback chamado %d vezes", callbackCalls)
			}
			if row := loadReceipt(t, db, request.DecisionID); row.State != Consumed || row.ConsumedAt == nil {
				t.Fatalf("receipt não consumida: %+v", row)
			}
			if countEvents(t, db, request.DecisionID) != 3 || countEffects(t, db) != 1 {
				t.Fatal("consumo não persistiu receipt, evento e efeito")
			}
		})
	}
}

func TestConsumeForDatabaseRejectsForeignNilAndTransactionalDatabases(t *testing.T) {
	cases := []struct {
		name string
		make func(t *testing.T, db *gorm.DB) *gorm.DB
	}{
		{
			name: "differentDB",
			make: func(t *testing.T, _ *gorm.DB) *gorm.DB {
				other, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "other.db")), &gorm.Config{})
				if err != nil {
					t.Fatalf("abrir outro sqlite temporário: %v", err)
				}
				sqlDB, err := other.DB()
				if err != nil {
					t.Fatalf("obter outro sql.DB: %v", err)
				}
				t.Cleanup(func() { _ = sqlDB.Close() })
				return other
			},
		},
		{
			name: "nil",
			make: func(_ *testing.T, _ *gorm.DB) *gorm.DB { return nil },
		},
		{
			name: "root tx",
			make: func(t *testing.T, db *gorm.DB) *gorm.DB {
				tx := db.Begin()
				if tx.Error != nil {
					t.Fatalf("abrir transação fixture: %v", tx.Error)
				}
				t.Cleanup(func() { _ = tx.Rollback().Error })
				return tx
			},
		},
		{
			name: "prepared root tx",
			make: func(t *testing.T, db *gorm.DB) *gorm.DB {
				prepared := db.Session(&gorm.Session{PrepareStmt: true})
				tx := prepared.Begin()
				if tx.Error != nil {
					t.Fatalf("abrir transação preparada fixture: %v", tx.Error)
				}
				t.Cleanup(func() { _ = tx.Rollback().Error })
				return tx
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, db, _, request := acceptedFixture(t)
			candidate := tc.make(t, db)
			callbackCalls := 0

			err := store.ConsumeForDatabase(context.Background(), candidate, request, func(tx *gorm.DB) error {
				callbackCalls++
				return tx.Create(&decisionEffectRow{ID: request.MutationID, Value: "must-not-apply"}).Error
			})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("banco inválido aceito: %v", err)
			}
			if callbackCalls != 0 {
				t.Fatal("callback chamado antes da validação do banco")
			}
			if row := loadReceipt(t, db, request.DecisionID); row.State != Accepted || row.ConsumedAt != nil {
				t.Fatalf("receipt alterada após rejeição: %+v", row)
			}
			if countEvents(t, db, request.DecisionID) != 2 || countEffects(t, db) != 0 {
				t.Fatal("rejeição deixou consumo ou efeito")
			}
		})
	}
}

func TestConsumeForDatabaseRejectsZeroValueGormDatabase(t *testing.T) {
	store, db, _, request := acceptedFixture(t)
	cases := []struct {
		name  string
		store *Store
		db    *gorm.DB
	}{
		{name: "database", store: store, db: &gorm.DB{}},
		{name: "store database", store: &Store{db: &gorm.DB{}, now: store.now}, db: db},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			callbackCalls := 0
			err := tc.store.ConsumeForDatabase(context.Background(), tc.db, request, func(*gorm.DB) error {
				callbackCalls++
				return nil
			})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("GORM zero-value aceito: %v", err)
			}
			if callbackCalls != 0 {
				t.Fatal("callback chamado para GORM zero-value")
			}
		})
	}

	if row := loadReceipt(t, db, request.DecisionID); row.State != Accepted || row.ConsumedAt != nil {
		t.Fatalf("receipt alterada após rejeição: %+v", row)
	}
	if countEvents(t, db, request.DecisionID) != 2 || countEffects(t, db) != 0 {
		t.Fatal("rejeição deixou consumo ou efeito")
	}
}

func TestConsumeForDatabaseRejectsTransactionalStoreDatabase(t *testing.T) {
	store, db, _, request := acceptedFixture(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("abrir transação fixture: %v", tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	transactionalStore, err := New(tx, store.presenter, store.now)
	if err != nil {
		t.Fatalf("criar Store transacional: %v", err)
	}
	callbackCalls := 0
	err = transactionalStore.ConsumeForDatabase(context.Background(), db, request, func(tx *gorm.DB) error {
		callbackCalls++
		return tx.Create(&decisionEffectRow{ID: request.MutationID, Value: "must-not-apply"}).Error
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Store transacional aceito: %v", err)
	}
	if callbackCalls != 0 {
		t.Fatal("callback chamado para Store transacional")
	}
	if row := loadReceipt(t, db, request.DecisionID); row.State != Accepted || row.ConsumedAt != nil {
		t.Fatalf("receipt alterada após rejeição: %+v", row)
	}
	if countEvents(t, db, request.DecisionID) != 2 || countEffects(t, db) != 0 {
		t.Fatal("rejeição deixou consumo ou efeito")
	}
}
