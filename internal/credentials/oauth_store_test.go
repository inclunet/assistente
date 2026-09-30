package credentials

import (
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"assistente/internal/oauthintegrations"
	"bytes"
	"context"
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOAuthVaultEncryptedScopedCASAndSessionInvalidation(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	key := bytes.Repeat([]byte{7}, 32)
	mgr := NewManagerWithStore(key, NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "owner")
	service := oauthflow.New(oauthintegrations.ChatGPT())
	mgr.SetOAuthService(service)
	store, err := mgr.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := service.Pending("id", "owner", "chatgpt")
	r.Tokens = oauthflow.Tokens{Access: "secret-access", Refresh: "secret-refresh", ID: "secret-id"}
	if err = store.Create(ctx, r); err != nil {
		t.Fatal(err)
	}
	var entry database.CredentialEntry
	if err = database.DB().First(&entry, "id = ?", "id").Error; err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-access", "secret-refresh", "secret-id"} {
		if strings.Contains(entry.OAuthEnc, secret) {
			t.Fatal("plaintext secret")
		}
	}
	if entry.TokenEnc != "" || entry.SourceConfigEnc != "" || entry.RefreshTokenEnc != "" {
		t.Fatal("split authorization storage")
	}
	otherCtx := database.WithUserID(context.Background(), "other")
	other, _ := mgr.OAuthStore(otherCtx)
	if _, err = other.Load(otherCtx, "id"); err == nil {
		t.Fatal("cross-user read")
	}
	next := r
	next.Revision++
	if err = store.CompareAndSwap(ctx, next, r.Revision); err != nil {
		t.Fatal(err)
	}
	if err = store.CompareAndSwap(ctx, next, r.Revision); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("stale revision accepted")
	}
	session, cancel := store.(*oauthStore).SessionContext(ctx)
	defer cancel()
	mgr.ClearCommandCache()
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("logout did not cancel OAuth")
	}
	if _, err = store.Load(ctx, "id"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("stale session read")
	}
	store, _ = mgr.OAuthStore(ctx)
	mgr.Reset(key, false)
	if _, err = store.Load(ctx, "id"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("reset retained store access")
	}
	if err = store.CompareAndSwap(ctx, next, r.Revision); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatal("reset retained write access")
	}
}
func TestOAuthDeletedRecordCannotBeResurrected(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	mgr := NewManagerWithStore(bytes.Repeat([]byte{1}, 32), NewDBStore(), true)
	ctx := database.WithUserID(context.Background(), "owner")
	store, _ := mgr.OAuthStore(ctx)
	r, _ := oauthflow.New(oauthintegrations.ChatGPT()).Pending("deleted", "owner", "chatgpt")
	if err := store.Create(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeletePattern(ctx, "oauth:deleted"); err != nil {
		t.Fatal(err)
	}
	r.Revision++
	if err := store.CompareAndSwap(ctx, r, 1); err == nil {
		t.Fatal("deleted authorization restored")
	}
}

func TestOAuthCreateConsumerCanceledBeforeCommitDoesNotPublish(t *testing.T) {
	// Cancellation may discard a database/sql connection. A file-backed fixture
	// keeps its schema when the subsequent verification opens another connection.
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "credentials.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.CredentialEntry{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	mgr := NewManagerWithStore(bytes.Repeat([]byte{7}, 32), &DBStore{db: db}, true)
	owner := database.WithUserID(context.Background(), "owner")
	ctx, cancel := context.WithCancel(owner)
	defer cancel()
	store, err := mgr.OAuthStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := oauthflow.New(oauthintegrations.ChatGPT()).Pending("canceled", "owner", "chatgpt")
	err = store.(*oauthStore).CreateWithConsumer(ctx, r, func(tx *gorm.DB) error { cancel(); return nil })
	if err == nil {
		t.Fatal("canceled transaction succeeded")
	}
	// A new context can read the database, but neither durable nor cached records exist.
	if _, err = store.Load(owner, r.ID); !errors.Is(err, oauthflow.ErrNotFound) {
		t.Fatal("orphan durable record", err)
	}
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	if len(mgr.credentials) != 0 {
		t.Fatal("uncommitted credential published in cache")
	}
}
