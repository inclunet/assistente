package credentials

import (
	"context"
	"testing"

	"assistente/internal/database"
)

// TestManager_LoadInstanceSecrets_DoesNotLeakUserCredentials garante o
// contrato do caminho pré-login: `LoadInstanceSecrets` só hidrata
// instance secrets (`internal-auth:*`/`internal-tls:*`), NUNCA
// credenciais user-scoped — mesmo que o store contenha várias.
//
// É o teste que protege a separação responsável por evitar o incident
// de 11/05/2026: na versão antiga, `LoadFromStore(context.Background())`
// caía em fallback para instance-only sem ninguém perceber, e os
// callers do Login achavam que estavam carregando "tudo". A nova API
// força o caller a escolher; este teste prova que a escolha
// "instance" não vaza nada de user.
func TestManager_LoadInstanceSecrets_DoesNotLeakUserCredentials(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	store := NewDBStore()
	key := []byte("test-key-exactly-32-bytes-long!!")

	seedMgr := NewManagerWithStoreAndPersistence(key, store, true)
	userCtx := database.WithUserID(context.Background(), "user-1")
	if err := seedMgr.RegisterPatternWithContext(userCtx, "ist-prod-litellm.nullmplatform.com", &AuthConfig{
		Type:  "bearer",
		Token: "sk-user-1",
	}); err != nil {
		t.Fatalf("seed user-scoped credential: %v", err)
	}
	if err := seedMgr.RegisterInstanceSecret(InstanceSecretAuthRefreshToken, "instance-refresh"); err != nil {
		t.Fatalf("seed instance secret: %v", err)
	}

	mgr := NewManagerWithStoreAndPersistence(key, store, true)
	if err := mgr.LoadInstanceSecrets(context.Background()); err != nil {
		t.Fatalf("LoadInstanceSecrets: %v", err)
	}

	tok, ok, err := mgr.GetInstanceSecret(InstanceSecretAuthRefreshToken)
	if err != nil {
		t.Fatalf("GetInstanceSecret: %v", err)
	}
	if !ok || tok != "instance-refresh" {
		t.Fatalf("instance secret não carregou: ok=%v tok=%q", ok, tok)
	}

	auth, err := mgr.GetByPatternWithContext(userCtx, "ist-prod-litellm.nullmplatform.com")
	if err != nil {
		t.Fatalf("GetByPatternWithContext: %v", err)
	}
	if auth != nil {
		t.Fatalf("LoadInstanceSecrets vazou credencial user-scoped para a memória: %+v", auth)
	}
}

// TestManager_LoadUserCredentials_HydratesAllUserCredentials valida o
// caminho pós-Login: `LoadUserCredentials(userID)` carrega TODAS as
// credenciais do user pedido — o que faltava no incident de 11/05/2026
// e é exatamente o que `ResolveForURLWithContext` precisa achar em
// memória depois.
//
// Inclui assertion de cross-user: carregar credenciais de "ana" não
// deve vazar para o escopo de "leo", e vice-versa.
func TestManager_LoadUserCredentials_HydratesAllUserCredentials(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	store := NewDBStore()
	key := []byte("test-key-exactly-32-bytes-long!!")

	anaCtx := database.WithUserID(context.Background(), "user-ana")
	leoCtx := database.WithUserID(context.Background(), "user-leo")

	patterns := []string{
		"api.openai.com",
		"api.anthropic.com",
		"ist-prod-litellm.nullmplatform.com",
	}
	seedMgr := NewManagerWithStoreAndPersistence(key, store, true)
	for _, p := range patterns {
		if err := seedMgr.RegisterPatternWithContext(anaCtx, p, &AuthConfig{Type: "bearer", Token: "ana-" + p}); err != nil {
			t.Fatalf("seed ana %s: %v", p, err)
		}
		if err := seedMgr.RegisterPatternWithContext(leoCtx, p, &AuthConfig{Type: "bearer", Token: "leo-" + p}); err != nil {
			t.Fatalf("seed leo %s: %v", p, err)
		}
	}

	mgr := NewManagerWithStoreAndPersistence(key, store, true)
	if err := mgr.LoadUserCredentials(context.Background(), "user-ana"); err != nil {
		t.Fatalf("LoadUserCredentials(ana): %v", err)
	}

	for _, p := range patterns {
		got, err := mgr.GetByPatternWithContext(anaCtx, p)
		if err != nil {
			t.Fatalf("ana get %s: %v", p, err)
		}
		if got == nil || got.Token != "ana-"+p {
			t.Fatalf("ana %s: got %+v want token=ana-%s", p, got, p)
		}
		leoSeen, err := mgr.GetByPatternWithContext(leoCtx, p)
		if err != nil {
			t.Fatalf("leo get %s: %v", p, err)
		}
		if leoSeen != nil {
			t.Fatalf("cross-user leak: leo viu credencial de %s antes do próprio LoadUserCredentials: %+v", p, leoSeen)
		}
	}

	if err := mgr.LoadUserCredentials(context.Background(), "user-leo"); err != nil {
		t.Fatalf("LoadUserCredentials(leo): %v", err)
	}

	for _, p := range patterns {
		anaAuth, _ := mgr.GetByPatternWithContext(anaCtx, p)
		leoAuth, _ := mgr.GetByPatternWithContext(leoCtx, p)
		if anaAuth == nil || anaAuth.Token != "ana-"+p {
			t.Fatalf("ana %s perdeu credencial após leo carregar: got %+v", p, anaAuth)
		}
		if leoAuth == nil || leoAuth.Token != "leo-"+p {
			t.Fatalf("leo %s: got %+v want token=leo-%s", p, leoAuth, p)
		}
	}
}

// TestManager_LoadUserCredentials_RejectsEmptyUserID prova que a API
// nova não aceita o equivalente do bug antigo: chamar
// `LoadUserCredentials` sem userID falha imediatamente em vez de
// silenciosamente carregar instance-only.
func TestManager_LoadUserCredentials_RejectsEmptyUserID(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)

	mgr := NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), NewDBStore(), true)
	err := mgr.LoadUserCredentials(context.Background(), "")
	if err == nil {
		t.Fatal("LoadUserCredentials(\"\") deveria falhar — userID é obrigatório")
	}
}
