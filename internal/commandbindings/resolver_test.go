package commandbindings

import (
	"fmt"
	"reflect"
	"testing"
)

func binding(id string, scope Scope, condition Facts) Candidate {
	c := Candidate{ID: id, Trigger: "Ctrl+M", CommandID: id, ArgumentsKey: "empty-args",
		ExecutionScopeKey: "chat:1", Scope: scope, Condition: condition, Enabled: true, LayerActive: true}
	if scope == Dialog {
		c.DialogID = "dialog:top"
	}
	return c
}

func resolve(t *testing.T, candidates []Candidate, facts Facts, dialog *DialogScope) Result {
	t.Helper()
	r, err := New(candidates)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Resolve("Ctrl+M", facts, dialog)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestEscopoAntesDePrioridade(t *testing.T) {
	for scope := Control; scope < Global; scope++ {
		t.Run(fmt.Sprint(scope), func(t *testing.T) {
			preferred := binding("preferred", scope, nil)
			lower := binding("lower", scope+1, nil)
			lower.LayerPriority, lower.BindingPriority = 100, 100
			got := resolve(t, []Candidate{lower, preferred}, Facts{AppFocused: false}, nil)
			if got.CommandID != "preferred" {
				t.Fatalf("resultado: %+v", got)
			}
		})
	}
}

func TestFallbackDeCamadaAusenteInativaOuNaoAplicavel(t *testing.T) {
	base := binding("default", Application, nil)
	for _, test := range []struct {
		name   string
		change func(*Candidate)
	}{
		{"desabilitado", func(c *Candidate) { c.Enabled = false }},
		{"camada inativa", func(c *Candidate) { c.LayerActive = false }},
		{"condição ausente", func(c *Candidate) { c.Condition = Facts{Profile: "dev"} }},
		{"outro acionador", func(c *Candidate) { c.Trigger = "Ctrl+N" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			custom := binding("custom", Surface, nil)
			test.change(&custom)
			got := resolve(t, []Candidate{custom, base}, nil, nil)
			if got.CommandID != "default" {
				t.Fatalf("resultado: %+v", got)
			}
		})
	}
}

func TestEspecificidadeAntesDePrioridade(t *testing.T) {
	base := binding("base", Surface, Facts{SurfaceType: "chat"})
	base.LayerPriority = 100
	specific := binding("specific", Surface, Facts{SurfaceType: "chat", Profile: "dev"})
	got := resolve(t, []Candidate{base, specific}, Facts{SurfaceType: "chat", Profile: "dev"}, nil)
	if got.CommandID != "specific" {
		t.Fatalf("resultado: %+v", got)
	}
}

func TestCondicoesIncomparaveisNaoUsamNumeroDeClausulas(t *testing.T) {
	a := binding("a", Surface, Facts{SurfaceType: "chat", Profile: "dev"})
	b := binding("b", Surface, Facts{Device: "deck"})
	facts := Facts{SurfaceType: "chat", Profile: "dev", Device: "deck"}
	got := resolve(t, []Candidate{a, b}, facts, nil)
	if got.Status != Conflict || got.CommandID != "" {
		t.Fatalf("resultado: %+v", got)
	}
	b.LayerPriority = 1
	got = resolve(t, []Candidate{a, b}, facts, nil)
	if got.CommandID != "b" {
		t.Fatalf("resultado: %+v", got)
	}
	a.LayerPriority = 1
	a.BindingPriority = 2
	got = resolve(t, []Candidate{a, b}, facts, nil)
	if got.CommandID != "a" {
		t.Fatalf("resultado: %+v", got)
	}
}

func TestDialogoBarreiraSemFallback(t *testing.T) {
	base := binding("base", Global, nil)
	action := binding("decision.respond", Dialog, nil)
	for _, dialog := range []*DialogScope{
		{ID: "dialog:top"}, {ID: "dialog:top", AllowedCommandIDs: []string{"decision.respond"}},
		{ID: "dialog:top", AllowedTriggers: []string{"Ctrl+M"}},
		{ID: "dialog:top", AllowedCommandIDs: []string{"base"}, AllowedTriggers: []string{"Ctrl+M"}},
	} {
		got := resolve(t, []Candidate{base, action}, nil, dialog)
		if got.Status != Blocked || len(got.BindingIDs) != 0 {
			t.Fatalf("resultado: %+v", got)
		}
	}
	got := resolve(t, []Candidate{base, action}, nil, &DialogScope{
		ID:                "dialog:top",
		AllowedCommandIDs: []string{"decision.respond"}, AllowedTriggers: []string{"Ctrl+M"}})
	if got.CommandID != "decision.respond" {
		t.Fatalf("resultado: %+v", got)
	}
	got = resolve(t, []Candidate{base, action}, nil, nil)
	if got.CommandID != "base" {
		t.Fatalf("diálogo inexistente: %+v", got)
	}
}

func TestForegroundExigeAppExplicitamenteSemFoco(t *testing.T) {
	base := binding("base", Global, nil)
	foreground := binding("external", Foreground, Facts{Process: "code.exe"})
	for _, facts := range []Facts{{Process: "code.exe"}, {Process: "code.exe", AppFocused: true}} {
		if got := resolve(t, []Candidate{base, foreground}, facts, nil); got.CommandID != "base" {
			t.Fatalf("resultado: %+v", got)
		}
	}
	if got := resolve(t, []Candidate{base, foreground}, Facts{Process: "code.exe", AppFocused: false}, nil); got.CommandID != "external" {
		t.Fatalf("resultado: %+v", got)
	}
}

func TestSomenteDialogoNoTopoParticipa(t *testing.T) {
	top := binding("top", Dialog, nil)
	top.CommandID, top.ExecutionScopeKey = "decision.respond", "top-target"
	lower := binding("lower", Dialog, nil)
	lower.CommandID, lower.ExecutionScopeKey = "decision.respond", "lower-target"
	lower.DialogID, lower.LayerPriority = "dialog:lower", 100
	dialog := &DialogScope{ID: top.DialogID, AllowedCommandIDs: []string{"decision.respond"}, AllowedTriggers: []string{"Ctrl+M"}}
	got := resolve(t, []Candidate{lower, top}, nil, dialog)
	if got.ExecutionScopeKey != "top-target" || !reflect.DeepEqual(got.BindingIDs, []string{"top"}) {
		t.Fatalf("resultado: %+v", got)
	}
	dialog.ID = lower.DialogID
	got = resolve(t, []Candidate{lower, top}, nil, dialog)
	if got.ExecutionScopeKey != "lower-target" {
		t.Fatalf("troca de topmost: %+v", got)
	}
	lower.Enabled = false
	if got = resolve(t, []Candidate{lower, top}, nil, dialog); got.Status != Blocked {
		t.Fatalf("fallback indevido: %+v", got)
	}
	r, _ := New([]Candidate{top})
	if _, err := r.Resolve("Ctrl+M", nil, &DialogScope{}); err == nil {
		t.Fatal("aceitou diálogo sem identidade")
	}
}

func TestIdentidadeNormalizadaVenceTipo(t *testing.T) {
	typed := binding("type", Surface, Facts{SurfaceType: "chat"})
	typed.LayerPriority = 100
	exact := binding("exact", Surface, Facts{SurfaceType: "chat", SurfaceID: "chat:1"})
	facts := Facts{SurfaceType: "chat", SurfaceID: "chat:1"}
	got := resolve(t, []Candidate{typed, exact}, facts, nil)
	if got.CommandID != "exact" {
		t.Fatalf("resultado: %+v", got)
	}
	facts[SurfaceID] = "chat:2"
	if got = resolve(t, []Candidate{typed, exact}, facts, nil); got.CommandID != "type" {
		t.Fatalf("outro alvo: %+v", got)
	}
	exact.Condition = Facts{SurfaceID: "chat:1"}
	if _, err := New([]Candidate{exact}); err == nil {
		t.Fatal("aceitou identidade sem tipo normalizado")
	}
}

func TestProvenienciaExcluiEquivalentesInelegiveis(t *testing.T) {
	top := binding("top", Dialog, nil)
	candidates := []Candidate{top}
	for i, change := range []func(*Candidate){
		func(c *Candidate) { c.Enabled = false },
		func(c *Candidate) { c.LayerActive = false },
		func(c *Candidate) { c.Condition = Facts{Profile: "other"} },
		func(c *Candidate) { c.DialogID = "dialog:lower" },
		func(c *Candidate) { c.Scope = Global; c.DialogID = "" },
	} {
		c := top
		c.ID = fmt.Sprint(i)
		change(&c)
		candidates = append(candidates, c)
	}
	got := resolve(t, candidates, nil, &DialogScope{ID: top.DialogID, AllowedCommandIDs: []string{top.CommandID}, AllowedTriggers: []string{top.Trigger}})
	if !reflect.DeepEqual(got.BindingIDs, []string{"top"}) {
		t.Fatalf("proveniência indevida: %+v", got)
	}
}

func TestEquivalentesPreservamProvenienciaSemDuplicar(t *testing.T) {
	a := binding("a", Surface, nil)
	b := binding("b", Global, nil)
	b.CommandID = a.CommandID
	got := resolve(t, []Candidate{b, a}, nil, nil)
	if got.Status != Selected || !reflect.DeepEqual(got.BindingIDs, []string{"a", "b"}) {
		t.Fatalf("resultado: %+v", got)
	}
	for _, change := range []func(*Candidate){
		func(c *Candidate) { c.ArgumentsKey = "other-args" },
		func(c *Candidate) { c.ExecutionScopeKey = "chat:2" },
	} {
		other := b
		other.Scope = Surface
		change(&other)
		got = resolve(t, []Candidate{a, other}, nil, nil)
		if got.Status != Conflict {
			t.Fatalf("destinos diferentes: %+v", got)
		}
	}
}

func TestSnapshotNaoMudaComEntradaOuResultado(t *testing.T) {
	a := binding("a", Surface, Facts{Profile: "dev"})
	input := []Candidate{a}
	r, err := New(input)
	if err != nil {
		t.Fatal(err)
	}
	input[0].CommandID = "changed"
	a.Condition[Profile] = "changed"
	got, err := r.Resolve("Ctrl+M", Facts{Profile: "dev"}, nil)
	if err != nil || got.CommandID != "a" {
		t.Fatalf("resultado: %+v, %v", got, err)
	}
	got.BindingIDs[0] = "changed"
	again, _ := r.Resolve("Ctrl+M", Facts{Profile: "dev"}, nil)
	if again.BindingIDs[0] != "a" {
		t.Fatalf("resultado mutável: %+v", again)
	}
}

func TestEntradasInvalidasSaoRecusadas(t *testing.T) {
	for _, change := range []func(*Candidate){
		func(c *Candidate) { c.ID = "" }, func(c *Candidate) { c.Trigger = "" },
		func(c *Candidate) { c.CommandID = "" }, func(c *Candidate) { c.ArgumentsKey = "" },
		func(c *Candidate) { c.ExecutionScopeKey = "" }, func(c *Candidate) { c.Scope = 255 },
		func(c *Candidate) { c.Condition = Facts{Field("script"): "true"} },
		func(c *Candidate) { c.Condition = Facts{Profile: []string{"dev"}} },
		func(c *Candidate) { c.Condition = Facts{Profile: ""} },
		func(c *Candidate) { c.Condition = Facts{AppFocused: "false"} },
	} {
		c := binding("a", Surface, nil)
		change(&c)
		if _, err := New([]Candidate{c}); err == nil {
			t.Fatalf("aceitou %+v", c)
		}
	}
	c := binding("a", Surface, nil)
	if _, err := New([]Candidate{c, c}); err == nil {
		t.Fatal("aceitou ID repetido")
	}
	r, _ := New([]Candidate{c})
	if _, err := r.Resolve("Ctrl+M", Facts{Profile: map[string]string{}}, nil); err == nil {
		t.Fatal("aceitou fato composto")
	}
	if got, _ := r.Resolve("unknown", nil, nil); got.Status != NoMatch {
		t.Fatalf("resultado: %+v", got)
	}
}

func TestTodasPermutacoesProduzemMesmoResultado(t *testing.T) {
	// A especificidade é parcial: a > b, enquanto c é incomparável com ambos.
	// Ordenar com um comparador par-a-par poderia depender da ordem de entrada.
	a := binding("a", Surface, Facts{Profile: "dev", SurfaceType: "chat"})
	b := binding("b", Surface, Facts{Profile: "dev"})
	c := binding("c", Surface, Facts{Device: "deck"})
	a.LayerPriority, b.LayerPriority, c.LayerPriority = 0, 20, 10
	facts := Facts{Profile: "dev", SurfaceType: "chat", Device: "deck"}
	for _, order := range [][]Candidate{{a, b, c}, {a, c, b}, {b, a, c}, {b, c, a}, {c, a, b}, {c, b, a}} {
		got := resolve(t, order, facts, nil)
		if got.CommandID != "c" {
			t.Fatalf("resultado depende da ordem: %+v", got)
		}
	}
	a.LayerPriority, b.LayerPriority, c.LayerPriority = 0, 0, 0
	for _, order := range [][]Candidate{{a, b, c}, {c, b, a}, {b, a, c}} {
		got := resolve(t, order, facts, nil)
		if got.Status != Conflict || !reflect.DeepEqual(got.BindingIDs, []string{"a", "c"}) {
			t.Fatalf("conflito instável: %+v", got)
		}
	}
}

func TestSnapshotPermiteLeiturasConcorrentes(t *testing.T) {
	r, err := New([]Candidate{binding("a", Surface, nil)})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			for j := 0; j < 100; j++ {
				got, err := r.Resolve("Ctrl+M", nil, nil)
				if err != nil || got.CommandID != "a" {
					t.Fatalf("resultado: %+v, %v", got, err)
				}
			}
		})
	}
}

func BenchmarkResolveIndice(b *testing.B) {
	for _, total := range []int{10, 1000, 10000} {
		b.Run(fmt.Sprint(total), func(b *testing.B) {
			candidates := make([]Candidate, total)
			for i := range candidates {
				candidates[i] = binding(fmt.Sprint(i), Surface, nil)
				candidates[i].Trigger = fmt.Sprintf("key-%d", i)
			}
			r, err := New(candidates)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, err := r.Resolve("key-0", nil, nil)
				if err != nil || got.CommandID != "0" {
					b.Fatal(got, err)
				}
			}
		})
	}
}
