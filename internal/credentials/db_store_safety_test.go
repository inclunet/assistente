package credentials

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
)

// TestDBStore_DeleteCredential_RejectsEmptyPattern documenta o
// contrato: `DeleteCredential("")` é sempre erro do caller. "Limpar
// tudo" tem que ser expressado como iteração sobre a lista visível,
// não como uma chamada sem nome que pode ser confundida com no-op ou,
// no futuro, com mass-delete por wildcard.
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

// TestDBStore_SaveCredential_RejectsEmptyPattern garante simetria com
// DeleteCredential: pattern é parte da identidade da credencial, não
// pode ser vazio.
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

// TestDBStore_DeleteCredential_DoesNotAffectOtherUsers prova o
// isolamento por user: deletar com user_id no contexto NUNCA toca em
// credenciais de outro usuário com o mesmo pattern (AEP-0052).
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
// prova o contrato: sem userID no contexto, ninguém apaga credenciais
// user-scoped (nem legacy órfãs). A única adoção válida é via
// `AdoptLegacyData`, que faz UPDATE do `user_id` — nunca DELETE.
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
