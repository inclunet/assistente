package commandcatalog

import (
	"reflect"
	"testing"
)

func validRegistration() Registration {
	return Registration{Definition: Definition{ID: "workspace.tab.new", Effect: Write, Decision: NoDecision,
		HasMutableTarget: true, AllowedSources: []Source{UI, KeyboardLocal}, Context: ContextPolicy{Facts: []ContextFact{
			{Provider: "workspace", Fact: "active_tab", Mode: ExactVersion},
		}}}, Handler: HandlerContract{Effect: Write}}
}

func TestRegistroRecusaContratosInseguros(t *testing.T) {
	tests := map[string]func(*Registration){
		"id sem namespace":        func(r *Registration) { r.Definition.ID = "new" },
		"id aproximado":           func(r *Registration) { r.Definition.ID = " workspace.tab.new" },
		"efeito desconhecido":     func(r *Registration) { r.Definition.Effect = "unknown" },
		"decisão desconhecida":    func(r *Registration) { r.Definition.Decision = "unknown" },
		"efeito divergente":       func(r *Registration) { r.Handler.Effect = Read },
		"mutabilidade divergente": func(r *Registration) { r.Handler.MutatesEffectiveCapability = true },
		"origens ausentes":        func(r *Registration) { r.Definition.AllowedSources = nil },
		"origem desconhecida":     func(r *Registration) { r.Definition.AllowedSources = []Source{"desktop"} },
		"origem duplicada":        func(r *Registration) { r.Definition.AllowedSources = []Source{UI, UI} },
		"escrita sem contexto":    func(r *Registration) { r.Definition.Context = ContextPolicy{None: true} },
		"contexto não declarado":  func(r *Registration) { r.Definition.Context = ContextPolicy{} },
		"none com facts":          func(r *Registration) { r.Definition.Context.None = true },
		"provider ausente":        func(r *Registration) { r.Definition.Context.Facts[0].Provider = "" },
		"fato ausente":            func(r *Registration) { r.Definition.Context.Facts[0].Fact = "" },
		"modo desconhecido":       func(r *Registration) { r.Definition.Context.Facts[0].Mode = "latest" },
		"TTL em versão exata":     func(r *Registration) { r.Definition.Context.Facts[0].MaxAgeMS = 1 },
		"TTL ausente":             func(r *Registration) { r.Definition.Context.Facts[0].Mode = MaxAge },
		"snapshot sem TTL":        func(r *Registration) { r.Definition.Context.Facts[0].Mode = EventSnapshot },
		"fato duplicado": func(r *Registration) {
			r.Definition.Context.Facts = append(r.Definition.Context.Facts, r.Definition.Context.Facts[0])
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			r := validRegistration()
			change(&r)
			if _, err := New([]Registration{r}); err == nil {
				t.Fatal("contrato inválido aceito")
			}
		})
	}
}

func TestDestrutivoExigeInteracaoEExcluiTodaOrigemHeadless(t *testing.T) {
	for _, source := range []Source{CLI, Event, System} {
		r := validRegistration()
		r.Definition.Effect = Destructive
		r.Handler.Effect = Destructive
		r.Definition.Decision = Interactive
		r.Definition.AllowedSources = append(r.Definition.AllowedSources, source)
		if _, err := New([]Registration{r}); err == nil {
			t.Fatalf("aceitou %s misturada com UI", source)
		}
	}
	r := validRegistration()
	r.Definition.Effect = Destructive
	r.Handler.Effect = Destructive
	if _, err := New([]Registration{r}); err == nil {
		t.Fatal("aceitou destrutivo sem decisão")
	}
	r.Definition.Decision = Interactive
	if _, err := New([]Registration{r}); err != nil {
		t.Fatal(err)
	}
}

func TestLeituraSemContextoSomenteSemAlvoOuMutabilidade(t *testing.T) {
	r := validRegistration()
	r.Definition.Effect = Read
	r.Handler.Effect = Read
	r.Definition.Context = ContextPolicy{None: true}
	r.Definition.HasMutableTarget = false
	if _, err := New([]Registration{r}); err != nil {
		t.Fatal(err)
	}
	r.Definition.HasMutableTarget = true
	if _, err := New([]Registration{r}); err == nil {
		t.Fatal("aceitou alvo mutável")
	}
	r.Definition.HasMutableTarget = false
	r.Definition.MutatesEffectiveCapability = true
	r.Handler.MutatesEffectiveCapability = true
	if _, err := New([]Registration{r}); err == nil {
		t.Fatal("aceitou mutação de capacidade")
	}
}

func TestPoliticasTemporais(t *testing.T) {
	for _, mode := range []ContextMode{MaxAge, EventSnapshot} {
		for _, age := range []int64{-1, 0, 100} {
			r := validRegistration()
			r.Definition.Context.Facts[0].Mode = mode
			r.Definition.Context.Facts[0].MaxAgeMS = age
			_, err := New([]Registration{r})
			if (err == nil) != (age > 0) {
				t.Fatalf("%s, idade %d: %v", mode, age, err)
			}
		}
	}
}

func TestSnapshotImutavelEIDsExatos(t *testing.T) {
	a := validRegistration()
	b := validRegistration()
	b.Definition.ID = "chat.model.select"
	r, err := New([]Registration{a, b})
	if err != nil {
		t.Fatal(err)
	}
	a.Definition.AllowedSources[0] = System
	a.Definition.Context.Facts[0].Provider = "changed"
	got, ok := r.Lookup("workspace.tab.new")
	if !ok || !got.AllowsSource(UI) || got.AllowsSource(System) || got.Context.Facts[0].Provider != "workspace" {
		t.Fatal(got)
	}
	got.AllowedSources[0] = System
	got.Context.Facts[0].Provider = "changed"
	again, _ := r.Lookup("workspace.tab.new")
	if !again.AllowsSource(UI) || again.Context.Facts[0].Provider != "workspace" {
		t.Fatal(again)
	}
	list := r.List()
	if !reflect.DeepEqual([]string{list[0].ID, list[1].ID}, []string{"chat.model.select", "workspace.tab.new"}) {
		t.Fatal(list)
	}
	list[0].AllowedSources[0] = System
	if !r.List()[0].AllowsSource(UI) {
		t.Fatal("List altera snapshot")
	}
	if _, ok := r.Lookup("Workspace.Tab.New"); ok {
		t.Fatal("aceitou ID aproximado")
	}
	if _, err := New([]Registration{b, b}); err == nil {
		t.Fatal("aceitou ID repetido")
	}
}

func presentation() *Presentation {
	return &Presentation{Version: "1", Locales: map[string]LocalizedMetadata{
		"pt-BR": {Name: "Nova aba", Description: "Cria uma aba", Category: "Workspace", Aliases: []string{"aba nova", "criar aba"}},
		"en":    {Name: "New tab", Description: "Creates a tab", Category: "Workspace", Aliases: []string{"new tab"}},
		"es":    {Name: "Nueva pestaña", Description: "Crea una pestaña", Category: "Espacio", Aliases: []string{"pestaña nueva"}},
	}}
}

func TestApresentacaoLocalizadaExigeContratoExato(t *testing.T) {
	tests := map[string]func(*Presentation){
		"versão ausente": func(p *Presentation) { p.Version = " " },
		"locale ausente": func(p *Presentation) { delete(p.Locales, "en") },
		"locale extra":   func(p *Presentation) { p.Locales["fr"] = p.Locales["en"] },
		"nome vazio":     func(p *Presentation) { m := p.Locales["pt-BR"]; m.Name = " "; p.Locales["pt-BR"] = m },
		"alias vazio": func(p *Presentation) {
			m := p.Locales["pt-BR"]
			m.Aliases = append(m.Aliases, " ")
			p.Locales["pt-BR"] = m
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			r := validRegistration()
			r.Definition.Presentation = presentation()
			change(r.Definition.Presentation)
			if _, err := New([]Registration{r}); err == nil {
				t.Fatal("aceitou apresentação inválida")
			}
		})
	}
}

func TestApresentacaoTemCopiaProfundaNaEntradaESaida(t *testing.T) {
	r := validRegistration()
	r.Definition.Presentation = presentation()
	registry, err := New([]Registration{r})
	if err != nil {
		t.Fatal(err)
	}
	r.Definition.Presentation.Locales["pt-BR"] = LocalizedMetadata{Name: "alterado"}
	got, _ := registry.Lookup(r.Definition.ID)
	got.Presentation.Locales["pt-BR"] = LocalizedMetadata{Name: "saída"}
	got.Presentation.Locales["en"].Aliases[0] = "saída"
	again, _ := registry.Lookup(r.Definition.ID)
	if again.Presentation.Locales["pt-BR"].Name != "Nova aba" || again.Presentation.Locales["en"].Aliases[0] != "new tab" {
		t.Fatal("apresentação não é imutável")
	}
}
