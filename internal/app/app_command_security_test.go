package app

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCommandAuthTransitionsInvalidateEvenWhenLegacyOperationFails(t *testing.T) {
	for name, action := range map[string]func(*App) error{
		"login":   func(a *App) error { _, err := a.Login(LoginRequest{}); return err },
		"refresh": func(a *App) error { _, err := a.RefreshAuth(RefreshRequest{}); return err },
		"logout":  func(a *App) error { return a.Logout(LogoutRequest{}) },
		"setup":   func(a *App) error { _, err := a.SetupVault(""); return err },
		"unlock":  func(a *App) error { return a.UnlockVault("", "") },
	} {
		t.Run(name, func(t *testing.T) {
			app := &App{} // ensureAuthCoreServices falha antes de qualquer I/O.
			service, err := app.commandSecurityService()
			if err != nil {
				t.Fatal(err)
			}
			user, session := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
			old, err := service.Capture(context.Background(), user, session)
			if err != nil {
				t.Fatal(err)
			}
			if err := action(app); err == nil {
				t.Fatal("erro legado não preservado")
			}
			if err := service.Admit(context.Background(), old, func(context.Context) error { return nil }, func() error { t.Fatal("snapshot antigo admitido"); return nil }); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
				t.Fatal(err)
			}
			if _, err := service.Capture(context.Background(), user, session); err != nil {
				t.Fatal("barreira não encerrada", err)
			}
		})
	}
}

func TestCommandAuthLogoutHooksRealSessionWithoutKeychain(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "command-fixture", PasswordHash: "unused", IsActive: true, Role: database.UserRoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{RefreshTokenPepper: bytes.Repeat([]byte{2}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := sessions.IssueSession(ctx, &user, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	app := &App{ctx: ctx, identitySvc: auth.NewIdentityService(db), sessionSvc: sessions, vaultSvc: auth.NewVaultService(nil, nil), credMgr: credentials.NewManager(bytes.Repeat([]byte{3}, 32))}
	app.setCurrentUserID(user.ID)
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: pair.SessionID, Role: user.Role})
	service, err := app.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	old, err := service.Capture(ctx, user.ID, pair.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	cleanupCalled := false
	app.authKeyringDelete = func() error {
		cleanupCalled = true
		if _, err := service.Capture(ctx, user.ID, pair.SessionID); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
			t.Error("admissão aberta durante limpeza")
		}
		return nil
	}
	if err := app.Logout(LogoutRequest{RefreshToken: pair.RefreshToken}); err != nil {
		t.Fatal(err)
	}
	if !cleanupCalled {
		t.Fatal("limpeza não executada")
	}
	if _, err := sessions.AuthenticateLocalAccess(ctx, pair.AccessToken); !errors.Is(err, auth.ErrUnauthenticatedLocalSession) {
		t.Fatal("sessão não revogada")
	}
	if app.currentUserID != "" || app.currentAuthUser != nil {
		t.Fatal("identidade local não foi limpa")
	}
	if err := service.Admit(ctx, old, func(context.Context) error { return nil }, func() error { t.Fatal("handoff após logout"); return nil }); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatal(err)
	}
	if _, err := service.Capture(ctx, user.ID, pair.SessionID); err != nil {
		t.Fatal("barreira ficou aberta", err)
	} // Capture não autentica.
}

func TestCommandAuthRollbackNestedTransitionRemainsClosed(t *testing.T) {
	app := &App{authKeyringDelete: func() error { return nil }}
	service, err := app.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	finish := app.beginCommandAuthTransition()
	defer finish()
	app.rollbackLoginState("")
	if _, err := service.Capture(context.Background(), uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatal("rollback reabriu transição externa")
	}
}
