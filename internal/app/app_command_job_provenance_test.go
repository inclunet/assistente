package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandexecution"
)

func TestCommandSelectedJobProvenanceRejectsMissingOrMalformedSourceProof(t *testing.T) {
	tests := []struct {
		name   string
		source commandbindings.LayerProvenance
	}{
		{name: "missing proof", source: commandbindings.LayerProvenance{SourceID: "layer.a"}},
		{name: "missing chain id", source: commandbindings.LayerProvenance{SourceID: "layer.a", Provenance: json.RawMessage(`{"_chain_history":[]}`)}},
		{name: "null chain history", source: commandbindings.LayerProvenance{SourceID: "layer.a", Provenance: json.RawMessage(`{"_chain_id":"chain-a","_chain_history":null}`)}},
		{name: "empty chain id", source: commandbindings.LayerProvenance{SourceID: "layer.a", Provenance: json.RawMessage(`{"_chain_id":"","_chain_history":[]}`)}},
		{name: "trimmed chain id", source: commandbindings.LayerProvenance{SourceID: "layer.a", Provenance: json.RawMessage(`{"_chain_id":" chain-a","_chain_history":[]}`)}},
		{name: "null job history entry", source: commandbindings.LayerProvenance{SourceID: "layer.a", Provenance: json.RawMessage(`{"_chain_id":"chain-a","_chain_history":[null]}`)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := commandSelectedJobProvenance([]commandbindings.LayerProvenance{tc.source}); !errors.Is(err, commandexecution.ErrDenied) {
				t.Fatalf("prova inválida aceita: err=%v", err)
			}
		})
	}
}

func TestCommandSelectedJobProvenancePreservesInvalidCommandHistoryForExecutor(t *testing.T) {
	source := commandbindings.LayerProvenance{
		SourceID: "layer.jobs",
		Provenance: json.RawMessage(`{
			"_chain_id":"chain-a",
			"_chain_history":["job-a"],
			"command_chain_history":null,
			"runtime_identity":{"user_id":"must-not-leak"},
			"claims":{"role":"must-not-leak"}
		}`),
	}
	got, err := commandSelectedJobProvenance([]commandbindings.LayerProvenance{source})
	if err != nil {
		t.Fatalf("proveniência com histórico inválido foi descartada cedo: %v", err)
	}
	if got == nil {
		t.Fatal("proveniência selecionada ausente")
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(*got, &document); err != nil {
		t.Fatal(err)
	}
	if string(document["command_chain_history"]) != "null" {
		t.Fatalf("histórico inválido não foi preservado para o executor: %s", document["command_chain_history"])
	}
	for _, field := range []string{"runtime_identity", "claims"} {
		if _, ok := document[field]; ok {
			t.Fatalf("campo privado %q vazou na proveniência: %s", field, *got)
		}
	}
}

func TestCommandSelectedJobProvenancePreservesValidSourceFields(t *testing.T) {
	source := commandbindings.LayerProvenance{
		SourceID:   "layer.jobs",
		Provenance: json.RawMessage(`{"_chain_id":"chain-a","_chain_history":["job-a"],"_source":"jobs","_source_job_id":"job-a"}`),
	}
	got, err := commandSelectedJobProvenance([]commandbindings.LayerProvenance{source})
	if err != nil || got == nil {
		t.Fatalf("campos de source válidos recusados: provenance=%v err=%v", got, err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(*got, &document); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{
		"_source":        "jobs",
		"_source_job_id": "job-a",
	} {
		var value string
		if err := json.Unmarshal(document[field], &value); err != nil || value != want {
			t.Fatalf("campo %s=%s, want %q", field, document[field], want)
		}
	}
	if string(document["version"]) != "1" {
		t.Fatalf("version=%s, want 1", document["version"])
	}
}

func TestCommandSelectedJobProvenanceRejectsMalformedSourceFields(t *testing.T) {
	for _, field := range []string{"_source", "_source_job_id"} {
		for _, value := range []string{"", " leading", "trailing ", "embedded\x00nul"} {
			t.Run(fmt.Sprintf("%s/%q", field, value), func(t *testing.T) {
				raw, marshalErr := json.Marshal(map[string]any{
					"_chain_id": "chain-a", "_chain_history": []string{"job-a"}, field: value,
				})
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				if _, err := commandSelectedJobProvenance([]commandbindings.LayerProvenance{{SourceID: "layer.jobs", Provenance: raw}}); !errors.Is(err, commandexecution.ErrDenied) {
					t.Fatalf("campo inválido aceito: %s=%q err=%v", field, value, err)
				}
			})
		}
	}
}

func TestCommandSelectedJobProvenanceAcceptsEquivalentDetachedSourcesAndRejectsDivergence(t *testing.T) {
	base := json.RawMessage(`{"_chain_id":"chain-a","_chain_history":["job-a"]}`)
	equivalent := []commandbindings.LayerProvenance{
		{SourceID: "layer.a", Provenance: append(json.RawMessage(nil), base...)},
		{SourceID: "layer.b", Provenance: json.RawMessage(` { "_chain_history": ["job-a"], "_chain_id": "chain-a" } `)},
	}
	got, err := commandSelectedJobProvenance(equivalent)
	if err != nil || got == nil {
		t.Fatalf("origens equivalentes recusadas: provenance=%v err=%v", got, err)
	}
	want := []byte(`{"_chain_history":["job-a"],"_chain_id":"chain-a","version":1}`)
	if !bytes.Equal(*got, want) {
		t.Fatalf("proveniência canônica = %s, want %s", *got, want)
	}
	equivalent[0].Provenance[0] = 'X'
	if bytes.Contains(*got, []byte("X")) {
		t.Fatal("proveniência retornada compartilha memória com a fonte")
	}

	divergent := []commandbindings.LayerProvenance{
		{SourceID: "layer.a", Provenance: base},
		{SourceID: "layer.b", Provenance: json.RawMessage(`{"_chain_id":"chain-b","_chain_history":["job-a"]}`)},
	}
	if _, err := commandSelectedJobProvenance(divergent); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("origens divergentes aceitas: %v", err)
	}
}

func TestCommandSelectedJobProvenanceReturnsNilWithoutSources(t *testing.T) {
	got, err := commandSelectedJobProvenance(nil)
	if err != nil || got != nil {
		t.Fatalf("sem fontes = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestCommandSelectedJobProvenanceConfigurationSelectsOnlyRequestedLayer(t *testing.T) {
	base, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	selectedRaw := json.RawMessage(`{"_chain_id":"chain-selected","_chain_history":["job-selected"]}`)
	config, err := base.WithLayerProvenance(map[string][]commandbindings.LayerProvenance{
		"layer.selected": {{SourceID: "source.selected", Provenance: selectedRaw}},
		"layer.other":    {{SourceID: "source.other"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	selected := config.LayerProvenance([]string{"layer.selected"})
	if len(selected) != 1 || selected[0].SourceID != "source.selected" {
		t.Fatalf("camada selecionada incorreta: %#v", selected)
	}
	if _, err := commandSelectedJobProvenance(selected); err != nil {
		t.Fatalf("metadata da layer não selecionada contaminou a seleção: %v", err)
	}
	if got := config.LayerProvenance([]string{"layer.other"}); len(got) != 1 || got[0].SourceID != "source.other" {
		t.Fatalf("API perdeu a metadata da outra layer fora da seleção: %#v", got)
	}
}
