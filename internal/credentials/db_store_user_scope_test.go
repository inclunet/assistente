package credentials

import (
	"context"
	"testing"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupScopedCredentialStoreTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.CredentialEntry{}, &database.CredentialKeyWrap{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.SetDB(db)
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		database.SetDB(nil)
	})
}

func TestDBStoreScopesCredentialsByContextUser(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	store := NewDBStore()
	anaCtx := database.WithUserID(context.Background(), "user-ana")
	leoCtx := database.WithUserID(context.Background(), "user-leo")
	cred := StoredCredential{
		Pattern: "api.openai.com",
		Auth: &AuthConfig{
			Type:  "bearer",
			Token: "token",
		},
	}

	if err := store.SaveCredential(anaCtx, cred); err != nil {
		t.Fatalf("save ana credential: %v", err)
	}
	if err := store.SaveCredential(leoCtx, cred); err != nil {
		t.Fatalf("same pattern should be allowed for another user: %v", err)
	}

	anaCredentials, err := store.ListCredentials(anaCtx)
	if err != nil {
		t.Fatalf("list ana credentials: %v", err)
	}
	if len(anaCredentials) != 1 || anaCredentials[0].UserID != "user-ana" {
		t.Fatalf("expected only ana credential, got %+v", anaCredentials)
	}

	if err := store.DeleteCredential(anaCtx, "api.openai.com"); err != nil {
		t.Fatalf("delete ana credential: %v", err)
	}
	leoCredentials, err := store.ListCredentials(leoCtx)
	if err != nil {
		t.Fatalf("list leo credentials: %v", err)
	}
	if len(leoCredentials) != 1 || leoCredentials[0].UserID != "user-leo" {
		t.Fatalf("expected leo credential to remain, got %+v", leoCredentials)
	}
}
