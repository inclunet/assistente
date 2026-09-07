package app

import (
	"testing"

	"assistente/internal/profiles"
)

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
