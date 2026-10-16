package commandcatalog

import (
	"reflect"
	"testing"
)

func searchableRegistration(id string, p *Presentation) Registration {
	r := validRegistration()
	r.Definition.ID = id
	r.Definition.Presentation = p
	return r
}

func TestSearchUsaLocaleCamposAliasesENormalizacao(t *testing.T) {
	first := searchableRegistration("z.command", presentation())
	second := searchableRegistration("a.command", presentation())
	second.Definition.Presentation.Locales["pt-BR"] = LocalizedMetadata{Name: "Executar relatório", Description: "Gera relatório", Category: "Relatórios", Aliases: []string{"relatório"}}
	r, err := New([]Registration{first, second})
	if err != nil {
		t.Fatal(err)
	}
	got := r.Search("pt-BR", "  ABA   NOVA ")
	if len(got) != 1 {
		t.Fatalf("quantidade inesperada: %#v", got)
	}
	if ids := []string{got[0].ID}; !reflect.DeepEqual(ids, []string{"z.command"}) {
		t.Fatal(ids)
	}
	got = r.Search("pt-BR", "RELATÓRIO")
	if len(got) != 1 || got[0].ID != "a.command" {
		t.Fatalf("busca por campo/alias falhou: %#v", got)
	}
	if len(r.Search("en", "pestaña")) != 0 {
		t.Fatal("busca atravessou locale")
	}
}

func TestSearchRetornaIDsOrdenadosEVaziaParaEntradaInvalida(t *testing.T) {
	a := searchableRegistration("z.command", presentation())
	b := searchableRegistration("a.command", presentation())
	r, err := New([]Registration{a, b})
	if err != nil {
		t.Fatal(err)
	}
	got := r.Search("en", "tab")
	if len(got) != 2 || got[0].ID != "a.command" || got[1].ID != "z.command" {
		t.Fatalf("ordem não determinística: %#v", got)
	}
	for _, query := range []string{"", " \t\n"} {
		if got := r.Search("en", query); len(got) != 0 {
			t.Fatalf("query vazia retornou resultados: %#v", got)
		}
	}
	if got := r.Search("pt", "tab"); len(got) != 0 {
		t.Fatalf("locale inválido retornou resultados: %#v", got)
	}
}

func TestSearchEncontraCadaCampoIndividualmente(t *testing.T) {
	base := presentation()
	base.Locales["pt-BR"] = LocalizedMetadata{
		Name: "Nome exclusivo", Description: "Descrição exclusiva", Category: "Categoria exclusiva",
		Aliases: []string{"Alias exclusivo"},
	}
	r, err := New([]Registration{searchableRegistration("id.exclusivo", base)})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"id.exclusivo", "descrição exclusiva", "categoria exclusiva"} {
		got := r.Search("pt-BR", query)
		if len(got) != 1 || got[0].ID != "id.exclusivo" {
			t.Fatalf("campo %q não encontrado: %#v", query, got)
		}
	}
}

func TestSearchRetornaCopiaProfunda(t *testing.T) {
	r, err := New([]Registration{searchableRegistration("workspace.tab.new", presentation())})
	if err != nil {
		t.Fatal(err)
	}
	got := r.Search("pt-BR", "aba")
	if len(got) != 1 || got[0].Presentation == nil {
		t.Fatalf("resultado inesperado: %#v", got)
	}
	got[0].Presentation.Locales["pt-BR"] = LocalizedMetadata{Name: "alterado"}
	got[0].Presentation.Locales["en"].Aliases[0] = "alterado"
	again := r.Search("pt-BR", "aba")
	if len(again) != 1 || again[0].Presentation.Locales["pt-BR"].Name != "Nova aba" {
		t.Fatalf("mutação do mapa vazou para o registry: %#v", again)
	}
	if again := r.Search("en", "new tab"); len(again) != 1 || again[0].Presentation.Locales["en"].Aliases[0] != "new tab" {
		t.Fatalf("mutação do alias vazou para o registry: %#v", again)
	}
}

func TestLookupNuncaResolveAlias(t *testing.T) {
	r, err := New([]Registration{searchableRegistration("workspace.tab.new", presentation())})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Lookup("aba nova"); ok {
		t.Fatal("Lookup resolveu alias")
	}
}
