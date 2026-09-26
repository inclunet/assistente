package app

import (
	"encoding/json"
	"image"
	"strconv"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commanddeck"
	"assistente/internal/workspace"
)

func TestWorkspaceTabDeckTargetTracksPositionAndStableIdentity(t *testing.T) {
	active := &workspace.Workspace{ID: "workspace-a", Tabs: workspace.TabsState{Items: []workspace.Tab{
		{ID: "tab-chat", Type: workspace.TabTypeChat, Title: "Conversa", Position: 0},
		{ID: "tab-editor", Type: workspace.TabTypeEditor, Title: "rascunho.md", Position: 1},
	}}}
	position := workspaceTabDeckTargetAtPosition(active, 2, "pt-BR")
	if position.state != workspaceTabDeckTargetReady || position.key != "tab-editor" || position.title != "rascunho.md" || position.icon != "workspace-tab-editor" {
		t.Fatalf("position target = %+v", position)
	}
	specific := workspaceTabDeckTargetFromArguments([]byte(`{"workspace_id":"workspace-a","target_mode":"specific","tab_id":"tab-editor"}`), active, "pt-BR")
	if specific != position {
		t.Fatalf("specific target = %+v, want %+v", specific, position)
	}
	active.Tabs.Items[1].Title = "novo nome.md"
	active.Tabs.Items[0].Position, active.Tabs.Items[1].Position = 1, 0
	if got := workspaceTabDeckTargetAtPosition(active, 1, "pt-BR"); got.title != "novo nome.md" || got.key != "tab-editor" {
		t.Fatalf("renamed/reordered position target = %+v", got)
	}
	if got := workspaceTabDeckTargetFromArguments([]byte(`{"workspace_id":"workspace-a","target_mode":"specific","tab_id":"tab-editor"}`), active, "pt-BR"); got.title != "novo nome.md" || got.key != "tab-editor" {
		t.Fatalf("renamed/reordered identity target = %+v", got)
	}
}

func TestWorkspaceTabDeckTargetFailsClosedForMissingOrForeignWorkspace(t *testing.T) {
	active := &workspace.Workspace{ID: "workspace-current", Tabs: workspace.TabsState{Items: []workspace.Tab{{ID: "tab-a", Type: workspace.TabTypeChat, Title: "Atual", Position: 0}}}}
	for name, raw := range map[string]string{
		"closed tab":        `{"workspace_id":"workspace-current","target_mode":"specific","tab_id":"closed"}`,
		"wrong workspace":   `{"workspace_id":"workspace-other","target_mode":"specific","tab_id":"tab-a"}`,
		"bad position":      `{"workspace_id":"workspace-current","target_mode":"position","position":2}`,
		"unknown field":     `{"workspace_id":"workspace-current","target_mode":"specific","tab_id":"tab-a","fallback":true}`,
		"malformed payload": `{"workspace_id":"workspace-current","target_mode":"position","position":1} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			got := workspaceTabDeckTargetFromArguments([]byte(raw), active, "pt-BR")
			if got.state != workspaceTabDeckTargetUnavailable || got.title != "Aba indisponível" || got.key != "unavailable" {
				t.Fatalf("target must fail closed: %+v", got)
			}
		})
	}
	if got := workspaceTabDeckTargetAtPosition(nil, 1, "en"); got.title != "Tab unavailable" || got.state != workspaceTabDeckTargetUnavailable {
		t.Fatalf("nil workspace = %+v", got)
	}
}

func TestWorkspaceTabDeckDirectCommandsUseLivePositionAndLocalizedIcons(t *testing.T) {
	active := &workspace.Workspace{ID: "workspace-a", Tabs: workspace.TabsState{Items: []workspace.Tab{
		{ID: "one", Type: workspace.TabTypeChat, Title: "Chat one", Position: 0},
		{ID: "two", Type: workspace.TabTypeTerminal, Title: "Shell", Position: 1},
	}}}
	for position := 3; position <= 9; position++ {
		active.Tabs.Items = append(active.Tabs.Items, workspace.Tab{ID: "tab-" + strconv.Itoa(position), Type: workspace.TabTypeEditor, Title: "Tab " + strconv.Itoa(position), Position: position - 1})
	}
	for _, tc := range []struct {
		command string
		want    workspace.Tab
		icon    string
	}{{commandWorkspaceTabFirstID, active.Tabs.Items[0], "workspace-tab-chat"}, {commandWorkspaceTabSecondID, active.Tabs.Items[1], "workspace-tab-terminal"}, {commandWorkspaceTabNinthID, active.Tabs.Items[8], "workspace-tab-editor"}} {
		t.Run(tc.command, func(t *testing.T) {
			base := commandDeckVisual{title: "Old command label"}
			got, _ := applyWorkspaceTabDeckVisual(nil, nil, "", []string{"binding"}, nil, nil, active, "en", base, nil, tc.command)
			if got.title != tc.want.Title || got.icon != tc.icon {
				t.Fatalf("automatic presentation = %+v", got)
			}
		})
	}
	conditional := LocalCommandPaletteCondition{CommandID: commandWorkspaceTabFirstID, BySurface: map[string]bool{"chat": true, "editor": true}}
	visual, _ := applyWorkspaceTabDeckVisual(nil, nil, "", []string{"binding"}, []LocalCommandPaletteCondition{conditional}, nil, active, "en", commandDeckVisual{title: "First tab"}, nil, "")
	if visual.title != "Chat one" || visual.icon != "workspace-tab-chat" {
		t.Fatalf("conditional first-tab visual = %+v", visual)
	}
}

func TestWorkspaceTabDeckTypeIconsRenderWithLocalCatalog(t *testing.T) {
	for _, icon := range []string{"workspace-tab-chat", "workspace-tab-editor", "workspace-tab-terminal", "workspace-tab-tasklist"} {
		t.Run(icon, func(t *testing.T) {
			if !commandDeckIconSupported(icon) {
				t.Fatalf("icon %q is not supported by the Deck renderer", icon)
			}
			canvas := image.NewRGBA(image.Rect(0, 0, 32, 32))
			commandDeckDrawIcon(canvas, icon, canvas.Bounds())
			painted := false
			for _, alpha := range canvas.Pix {
				if alpha != 0 {
					painted = true
					break
				}
			}
			if !painted {
				t.Fatalf("icon %q rendered no pixels", icon)
			}
		})
	}
}

func TestWorkspaceTabDeckBranchTargetsRequireAgreement(t *testing.T) {
	active := &workspace.Workspace{ID: "workspace-a", Tabs: workspace.TabsState{Items: []workspace.Tab{
		{ID: "tab-a", Type: workspace.TabTypeChat, Title: "A", Position: 0},
		{ID: "tab-b", Type: workspace.TabTypeEditor, Title: "B", Position: 1},
	}}}
	argsA := json.RawMessage(`{"workspace_id":"workspace-a","target_mode":"specific","tab_id":"tab-a"}`)
	argsB := json.RawMessage(`{"workspace_id":"workspace-a","target_mode":"specific","tab_id":"tab-b"}`)
	condition := LocalCommandPaletteCondition{CommandID: commandWorkspaceTabGoToID, BySurface: map[string]bool{"chat": true, "editor": true}, BySurfaceArguments: map[string]json.RawMessage{"chat": argsA, "editor": argsB}}
	base, _ := applyWorkspaceTabDeckVisual(nil, nil, "", []string{"binding"}, []LocalCommandPaletteCondition{condition}, nil, active, "en", commandDeckVisual{title: "Go to tab"}, nil, "")
	if base.title != "Context-dependent destination" {
		t.Fatalf("divergent branches must not imply one target: %+v", base)
	}
	condition.BySurfaceArguments["editor"] = argsA
	base, _ = applyWorkspaceTabDeckVisual(nil, nil, "", []string{"binding"}, []LocalCommandPaletteCondition{condition}, nil, active, "en", commandDeckVisual{title: "Go to tab"}, nil, "")
	if base.title != "A" || base.icon != "workspace-tab-chat" {
		t.Fatalf("unanimous branch target = %+v", base)
	}
	condition.BySurfaceArguments["editor"] = json.RawMessage(`{"workspace_id":"workspace-a","target_mode":"specific","tab_id":"closed"}`)
	base, _ = applyWorkspaceTabDeckVisual(nil, nil, "", []string{"binding"}, []LocalCommandPaletteCondition{condition}, nil, active, "pt-BR", commandDeckVisual{title: "Go to tab"}, nil, "")
	if base.title != "Aba indisponível" || base.icon != "" {
		t.Fatalf("missing branch target = %+v", base)
	}
}

func TestWorkspaceTabDeckMixedConditionsKeepCombinedPresentation(t *testing.T) {
	active := &workspace.Workspace{ID: "workspace-a", Tabs: workspace.TabsState{Items: []workspace.Tab{
		{ID: "tab-a", Type: workspace.TabTypeChat, Title: "Chat one", Position: 0},
	}}}
	conditions := []LocalCommandPaletteCondition{
		{CommandID: commandWorkspaceTabFirstID, BySurface: map[string]bool{"chat": true}},
		{CommandID: "navigation.settings.open", BySurface: map[string]bool{"chat": true}},
	}
	base := commandDeckVisual{title: "First tab / Settings"}
	got, _ := applyWorkspaceTabDeckVisual(nil, nil, "", []string{"binding"}, conditions, nil, active, "en", base, nil, "")
	if got.title != "First tab / Settings — Chat one" || got.icon != "workspace-tab-chat" {
		t.Fatalf("mixed contextual presentation must retain the combination and identify the tab target: %+v", got)
	}
}

func TestWorkspaceTabDeckAmbiguousTargetSuffixPreservesCustomTitles(t *testing.T) {
	candidate := commandbindings.Candidate{ID: "binding", Trigger: "streamdeck.key:DECK:key:0", CommandID: commandWorkspaceTabGoToID, ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		candidate.ID: {
			TitleByLocale: map[string]string{"en": "Pinned title"},
			Icon:          "star",
			States: map[string]commandbindings.BindingPresentation{
				"running": {TitleByLocale: map[string]string{"en": "Running title"}},
			},
		},
	}))
	active := &workspace.Workspace{ID: "workspace-a", Tabs: workspace.TabsState{Items: []workspace.Tab{
		{ID: "tab-a", Type: workspace.TabTypeChat, Title: "Chat", Position: 0},
		{ID: "tab-b", Type: workspace.TabTypeEditor, Title: "Draft", Position: 1},
	}}}
	condition := LocalCommandPaletteCondition{
		CommandID: commandWorkspaceTabGoToID,
		BySurface: map[string]bool{"chat": true, "editor": true},
		BySurfaceArguments: map[string]json.RawMessage{
			"chat":   json.RawMessage(`{"workspace_id":"workspace-a","target_mode":"specific","tab_id":"tab-a"}`),
			"editor": json.RawMessage(`{"workspace_id":"workspace-a","target_mode":"specific","tab_id":"tab-b"}`),
		},
	}
	base := commandDeckVisual{title: "Pinned title", icon: "star"}
	variants := map[string]commandDeckVisual{"running": {title: "Running title", icon: "running-icon"}}
	base, variants = applyWorkspaceTabDeckVisual(configuration, nil, "", []string{candidate.ID}, []LocalCommandPaletteCondition{condition}, nil, active, "en", base, variants, "")
	const suffix = " — Context-dependent destination"
	if base.title != "Pinned title"+suffix || base.icon != "star" {
		t.Fatalf("ambiguous marker must suffix, not replace, the custom title: %+v", base)
	}
	if variants["running"].title != "Running title"+suffix || variants["running"].icon != "running-icon" {
		t.Fatalf("state-specific custom title/fields must survive ambiguity: %+v", variants["running"])
	}
}

func TestWorkspaceTabDeckVisualPreservesCustomFieldsAndStateOverrides(t *testing.T) {
	candidate := commandbindings.Candidate{ID: "binding", Trigger: "streamdeck.key:DECK:key:0", CommandID: commandWorkspaceTabGoToID, ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		candidate.ID: {
			TitleByLocale: map[string]string{"en": "Pinned title"},
			Icon:          "star",
			ImageRef:      "custom-image",
			States: map[string]commandbindings.BindingPresentation{
				"running": {TitleByLocale: map[string]string{"en": "Running title"}, Icon: "stop"},
			},
		},
	}))
	active := &workspace.Workspace{ID: "workspace-a", Tabs: workspace.TabsState{Items: []workspace.Tab{{ID: "tab-a", Type: workspace.TabTypeEditor, Title: "live.md", Position: 0}}}}
	args := []byte(`{"workspace_id":"workspace-a","target_mode":"specific","tab_id":"tab-a"}`)
	base := commandDeckVisual{title: "Pinned title", icon: "star", imageRef: "custom-image"}
	variants := map[string]commandDeckVisual{"running": {title: "Running title", icon: "stop", imageRef: "custom-image"}, "succeeded": {title: "Pinned title", icon: "star", imageRef: "custom-image"}}
	base, variants = applyWorkspaceTabDeckVisual(configuration, nil, "", []string{candidate.ID}, nil, args, active, "en", base, variants, commandWorkspaceTabGoToID)
	if base.title != "Pinned title" || base.icon != "star" || base.imageRef != "custom-image" {
		t.Fatalf("base override was not preserved fieldwise: %+v", base)
	}
	if variants["running"].title != "Running title" || variants["running"].icon != "stop" || variants["running"].imageRef != "custom-image" {
		t.Fatalf("state override was not preserved: %+v", variants["running"])
	}
	if variants["succeeded"].title != "Pinned title" || variants["succeeded"].icon != "star" || variants["succeeded"].imageRef != "custom-image" {
		t.Fatalf("inherited fields were not preserved: %+v", variants["succeeded"])
	}
	closed := &workspace.Workspace{ID: "workspace-a"}
	base, _ = applyWorkspaceTabDeckVisual(configuration, nil, "", []string{candidate.ID}, nil, args, closed, "en", base, variants, commandWorkspaceTabGoToID)
	if base.title != "Pinned title — Tab unavailable" || base.icon != "star" || base.imageRef != "custom-image" {
		t.Fatalf("unavailable status should retain custom fields and remain explicit: %+v", base)
	}
}

func TestWorkspaceTabDeckCustomPresentationRequiresEffectiveConsensus(t *testing.T) {
	candidates := []commandbindings.Candidate{
		{ID: "binding-a", Trigger: "streamdeck.key:DECK:key:0", CommandID: commandWorkspaceTabGoToID, ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true},
		{ID: "binding-b", Trigger: "streamdeck.key:DECK:key:1", CommandID: commandWorkspaceTabGoToID, ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true},
	}
	configuration, err := commandbindings.NewConfiguration(nil, nil, candidates)
	if err != nil {
		t.Fatal(err)
	}
	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		"binding-a": {
			TitleByLocale: map[string]string{"en": "Custom base"},
			Icon:          "star",
			States: map[string]commandbindings.BindingPresentation{
				"running": {TitleByLocale: map[string]string{"en": "Custom base"}, Icon: "star"},
			},
		},
		"binding-b": {TitleByLocale: map[string]string{"en": "Custom base"}, Icon: "star"},
	}))
	active := &workspace.Workspace{ID: "workspace-a", Tabs: workspace.TabsState{Items: []workspace.Tab{{ID: "tab-a", Type: workspace.TabTypeEditor, Title: "Current tab", Position: 0}}}}
	ids := []string{"binding-a", "binding-b"}
	base := commandDeckVisual{title: "Custom base", icon: "star"}
	variants := map[string]commandDeckVisual{"running": {title: "Custom base", icon: "star"}}
	base, variants = applyWorkspaceTabDeckVisual(configuration, nil, "", ids, nil, []byte(`{"workspace_id":"workspace-a","target_mode":"position","position":1}`), active, "en", base, variants, commandWorkspaceTabGoToID)
	if base.title != "Custom base" || base.icon != "star" {
		t.Fatalf("unanimous base presentation should be preserved: %+v", base)
	}
	if variants["running"].title != "Custom base" || variants["running"].icon != "star" {
		t.Fatalf("explicit state values matching the inherited presentation should be preserved: %+v", variants["running"])
	}

	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		"binding-a": {
			TitleByLocale: map[string]string{"en": "Custom base"},
			Icon:          "star",
			States: map[string]commandbindings.BindingPresentation{
				"running": {TitleByLocale: map[string]string{"en": "Running custom"}, Icon: "bolt"},
			},
		},
		"binding-b": {TitleByLocale: map[string]string{"en": "Custom base"}, Icon: "star"},
	}))
	variants = map[string]commandDeckVisual{"running": {title: "Running custom", icon: "bolt"}}
	_, variants = applyWorkspaceTabDeckVisual(configuration, nil, "", ids, nil, []byte(`{"workspace_id":"workspace-a","target_mode":"position","position":1}`), active, "en", commandDeckVisual{title: "Custom base", icon: "star"}, variants, commandWorkspaceTabGoToID)
	if variants["running"].title != "Current tab" || variants["running"].icon != "workspace-tab-editor" {
		t.Fatalf("divergent effective state presentation should fall back to target: %+v", variants["running"])
	}

	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		"binding-a": {
			TitleByLocale: map[string]string{"en": "Custom base"},
			Icon:          "star",
			States: map[string]commandbindings.BindingPresentation{
				"running": {TitleByLocale: map[string]string{"en": ""}},
			},
		},
		"binding-b": {TitleByLocale: map[string]string{"en": "Custom base"}, Icon: "star"},
	}))
	variants = map[string]commandDeckVisual{"running": {title: "Custom base", icon: "star"}}
	_, variants = applyWorkspaceTabDeckVisual(configuration, nil, "", ids, nil, []byte(`{"workspace_id":"workspace-a","target_mode":"position","position":1}`), active, "en", commandDeckVisual{title: "Custom base", icon: "star"}, variants, commandWorkspaceTabGoToID)
	if variants["running"].title != "Current tab" || variants["running"].icon != "star" {
		t.Fatalf("explicitly empty state title must fall back automatically while the inherited icon remains unanimous: %+v", variants["running"])
	}

	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		"binding-a": {TitleByLocale: map[string]string{"en": "Custom base"}, Icon: "star"},
		"binding-b": {},
	}))
	base, _ = applyWorkspaceTabDeckVisual(configuration, nil, "", ids, nil, []byte(`{"workspace_id":"workspace-a","target_mode":"position","position":1}`), active, "en", commandDeckVisual{title: "Custom base", icon: "star"}, nil, commandWorkspaceTabGoToID)
	if base.title != "Current tab" || base.icon != "workspace-tab-editor" {
		t.Fatalf("partial custom presentation must not suppress automatic target: %+v", base)
	}

	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		"binding-a": {TitleByLocale: map[string]string{"en": "Custom A"}, Icon: "star"},
		"binding-b": {TitleByLocale: map[string]string{"en": "Custom B"}, Icon: "star"},
	}))
	base, _ = applyWorkspaceTabDeckVisual(configuration, nil, "", ids, nil, []byte(`{"workspace_id":"workspace-a","target_mode":"position","position":1}`), active, "en", commandDeckVisual{title: "Go to tab", icon: "star"}, nil, commandWorkspaceTabGoToID)
	if base.title != "Current tab" || base.icon != "star" {
		t.Fatalf("divergent titles must not suppress automatic target: %+v", base)
	}
}

func TestWorkspaceTabDeckRenameAndCloseReachRenderedFrame(t *testing.T) {
	active := &workspace.Workspace{ID: "workspace-a", Tabs: workspace.TabsState{Items: []workspace.Tab{{ID: "tab-a", Type: workspace.TabTypeEditor, Title: "Draft", Position: 0}}}}
	model := commanddeck.Model{ID: "test", Name: "Test", Rows: 1, Columns: 1, KeyImageW: 72, KeyImageH: 72}
	renderer := commanddeck.NewRenderer()
	if err := renderer.OpenDevice("deck", model); err != nil {
		t.Fatal(err)
	}
	render := func() commanddeck.RenderPlan {
		visual, _ := applyWorkspaceTabDeckVisual(nil, nil, "", []string{"binding"}, nil, nil, active, "en", commandDeckVisual{title: "First tab"}, nil, commandWorkspaceTabFirstID)
		binding := commandDeckBinding{commandID: commandWorkspaceTabFirstID, title: visual.title, icon: visual.icon}
		view := commandDeckKeyView(binding, "en", model)
		plan, err := renderer.Render(commanddeck.Frame{Device: "deck", Model: model, Keys: map[int]commanddeck.KeyView{0: view}})
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}
	if plan := render(); !plan.FullFrame {
		t.Fatalf("initial target frame = %+v", plan)
	}
	active.Tabs.Items[0].Title = "Renamed draft"
	if plan := render(); len(plan.Updates) != 1 || plan.Updates[0].Index != 0 {
		t.Fatalf("rename must refresh the rendered key: %+v", plan)
	}
	active.Tabs.Items = nil
	if plan := render(); len(plan.Updates) != 1 || plan.Updates[0].View.Title != "Tab unavailable" {
		t.Fatalf("closed target must clear stale title: %+v", plan)
	}
}
