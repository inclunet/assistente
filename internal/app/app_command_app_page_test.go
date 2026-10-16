package app

import (
	"context"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

func TestCommandAppPageConditionParsingAndSourceAllowlist(t *testing.T) {
	facts, err := commandSettingsConditionFacts(&CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "app.page", Op: "eq", Value: "settings"}}})
	if err != nil || facts[commandbindings.AppPage] != "settings" {
		t.Fatalf("known page condition = %v, %v", facts, err)
	}
	for _, invalid := range []any{"tasklist", "settings.commands", "", true, nil} {
		if _, err := commandSettingsConditionFacts(&CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "app.page", Value: invalid}}}); err == nil {
			t.Errorf("invalid app.page %v accepted", invalid)
		}
	}
	for _, test := range []struct {
		origin string
		class  commandExecutionClass
		want   bool
	}{{"keyboard.local", commandExecutionLocalUI, true}, {"palette", commandExecutionLocalUI, true}, {"keyboard.global", commandExecutionLocalUI, false}, {"streamdeck.key", commandExecutionLocalUI, true}, {"streamdeck.key", commandExecutionDurable, false}} {
		if got := commandSettingsOriginSupportsFieldForClass(test.origin, test.class, commandbindings.AppPage); got != test.want {
			t.Errorf("origin %s supports app.page=%v; want %v", test.origin, got, test.want)
		}
	}
}

func TestCaptureLocalKeyboardContextRequiresKnownApplicationPage(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	snapshot, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		context  LocalCommandKeyboardContext
		wantOkay bool
	}{{"known toolbar page", LocalCommandKeyboardContext{SurfaceID: "command-toolbar", SurfaceType: "toolbar", AppPage: "settings"}, true},
		{"canonical profiles route surface", LocalCommandKeyboardContext{SurfaceID: "command-toolbar", SurfaceType: "profiles", AppPage: "profiles"}, true},
		{"canonical tasklists route surface", LocalCommandKeyboardContext{SurfaceID: "command-toolbar", SurfaceType: "tasklists", AppPage: "tasklists"}, true},
		{"canonical history route surface", LocalCommandKeyboardContext{SurfaceID: "command-toolbar", SurfaceType: "history", AppPage: "history"}, true},
		{"canonical workspace route surface", LocalCommandKeyboardContext{SurfaceID: "command-toolbar", SurfaceType: "toolbar", AppPage: "workspace"}, true},
		{"unknown page", LocalCommandKeyboardContext{SurfaceID: "command-toolbar", SurfaceType: "toolbar", AppPage: "settings.commands"}, false},
		{"missing toolbar page", LocalCommandKeyboardContext{SurfaceID: "command-toolbar", SurfaceType: "toolbar"}, false},
		{"profiles page cannot claim generic toolbar", LocalCommandKeyboardContext{SurfaceID: "command-toolbar", SurfaceType: "toolbar", AppPage: "profiles"}, false},
		{"settings page cannot claim profiles surface", LocalCommandKeyboardContext{SurfaceID: "command-toolbar", SurfaceType: "profiles", AppPage: "settings"}, false},
		{"route surface cannot use a workspace tab id", LocalCommandKeyboardContext{SurfaceID: snapshot.Tab.ID, SurfaceType: "profiles", AppPage: "profiles"}, false},
		{"workspace tab cannot claim a route page", LocalCommandKeyboardContext{SurfaceID: snapshot.Tab.ID, SurfaceType: string(snapshot.Tab.Type), AppPage: "profiles"}, false},
		{"real workspace tab still independently valid", LocalCommandKeyboardContext{SurfaceID: snapshot.Tab.ID, SurfaceType: string(snapshot.Tab.Type), AppPage: "workspace"}, true}} {
		t.Run(test.name, func(t *testing.T) {
			proof, err := a.captureLocalKeyboardContext(test.context)
			if test.wantOkay {
				if err != nil || proof == nil {
					t.Fatalf("valid context rejected: proof=%+v err=%v", proof, err)
				}
			} else if err == nil || proof != nil {
				t.Fatalf("invalid context accepted: proof=%+v err=%v", proof, err)
			}
		})
	}
}

func TestContextualKeyboardRouteSurfaceUsesCapturedCanonicalFact(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	const trigger = "keyboard.local:Control+KeyP"
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyP", Modifiers: []string{"Control"}}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
		ID: "profiles-route-binding", Trigger: trigger, CommandID: "navigation.profiles.open", ArgumentsKey: "{}", ExecutionScopeKey: "global",
		Scope: commandbindings.Application, Enabled: true, LayerActive: true,
		Condition: commandbindings.Facts{commandbindings.AppPage: "profiles", commandbindings.SurfaceType: "profiles"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	observed := LocalCommandKeyboardContext{SurfaceID: "command-toolbar", SurfaceType: "profiles", AppPage: "profiles"}
	proof, err := a.captureLocalKeyboardContext(observed)
	if err != nil || proof == nil || !proof.routePage {
		t.Fatalf("canonical route proof rejected: proof=%+v err=%v", proof, err)
	}
	state := &localCommandKeyboardState{configuration: configuration}
	binding, ok := localKeyboardOccurrenceBinding(state, a.commandProduct.Load().registry, trigger, shortcut, proof)
	if !ok || binding.CommandID != "navigation.profiles.open" || binding.Handler != "local_ui" {
		t.Fatalf("route surface fact was not resolved from the canonical page frame: binding=%+v ok=%v", binding, ok)
	}

	// A route label does not widen the real contextual projection to durable
	// commands: those still require one of the authenticated workspace surfaces.
	durable, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
		ID: "durable-route-binding", Trigger: trigger, CommandID: commandProductWorkspaceListID, ArgumentsKey: "{}", ExecutionScopeKey: "global",
		Scope: commandbindings.Application, Enabled: true, LayerActive: true,
		Condition: commandbindings.Facts{commandbindings.AppPage: "profiles", commandbindings.SurfaceType: "profiles"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := contextualKeyboardBinding(context.Background(), durable, a.commandProduct.Load().registry, trigger, shortcut)
	if !ok || entry.ByPage["profiles"] == nil || entry.ByPage["profiles"].BySurface["profiles"] != nil {
		t.Fatalf("route surface unexpectedly projected a durable command: entry=%+v ok=%v", entry, ok)
	}
}

func TestContextualKeyboardAppPageProjectionDoesNotInventSurfaceCondition(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	registry := a.commandProduct.Load().registry
	const trigger = "keyboard.local:Control+KeyP"
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyP", Modifiers: []string{"Control"}}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
		ID: "page-binding", Trigger: trigger, CommandID: "navigation.settings.open", ArgumentsKey: "{}", ExecutionScopeKey: "global",
		Scope: commandbindings.Application, Enabled: true, LayerActive: true, Condition: commandbindings.Facts{commandbindings.AppPage: "settings"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := contextualKeyboardBinding(context.Background(), configuration, registry, trigger, shortcut)
	if !ok || entry.ByPage["settings"] == nil || entry.ByPage["settings"].BySurface["toolbar"] == nil ||
		entry.ByPage["settings"].BySurface["toolbar"].CommandID != "navigation.settings.open" {
		t.Fatalf("settings page branch unavailable: %+v, %v", entry, ok)
	}
	if entry.ByPage["workspace"] == nil || entry.ByPage["workspace"].BySurface["toolbar"] != nil {
		t.Fatalf("page condition leaked to workspace: %+v", entry.ByPage["workspace"])
	}
	// The existing Deck UI ingress receives the route frame; durable/native Deck
	// execution does not, and remains ineligible for this condition.
	if !commandSettingsOriginSupportsFieldForClass(string(commandcatalog.StreamDeck), commandExecutionLocalUI, commandbindings.AppPage) ||
		commandSettingsOriginSupportsFieldForClass(string(commandcatalog.StreamDeck), commandExecutionDurable, commandbindings.AppPage) {
		t.Fatal("Deck app.page support escaped its existing contextual UI ingress")
	}
}
