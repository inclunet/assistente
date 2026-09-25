package commandbindings

import (
	"encoding/json"
	"testing"
)

func TestExecutionDependencyRequiresExactResolutionContextAndProvenance(t *testing.T) {
	base, err := NewConfiguration(nil, nil, []Candidate{
		{ID: "binding.base", Trigger: "palette:workspace.list", CommandID: "workspace.list", ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: Global, Condition: Facts{AppFocused: true}, Enabled: true, LayerActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	base, err = base.WithLayerProvenance(map[string][]LayerProvenance{"application": {{SourceID: "activation.base", Provenance: json.RawMessage(`{"run":"base"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	facts := Facts{AppFocused: true, Profile: "dev", SurfaceType: "workspace"}
	result, err := base.Resolve("palette:workspace.list", facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	dependency, err := base.CaptureExecutionDependency("palette:workspace.list", facts, nil, result)
	if err != nil {
		t.Fatal(err)
	}

	unrelated, err := NewConfiguration(nil, nil, []Candidate{
		{ID: "binding.base", Trigger: "palette:workspace.list", CommandID: "workspace.list", ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: Global, Condition: Facts{AppFocused: true}, Enabled: true, LayerActive: true},
		{ID: "binding.other", Trigger: "palette:settings", CommandID: "settings.open", ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: Global, Enabled: true, LayerActive: true, LayerRef: "new-layer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err = unrelated.WithLayerProvenance(map[string][]LayerProvenance{"application": {{SourceID: "activation.base", Provenance: json.RawMessage(`{"run":"base"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	if !dependency.UnaffectedBy(unrelated) {
		resolved, _ := unrelated.Resolve("palette:workspace.list", facts, nil)
		t.Fatalf("claim de acionador distinto invalidou uma resolução não afetada: expected=%+v got=%+v provenance=%+v", dependency.result, resolved, unrelated.LayerProvenance(dependency.result.LayerRefs))
	}

	cases := map[string]func() *Configuration{
		"command/target/binding change": func() *Configuration {
			changed, _ := NewConfiguration(nil, nil, []Candidate{{ID: "binding.new", Trigger: "palette:workspace.list", CommandID: "workspace.export", ArgumentsKey: `{"format":"csv"}`, ExecutionScopeKey: "workspace", Scope: Workspace, Enabled: true, LayerActive: true, LayerRef: "new-layer"}})
			return changed
		},
		"suppression/precedence change": func() *Configuration {
			changed, _ := NewConfiguration(nil, nil, []Candidate{
				{ID: "binding.base", Trigger: "palette:workspace.list", CommandID: "workspace.list", ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: Global, Enabled: true, LayerActive: true},
				{ID: "binding.shadow", Trigger: "palette:workspace.list", CommandID: "workspace.export", ArgumentsKey: `{}`, ExecutionScopeKey: "workspace", Scope: Workspace, Enabled: true, LayerActive: true, LayerRef: "new-layer"},
			})
			return changed
		},
		"provenance change": func() *Configuration {
			changed, _ := NewConfiguration(nil, nil, []Candidate{{ID: "binding.base", Trigger: "palette:workspace.list", CommandID: "workspace.list", ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: Global, Enabled: true, LayerActive: true}})
			changed, _ = changed.WithLayerProvenance(map[string][]LayerProvenance{"application": {{SourceID: "activation.other", Provenance: json.RawMessage(`{"run":"other"}`)}}})
			return changed
		},
		"condition changes while still matching": func() *Configuration {
			changed, _ := NewConfiguration(nil, nil, []Candidate{{ID: "binding.base", Trigger: "palette:workspace.list", CommandID: "workspace.list", ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: Global, Condition: Facts{AppFocused: true, Profile: "dev", SurfaceType: "workspace"}, Enabled: true, LayerActive: true}})
			changed, _ = changed.WithLayerProvenance(map[string][]LayerProvenance{"application": {{SourceID: "activation.base", Provenance: json.RawMessage(`{"run":"base"}`)}}})
			return changed
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			if dependency.UnaffectedBy(build()) {
				t.Fatal("dependência alterada foi preservada")
			}
		})
	}
}
