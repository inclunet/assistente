package commandbindings

import (
	"reflect"
	"testing"
)

func TestSurfaceValuesIncludesReviewConditionsOnBothTriggers(t *testing.T) {
	for _, inherited := range []bool{false, true} {
		base := Default{Candidate: Candidate{ID: "base", Trigger: "old", CommandID: "open.menu", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Surface, Enabled: true, LayerActive: true}, Version: "1", Fingerprint: "fp"}
		delta := Delta{ID: "review", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "previous-fp", Trigger: "new", Effect: Suppress, ReviewStatus: NeedsReview}
		if inherited {
			base.Candidate.Condition = Facts{SurfaceType: "editor"}
		} else {
			delta.Condition = Facts{SurfaceType: "editor"}
		}
		config, err := NewConfiguration([]Default{base}, []Delta{delta}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, trigger := range []string{"old", "new"} {
			if got := config.SurfaceValues(trigger); !reflect.DeepEqual(got, []string{"editor"}) {
				t.Fatalf("inherited=%v trigger=%s values=%v", inherited, trigger, got)
			}
			result, err := config.Resolve(trigger, Facts{SurfaceType: "editor"}, nil)
			if err != nil || result.Status != ReviewRequired {
				t.Fatalf("review: %+v %v", result, err)
			}
		}
	}
}

func TestContextualSuppressionCannotDefeatHigherScopeOrPriority(t *testing.T) {
	for _, higherScope := range []bool{false, true} {
		base := Default{Candidate: Candidate{ID: "base", Trigger: "key", CommandID: "global.menu", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Global, Condition: Facts{SurfaceType: "editor"}, Enabled: true, LayerActive: true}, Version: "1", Fingerprint: "fp"}
		delta := Delta{ID: "suppress", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "fp", Trigger: "key", Effect: Suppress, Enabled: true, LayerActive: true, ReviewStatus: Active}
		custom := base.Candidate
		custom.ID, custom.CommandID = "custom", "editor.menu"
		if higherScope {
			custom.Scope, custom.Condition = Surface, nil
		} else {
			custom.BindingPriority = 1
		}
		config, err := NewConfiguration([]Default{base}, []Delta{delta}, []Candidate{custom})
		if err != nil {
			t.Fatal(err)
		}
		result, err := config.Resolve("key", Facts{SurfaceType: "editor"}, nil)
		if err != nil || result.Status != Selected || result.CommandID != custom.CommandID {
			t.Fatalf("higherScope=%v result=%+v err=%v", higherScope, result, err)
		}
	}
}
