package commandbindings

import (
	"context"
	"reflect"
	"testing"
)

func TestCheckConflictsIncluiInterseccaoDosGatesOR(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
		wantWitness bool
	}{
		{name: "chat vs chat", left: "chat", right: "chat", wantWitness: true},
		{name: "chat vs editor", left: "chat", right: "editor", wantWitness: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left := conflictCandidate("left", "command.left", nil)
			left.LayerConditions = []Facts{{SurfaceType: test.left}}
			right := conflictCandidate("right", "command.right", nil)
			right.LayerConditions = []Facts{{SurfaceType: test.right}}
			configuration, err := NewConfiguration(nil, nil, []Candidate{left, right})
			if err != nil {
				t.Fatal(err)
			}
			witnesses, err := configuration.CheckConflicts(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if (len(witnesses) > 0) != test.wantWitness {
				t.Fatalf("interseção dos gates incorreta: witnesses=%+v", witnesses)
			}
			if test.wantWitness && !reflect.DeepEqual(witnesses[0].Facts, Facts{SurfaceType: test.left}) {
				t.Fatalf("witness não preservou o fato do gate: %+v", witnesses[0])
			}
		})
	}
}

func TestNewPreservaNilVersusLayerConditionsVazioComoGateNegador(t *testing.T) {
	nilCandidate := conflictCandidate("nil", "command.nil", nil)
	emptyCandidate := conflictCandidate("empty", "command.empty", nil)
	emptyCandidate.LayerConditions = []Facts{}
	nilResolver, err := New([]Candidate{nilCandidate})
	if err != nil {
		t.Fatal(err)
	}
	emptyResolver, err := New([]Candidate{emptyCandidate})
	if err != nil {
		t.Fatal(err)
	}
	nilResult, err := nilResolver.Resolve(nilCandidate.Trigger, nil, nil)
	if err != nil || nilResult.Status != Selected {
		t.Fatalf("gate nil não deveria bloquear: result=%+v err=%v", nilResult, err)
	}
	emptyResult, err := emptyResolver.Resolve(emptyCandidate.Trigger, nil, nil)
	if err != nil || emptyResult.Status != NoMatch {
		t.Fatalf("gate vazio deveria negar: result=%+v err=%v", emptyResult, err)
	}
	if emptyResolver.byTrigger[emptyCandidate.Trigger][0].LayerConditions == nil {
		t.Fatal("clone perdeu a diferença nil versus slice vazia")
	}
}
func TestWithLayerProvenancePreservaGatesDaCamada(t *testing.T) {
	candidate := conflictCandidate("context", "command.context", nil)
	candidate.LayerConditions = []Facts{{SurfaceType: "chat"}}
	configuration, err := NewConfiguration(nil, nil, []Candidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	withProvenance, err := configuration.WithLayerProvenance(map[string][]LayerProvenance{
		"layer.user": {{SourceID: "activation"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range []*Configuration{configuration, withProvenance} {
		chat, err := current.Resolve(candidate.Trigger, Facts{SurfaceType: "chat"}, nil)
		if err != nil || chat.Status != Selected {
			t.Fatalf("gate foi alterado no clone de proveniência: result=%+v err=%v", chat, err)
		}
		editor, err := current.Resolve(candidate.Trigger, Facts{SurfaceType: "editor"}, nil)
		if err != nil || editor.Status != NoMatch {
			t.Fatalf("gate foi removido no clone de proveniência: result=%+v err=%v", editor, err)
		}
	}
}
