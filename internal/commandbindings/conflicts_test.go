package commandbindings

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func conflictCandidate(id, command string, condition Facts) Candidate {
	return Candidate{ID: id, Trigger: "keyboard.local:KeyC", CommandID: command, ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Global, Condition: condition, Enabled: true, LayerActive: true}
}
func TestCheckConflictsRetornaWitnessDeterministicoDeConjuncoes(t *testing.T) {
	configuration, err := NewConfiguration(nil, nil, []Candidate{
		conflictCandidate("profile", "command.profile", Facts{Profile: "dev"}),
		conflictCandidate("device", "command.device", Facts{Device: "keyboard"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := configuration.CheckConflicts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := configuration.CheckConflicts(context.Background())
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("diagnóstico não determinístico: first=%+v second=%+v err=%v", first, second, err)
	}
	if len(first) != 1 || first[0].Trigger != "keyboard.local:KeyC" || !reflect.DeepEqual(first[0].Facts, Facts{Profile: "dev", Device: "keyboard"}) || !reflect.DeepEqual(first[0].BindingIDs, []string{"device", "profile"}) {
		t.Fatalf("witness incorreto: %+v", first)
	}
}

func TestCheckConflictsRespeitaSupressaoDeltaEPrecedencia(t *testing.T) {
	defaultCandidate := conflictCandidate("default", "command.default", Facts{})
	delta := Delta{ID: "suppress-dev", DefaultID: "default", DefaultVersion: "1", DefaultFingerprint: "fp", Trigger: defaultCandidate.Trigger, Effect: Suppress, Condition: Facts{Profile: "dev"}, Enabled: true, LayerActive: true, ReviewStatus: Active}
	custom := conflictCandidate("custom-dev", "command.custom", Facts{Profile: "dev"})
	configuration, err := NewConfiguration([]Default{{Candidate: defaultCandidate, Version: "1", Fingerprint: "fp"}}, []Delta{delta}, []Candidate{custom})
	if err != nil {
		t.Fatal(err)
	}
	witnesses, err := configuration.CheckConflicts(context.Background())
	if err != nil || len(witnesses) != 0 {
		t.Fatalf("supressão virou conflito: witnesses=%+v err=%v", witnesses, err)
	}

	workspace := defaultCandidate
	workspace.ID, workspace.CommandID, workspace.Scope = "workspace", "command.workspace", Workspace
	workspace.LayerPriority, workspace.BindingPriority = 99, 99
	configuration, err = NewConfiguration(nil, nil, []Candidate{defaultCandidate, workspace})
	if err != nil {
		t.Fatal(err)
	}
	witnesses, err = configuration.CheckConflicts(context.Background())
	if err != nil || len(witnesses) != 0 {
		t.Fatalf("precedência de escopo ignorada: witnesses=%+v err=%v", witnesses, err)
	}
}

func TestCheckConflictsFalhaFechadoNoLimiteDeCombinacoes(t *testing.T) {
	const trigger = "keyboard.local:KeyC"
	candidates := make([]Candidate, 0, 32)
	for i := 0; i < 7; i++ {
		candidates = append(candidates, Candidate{ID: fmt.Sprintf("profile-%d", i), Trigger: trigger, CommandID: fmt.Sprintf("command.profile_%d", i), ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Global, Condition: Facts{Profile: fmt.Sprintf("profile-%d", i)}, Enabled: true, LayerActive: true})
		candidates = append(candidates, Candidate{ID: fmt.Sprintf("device-%d", i), Trigger: trigger, CommandID: fmt.Sprintf("command.device_%d", i), ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Global, Condition: Facts{Device: fmt.Sprintf("device-%d", i)}, Enabled: true, LayerActive: true})
		candidates = append(candidates, Candidate{ID: fmt.Sprintf("process-%d", i), Trigger: trigger, CommandID: fmt.Sprintf("command.process_%d", i), ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Global, Condition: Facts{Process: fmt.Sprintf("process-%d", i)}, Enabled: true, LayerActive: true})
		candidates = append(candidates, Candidate{ID: fmt.Sprintf("surface-type-%d", i), Trigger: trigger, CommandID: fmt.Sprintf("command.surface_type_%d", i), ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Global, Condition: Facts{SurfaceType: fmt.Sprintf("surface-type-%d", i)}, Enabled: true, LayerActive: true})
		candidates = append(candidates, Candidate{ID: fmt.Sprintf("surface-id-%d", i), Trigger: trigger, CommandID: fmt.Sprintf("command.surface_id_%d", i), ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Global, Condition: Facts{SurfaceType: "surface-type-0", SurfaceID: fmt.Sprintf("surface-id-%d", i)}, Enabled: true, LayerActive: true})
	}
	configuration, err := NewConfiguration(nil, nil, candidates)
	if err != nil {
		t.Fatal(err)
	}
	witnesses, err := configuration.CheckConflicts(context.Background())
	if !errors.Is(err, ErrConflictSearchLimit) || witnesses != nil {
		t.Fatalf("limite não falhou fechado: witnesses=%+v err=%v", witnesses, err)
	}
}
