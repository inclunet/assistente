package commandbindings

import (
	"reflect"
	"testing"
)

func TestWithoutDeltasRestauraSupressaoOverrideEPendencia(t *testing.T) {
	d, override := defaultFixture()
	for _, tc := range []struct {
		name  string
		delta Delta
		want  Result
	}{
		{name: "override", delta: override, want: Result{Status: Selected, CommandID: "default", ArgumentsKey: "empty-args", ExecutionScopeKey: "chat:1", BindingIDs: []string{"default"}}},
		{name: "supressao", delta: func() Delta {
			delta := override
			delta.ID = "suppress"
			delta.Effect = Suppress
			delta.CommandID = ""
			delta.ArgumentsKey = ""
			return delta
		}(), want: Result{Status: Selected, CommandID: "default", ArgumentsKey: "empty-args", ExecutionScopeKey: "chat:1", BindingIDs: []string{"default"}}},
		{name: "pendencia", delta: func() Delta {
			delta := override
			delta.ID = "pending"
			delta.ReviewStatus = NeedsReview
			return delta
		}(), want: Result{Status: Selected, CommandID: "default", ArgumentsKey: "empty-args", ExecutionScopeKey: "chat:1", BindingIDs: []string{"default"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, err := NewConfiguration([]Default{d}, []Delta{tc.delta}, nil)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := config.WithoutDeltas([]string{tc.delta.ID})
			if err != nil {
				t.Fatal(err)
			}
			got, err := restored.Resolve("Ctrl+M", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s: got=%+v want=%+v", tc.name, got, tc.want)
			}
		})
	}
}

func TestWithoutDeltasNaoMutaOriginalEPreservaOutrosDeltas(t *testing.T) {
	d, first := defaultFixture()
	second := first
	second.ID = "second"
	second.Condition = Facts{Profile: "dev"}
	config, err := NewConfiguration([]Default{d}, []Delta{first, second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	originalAdjustments := config.Adjustments()
	restored, err := config.WithoutDeltas([]string{"override"})
	if err != nil {
		t.Fatal(err)
	}

	original, err := config.Resolve("Ctrl+M", nil, nil)
	if err != nil || !reflect.DeepEqual(original, Result{Status: Selected, CommandID: "custom", ArgumentsKey: "empty-args", ExecutionScopeKey: "chat:1", BindingIDs: []string{"override"}}) {
		t.Fatalf("original inesperado: %+v, %v", original, err)
	}
	got, err := restored.Resolve("Ctrl+M", Facts{Profile: "dev"}, nil)
	if err != nil || !reflect.DeepEqual(got, Result{Status: Selected, CommandID: "custom", ArgumentsKey: "empty-args", ExecutionScopeKey: "chat:1", BindingIDs: []string{"second"}}) {
		t.Fatalf("delta preservado: %+v, %v", got, err)
	}
	if !reflect.DeepEqual(config.Adjustments(), originalAdjustments) {
		t.Fatal("ajustes do original alterados")
	}
	if !reflect.DeepEqual(restored.Adjustments(), originalAdjustments) {
		t.Fatalf("ajustes preservados incorretamente: %+v", restored.Adjustments())
	}

	mutatedDefault := config.defaults["default"]
	mutatedDefault.Candidate.Condition = Facts{Profile: "mutated"}
	config.defaults["default"] = mutatedDefault
	mutatedDelta := config.deltas["default"][1]
	mutatedDelta.Condition = Facts{Profile: "mutated"}
	config.deltas["default"][1] = mutatedDelta
	got, err = restored.Resolve("Ctrl+M", Facts{Profile: "dev"}, nil)
	if err != nil || !reflect.DeepEqual(got, Result{Status: Selected, CommandID: "custom", ArgumentsKey: "empty-args", ExecutionScopeKey: "chat:1", BindingIDs: []string{"second"}}) {
		t.Fatalf("snapshot não independente: %+v, %v", got, err)
	}
}

func TestWithoutDeltasListaVaziaCriaSnapshotIndependente(t *testing.T) {
	d, delta := defaultFixture()
	custom := binding("custom", Application, nil)
	config, err := NewConfiguration([]Default{d}, []Delta{delta}, []Candidate{custom})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := config.WithoutDeltas(nil)
	if err != nil {
		t.Fatal(err)
	}
	if restored == config {
		t.Fatal("snapshot reutilizado")
	}
	want, err := NewConfiguration([]Default{d}, []Delta{delta}, []Candidate{custom})
	if err != nil {
		t.Fatal(err)
	}
	got, err := restored.Resolve("Ctrl+M", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantResult, err := want.Resolve("Ctrl+M", nil, nil)
	if err != nil || !reflect.DeepEqual(got, wantResult) {
		t.Fatalf("snapshot equivalente: got=%+v want=%+v", got, wantResult)
	}
	before, err := restored.Resolve("Ctrl+M", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	customCandidate := config.custom.byTrigger["Ctrl+M"][0]
	customCandidate.Condition = Facts{Profile: "mutated"}
	config.custom.byTrigger["Ctrl+M"][0] = customCandidate
	after, err := restored.Resolve("Ctrl+M", nil, nil)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("aliasing no snapshot vazio: antes=%+v depois=%+v err=%v", before, after, err)
	}
}

func TestWithoutDeltasPreservaAdjustmentDeVersaoDetached(t *testing.T) {
	d, delta := defaultFixture()
	d.Version = "2"
	config, err := NewConfiguration([]Default{d}, []Delta{delta}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Adjustments()) != 1 || config.deltas["default"][0].DefaultVersion != "2" {
		t.Fatalf("fixture sem avanço de versão: %+v", config.Adjustments())
	}
	restored, err := config.WithoutDeltas(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Adjustments(), config.Adjustments()) {
		t.Fatalf("adjustment não preservado: got=%+v want=%+v", restored.Adjustments(), config.Adjustments())
	}
	if restored.deltas["default"][0].ReviewStatus != Active {
		t.Fatal("delta ativo foi rebaixado ou reativado incorretamente")
	}
}

func TestWithoutDeltasPreservaNeedsReview(t *testing.T) {
	d, delta := defaultFixture()
	d.Fingerprint = "changed"
	config, err := NewConfiguration([]Default{d}, []Delta{delta}, nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := config.WithoutDeltas(nil)
	if err != nil {
		t.Fatal(err)
	}
	if restored.deltas["default"][0].ReviewStatus != NeedsReview {
		t.Fatal("delta pendente foi reativado")
	}
	result, err := restored.Resolve(delta.Trigger, nil, nil)
	if err != nil || result.Status != ReviewRequired {
		t.Fatalf("pendência não preservada: %+v, %v", result, err)
	}
}

func TestWithoutDeltasRejeitaIDsSemRemocaoParcial(t *testing.T) {
	d, first := defaultFixture()
	second := first
	second.ID = "second"
	config, err := NewConfiguration([]Default{d}, []Delta{first, second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := config.Resolve("Ctrl+M", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{""}, {"missing"}, {"override", "override"}, {"override", "missing"}} {
		if _, err := config.WithoutDeltas(ids); err == nil {
			t.Fatalf("IDs aceitos: %v", ids)
		}
		after, resolveErr := config.Resolve("Ctrl+M", nil, nil)
		if resolveErr != nil || !reflect.DeepEqual(after, before) {
			t.Fatalf("remoção parcial para %v: %+v, %v", ids, after, resolveErr)
		}
	}
}

func TestWithoutDeltasRemoveMultiplosIDsValidos(t *testing.T) {
	d, first := defaultFixture()
	second := first
	second.ID = "second"
	second.Condition = Facts{Profile: "dev"}
	config, err := NewConfiguration([]Default{d}, []Delta{first, second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := config.WithoutDeltas([]string{"override", "second"})
	if err != nil {
		t.Fatal(err)
	}
	for _, facts := range []Facts{nil, {Profile: "dev"}} {
		got, err := restored.Resolve("Ctrl+M", facts, nil)
		want := Result{Status: Selected, CommandID: "default", ArgumentsKey: "empty-args", ExecutionScopeKey: "chat:1", BindingIDs: []string{"default"}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("múltiplos removidos: got=%+v want=%+v err=%v", got, want, err)
		}
	}
}

func TestWithoutDeltasRemoveDeltaOrfao(t *testing.T) {
	_, delta := defaultFixture()
	config, err := NewConfiguration(nil, []Delta{delta}, nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := config.WithoutDeltas([]string{delta.ID})
	if err != nil {
		t.Fatal(err)
	}
	got, err := restored.Resolve(delta.Trigger, nil, nil)
	if err != nil || got.Status != NoMatch {
		t.Fatalf("órfão não removido: %+v, %v", got, err)
	}
}
