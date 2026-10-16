package commandconfig

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestLoadReadsOneSnapshotAcrossConcurrentCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atomic.db")
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
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
	db := open()
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	writer := open()
	user := uuid.Must(uuid.NewV7()).String()
	layer := Layer{ID: uuid.Must(uuid.NewV7()).String(), UserID: user, Name: "before", Enabled: true, Source: "user", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	generation := Generation{ID: uuid.Must(uuid.NewV7()).String(), UserID: user, Generation: 1, UpdatedAt: time.Now()}
	if err := db.Create(&layer).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&generation).Error; err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	var writeErr error
	if err := db.Callback().Query().After("gorm:query").Register("test:concurrent_config_commit", func(tx *gorm.DB) {
		if tx.Statement.Table != "command_layers" {
			return
		}
		once.Do(func() {
			writeErr = writer.WithContext(tx.Statement.Context).Transaction(func(write *gorm.DB) error {
				if err := write.Model(&Layer{}).Where("id = ?", layer.ID).Update("name", "after").Error; err != nil {
					return err
				}
				return write.Model(&Generation{}).Where("id = ?", generation.ID).Update("generation", 2).Error
			})
		})
	}); err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snapshot, err := store.Load(ctx, Scope{UserID: user})
	if err != nil || writeErr != nil {
		t.Fatal("leitura/escrita", err, writeErr)
	}
	if len(snapshot.Layers) != 1 || snapshot.Layers[0].Name != "before" || len(snapshot.Generations) != 1 || snapshot.Generations[0].Generation != 1 {
		t.Fatal("leitura misturou dados de transações diferentes", snapshot.Layers, snapshot.Generations)
	}
	if err := store.CheckCurrent(ctx, snapshot); !errors.Is(err, ErrStale) {
		t.Fatal("commit concorrente não detectado", err)
	}
}
