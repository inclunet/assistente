package commandcatalog

import (
	"bytes"
	"strings"
	"testing"
)

func completeRegistration() Registration {
	argumentSchema := &Schema{
		Type: SchemaObject,
		Properties: map[string]Schema{
			"secret": {Type: SchemaString, MinLength: intPointer(1)},
			"note":   {Type: SchemaString, Optional: true, Nullable: true},
			"items":  {Type: SchemaArray, Items: &Schema{Type: SchemaInteger}, MinItems: intPointer(1)},
		},
		Required: []string{"secret", "items"},
	}
	resultSchema := &Schema{
		Type: SchemaObject,
		Properties: map[string]Schema{
			"ok": {Type: SchemaBoolean},
		},
		Required: []string{"ok"},
	}
	return Registration{
		Definition: Definition{
			ID:                    "workspace.tab.new",
			Effect:                Write,
			Decision:              NoDecision,
			HasMutableTarget:      true,
			AllowedSources:        []Source{UI, KeyboardLocal},
			Context:               ContextPolicy{Facts: []ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: ExactVersion}}},
			Presentation:          presentation(),
			ArgumentsSchema:       argumentSchema,
			ResultSchema:          resultSchema,
			Risk:                  RiskMedium,
			SensitivePaths:        SensitivePaths{Input: []string{"/secret"}},
			Persistence:           PersistencePolicy{Arguments: PersistenceRedacted, Result: PersistenceSummary, Audit: PersistenceRedacted},
			Scopes:                []Scope{ScopeWorkspace, ScopeApplication},
			Availability:          Availability{Status: Available},
			HandlerRoute:          "internal/workspace/tab/new",
			HandlerClassification: HandlerInternal,
		},
		Handler: HandlerContract{Effect: Write, HasMutableTarget: true, Route: "internal/workspace/tab/new", Classification: HandlerInternal},
	}
}

func intPointer(value int) *int { return &value }

func TestNewCompleteERegistryCompleteSaoOptIn(t *testing.T) {
	r, err := New([]Registration{completeRegistration()})
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete() || completeRegistration().Definition.IsComplete() == false {
		t.Fatal("New deve permanecer protótipo e a definição completa deve ser reconhecida")
	}
	r, err = NewComplete([]Registration{completeRegistration()})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Complete() {
		t.Fatal("NewComplete não marcou o snapshot como completo")
	}
}

func TestNewCompleteMantemIconeEEstadosOpcionais(t *testing.T) {
	r := completeRegistration()
	r.Definition.Presentation.Icon = ""
	r.Definition.Presentation.States = nil
	if _, err := NewComplete([]Registration{r}); err != nil {
		t.Fatal(err)
	}
}

func TestNewCompleteRecusaContratoSensivelInseguro(t *testing.T) {
	tests := map[string]func(*Registration){
		"alvo mutável divergente":         func(r *Registration) { r.Handler.HasMutableTarget = false },
		"plaintext de argumento sensível": func(r *Registration) { r.Definition.Persistence.Arguments = PersistencePlaintext },
		"path inexistente":                func(r *Registration) { r.Definition.SensitivePaths.Input = []string{"/missing"} },
		"pointer inválido":                func(r *Registration) { r.Definition.SensitivePaths.Input = []string{"/secret~2value"} },
		"classificação arbitrária":        func(r *Registration) { r.Definition.HandlerClassification = "plugin" },
		"motivo livre": func(r *Registration) {
			r.Definition.Availability = Availability{Status: Unavailable, Reason: "senha do usuário"}
		},
		"rota com espaços": func(r *Registration) { r.Definition.HandlerRoute = " internal/workspace/tab/new" },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			r := completeRegistration()
			change(&r)
			if _, err := NewComplete([]Registration{r}); err == nil {
				t.Fatal("contrato inseguro aceito")
			}
		})
	}
}

func TestValidateArgumentsClosedOptionalNullableEnumArrayEInteiroGrande(t *testing.T) {
	r, err := NewComplete([]Registration{completeRegistration()})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := r.ValidateArguments("workspace.tab.new", []byte(`{"items":[1,2e3],"secret":"valor","note":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, []byte(`{"items":[1,2000],"note":null,"secret":"valor"}`)) {
		t.Fatalf("retorno não foi canônico: %s", canonical)
	}
	for _, raw := range []string{
		`{"items":[],"secret":"valor"}`,
		`{"items":[1],"secret":"valor","unknown":true}`,
		`{"items":[1.5],"secret":"valor"}`,
		`{"items":[1],"secret":null}`,
	} {
		if _, err := r.ValidateArguments("workspace.tab.new", []byte(raw)); err == nil {
			t.Errorf("payload inválido aceito: %s", raw)
		}
	}
	if _, err := r.ValidateArguments("workspace.tab.new", []byte(`{"items":[999999999999999999999999999],"secret":"valor"}`)); err != nil {
		t.Fatalf("inteiro JSON grande válido foi recusado: %v", err)
	}
}

func TestValidateResultEIDExato(t *testing.T) {
	r, err := NewComplete([]Registration{completeRegistration()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ValidateResult("Workspace.Tab.New", []byte(`{"ok":true}`)); err == nil {
		t.Fatal("ID aproximado aceito")
	}
	if _, err := r.ValidateResult("workspace.tab.new", []byte(`{"ok":true,"leak":"não deve aparecer no erro"}`)); err == nil || strings.Contains(err.Error(), "não deve aparecer") || strings.Contains(err.Error(), "leak") {
		t.Fatalf("erro inseguro ou payload aceito: %v", err)
	}
}

func TestSchemaCiclicoNaoEstouraAStack(t *testing.T) {
	cyclic := &Schema{Type: SchemaArray}
	cyclic.Items = cyclic
	r := completeRegistration()
	r.Definition.ArgumentsSchema = cyclic
	if _, err := NewComplete([]Registration{r}); err == nil {
		t.Fatal("schema cíclico aceito")
	}
	if _, err := New([]Registration{r}); err != nil {
		t.Fatal("New compatível não deve quebrar por schema inválido: ", err)
	}
}

func TestLookupIsolaSchemaProfundamente(t *testing.T) {
	r := completeRegistration()
	r.Definition.ArgumentsSchema.Properties["note"] = Schema{
		Type: SchemaObject, Optional: true,
		Properties: map[string]Schema{"nested": {Type: SchemaArray, Items: &Schema{Type: SchemaString}}},
		Enum:       []any{map[string]any{"nested": []any{"original"}}},
	}
	registry, err := NewComplete([]Registration{r})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := registry.Lookup(r.Definition.ID)
	if !ok {
		t.Fatal("comando não encontrado")
	}
	got.ArgumentsSchema.Properties["note"].Enum[0].(map[string]any)["nested"].([]any)[0] = "alterado"
	again, _ := registry.Lookup(r.Definition.ID)
	nested := again.ArgumentsSchema.Properties["note"].Enum[0].(map[string]any)["nested"].([]any)
	if nested[0] != "original" {
		t.Fatal("mutação do schema vazou para o snapshot")
	}
}

func TestEnumCiclicoFalhaFechado(t *testing.T) {
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	r := completeRegistration()
	r.Definition.ResultSchema.Properties["ok"] = Schema{Type: SchemaString, Enum: []any{cyclic}}
	if _, err := NewComplete([]Registration{r}); err == nil {
		t.Fatal("enum cíclico aceito")
	}
}
