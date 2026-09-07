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

// TestRollbackLoginStateRevokesSessionAndClearsLocal cobre B5 do review
// da Fatia 1: depois de uma falha tardia em Login/RefreshAuth, o helper
// `rollbackLoginState` precisa revogar a sessão remota, apagar o
// refresh token local e zerar currentUserID/currentAuthUser. Sem isso,
// o frontend recebia o erro mas o backend ficava em estado consistente
// para o usuário voltar a entrar pelo refresh.
func TestRollbackLoginStateRevokesSessionAndClearsLocal(t *testing.T) {
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
	pair, err := sessionSvc.IssueSession(context.Background(), user, "test")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	credStore := credentials.NewDBStore()
	credMgr := credentials.NewManagerWithStore([]byte("test-key-exactly-32-bytes-long!!"), credStore, true)
	if err := credMgr.RegisterInstanceSecret(credentials.InstanceSecretAuthRefreshToken, pair.RefreshToken); err != nil {
		t.Fatalf("store refresh token: %v", err)
	}

	app := &App{
		ctx:               context.Background(),
		credMgr:           credMgr,
		credStore:         credStore,
		sessionSvc:        sessionSvc,
		llmRegistry:       llm.NewProviderRegistry(),
		authKeyringLoad:   func() (string, error) { return pair.RefreshToken, nil },
		authKeyringSave:   func(string) error { return nil },
		authKeyringDelete: func() error { return nil },
	}
	app.setCurrentUserID(user.ID)
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: pair.SessionID, Role: database.UserRoleAdmin})

	app.rollbackLoginState(pair.RefreshToken)

	var session database.Session
	if err := db.First(&session, "id = ?", pair.SessionID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.RevokedAt == nil {
		t.Fatal("rollback should revoke the remote session")
	}

	stored, ok, err := credMgr.GetInstanceSecret(credentials.InstanceSecretAuthRefreshToken)
	if err != nil {
		t.Fatalf("get instance secret after rollback: %v", err)
	}
	if ok && stored != "" {
		t.Fatalf("rollback should have cleared the local refresh token, got %q", stored)
	}

	app.authMu.RLock()
	defer app.authMu.RUnlock()
	if app.currentUserID != "" {
		t.Fatalf("rollback should clear currentUserID, got %q", app.currentUserID)
	}
	if app.currentAuthUser != nil {
		t.Fatalf("rollback should clear currentAuthUser, got %+v", app.currentAuthUser)
	}
}

// TestLogoutIsBestEffortWhenRevokeFails cobre M6 do review da Fatia 1:
// se a revogação remota falha (ex: DB indisponível), Logout DEVE limpar
// o estado local e retornar nil — não há benefício em propagar o erro
// para o frontend porque o usuário fez logout, e a sessão remota
// expira em RefreshTTL de qualquer jeito.
func TestLogoutIsBestEffortWhenRevokeFails(t *testing.T) {
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
	pair, err := sessionSvc.IssueSession(context.Background(), user, "test")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}

	credStore := credentials.NewDBStore()
	credMgr := credentials.NewManagerWithStore([]byte("test-key-exactly-32-bytes-long!!"), credStore, true)
	if err := credMgr.RegisterInstanceSecret(credentials.InstanceSecretAuthRefreshToken, pair.RefreshToken); err != nil {
		t.Fatalf("store refresh token: %v", err)
	}

	app := &App{
		ctx:               context.Background(),
		credMgr:           credMgr,
		credStore:         credStore,
		identitySvc:       auth.NewIdentityService(db),
		vaultSvc:          auth.NewVaultService(credStore, nil),
		sessionSvc:        sessionSvc,
		llmRegistry:       llm.NewProviderRegistry(),
		authKeyringLoad:   func() (string, error) { return pair.RefreshToken, nil },
		authKeyringSave:   func(string) error { return nil },
		authKeyringDelete: func() error { return nil },
	}
	app.setCurrentUserID(user.ID)
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: pair.SessionID, Role: database.UserRoleAdmin})

	// Sabota o DB para forçar erro em Logout->sessionSvc.Logout.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("acessar sql.DB: %v", err)
	}
	_ = sqlDB.Close()

	if err := app.Logout(LogoutRequest{RefreshToken: pair.RefreshToken}); err != nil {
		t.Fatalf("Logout deveria ser best-effort, got %v", err)
	}

	app.authMu.RLock()
	defer app.authMu.RUnlock()
	if app.currentUserID != "" {
		t.Fatalf("Logout deveria ter limpado currentUserID mesmo com erro de revoke, got %q", app.currentUserID)
	}
	if app.currentAuthUser != nil {
		t.Fatalf("Logout deveria ter limpado currentAuthUser, got %+v", app.currentAuthUser)
	}
}

func TestGetAuthStatusDoesNotRequireSessionService(t *testing.T) {
	db := setupAuthAppTestDB(t)
	if err := db.Create(&database.User{
		Username:     "admin",
		PasswordHash: "hash",
		Role:         database.UserRoleAdmin,
		IsActive:     true,
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	credStore := credentials.NewDBStore()
	app := &App{
		ctx:         context.Background(),
		credStore:   credStore,
		identitySvc: auth.NewIdentityService(db),
		vaultSvc:    auth.NewVaultService(credStore, nil),
		sessionSvc:  nil,
	}

	status, err := app.GetAuthStatus()
	if err != nil {
		t.Fatalf("GetAuthStatus() should not depend on JWT session service: %v", err)
	}
	if !status.HasUsers {
		t.Fatalf("expected HasUsers=true, got %+v", status)
	}
}
