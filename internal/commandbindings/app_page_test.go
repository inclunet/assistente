package commandbindings

import (
	"reflect"
	"testing"
)

func TestAppPageIsClosedAndIndependentFromSurfaceType(t *testing.T) {
	want := []string{"workspace", "settings", "profiles", "history", "help", "about", "update", "tasklists", "jobs", "memories"}
	if got := AppPages(); !reflect.DeepEqual(got, want) {
		t.Fatalf("closed app.page enum = %v, want %v", got, want)
	}
	for _, page := range want {
		if !IsAppPage(page) {
			t.Errorf("known page %q rejected", page)
		}
	}
	for _, page := range []string{"", "chat", "tasklist", "settings.commands", "unknown", " Settings"} {
		if IsAppPage(page) {
			t.Errorf("unknown page %q accepted", page)
		}
	}
	configuration, err := NewConfiguration(nil, nil, []Candidate{{
		ID: "settings-page", Trigger: "keyboard.local:Control+KeyK", CommandID: "help.shortcuts.show",
		ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Application, Enabled: true, LayerActive: true,
		Condition: Facts{AppPage: "settings"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		facts Facts
		want  Status
	}{{Facts{AppFocused: true, AppPage: "settings"}, Selected}, {Facts{AppFocused: true, AppPage: "workspace"}, NoMatch}, {Facts{AppFocused: true}, NoMatch}} {
		got, err := configuration.Resolve("keyboard.local:Control+KeyK", test.facts, nil)
		if err != nil || got.Status != test.want {
			t.Errorf("Resolve(%v) = %+v, %v; want %s", test.facts, got, err, test.want)
		}
	}
}
