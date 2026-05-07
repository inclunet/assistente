package app

import (
	"context"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAuthAppTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Session{},
		&database.CredentialEntry{},
		&database.CredentialKeyWrap{},
		&database.LLMProvider{},
		&database.Conversation{},
		&database.TaskList{},
	); err != nil {
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
	return db
}

func TestRefreshAuthPrefersStoredRefreshTokenOverStaleRequest(t *testing.T) {
	db := setupAuthAppTestDB(t)
	user := &database.User{
		Username:     "admin",
		PasswordHash: "hash",
		Role:         database.UserRoleAdmin,
		IsActive:     true,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	signer, err := auth.NewTokenSigner()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	sessionSvc, err := auth.NewSessionService(db, auth.SessionConfig{Signer: signer})
	if err != nil {
		t.Fatalf("new session service: %v", err)
	}
	firstPair, err := sessionSvc.IssueSession(context.Background(), user, "test")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	currentPair, err := sessionSvc.Refresh(context.Background(), firstPair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh initial session: %v", err)
	}

	credStore := credentials.NewDBStore()
	credMgr := credentials.NewManagerWithStore([]byte("test-key-exactly-32-bytes-long!!"), credStore, true)
	if err := credMgr.RegisterInstanceSecret(credentials.InstanceSecretAuthRefreshToken, currentPair.RefreshToken); err != nil {
		t.Fatalf("store current refresh token: %v", err)
	}

	app := &App{
		ctx:               context.Background(),
		credMgr:           credMgr,
		credStore:         credStore,
		vaultSvc:          auth.NewVaultService(credStore, nil),
		identitySvc:       auth.NewIdentityService(db),
		sessionSvc:        sessionSvc,
		llmRegistry:       llm.NewProviderRegistry(),
		authKeyringLoad:   func() (string, error) { return "", credentials.ErrKeyWrapNotFound },
		authKeyringSave:   func(string) error { return nil },
		authKeyringDelete: func() error { return nil },
	}

	authUser, err := app.RefreshAuth(RefreshRequest{RefreshToken: firstPair.RefreshToken})
	if err != nil {
		t.Fatalf("RefreshAuth should use stored current token instead of stale request token: %v", err)
	}
	if authUser == nil || authUser.UserID != user.ID {
		t.Fatalf("unexpected auth user: %+v", authUser)
	}

	var session database.Session
	if err := db.First(&session, "id = ?", currentPair.SessionID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.RevokedAt != nil {
		t.Fatal("stale request token should not revoke the current stored session")
	}
}

func TestRefreshAuthTriesCredentialStoreAfterStaleKeyringToken(t *testing.T) {
	db := setupAuthAppTestDB(t)
	user := &database.User{
		Username:     "admin",
		PasswordHash: "hash",
		Role:         database.UserRoleAdmin,
		IsActive:     true,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	signer, err := auth.NewTokenSigner()
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	sessionSvc, err := auth.NewSessionService(db, auth.SessionConfig{Signer: signer})
	if err != nil {
		t.Fatalf("new session service: %v", err)
	}
	firstPair, err := sessionSvc.IssueSession(context.Background(), user, "test")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	currentPair, err := sessionSvc.Refresh(context.Background(), firstPair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh initial session: %v", err)
	}

	credStore := credentials.NewDBStore()
	credMgr := credentials.NewManagerWithStore([]byte("test-key-exactly-32-bytes-long!!"), credStore, true)
	if err := credMgr.RegisterInstanceSecret(credentials.InstanceSecretAuthRefreshToken, currentPair.RefreshToken); err != nil {
		t.Fatalf("store current refresh token: %v", err)
	}

	app := &App{
		ctx:               context.Background(),
		credMgr:           credMgr,
		credStore:         credStore,
		vaultSvc:          auth.NewVaultService(credStore, nil),
		identitySvc:       auth.NewIdentityService(db),
		sessionSvc:        sessionSvc,
		llmRegistry:       llm.NewProviderRegistry(),
		authKeyringLoad:   func() (string, error) { return firstPair.RefreshToken, nil },
		authKeyringSave:   func(string) error { return nil },
		authKeyringDelete: func() error { return nil },
	}

	authUser, err := app.RefreshAuth(RefreshRequest{})
	if err != nil {
		t.Fatalf("RefreshAuth should try credential-store token after stale keyring token: %v", err)
	}
	if authUser == nil || authUser.UserID != user.ID {
		t.Fatalf("unexpected auth user: %+v", authUser)
	}

	var session database.Session
	if err := db.First(&session, "id = ?", currentPair.SessionID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.RevokedAt != nil {
		t.Fatal("stale keyring token should not revoke the current stored session")
	}
}
