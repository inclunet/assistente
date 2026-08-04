package app

import (
	"strings"
	"testing"

	"assistente/internal/acptrust"
	"assistente/internal/profiles"
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

func TestClasseQueOAppNaoConheceAindaPodeSerRevogada(t *testing.T) {
	// A classe vai como o arquivo a guarda. Passá-la pelo conjunto conhecido
	// hoje transformaria em "other" o que uma versão futura (ou um arquivo
	// editado à mão) tivesse gravado, e a revogação então não casaria com a
	// entrada: a linha ficaria na tela, impossível de tirar. Quem exibe é que
	// traduz o desconhecido para a frase genérica.
	a := appComAutorizacoes(t)
	if err := a.acpTrust.Allow("cursor", "classe-nova"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	lista := a.GetAgentPermissions()

	if len(lista) != 1 || lista[0].Action != "classe-nova" {
		t.Fatalf("classe na tela = %+v, quer a que está guardada", lista)
	}
	if err := a.RevokeAgentPermission(lista[0].ProfileSlug, lista[0].Action); err != nil {
		t.Errorf("revogar o que a tela mostra: %v", err)
	}
	if len(a.GetAgentPermissions()) != 0 {
		t.Error("a autorização continuou na lista depois de revogada")
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

func TestNomeDoPerfilCasaComOSlugQueOArquivoGuarda(t *testing.T) {
	// O arquivo é nomeado pelo slug saneado. Comparar com o slug cru faria um
	// perfil escrito com maiúsculas não reconhecer a própria linha, que
	// apareceria pelo slug como se ele tivesse sido apagado.
	nomes := profileNamesFrom([]profiles.ProfileInfo{
		{Slug: "Cursor", Name: "Agente de código"},
		{Slug: "sem-nome", Name: "   "},
	})

	if nomes["cursor"] != "Agente de código" {
		t.Errorf("nome do perfil = %q, quer o nome que a pessoa deu", nomes["cursor"])
	}
	// Nome em branco não é nome: a tela cai no slug, que ao menos identifica a
	// linha para quem vai revogá-la.
	if _, temNome := nomes["sem-nome"]; temNome {
		t.Error("nome em branco entrou como nome do perfil")
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

	lista := a.GetAgentPermissions()

	if len(lista) != 0 {
		t.Errorf("autorizações na tela = %+v, quer nenhuma", lista)
	}
	// Lista vazia é lista: nula chegaria à interface como null, e o tipo
	// gerado promete um array.
	if lista == nil {
		t.Error("a lista vazia foi entregue como ausência de lista")
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
