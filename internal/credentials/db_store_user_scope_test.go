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
		Auth: &AuthConfig{Source: "static",
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

func TestDBStoreSaveCredentialAndGetIDReturnsCommittedIdentity(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	store := NewDBStore()
	ctx := database.WithUserID(context.Background(), "user-1")
	cred := StoredCredential{Pattern: "api.example.com", Auth: &AuthConfig{Source: "static", Type: "bearer", Token: "encrypted-token"}}

	id, err := store.SaveCredentialAndGetID(ctx, cred)
	if err != nil {
		t.Fatalf("SaveCredentialAndGetID() error = %v", err)
	}
	if id == "" {
		t.Fatal("SaveCredentialAndGetID() returned an empty ID")
	}
	rows, err := store.ListCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("persisted identity = %+v, returned ID = %q", rows, id)
	}

	updated := cred
	updated.Auth = &AuthConfig{Source: "static", Type: "bearer", Token: "rotated-token"}
	updatedID, err := store.SaveCredentialAndGetID(ctx, updated)
	if err != nil {
		t.Fatalf("SaveCredentialAndGetID() on replacement error = %v", err)
	}
	if updatedID != id {
		t.Fatalf("pattern replacement changed credential ID: old=%q new=%q", id, updatedID)
	}
}

func TestClientRegistrationGrantSurvivesReload(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	ctx := database.WithUserID(context.Background(), "user")
	key := []byte("test-key-exactly-32-bytes-long!!")
	store := NewDBStore()
	manager := NewManagerWithStoreAndPersistence(key, store, true)
	const pattern = "mcp-client:device"
	if err := manager.RegisterPatternWithContext(ctx, pattern, &AuthConfig{Source: "static", Type: "oauth2", ClientID: "client", ClientSecret: "secret", ClientGrantType: "urn:ietf:params:oauth:grant-type:device_code"}); err != nil {
		t.Fatal(err)
	}
	reopened := NewManagerWithStoreAndPersistence(key, store, true)
	if err := reopened.LoadUserCredentials(ctx, "user"); err != nil {
		t.Fatal(err)
	}
	auth, err := reopened.GetByPatternWithContext(ctx, pattern)
	if err != nil || auth == nil || auth.ClientID != "client" || auth.ClientSecret != "secret" || auth.ClientGrantType != "urn:ietf:params:oauth:grant-type:device_code" {
		t.Fatalf("registration metadata lost: err=%v", err)
	}
	if err := reopened.RegisterPatternWithContext(ctx, pattern, &AuthConfig{Source: "static", Type: "oauth2", ClientID: "manual"}); err != nil {
		t.Fatal(err)
	}
	replaced := NewManagerWithStoreAndPersistence(key, store, true)
	if err := replaced.LoadUserCredentials(ctx, "user"); err != nil {
		t.Fatal(err)
	}
	auth, err = replaced.GetByPatternWithContext(ctx, pattern)
	if err != nil || auth == nil || auth.ClientGrantType != "" {
		t.Fatal("manual replacement inherited Device-only registration")
	}
}
