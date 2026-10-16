package credentials

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"assistente/internal/oauthintegrations"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestOAuthCASRetriesLocalContention(t *testing.T) {
	for _, withConsumer := range []bool{false, true} {
		t.Run(map[bool]string{false: "grant", true: "grant-and-consumer"}[withConsumer], func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "oauth.db") + "?_pragma=busy_timeout(1)&_pragma=journal_mode(WAL)"
			open := func() *gorm.DB {
				db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
				if err != nil {
					t.Fatal(err)
				}
				pool, err := db.DB()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = pool.Close() })
				return db
			}
			db, competing := open(), open()
			if err := db.AutoMigrate(&database.CredentialEntry{}); err != nil {
				t.Fatal(err)
			}
			manager := NewManagerWithStore(bytes.Repeat([]byte{7}, 32), &DBStore{db: db}, true)
			ctx := database.WithUserID(context.Background(), "owner")
			store, err := manager.OAuthStore(ctx)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := oauthflow.New(oauthintegrations.ChatGPT()).Pending("grant", "owner", "chatgpt")
			if err := store.Create(ctx, before); err != nil {
				t.Fatal(err)
			}
			next := before
			next.Revision++
			next.State = "connected"
			next.Tokens = oauthflow.Tokens{Access: "fresh", Refresh: "rotated", Type: "Bearer"}
			// A real second SQLite connection holds the writer across several attempts.
			tx := competing.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			if err := tx.Model(&database.CredentialEntry{}).Where("id = ?", before.ID).Update("auth_type", "bearer").Error; err != nil {
				t.Fatal(err)
			}
			release := make(chan error, 1)
			go func() { time.Sleep(150 * time.Millisecond); release <- tx.Commit().Error }()
			var published atomic.Int32
			if withConsumer {
				err = store.(*oauthStore).CompareAndSwapWithConsumerAndPublish(ctx, next, before.Revision, func(tx *gorm.DB) error {
					return tx.Transaction(func(nested *gorm.DB) error { return nested.Exec("SELECT 1").Error })
				}, func() { published.Add(1) })
			} else {
				err = store.CompareAndSwap(ctx, next, before.Revision)
			}
			if releaseErr := <-release; releaseErr != nil {
				t.Fatal(releaseErr)
			}
			if err != nil {
				t.Fatalf("lost issued tokens to transient writer lock: %v", err)
			}
			if withConsumer && published.Load() != 1 {
				t.Fatal("publication must follow one successful commit")
			}
			// Reopen the vault to prove durability rather than just its memory cache.
			reopened := NewManagerWithStore(bytes.Repeat([]byte{7}, 32), &DBStore{db: db}, true)
			durable, err := reopened.OAuthStore(ctx)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := durable.Load(ctx, before.ID)
			if err != nil || saved.Tokens.Access != "fresh" || saved.Tokens.Refresh != "rotated" || saved.Revision != next.Revision {
				t.Fatalf("grant not durable: %v", err)
			}
			if err := store.CompareAndSwap(ctx, next, before.Revision); !errors.Is(err, oauthflow.ErrConflict) {
				t.Fatalf("stale revision accepted: %v", err)
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			newer := next
			newer.Revision++
			if err := store.CompareAndSwap(cancelled, newer, next.Revision); err == nil {
				t.Fatal("cancelled write accepted")
			}
		})
	}
}
