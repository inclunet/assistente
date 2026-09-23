package commandportability

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestImportReportRestrictedJSON(t *testing.T) {
	secret := "SECRET_REPORT_MARKER"
	// Preenche todos os campos string do conteúdo descartado, inclusive
	// pointers, para detectar exposição acidental de um DTO inteiro.
	fill := func(value any) {
		v := reflect.ValueOf(value).Elem()
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if field.Kind() == reflect.String {
				field.SetString(secret)
			} else if field.Kind() == reflect.Pointer && field.Type().Elem().Kind() == reflect.String {
				p := reflect.New(field.Type().Elem())
				p.Elem().SetString(secret)
				field.Set(p)
			}
		}
	}
	var binding BindingExport
	var rule ActivationRuleExport
	var layer LayerExport
	fill(&binding)
	fill(&rule)
	fill(&layer)
	layer.Scope = PortableScope{Kind: WorkspaceScope, WorkspaceID: secret}
	layer.Bindings, layer.BuiltinDeltas = []BindingExport{binding}, []BindingExport{binding}
	layer.ActivationRules, layer.BuiltinRuleDeltas = []ActivationRuleExport{rule}, []ActivationRuleExport{rule}
	plan := Plan{Version: 2, Layers: []PlannedLayer{{Layer: layer, TargetID: "copied-target", TargetScope: PortableScope{Kind: GlobalScope}, Action: CopyMode}}, Warnings: []Warning{{Code: "credential_missing", Identifier: secret}}}
	raw, err := json.Marshal(buildImportReport(plan, false))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":2,"noChanges":false,"layers":[{"targetId":"copied-target","scope":{"kind":"global"},"action":"copy","deltaOnly":false}],"warnings":[{"code":"credential_missing","count":1}]}`
	if strings.Contains(string(raw), secret) || string(raw) != want {
		t.Fatalf("JSON fora do contrato restrito: %s", raw)
	}
}

func TestImportReportOrderWarningsAndIsolation(t *testing.T) {
	plan := Plan{Version: 2, Layers: []PlannedLayer{
		{TargetID: "target-z", TargetScope: PortableScope{Kind: WorkspaceScope, WorkspaceID: "workspace"}, Action: CopyMode},
		{TargetScope: PortableScope{Kind: GlobalScope}, Action: KeepMode, Layer: LayerExport{DeltaOnly: true}},
	}, Warnings: []Warning{{Code: "z", Identifier: "secret1"}, {Code: "a", Identifier: "secret2"}, {Code: "z", Identifier: "secret3"}}}
	report := buildImportReport(plan, true)
	want := ImportReport{Version: 2, NoChanges: true, Layers: []ImportLayerReport{
		{TargetID: "target-z", Scope: PortableScope{Kind: WorkspaceScope, WorkspaceID: "workspace"}, Action: CopyMode},
		{Scope: PortableScope{Kind: GlobalScope}, Action: KeepMode, DeltaOnly: true},
	}, Warnings: []ImportWarningCount{{Code: "a", Count: 1}, {Code: "z", Count: 2}}}
	if !reflect.DeepEqual(report, want) {
		t.Fatalf("relatório=%+v esperado=%+v", report, want)
	}
	for i := 0; i < 20; i++ {
		if got := buildImportReport(plan, true); !reflect.DeepEqual(got, report) {
			t.Fatal("relatório não determinístico")
		}
	}
	plan.Layers[0].TargetID = "changed"
	plan.Layers[0].TargetScope.WorkspaceID = "changed"
	plan.Warnings[0].Code = "changed"
	if !reflect.DeepEqual(report, want) {
		t.Fatal("mutação do plano alterou relatório")
	}
	report.Layers[1].Action = ReplaceMode
	report.Warnings[1].Code = "changed-report"
	if plan.Layers[1].Action != KeepMode || plan.Warnings[2].Code != "z" {
		t.Fatal("mutação do relatório alterou plano")
	}
}

func TestImportReportKeepDoesNotDescribeCandidateState(t *testing.T) {
	plan := Plan{Version: 2, Layers: []PlannedLayer{{TargetID: "kept", TargetScope: PortableScope{Kind: GlobalScope}, Action: KeepMode}}}
	before := buildImportReport(plan, true)
	plan.Layers[0].Enabled = true
	plan.Layers[0].Layer.Bindings = []BindingExport{{Enabled: true}, {Enabled: false}}
	plan.Layers[0].Layer.ActivationRules = []ActivationRuleExport{{Enabled: true}}
	if after := buildImportReport(plan, true); !reflect.DeepEqual(before, after) {
		t.Fatal("Keep atribuiu conteúdo candidato ao resultado")
	}
	if got := buildImportReport(Plan{Version: 2}, true); got.Layers == nil || got.Warnings == nil || !got.NoChanges {
		t.Fatalf("relatório vazio: %+v", got)
	}
}
