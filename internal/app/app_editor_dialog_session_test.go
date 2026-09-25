package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
)

const (
	editorDialogTestUser    = "01991f7c-1000-7000-8000-000000000101"
	editorDialogTestSession = "01991f7c-1000-7000-8000-000000000102"
)

func editorDialogSessionApp(t *testing.T) *App {
	t.Helper()
	db := setupAuthAppTestDB(t)
	if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: editorDialogTestUser}, Username: "editor-dialog", PasswordHash: "hash", Role: database.UserRoleUser, IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.Session{UUIDModel: database.UUIDModel{ID: editorDialogTestSession}, UserID: editorDialogTestUser, RefreshTokenHash: "editor-dialog-hash", ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{})
	if err != nil {
		t.Fatal(err)
	}
	a := &App{
		currentUserID: editorDialogTestUser,
		sessionSvc:    sessions,
		currentAuthUser: &AuthUser{
			UserID:    editorDialogTestUser,
			SessionID: editorDialogTestSession,
		},
	}
	return a
}

func TestCaptureEditorDialogSessionRevalidaOwnerESessao(t *testing.T) {
	a := editorDialogSessionApp(t)
	validate, err := a.captureEditorDialogSession(database.WithUserID(context.Background(), editorDialogTestUser))
	if err != nil {
		t.Fatal(err)
	}
	if err := validate(); err != nil {
		t.Fatalf("sessão válida recusada: %v", err)
	}

	a.authMu.Lock()
	a.currentAuthUser = &AuthUser{UserID: editorDialogTestUser, SessionID: "01991f7c-1000-7000-8000-000000000103"}
	a.authMu.Unlock()
	if err := validate(); !errors.Is(err, errEditorDialogSessionStale) && !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatalf("troca de sessão aceita: %v", err)
	}
}

func TestCaptureEditorDialogSessionRejectsContextFromPreviousOwner(t *testing.T) {
	a := editorDialogSessionApp(t)
	if validate, err := a.captureEditorDialogSession(database.WithUserID(context.Background(), "previous-owner")); err == nil || validate != nil {
		t.Fatal("contexto de outro usuário aceito")
	}
}

func TestCaptureEditorDialogSessionRejeitaTrocaDeUsuarioELogoutMesmoUsuario(t *testing.T) {
	a := editorDialogSessionApp(t)
	validate, err := a.captureEditorDialogSession(database.WithUserID(context.Background(), editorDialogTestUser))
	if err != nil {
		t.Fatal(err)
	}
	a.authMu.Lock()
	a.currentUserID = "01991f7c-1000-7000-8000-000000000104"
	a.currentAuthUser = &AuthUser{UserID: a.currentUserID, SessionID: "01991f7c-1000-7000-8000-000000000105"}
	a.authMu.Unlock()
	if err := validate(); !errors.Is(err, errEditorDialogSessionStale) && !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatalf("troca de usuário aceita: %v", err)
	}

	a = editorDialogSessionApp(t)
	validate, err = a.captureEditorDialogSession(database.WithUserID(context.Background(), editorDialogTestUser))
	if err != nil {
		t.Fatal(err)
	}
	a.setCurrentUserID("")
	a.setCurrentAuthUser(nil)
	if err := validate(); !errors.Is(err, errEditorDialogSessionStale) && !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatalf("logout aceito: %v", err)
	}
}

func TestCaptureEditorDialogSessionRejeitaRevogacaoDoEpoch(t *testing.T) {
	a := editorDialogSessionApp(t)
	validate, err := a.captureEditorDialogSession(database.WithUserID(context.Background(), editorDialogTestUser))
	if err != nil {
		t.Fatal(err)
	}
	epochs, err := a.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	if err := epochs.InvalidateSession(context.Background(), editorDialogTestUser, editorDialogTestSession); err != nil {
		t.Fatal(err)
	}
	if err := validate(); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatalf("epoch revogado aceito: %v", err)
	}
}
