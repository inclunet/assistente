package commandcatalog

import (
	"reflect"
	"sync"
	"testing"
)

func TestApresentacaoRecusaLocaleDesconhecidoMesmoComTresEntradas(t *testing.T) {
	p := presentation()
	delete(p.Locales, "es")
	p.Locales["fr"] = LocalizedMetadata{
		Name: "Nouvel onglet", Description: "Crée un onglet", Category: "Espace",
	}
	r := validRegistration()
	r.Definition.Presentation = p

	if _, err := New([]Registration{r}); err == nil {
		t.Fatal("aceitou locale desconhecido com exatamente três entradas")
	}
}

func TestApresentacaoRecusaNomeDescricaoECategoriaAusentes(t *testing.T) {
	for _, field := range []string{"Name", "Description", "Category"} {
		t.Run(field, func(t *testing.T) {
			p := presentation()
			metadata := p.Locales["pt-BR"]
			switch field {
			case "Name":
				metadata.Name = " \t"
			case "Description":
				metadata.Description = "\n"
			case "Category":
				metadata.Category = " "
			}
			p.Locales["pt-BR"] = metadata
			r := validRegistration()
			r.Definition.Presentation = p
			if _, err := New([]Registration{r}); err == nil {
				t.Fatalf("aceitou %s ausente", field)
			}
		})
	}
}

func TestApresentacaoRecusaAliasesDuplicadosAposNormalizacao(t *testing.T) {
	p := presentation()
	metadata := p.Locales["pt-BR"]
	metadata.Aliases = []string{"  Abrir\taba ", "abrir aba"}
	p.Locales["pt-BR"] = metadata
	r := validRegistration()
	r.Definition.Presentation = p

	if _, err := New([]Registration{r}); err == nil {
		t.Fatal("aceitou aliases equivalentes após normalização")
	}
}

func TestNewIsolaMapaEListaDeAliasesDaEntrada(t *testing.T) {
	p := presentation()
	aliases := p.Locales["pt-BR"].Aliases
	r := validRegistration()
	r.Definition.Presentation = p
	registry, err := New([]Registration{r})
	if err != nil {
		t.Fatal(err)
	}

	p.Locales["pt-BR"] = LocalizedMetadata{Name: "entrada alterada"}
	aliases[0] = "entrada alterada"
	got, ok := registry.Lookup(r.Definition.ID)
	if !ok || got.Presentation.Locales["pt-BR"].Name != "Nova aba" || got.Presentation.Locales["pt-BR"].Aliases[0] != "aba nova" {
		t.Fatalf("entrada mutada vazou para o snapshot: %#v", got)
	}
}

func TestListESearchIsolamProfundamenteTodasAsSaidas(t *testing.T) {
	r, err := New([]Registration{
		searchableRegistration("workspace.tab.new", presentation()),
		searchableRegistration("chat.model.select", presentation()),
	})
	if err != nil {
		t.Fatal(err)
	}

	wantChat, _ := r.Lookup("chat.model.select")
	wantWorkspace, _ := r.Lookup("workspace.tab.new")
	list := r.List()
	list[0].AllowedSources[0] = System
	list[0].Context.Facts[0].Provider = "alterado"
	list[0].Presentation.Locales["pt-BR"].Aliases[0] = "alterado"
	search := r.Search("pt-BR", "workspace.tab.new")
	if len(search) != 1 {
		t.Fatalf("resultado inesperado: %#v", search)
	}
	search[0].AllowedSources[0] = System
	search[0].Context.Facts[0].Provider = "alterado"
	search[0].Presentation.Locales["pt-BR"].Aliases[0] = "alterado"

	gotChat, _ := r.Lookup("chat.model.select")
	gotWorkspace, _ := r.Lookup("workspace.tab.new")
	if !reflect.DeepEqual(gotChat, wantChat) {
		t.Fatalf("mutação de List vazou para chat.model.select: %#v", gotChat)
	}
	if !reflect.DeepEqual(gotWorkspace, wantWorkspace) {
		t.Fatalf("mutação de Search vazou para workspace.tab.new: %#v", gotWorkspace)
	}
}

func TestSearchNaoAutorizaNemModificaAllowedSources(t *testing.T) {
	r := validRegistration()
	r.Definition.Presentation = presentation()
	registry, err := New([]Registration{r})
	if err != nil {
		t.Fatal(err)
	}

	got := registry.Search("pt-BR", "nova")
	if len(got) != 1 || !got[0].AllowsSource(UI) || got[0].AllowsSource(System) {
		t.Fatalf("AllowedSources inesperadas: %#v", got)
	}
	got[0].AllowedSources[0] = System
	definition, _ := registry.Lookup(r.Definition.ID)
	if !definition.AllowsSource(UI) || definition.AllowsSource(System) {
		t.Fatal("Search modificou ou ampliou AllowedSources")
	}
}

func TestSearchOmiteMetadataOpcionalMasLookupExatoDisponivel(t *testing.T) {
	r := validRegistration()
	registry, err := New([]Registration{r})
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.Search("pt-BR", r.Definition.ID); len(got) != 0 {
		t.Fatalf("Search expôs comando sem metadata: %#v", got)
	}
	got, ok := registry.Lookup(r.Definition.ID)
	if !ok || got.ID != r.Definition.ID || got.Presentation != nil {
		t.Fatalf("Lookup exato não preservou definição: %#v, %v", got, ok)
	}
}

func TestSearchNormalizaCaixaEspacosUnicodeSemAccentFolding(t *testing.T) {
	p := presentation()
	metadata := p.Locales["pt-BR"]
	metadata.Name = "  Água\tFria  "
	p.Locales["pt-BR"] = metadata
	r, err := New([]Registration{searchableRegistration("water.command", p)})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Search("pt-BR", "  ÁGUA   FRIA "); len(got) != 1 {
		t.Fatalf("caixa/espaços Unicode não normalizados: %#v", got)
	}
	if got := r.Search("pt-BR", "agua fria"); len(got) != 0 {
		t.Fatalf("busca aplicou accent folding inesperado: %#v", got)
	}
}

func TestLeiturasDoRegistrySaoSegurasEmConcorrencia(t *testing.T) {
	r, err := New([]Registration{
		searchableRegistration("workspace.tab.new", presentation()),
		searchableRegistration("chat.model.select", presentation()),
	})
	if err != nil {
		t.Fatal(err)
	}

	const readers = 16
	const iterations = 100
	var wg sync.WaitGroup
	errs := make(chan string, readers*iterations*3)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if got := r.List(); len(got) != 2 {
					errs <- "List retornou quantidade inesperada"
				}
				if got := r.Search("en", "tab"); len(got) != 2 {
					errs <- "Search retornou quantidade inesperada"
				}
				if got, ok := r.Lookup("workspace.tab.new"); !ok || got.ID != "workspace.tab.new" {
					errs <- "Lookup falhou"
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
