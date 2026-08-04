package app

import (
	"strings"
	"testing"

	"assistente/internal/acptrust"
)

// appComAutorizacoes monta o app só com o que a tela de autorizações usa.
func appComAutorizacoes(tb testing.TB) *App {
	tb.Helper()
	return &App{acpTrust: acptrust.NewStoreWithDir(tb.TempDir())}
}

func TestATelaMostraOQueCadaPerfilAutorizou(t *testing.T) {
	a := appComAutorizacoes(t)
	if err := a.acpTrust.Allow("cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}
	if err := a.acpTrust.Allow("claude-code", "read"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	lista := a.GetAgentPermissions()

	if len(lista) != 2 {
		t.Fatalf("autorizações na tela = %d, quer 2", len(lista))
	}
	// Ordem estável: quem lê por leitor de telas percorre a lista em
	// sequência, e uma ordem que muda a cada carregamento faz procurar de novo
	// o que acabou de ser encontrado.
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

func TestClasseQueNinguemReconheceNaoViraCodigoCruNaTela(t *testing.T) {
	// O que o agente inventou passa pelo conjunto do protocolo antes de virar
	// linha na tela, como em todo o resto do D11.
	a := appComAutorizacoes(t)
	if err := a.acpTrust.Allow("cursor", "faz-tudo"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	lista := a.GetAgentPermissions()

	if len(lista) != 1 || lista[0].Action != "other" {
		t.Errorf("classe na tela = %+v, quer other", lista)
	}
}

func TestPerfilApagadoAindaAparecePeloSlug(t *testing.T) {
	// A autorização sobrevive ao perfil, e continuaria valendo se ele voltasse
	// com o mesmo slug. Esconder a linha por falta de nome deixaria a pessoa
	// sem como revogá-la.
	a := appComAutorizacoes(t)
	if err := a.acpTrust.Allow("perfil-que-nao-existe-mais", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	lista := a.GetAgentPermissions()

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
	a := appComAutorizacoes(t)
	if err := a.acpTrust.Allow("cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if err := a.RevokeAgentPermission("cursor", "execute"); err != nil {
		t.Fatalf("revogar: %v", err)
	}

	if lista := a.GetAgentPermissions(); len(lista) != 0 {
		t.Errorf("autorizações na tela = %+v, quer nenhuma", lista)
	}
	if a.acpTrust.Allows("cursor", "execute") {
		t.Error("o agente continua autorizado depois da revogação")
	}
}

func TestRevogarOQueNaoExisteNaoDizQueFechouAPorta(t *testing.T) {
	a := appComAutorizacoes(t)

	err := a.RevokeAgentPermission("cursor", "execute")

	if err == nil {
		t.Fatal("disse ter revogado uma autorização que não existia")
	}
	if !strings.Contains(err.Error(), "não existe") {
		t.Errorf("erro = %q, quer dizer que a autorização não existe mais", err)
	}
}

func TestSemAutorizacaoNenhumaATelaAbreVazia(t *testing.T) {
	a := appComAutorizacoes(t)

	if lista := a.GetAgentPermissions(); len(lista) != 0 {
		t.Errorf("autorizações na tela = %+v, quer nenhuma", lista)
	}
}

func TestAppSemArmazenamentoNaoQuebraATela(t *testing.T) {
	// Inicialização parcial (testes, app subindo) não pode derrubar a tela nem
	// fingir que revogou algo.
	a := &App{}

	if lista := a.GetAgentPermissions(); lista != nil {
		t.Errorf("autorizações = %+v, quer nenhuma", lista)
	}
	if err := a.RevokeAgentPermission("cursor", "execute"); err == nil {
		t.Error("disse ter revogado sem ter onde revogar")
	}
}
