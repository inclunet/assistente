package commandbindings

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestLayerProvenanceCopiesScopesAndPreservesMissingProof(t *testing.T) {
	base, err := NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"kind":"source"}`)
	input := map[string][]LayerProvenance{
		"layer.b": {{SourceID: "source.b", Provenance: raw}},
		"layer.a": {{SourceID: "source.a"}},
	}
	got, err := base.WithLayerProvenance(input)
	if err != nil {
		t.Fatal(err)
	}
	input["layer.b"][0].Provenance[0] = 'X'
	input["layer.a"][0].SourceID = "mutated"
	if base.LayerProvenance([]string{"layer.a"}) != nil {
		t.Fatal("configuração original recebeu proveniência")
	}
	selected := got.LayerProvenance([]string{"layer.a", "layer.b", "layer.a"})
	if len(selected) != 2 || selected[0].SourceID != "source.a" || selected[0].Provenance != nil {
		t.Fatalf("snapshot/missing proof incorretos: %#v", selected)
	}
	if string(selected[1].Provenance) != `{"kind":"source"}` {
		t.Fatalf("RawMessage não foi detached: %s", selected[1].Provenance)
	}
	selected[1].Provenance[0] = 'Y'
	again := got.LayerProvenance([]string{"layer.b"})
	if string(again[0].Provenance) != `{"kind":"source"}` {
		t.Fatal("resultado LayerProvenance não é detached")
	}
}

func TestLayerProvenanceSortsAndDeduplicatesOnlyExactEntries(t *testing.T) {
	base, _ := NewConfiguration(nil, nil, nil)
	got, err := base.WithLayerProvenance(map[string][]LayerProvenance{
		"layer.z": {
			{SourceID: "source.b", Provenance: json.RawMessage(`{"v":2}`)},
			{SourceID: "source.a", Provenance: json.RawMessage(`{"v":1}`)},
			{SourceID: "source.a", Provenance: json.RawMessage(`{"v":1}`)},
			{SourceID: "source.a", Provenance: json.RawMessage(`{"v":2}`)},
		},
		"layer.a": {{SourceID: "source.a", Provenance: json.RawMessage(`{"v":1}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	selected := got.LayerProvenance([]string{"layer.z", "layer.a"})
	if len(selected) != 3 {
		t.Fatalf("deduplicação ocultou/preservou quantidade errada: %#v", selected)
	}
	if selected[0].SourceID != "source.a" || string(selected[0].Provenance) != `{"v":1}` ||
		selected[1].SourceID != "source.a" || string(selected[1].Provenance) != `{"v":2}` ||
		selected[2].SourceID != "source.b" {
		t.Fatalf("ordenação/conflito incorretos: %#v", selected)
	}
}

func TestLayerProvenanceValidationAndEquivalence(t *testing.T) {
	base, _ := NewConfiguration(nil, nil, nil)
	empty, err := base.WithLayerProvenance(map[string][]LayerProvenance{})
	if err != nil || !base.Equivalent(empty) {
		t.Fatalf("nil e mapa vazio deveriam ser equivalentes: %v", err)
	}
	for name, input := range map[string]map[string][]LayerProvenance{
		"empty ref":    {"": {{SourceID: "source", Provenance: json.RawMessage(`{}`)}}},
		"spaced ref":   {" layer": {{SourceID: "source", Provenance: json.RawMessage(`{}`)}}},
		"empty source": {"layer": {{SourceID: "", Provenance: json.RawMessage(`{}`)}}},
		"invalid json": {"layer": {{SourceID: "source", Provenance: json.RawMessage(`{`)}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := base.WithLayerProvenance(input); err == nil {
				t.Fatal("proveniência inválida aceita")
			}
		})
	}
	first, err := base.WithLayerProvenance(map[string][]LayerProvenance{"layer": {{SourceID: "source", Provenance: json.RawMessage(`{"v":1}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := base.WithLayerProvenance(map[string][]LayerProvenance{"layer": {{SourceID: "source", Provenance: json.RawMessage(`{"v":1}`)}}})
	if err != nil || !first.Equivalent(second) {
		t.Fatalf("snapshots semanticamente iguais divergiram: %v", err)
	}
	changed, _ := base.WithLayerProvenance(map[string][]LayerProvenance{"layer": {{SourceID: "source", Provenance: json.RawMessage(`{"v":2}`)}}})
	if first.Equivalent(changed) {
		t.Fatal("mudança de provenance ignorada por Equivalent")
	}
	if reflect.DeepEqual(first.LayerProvenance([]string{"layer"}), changed.LayerProvenance([]string{"layer"})) {
		t.Fatal("fixtures de equivalência não diferem")
	}
}

func TestWithoutDeltasPreservesLayerProvenance(t *testing.T) {
	d, delta := defaultFixture()
	config, err := NewConfiguration([]Default{d}, []Delta{delta}, nil)
	if err != nil {
		t.Fatal(err)
	}
	config, err = config.WithLayerProvenance(map[string][]LayerProvenance{
		"layer.user": {{SourceID: "activation-1", Provenance: nil}},
	})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := config.WithoutDeltas([]string{delta.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := restored.LayerProvenance([]string{"layer.user"})
	if len(got) != 1 || got[0].SourceID != "activation-1" || got[0].Provenance != nil {
		t.Fatalf("proveniência perdida na restauração: %#v", got)
	}
}
