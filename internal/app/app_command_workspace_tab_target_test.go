package app

import (
	"encoding/json"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

func TestWorkspaceTabGoToArgumentsReachKeyboardDeckAndPaletteProjection(t *testing.T) {
	definition, handler := commandWorkspaceTabNavigationRegistration(commandWorkspaceTabGoToID)
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: handler}})
	if err != nil {
		t.Fatal(err)
	}
	arguments := `{"workspace_id":"workspace-a","target_mode":"specific","tab_id":"tab-a"}`
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyG", Modifiers: []string{"Control", "Alt"}}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		{ID: "keyboard-target", Trigger: "keyboard.local:KeyG", CommandID: commandWorkspaceTabGoToID, ArgumentsKey: arguments, ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true},
		{ID: "palette-target", Trigger: "palette:" + commandWorkspaceTabGoToID, CommandID: commandWorkspaceTabGoToID, ArgumentsKey: arguments, ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	keyboard, ok := resolvedLocalKeyboardBinding(configuration, registry, "keyboard.local:KeyG", shortcut, commandbindings.Facts{commandbindings.AppFocused: true})
	if !ok || keyboard.CommandID != commandWorkspaceTabGoToID || keyboard.Handler != "local_ui" || string(keyboard.Arguments) != arguments {
		t.Fatalf("binding local perdeu alvo parametrizado: %+v ok=%t", keyboard, ok)
	}
	palette := localPaletteUIArguments(configuration, registry)
	if string(palette[commandWorkspaceTabGoToID]) != arguments {
		t.Fatalf("paleta local perdeu alvo parametrizado: %s", palette[commandWorkspaceTabGoToID])
	}

	cloned := cloneLocalPaletteArguments(palette)
	cloned[commandWorkspaceTabGoToID][0] = ' '
	if json.Valid(cloned[commandWorkspaceTabGoToID]) || string(palette[commandWorkspaceTabGoToID]) != arguments {
		t.Fatal("projeção de argumentos compartilha ou não copia o payload")
	}
}

func TestWorkspaceTabGoToConditionalPaletteAndDeckKeepArgumentsPerSurface(t *testing.T) {
	definition, handler := commandWorkspaceTabNavigationRegistration(commandWorkspaceTabGoToID)
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: handler}})
	if err != nil {
		t.Fatal(err)
	}
	positionArgs := `{"position":2,"target_mode":"position","workspace_id":"workspace-a"}`
	specificArgs := `{"tab_id":"tab-z","target_mode":"specific","workspace_id":"workspace-a"}`
	paletteConfig, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		{ID: "palette-chat", Trigger: "palette:" + commandWorkspaceTabGoToID, CommandID: commandWorkspaceTabGoToID, ArgumentsKey: positionArgs, ExecutionScopeKey: "global", Scope: commandbindings.Global, Condition: commandbindings.Facts{commandbindings.SurfaceType: "chat"}, Enabled: true, LayerActive: true},
		{ID: "palette-editor", Trigger: "palette:" + commandWorkspaceTabGoToID, CommandID: commandWorkspaceTabGoToID, ArgumentsKey: specificArgs, ExecutionScopeKey: "global", Scope: commandbindings.Global, Condition: commandbindings.Facts{commandbindings.SurfaceType: "editor"}, Enabled: true, LayerActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	palette, ok := localPaletteUICondition(paletteConfig, registry, "palette:"+commandWorkspaceTabGoToID)
	if !ok || !palette.BySurface["chat"] || !palette.BySurface["editor"] || string(palette.BySurfaceArguments["chat"]) != positionArgs || string(palette.BySurfaceArguments["editor"]) != specificArgs {
		t.Fatalf("palette lost branch-specific targets: %+v", palette)
	}
	deckConfig, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		{ID: "deck-chat", Trigger: deckConditionTestTrigger, CommandID: commandWorkspaceTabGoToID, ArgumentsKey: positionArgs, ExecutionScopeKey: "global", Scope: commandbindings.Application, Condition: commandbindings.Facts{commandbindings.SurfaceType: "chat"}, Enabled: true, LayerActive: true},
		{ID: "deck-editor", Trigger: deckConditionTestTrigger, CommandID: commandWorkspaceTabGoToID, ArgumentsKey: specificArgs, ExecutionScopeKey: "global", Scope: commandbindings.Application, Condition: commandbindings.Facts{commandbindings.SurfaceType: "editor"}, Enabled: true, LayerActive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	conditions := localDeckUIConditions(deckConfig, registry, deckConditionTestTrigger)
	if len(conditions) != 1 || !conditions[0].BySurface["chat"] || !conditions[0].BySurface["editor"] || string(conditions[0].BySurfaceArguments["chat"]) != positionArgs || string(conditions[0].BySurfaceArguments["editor"]) != specificArgs {
		t.Fatalf("Deck lost branch-specific targets: %+v", conditions)
	}
}
