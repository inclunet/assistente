package credentials

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
)

// TestDBStore_DeleteCredential_RejectsEmptyPattern blinda o invariante
// do incident report AEP-0053 (10/05/2026): `DeleteCredential("")` é
// SEMPRE um bug do caller. Falhar fechado evita que (a) "limpar tudo"
// seja confundido com `pattern=""` (silent no-op), (b) refatorações
// futuras que aceitem wildcards se tornem vetor de mass-delete.
func TestDBStore_DeleteCredential_RejectsEmptyPattern(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	store := NewDBStore()
	ctx := database.WithUserID(context.Background(), "user-1")
	cred := StoredCredential{
		Pattern: "api.openai.com",
		Auth:    &AuthConfig{Type: "bearer", Token: "secret"},
	}
	if err := store.SaveCredential(ctx, cred); err != nil {
		t.Fatalf("setup credential: %v", err)
	}

	if err := store.DeleteCredential(ctx, ""); !errors.Is(err, ErrEmptyPatternDelete) {
		t.Fatalf("expected ErrEmptyPatternDelete, got %v", err)
	}
	if err := store.DeleteCredential(ctx, "   "); !errors.Is(err, ErrEmptyPatternDelete) {
		t.Fatalf("expected ErrEmptyPatternDelete for whitespace pattern, got %v", err)
	}

	creds, err := store.ListCredentials(ctx)
	if err != nil {
		t.Fatalf("list credentials: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("expected credential to survive empty-pattern delete attempt, got %d rows", len(creds))
	}
}

// TestDBStore_SaveCredential_RejectsEmptyPattern garante simetria:
// não dá para gravar uma row com pattern vazio (que viraria target de
// um eventual bug de mass-delete).
func TestDBStore_SaveCredential_RejectsEmptyPattern(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	store := NewDBStore()
	ctx := database.WithUserID(context.Background(), "user-1")

	err := store.SaveCredential(ctx, StoredCredential{
		Pattern: "",
		Auth:    &AuthConfig{Type: "bearer", Token: "secret"},
	})
	if err == nil {
		t.Fatal("expected error for empty pattern in SaveCredential")
	}
	err = store.SaveCredential(ctx, StoredCredential{
		Pattern: "  ",
		Auth:    &AuthConfig{Type: "bearer", Token: "secret"},
	})
	if err == nil {
		t.Fatal("expected error for whitespace pattern in SaveCredential")
	}
}

// TestDBStore_DeleteCredential_DoesNotAffectOtherUsers blinda o
// invariante de scope: deletar pattern com user_id no contexto NUNCA
// pode tocar credenciais de outro usuário com o mesmo pattern. Esse é
// o invariante mais crítico do AEP-0052: um caller comprometido não
// consegue derrubar credenciais de outros usuários via pattern guess.
func TestDBStore_DeleteCredential_DoesNotAffectOtherUsers(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	store := NewDBStore()
	anaCtx := database.WithUserID(context.Background(), "user-ana")
	leoCtx := database.WithUserID(context.Background(), "user-leo")
	pattern := "api.openai.com"
	cred := StoredCredential{Pattern: pattern, Auth: &AuthConfig{Type: "bearer", Token: "x"}}

	if err := store.SaveCredential(anaCtx, cred); err != nil {
		t.Fatalf("save ana: %v", err)
	}
	if err := store.SaveCredential(leoCtx, cred); err != nil {
		t.Fatalf("save leo: %v", err)
	}

	if err := store.DeleteCredential(anaCtx, pattern); err != nil {
		t.Fatalf("delete ana: %v", err)
	}

	leoCreds, err := store.ListCredentials(leoCtx)
	if err != nil {
		t.Fatalf("list leo: %v", err)
	}
	if len(leoCreds) != 1 {
		t.Fatalf("leo credential desapareceu após ana deletar mesmo pattern; rows=%d (%+v)", len(leoCreds), leoCreds)
	}
}

// TestDBStore_DeleteCredential_InstanceSecretScopedToInstance valida
// que instance secrets (`internal-auth:*`/`internal-tls:*`) só são
// deletados na linha com `user_id=''`. Um delete via user-scoped ctx
// não pode tocar a row instance-scoped.
func TestDBStore_DeleteCredential_InstanceSecretScopedToInstance(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	store := NewDBStore()
	pattern := "internal-auth:refresh-token"
	cred := StoredCredential{Pattern: pattern, Auth: &AuthConfig{Type: "bearer", Token: "secret"}}

	instanceCtx := context.Background()
	if err := store.SaveCredential(instanceCtx, cred); err != nil {
		t.Fatalf("save instance: %v", err)
	}

	userCtx := database.WithUserID(context.Background(), "user-leo")
	if err := store.DeleteCredential(userCtx, pattern); err != nil {
		t.Fatalf("delete instance via user ctx: %v", err)
	}

	instanceCreds, err := store.ListInstanceCredentials(instanceCtx)
	if err != nil {
		t.Fatalf("list instance: %v", err)
	}
	if len(instanceCreds) != 0 {
		t.Fatalf("expected instance secret to be removed (IsInstanceSecretPattern path), got %+v", instanceCreds)
	}
}

// TestDBStore_DeleteCredential_UnauthenticatedCannotDeleteUserScoped
// valida o invariante anti-mass-delete: um caller sem user no contexto
// NUNCA consegue apagar credenciais user-scoped (mesmo órfãs). Isso é
// fundamental para que callers de bootstrap não consigam derrubar
// credenciais legacy por engano: a única forma de adoção é via
// AdoptLegacyData (que faz UPDATE do user_id, não DELETE).
func TestDBStore_DeleteCredential_UnauthenticatedCannotDeleteUserScoped(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	if err := database.DB().Create(&database.CredentialEntry{
		Pattern:  "api.openai.com",
		AuthType: "bearer",
		TokenEnc: "valuable-token",
	}).Error; err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	store := NewDBStore()
	err := store.DeleteCredential(context.Background(), "api.openai.com")
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("esperava ErrUserScopeRequired (delete sem ctx user é bloqueado), got %v", err)
	}

	var count int64
	if err := database.DB().Model(&database.CredentialEntry{}).
		Where("pattern = ?", "api.openai.com").
		Count(&count).Error; err != nil {
		t.Fatalf("count orphans: %v", err)
	}
	if count != 1 {
		t.Fatalf("órfã deveria sobreviver à tentativa anônima de delete, %d rows remain", count)
	}
}
