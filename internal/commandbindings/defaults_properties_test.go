package commandbindings

import (
	"reflect"
	"sync"
	"testing"
)

func permute[T any](in []T) [][]T {
	out := make([][]T, 0)
	var visit func([]T, int)
	visit = func(current []T, n int) {
		if n == len(current) {
			out = append(out, append([]T(nil), current...))
			return
		}
		for i := n; i < len(current); i++ {
			current[n], current[i] = current[i], current[n]
			visit(current, n+1)
			current[n], current[i] = current[i], current[n]
		}
	}
	visit(append([]T(nil), in...), 0)
	return out
}

func resolveDefaultsProperty(t *testing.T, defaults []Default, deltas []Delta, custom []Candidate) Result {
	t.Helper()
	c, err := NewConfiguration(defaults, deltas, custom)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Resolve("Ctrl+M", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestConfigurationPermutationsPreservamResultado(t *testing.T) {
	first, firstDelta := defaultFixture()
	second := Default{Candidate: binding("second", Global, nil), Version: "4", Fingerprint: "semantic-second-v1"}
	secondDelta := firstDelta
	secondDelta.ID = "second-override"
	secondDelta.DefaultID = second.Candidate.ID
	secondDelta.DefaultVersion = second.Version
	secondDelta.DefaultFingerprint = second.Fingerprint
	secondDelta.CommandID = "second-custom"

	custom := []Candidate{binding("custom-a", Application, nil), binding("custom-b", Application, nil)}
	custom[0].CommandID, custom[1].CommandID = firstDelta.CommandID, firstDelta.CommandID

	want := Result{Status: Selected, CommandID: firstDelta.CommandID, ArgumentsKey: firstDelta.ArgumentsKey,
		ExecutionScopeKey: first.Candidate.ExecutionScopeKey, BindingIDs: []string{"custom-a", "custom-b", "override"}}
	for _, defaults := range permute([]Default{first, second}) {
		for _, deltas := range permute([]Delta{firstDelta, secondDelta}) {
			for _, candidates := range permute(custom) {
				if got := resolveDefaultsProperty(t, defaults, deltas, candidates); !reflect.DeepEqual(got, want) {
					t.Fatalf("permutação produziu resultado diferente: got=%+v want=%+v", got, want)
				}
			}
		}
	}
}

func TestConfigurationOverridesEquivalentesPreservamProveniencia(t *testing.T) {
	d, first := defaultFixture()
	second := first
	second.ID = "override-equivalente"
	got := resolveDefaultsProperty(t, []Default{d}, []Delta{first, second}, nil)
	want := Result{Status: Selected, CommandID: first.CommandID, ArgumentsKey: first.ArgumentsKey,
		ExecutionScopeKey: d.Candidate.ExecutionScopeKey, BindingIDs: []string{"override", "override-equivalente"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("overrides equivalentes: got=%+v want=%+v", got, want)
	}
}

func TestConfigurationOverridesConflitantesFalhamFechado(t *testing.T) {
	d, first := defaultFixture()
	second := first
	second.ID = "override-conflitante"
	second.CommandID = "other-command"
	got := resolveDefaultsProperty(t, []Default{d}, []Delta{first, second}, nil)
	if got.Status != Conflict || got.CommandID != "" || got.ArgumentsKey != "" || got.ExecutionScopeKey != "" ||
		!reflect.DeepEqual(got.BindingIDs, []string{"override", "override-conflitante"}) {
		t.Fatalf("override conflitante não falhou fechado: %+v", got)
	}
}

func TestConfigurationSuppressAntesDeDedupPreservaOutroDefault(t *testing.T) {
	d, suppress := defaultFixture()
	suppress.Effect, suppress.CommandID, suppress.ArgumentsKey = Suppress, "", ""
	other := d
	other.Candidate.ID = "other-default"
	defaults := []Default{d, other}
	got := resolveDefaultsProperty(t, defaults, []Delta{suppress}, nil)
	want := Result{Status: Selected, CommandID: d.Candidate.CommandID, ArgumentsKey: d.Candidate.ArgumentsKey,
		ExecutionScopeKey: d.Candidate.ExecutionScopeKey, BindingIDs: []string{"other-default"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("supressão removeu mais que o default referenciado: got=%+v want=%+v", got, want)
	}
}

func TestConfigurationRestauracaoNaoMutaSnapshotAnterior(t *testing.T) {
	d, delta := defaultFixture()
	base, err := NewConfiguration([]Default{d}, []Delta{delta}, nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewConfiguration([]Default{d}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restored.Resolve("Ctrl+M", nil, nil)
	if err != nil || got.CommandID != d.Candidate.CommandID {
		t.Fatalf("restauração inesperada: %+v, %v", got, err)
	}
	got.BindingIDs[0] = "corrompido"
	restoredAgain, err := restored.Resolve("Ctrl+M", nil, nil)
	if err != nil || restoredAgain.CommandID != d.Candidate.CommandID || !reflect.DeepEqual(restoredAgain.BindingIDs, []string{"default"}) {
		t.Fatalf("retorno alterou o snapshot restaurado: %+v, %v", restoredAgain, err)
	}
	previous, err := base.Resolve("Ctrl+M", nil, nil)
	if err != nil || previous.CommandID != delta.CommandID || reflect.DeepEqual(previous.BindingIDs, got.BindingIDs) {
		t.Fatalf("restauração alterou snapshot anterior: %+v, %v", previous, err)
	}
}

func TestConfigurationResolveConcorrenteUsaSnapshotsImutaveisERetornoDetached(t *testing.T) {
	d, delta := defaultFixture()
	candidate := binding("equivalent", Application, nil)
	candidate.CommandID, candidate.ArgumentsKey = delta.CommandID, delta.ArgumentsKey
	c, err := NewConfiguration([]Default{d}, []Delta{delta}, []Candidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"equivalent", "override"}
	const workers = 32
	results := make(chan Result, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, resolveErr := c.Resolve("Ctrl+M", nil, nil)
			if resolveErr != nil {
				t.Errorf("resolução concorrente: %v", resolveErr)
				return
			}
			results <- got
		}()
	}
	wg.Wait()
	close(results)
	for got := range results {
		if got.Status != Selected || !reflect.DeepEqual(got.BindingIDs, want) {
			t.Fatalf("snapshot concorrente inconsistente: %+v", got)
		}
		got.BindingIDs[0] = "mutado"
	}
	untouched, err := c.Resolve("Ctrl+M", nil, nil)
	if err != nil || !reflect.DeepEqual(untouched.BindingIDs, want) {
		t.Fatalf("resultado não detached: %+v, %v", untouched, err)
	}
}
