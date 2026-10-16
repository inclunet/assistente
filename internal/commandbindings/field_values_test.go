package commandbindings

import (
	"reflect"
	"testing"
)

func TestFieldValuesIncluiBindingsCamadasEDeltasEmOrdem(t *testing.T) {
	trigger := "keyboard.local:Control+KeyK"
	base := Default{Candidate: Candidate{
		ID: "base", Trigger: trigger, CommandID: "workspace.list", ArgumentsKey: "{}",
		ExecutionScopeKey: "global", Scope: Application, Enabled: true, LayerActive: true,
		Condition:       Facts{SurfaceType: "chat"},
		LayerConditions: []Facts{{SurfaceID: "chat-1"}},
	}, Version: "1", Fingerprint: "base-fp"}
	configuration, err := NewConfiguration([]Default{base}, []Delta{{
		ID: "delta", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "base-fp",
		Trigger: trigger, Effect: Execute, CommandID: "workspace.list", ArgumentsKey: "{}",
		Condition:    Facts{SurfaceID: "chat-2"},
		ReviewStatus: NeedsReview,
	}}, []Candidate{{
		ID: "editor", Trigger: trigger, CommandID: "workspace.list", ArgumentsKey: "{}",
		ExecutionScopeKey: "global", Scope: Surface, Enabled: true, LayerActive: true,
		Condition:       Facts{SurfaceType: "editor", SurfaceID: "editor-1"},
		LayerConditions: []Facts{{SurfaceID: "editor-2"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := configuration.FieldValues(trigger, SurfaceType), []string{"chat", "editor"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("valores de surface.type = %v, want %v", got, want)
	}
	if got, want := configuration.FieldValues(trigger, SurfaceID), []string{"chat-1", "chat-2", "editor-1", "editor-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("valores de surface.id = %v, want %v", got, want)
	}
	if got := configuration.SurfaceValues(trigger); !reflect.DeepEqual(got, []string{"chat", "editor"}) {
		t.Fatalf("atalho SurfaceValues divergiu: %v", got)
	}
}
