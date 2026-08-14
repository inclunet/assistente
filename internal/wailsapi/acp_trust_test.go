package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/acptrust"
)

func TestACPTrustNotWired(t *testing.T) {
	t.Parallel()
	api := NewACPTrust()
	if _, err := api.GetAgentPermissions(); !errors.Is(err, ErrACPTrustNotWired) {
		t.Fatalf("GetAgentPermissions: got %v", err)
	}
	if err := api.RevokeAgentPermission("cursor", "execute"); !errors.Is(err, ErrACPTrustNotWired) {
		t.Fatalf("RevokeAgentPermission: got %v", err)
	}
}

func TestACPTrustUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "acp_trust.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("acp_trust.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("acp_trust.go deve chamar WithUser(")
	}
}

func trustAPI(tb testing.TB, names map[string]string) (*ACPTrust, *acptrust.Store) {
	tb.Helper()
	store := acptrust.NewStoreWithDir(tb.TempDir())
	api := NewACPTrust()
	AttachACPTrust(api, stubSession{ctx: context.Background()}, store, func() map[string]string {
		return names
	})
	return api, store
}

func TestATelaMostraOQueCadaPerfilAutorizou(t *testing.T) {
	t.Parallel()
	api, store := trustAPI(t, nil)
	if err := store.Allow("cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}
	if err := store.Allow("claude-code", "read"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	lista, err := api.GetAgentPermissions()
	if err != nil {
		t.Fatalf("GetAgentPermissions: %v", err)
	}

	if len(lista) != 2 {
		t.Fatalf("autorizações na tela = %d, quer 2", len(lista))
	}
	if lista[0].ProfileSlug != "claude-code" || lista[1].ProfileSlug != "cursor" {
		t.Errorf("ordem = %q, quer estável por perfil", []string{lista[0].ProfileSlug, lista[1].ProfileSlug})
	}
	if lista[1].Action != "execute" {
		t.Errorf("classe = %q, quer execute", lista[1].Action)
	}
	if lista[1].GrantedAt == "" {
		t.Error("a autorização não diz quando foi concedida")
	}
}

func TestClasseQueOAppNaoConheceAindaPodeSerRevogada(t *testing.T) {
	t.Parallel()
	api, store := trustAPI(t, nil)
	if err := store.Allow("cursor", "classe-nova"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	lista, err := api.GetAgentPermissions()
	if err != nil {
		t.Fatalf("GetAgentPermissions: %v", err)
	}

	if len(lista) != 1 || lista[0].Action != "classe-nova" {
		t.Fatalf("classe na tela = %+v, quer a que está guardada", lista)
	}
	if err := api.RevokeAgentPermission(lista[0].ProfileSlug, lista[0].Action); err != nil {
		t.Errorf("revogar o que a tela mostra: %v", err)
	}
	lista, err = api.GetAgentPermissions()
	if err != nil {
		t.Fatalf("GetAgentPermissions após revogar: %v", err)
	}
	if len(lista) != 0 {
		t.Error("a autorização continuou na lista depois de revogada")
	}
}

func TestPerfilApagadoAindaAparecePeloSlug(t *testing.T) {
	t.Parallel()
	api, store := trustAPI(t, nil)
	if err := store.Allow("perfil-que-nao-existe-mais", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	lista, err := api.GetAgentPermissions()
	if err != nil {
		t.Fatalf("GetAgentPermissions: %v", err)
	}

	if len(lista) != 1 {
		t.Fatalf("autorizações na tela = %d, quer 1", len(lista))
	}
	if lista[0].ProfileName != "" {
		t.Errorf("nome = %q, quer vazio: a tela mostra o slug", lista[0].ProfileName)
	}
	if lista[0].ProfileSlug != "perfil-que-nao-existe-mais" {
		t.Errorf("slug = %q, quer o do perfil apagado", lista[0].ProfileSlug)
	}
}

func TestRevogarTiraAAutorizacaoDaLista(t *testing.T) {
	t.Parallel()
	api, store := trustAPI(t, nil)
	if err := store.Allow("cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if err := api.RevokeAgentPermission("cursor", "execute"); err != nil {
		t.Fatalf("revogar: %v", err)
	}

	lista, err := api.GetAgentPermissions()
	if err != nil {
		t.Fatalf("GetAgentPermissions: %v", err)
	}
	if len(lista) != 0 {
		t.Errorf("autorizações na tela = %+v, quer nenhuma", lista)
	}
	if store.Allows("cursor", "execute") {
		t.Error("o agente continua autorizado depois da revogação")
	}
}

func TestRevogarOQueNaoExisteNaoDizQueFechouAPorta(t *testing.T) {
	t.Parallel()
	api, _ := trustAPI(t, nil)

	err := api.RevokeAgentPermission("cursor", "execute")

	if err == nil {
		t.Fatal("disse ter revogado uma autorização que não existia")
	}
	if !errors.Is(err, ErrAgentPermissionNotFound) {
		t.Errorf("erro = %v, quer ErrAgentPermissionNotFound", err)
	}
	// A interface traduz esse código; se ele virar frase, a tela volta a exibir
	// português para quem escolheu outro idioma.
	if err.Error() != "agent_permission_not_found" {
		t.Errorf("mensagem = %q, quer o código estável agent_permission_not_found", err)
	}
}

func TestSemAutorizacaoNenhumaATelaAbreVazia(t *testing.T) {
	t.Parallel()
	api, _ := trustAPI(t, nil)

	lista, err := api.GetAgentPermissions()
	if err != nil {
		t.Fatalf("GetAgentPermissions: %v", err)
	}

	if len(lista) != 0 {
		t.Errorf("autorizações na tela = %+v, quer nenhuma", lista)
	}
	if lista == nil {
		t.Error("a lista vazia foi entregue como ausência de lista")
	}
}

func TestACPTrustAuthError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("sem sessão")
	store := acptrust.NewStoreWithDir(t.TempDir())
	api := NewACPTrust()
	AttachACPTrust(api, stubSession{err: wantErr}, store, nil)

	if _, err := api.GetAgentPermissions(); !errors.Is(err, wantErr) {
		t.Fatalf("GetAgentPermissions: got %v, want %v", err, wantErr)
	}
	if err := api.RevokeAgentPermission("cursor", "execute"); !errors.Is(err, wantErr) {
		t.Fatalf("RevokeAgentPermission: got %v, want %v", err, wantErr)
	}
}
