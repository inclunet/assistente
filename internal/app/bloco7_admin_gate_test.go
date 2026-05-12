package app

import (
	"errors"
	"testing"

	"assistente/internal/database"
)

// TestRequireAdminContext_FailsPreLogin garante que `requireAdminContext`
// devolve `ErrUserScopeRequired` quando não há sessão autenticada — não
// importa o role: pré-login não há `AuthUser` e o gate fail-close.
//
// Cobre o blocker do Bloco 7 (ResetDatabase sem auth gate, descoberto
// durante o triage): qualquer caller pré-login que tente derrubar o DB
// recai no mesmo fluxo que qualquer outra binding pós-login normal.
func TestRequireAdminContext_FailsPreLogin(t *testing.T) {
	a := &App{}

	_, err := a.requireAdminContext()
	if err == nil {
		t.Fatal("requireAdminContext deveria falhar pré-login")
	}
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("erro esperado %v, got %v", database.ErrUserScopeRequired, err)
	}
}

// TestRequireAdminContext_FailsForRegularUser garante que mesmo um usuário
// autenticado (com `currentUserID` + `currentAuthUser` setados) é
// rejeitado quando o role não é admin. É o invariante central do
// Bloco 7: operações instance-wide (ResetDatabase) só passam para admins
// — em deployment multi-user, um usuário comum não derruba dados de
// todo mundo.
func TestRequireAdminContext_FailsForRegularUser(t *testing.T) {
	a := &App{}
	a.setCurrentUserID("user-123")
	a.setCurrentAuthUser(&AuthUser{
		UserID:    "user-123",
		SessionID: "sess-1",
		Role:      database.UserRoleUser,
	})

	_, err := a.requireAdminContext()
	if err == nil {
		t.Fatal("requireAdminContext deveria falhar para role=user")
	}
	if !errors.Is(err, ErrAdminRequired) {
		t.Fatalf("erro esperado %v, got %v", ErrAdminRequired, err)
	}
}

// TestRequireAdminContext_AllowsAdminUser garante o caminho positivo:
// usuário autenticado com role admin recebe um context com userID
// injetado (compatível com `database.UserIDFromContext`), pronto para ser
// propagado a repositórios fail-closed.
func TestRequireAdminContext_AllowsAdminUser(t *testing.T) {
	a := &App{}
	a.setCurrentUserID("admin-1")
	a.setCurrentAuthUser(&AuthUser{
		UserID:    "admin-1",
		SessionID: "sess-1",
		Role:      database.UserRoleAdmin,
	})

	ctx, err := a.requireAdminContext()
	if err != nil {
		t.Fatalf("admin deveria passar: %v", err)
	}
	uid, ok := database.UserIDFromContext(ctx)
	if !ok {
		t.Fatal("ctx devolvido por admin deveria carregar userID")
	}
	if uid != "admin-1" {
		t.Fatalf("userID esperado admin-1, got %q", uid)
	}
}

// TestResetDatabase_RejectsPreAuth garante que `ResetDatabase`,
// exposto via Wails Bind como método público de `*App`, recusa
// chamadas pré-login antes de tocar em qualquer arquivo. Sem isso
// o método silenciosamente derrubava o DB inteiro — descoberto
// durante triage do Bloco 7 (não estava no review original).
func TestResetDatabase_RejectsPreAuth(t *testing.T) {
	a := &App{}

	err := a.ResetDatabase()
	if err == nil {
		t.Fatal("ResetDatabase pré-login deveria falhar")
	}
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("erro esperado %v, got %v", database.ErrUserScopeRequired, err)
	}
}

// TestResetDatabase_RejectsRegularUser confirma que mesmo um usuário
// autenticado (não-admin) não consegue derrubar o DB. É o vetor
// concreto que torna `ResetDatabase` seguro em multi-user.
func TestResetDatabase_RejectsRegularUser(t *testing.T) {
	a := &App{}
	a.setCurrentUserID("user-123")
	a.setCurrentAuthUser(&AuthUser{
		UserID:    "user-123",
		SessionID: "sess-1",
		Role:      database.UserRoleUser,
	})

	err := a.ResetDatabase()
	if err == nil {
		t.Fatal("ResetDatabase para role=user deveria falhar")
	}
	if !errors.Is(err, ErrAdminRequired) {
		t.Fatalf("erro esperado %v, got %v", ErrAdminRequired, err)
	}
}
