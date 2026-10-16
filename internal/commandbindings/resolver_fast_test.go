package commandbindings

import (
	"reflect"
	"testing"
)

// Compara o caminho de um candidato com o algoritmo geral, forçado por um
// segundo candidato desabilitado no mesmo acionador.
func TestResolveSingleCandidateMatchesGeneralPath(t *testing.T) {
	base := Candidate{ID: "one", Trigger: "ctrl+n", CommandID: "new",
		ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Global,
		Enabled: true, LayerActive: true}
	for _, name := range []string{"selected", "disabled", "inactive", "missing_fact", "matching_fact", "foreground_unknown", "foreground_unfocused", "foreground_focused", "blocked_dialog", "allowed_dialog", "wrong_dialog", "denied_command", "denied_trigger", "invalid_facts", "invalid_dialog"} {
		t.Run(name, func(t *testing.T) {
			c := base
			facts := Facts{}
			var dialog *DialogScope
			switch name {
			case "disabled":
				c.Enabled = false
			case "inactive":
				c.LayerActive = false
			case "missing_fact":
				c.Condition = Facts{AppFocused: true}
			case "matching_fact":
				c.Condition = Facts{AppFocused: true}
				facts[AppFocused] = true
			case "foreground_unknown":
				c.Scope = Foreground
			case "foreground_unfocused":
				c.Scope = Foreground
				facts[AppFocused] = false
			case "foreground_focused":
				c.Scope = Foreground
				facts[AppFocused] = true
			case "blocked_dialog":
				dialog = &DialogScope{ID: "modal"}
			case "allowed_dialog", "wrong_dialog", "denied_command", "denied_trigger":
				c.Scope, c.DialogID = Dialog, "modal"
				dialog = &DialogScope{ID: "modal", AllowedCommandIDs: []string{"new"}, AllowedTriggers: []string{"ctrl+n"}}
				if name == "wrong_dialog" {
					dialog.ID = "other"
				}
				if name == "denied_command" {
					dialog.AllowedCommandIDs = nil
				}
				if name == "denied_trigger" {
					dialog.AllowedTriggers = nil
				}
			case "invalid_facts":
				facts[AppFocused] = "true"
			case "invalid_dialog":
				dialog = &DialogScope{}
			}
			extra := base
			extra.ID, extra.Enabled = "disabled-extra", false
			fast, err := New([]Candidate{c})
			if err != nil {
				t.Fatal(err)
			}
			general, err := New([]Candidate{c, extra})
			if err != nil {
				t.Fatal(err)
			}
			got, gotErr := fast.Resolve(base.Trigger, facts, dialog)
			want, wantErr := general.Resolve(base.Trigger, facts, dialog)
			if (gotErr == nil) != (wantErr == nil) || !reflect.DeepEqual(got, want) {
				t.Fatalf("fast=(%+v,%v), general=(%+v,%v)", got, gotErr, want, wantErr)
			}
			if got.Status == Selected {
				got.BindingIDs[0] = "tampered"
				again, err := fast.Resolve(base.Trigger, facts, dialog)
				if err != nil || again.BindingIDs[0] != c.ID {
					t.Fatalf("resultado alterou snapshot: %+v %v", again, err)
				}
			}
		})
	}
}
