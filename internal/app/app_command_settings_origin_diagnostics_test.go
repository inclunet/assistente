package app

import (
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
)

func TestCommandSettingsContextualPaletteWorkspaceDiagnostics(t *testing.T) {
	a := readyCommandProduct(t)
	registry := a.commandProduct.Load().registry
	for _, tc := range []struct {
		id        string
		supported bool
	}{
		{"workspace.create", true}, {"workspace.chat.open", true},
		{"workspace.tab.chat.create", true}, {"workspace.tab.editor.create", true},
		{"workspace.tab.tasklist.create", true}, {"workspace.tab.terminal.create", true},
		{"workspace.tab.close", true},
		{"editor.mode.markdown", true}, {"editor.mode.rich", true}, {"editor.mode.view", true},
		{commandConversationClearID, true}, {commandTerminalInterruptID, true},
		{"terminal.session.create", true}, {"terminal.session.close", true},
		{"chat.message.send", true}, {"chat.message.copy", true}, {"chat.message.delete", true},
		{"editor.format.bold", true}, {"editor.file.save", true},
		{"workspace.list", false}, {"profiles.delete", false},
		{commandLayerActivateID, true}, {commandLayerToggleID, true}, {commandLayerBackID, true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			definition, exists := registry.Lookup(tc.id)
			if !exists {
				t.Fatalf("comando ausente do catálogo: %s", tc.id)
			}
			class := commandExecutionClassForDefinition(definition)
			if tc.supported && class != commandExecutionDurable && class != commandExecutionAuditedUI {
				t.Fatalf("classe inesperada: %v", class)
			}
			for _, suppress := range []bool{false, true} {
				spec, _ := json.Marshal(map[string]any{"version": 1, "selection": tc.id})
				row := commandconfig.Binding{ID: "binding", CommandID: &tc.id, TriggerType: "palette", TriggerSpec: string(spec), Enabled: true, Condition: `{"version":1,"clauses":[{"field":"surface.id","op":"eq","value":"chat-one"},{"field":"surface.type","op":"eq","value":"chat"}]}`}
				if suppress {
					row.CommandID = nil
					row.Effect = "suppress"
				}
				if got := commandSettingsBindingDiagnostics(row); (len(got) == 0) != tc.supported {
					t.Fatalf("direct suppress=%v: %+v", suppress, got)
				}
				configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{ID: row.ID, Trigger: "palette:" + tc.id, CommandID: tc.id, ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true, LayerConditions: []commandbindings.Facts{{commandbindings.AppFocused: true, commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-one", commandbindings.Profile: "dev"}}}})
				if err != nil {
					t.Fatal(err)
				}
				if got := commandSettingsLocalAdapterDiagnostics(commandconfig.Snapshot{Bindings: []commandconfig.Binding{row}}, registry, configuration); (len(got) == 0) != tc.supported {
					t.Fatalf("effective suppress=%v: %+v", suppress, got)
				}
				row.TriggerType = "streamdeck.key"
				deckSupported := !suppress && isContextualDeckUICommand(tc.id) && !isContextualPagePaletteCommand(tc.id)
				if got := commandSettingsBindingDiagnostics(row); (len(got) == 0) != deckSupported {
					t.Fatalf("Deck visual support=%v: %+v", deckSupported, got)
				}
			}
		})
	}
}

func TestCommandSettingsContextualPagePaletteDiagnostics(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	for _, id := range []string{"tasklists.duplicate", "tasklists.delete", "tasklists.clear", "profiles.duplicate", "profiles.delete", "profiles.activate"} {
		t.Run(id, func(t *testing.T) {
			for _, suppress := range []bool{false, true} {
				for _, field := range []commandbindings.Field{commandbindings.AppFocused, commandbindings.SurfaceType, commandbindings.Profile, commandbindings.SurfaceID, commandbindings.Process, commandbindings.Device} {
					value := any("dev")
					if field == commandbindings.AppFocused {
						value = true
					}
					if field == commandbindings.SurfaceType {
						value = "tasklists"
					}
					if field == commandbindings.Process {
						value = "editor.exe"
					}
					facts := commandbindings.Facts{field: value}
					clauses := []CommandSettingsConditionClause{{Field: string(field), Op: "eq", Value: value}}
					if field == commandbindings.SurfaceID {
						facts[commandbindings.SurfaceType] = "tasklists"
						clauses = append(clauses, CommandSettingsConditionClause{Field: "surface.type", Op: "eq", Value: "tasklists"})
					}
					condition, _ := json.Marshal(CommandSettingsCondition{Version: 1, Clauses: clauses})
					spec, _ := json.Marshal(map[string]any{"version": 1, "selection": id})
					row := commandconfig.Binding{ID: "page", CommandID: &id, Enabled: true, TriggerType: "palette", TriggerSpec: string(spec), Condition: string(condition)}
					if suppress {
						row.CommandID = nil
						row.Effect = "suppress"
					}
					allowed := field == commandbindings.AppFocused || field == commandbindings.SurfaceType || field == commandbindings.Profile
					if got := commandSettingsBindingDiagnostics(row); (len(got) == 0) != allowed {
						t.Fatalf("direct %s suppress=%v: %+v", field, suppress, got)
					}
					base := contextualPaletteCandidate("base", id, nil)
					base.LayerConditions = []commandbindings.Facts{facts}
					config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{base})
					if err != nil {
						t.Fatal(err)
					}
					row.Condition = ""
					if got := commandSettingsLocalAdapterDiagnostics(commandconfig.Snapshot{Bindings: []commandconfig.Binding{row}}, registry, config); (len(got) == 0) != allowed {
						t.Fatalf("inherited %s suppress=%v: %+v", field, suppress, got)
					}
					if field == commandbindings.SurfaceType {
						row.TriggerType = "streamdeck.key"
						row.Condition = string(condition)
						if got := commandSettingsBindingDiagnostics(row); (len(got) == 0) != !suppress {
							t.Fatalf("page Deck visual suppress=%v: %+v", suppress, got)
						}
					}
				}
			}
		})
	}
}

func TestCommandSettingsOriginConditionsNeverClaimForeignFacts(t *testing.T) {
	for _, tc := range []struct {
		origin, condition string
		supported         bool
	}{
		{"keyboard.local", `{"version":1,"clauses":[{"field":"surface.type","op":"eq","value":"chat"},{"field":"surface.id","op":"eq","value":"chat-1"}]}`, true},
		{"keyboard.local", `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"dev"}]}`, true},
		{"palette", `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"dev"}]}`, true},
		{"palette", `{"version":1,"clauses":[{"field":"surface.type","op":"eq","value":"chat"}]}`, false},
		{"streamdeck.key", `{"version":1,"clauses":[{"field":"surface.type","op":"eq","value":"chat"}]}`, false},
		{"keyboard.global", `{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true}]}`, false},
		{"streamdeck.key", `{"version":1,"clauses":[{"field":"foreground.process","op":"eq","value":"code.exe"}]}`, true},
		{"keyboard.global", `{"version":1,"clauses":[{"field":"foreground.process","op":"eq","value":"code.exe"}]}`, true},
		{"streamdeck.key", `{"version":1,"clauses":[{"field":"device","op":"eq","value":"DECK"}]}`, true},
		{"keyboard.global", `{"version":1,"clauses":[{"field":"device","op":"eq","value":"DECK"}]}`, false},
		{"palette", `{"version":1,"clauses":[{"field":"foreground.process","op":"eq","value":"code.exe"}]}`, false},
	} {
		t.Run(tc.origin+tc.condition, func(t *testing.T) {
			diagnostics := commandSettingsBindingDiagnostics(commandconfig.Binding{ID: "binding", TriggerType: tc.origin, Condition: tc.condition})
			if (len(diagnostics) == 0) != tc.supported {
				t.Fatalf("support=%v: %+v", tc.supported, diagnostics)
			}
		})
	}
}

func TestCommandSettingsPaletteLocalUIUsesEffectiveFactsButDeckDoesNot(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	command := "workspace.tab.next"
	condition := `{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true},{"field":"profile","op":"eq","value":"dev"},{"field":"surface.id","op":"eq","value":"chat-1"},{"field":"surface.type","op":"eq","value":"chat"}]}`
	paletteDiagnostics := commandSettingsBindingDiagnostics(commandconfig.Binding{ID: "palette-local-ui", CommandID: &command, TriggerType: "palette", Condition: condition})
	if len(paletteDiagnostics) != 0 {
		t.Fatalf("palette local_ui recusou fatos suportados: %+v", paletteDiagnostics)
	}
	deckDiagnostics := commandSettingsBindingDiagnostics(commandconfig.Binding{ID: "deck-local-ui", CommandID: &command, TriggerType: "streamdeck.key", Condition: `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"dev"}]}`})
	if len(deckDiagnostics) != 0 {
		t.Fatalf("Deck local_ui recusou fatos visuais suportados: %+v", deckDiagnostics)
	}

	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
		ID: "palette-local-ui-effective", Trigger: "palette:workspace.tab.next", CommandID: command, ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true,
		Condition:       commandbindings.Facts{commandbindings.AppFocused: true, commandbindings.Profile: "dev", commandbindings.SurfaceID: "chat-1", commandbindings.SurfaceType: "chat"},
		LayerConditions: []commandbindings.Facts{{commandbindings.Profile: "dev", commandbindings.SurfaceType: "chat"}},
	}, {
		ID: "palette-local-ui-suppress", Trigger: "palette:help.shortcuts.show", CommandID: "help.shortcuts.show", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true,
		Condition: commandbindings.Facts{commandbindings.SurfaceID: "chat-1", commandbindings.SurfaceType: "chat"},
	}, {
		ID: "deck-suppress-no-target", Trigger: "streamdeck.key:DECK:key:0", CommandID: "workspace.list", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true,
		Condition: commandbindings.Facts{commandbindings.Profile: "dev"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := commandconfig.Snapshot{Bindings: []commandconfig.Binding{{ID: "palette-local-ui-effective", CommandID: &command, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"workspace.tab.next"}`, Enabled: true, Condition: "{}"}}}
	if diagnostics := commandSettingsLocalAdapterDiagnostics(snapshot, p.registry, configuration); len(diagnostics) != 0 {
		t.Fatalf("palette local_ui recusou effectiveRequiredFacts: %+v", diagnostics)
	}

	localUISuppression := commandconfig.Binding{ID: "palette-local-ui-suppress", TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"help.shortcuts.show"}`, Effect: "suppress", Condition: `{"version":1,"clauses":[{"field":"surface.id","op":"eq","value":"chat-1"},{"field":"surface.type","op":"eq","value":"chat"}]}`}
	if diagnostics := commandSettingsBindingDiagnostics(localUISuppression); len(diagnostics) != 0 {
		t.Fatalf("suppression palette local_ui recusou seleção canônica: %+v", diagnostics)
	}
	backendSuppression := commandconfig.Binding{ID: "palette-backend-suppress", TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"workspace.list"}`, Effect: "suppress", Condition: `{"version":1,"clauses":[{"field":"surface.type","op":"eq","value":"chat"}]}`}
	if diagnostics := commandSettingsBindingDiagnostics(backendSuppression); len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_origin_condition" {
		t.Fatalf("suppression palette backend visual foi anunciada como suportada: %+v", diagnostics)
	}

	suppressionSnapshot := commandconfig.Snapshot{Bindings: []commandconfig.Binding{localUISuppression}}
	if diagnostics := commandSettingsLocalAdapterDiagnostics(suppressionSnapshot, p.registry, configuration); len(diagnostics) != 0 {
		t.Fatalf("effective diagnostics recusou suppression palette local_ui: %+v", diagnostics)
	}
	backendCommand := "workspace.list"
	backendDeck := commandconfig.Binding{ID: "deck-backend", CommandID: &backendCommand, TriggerType: "streamdeck.key", Condition: `{"version":1,"clauses":[{"field":"foreground.process","op":"eq","value":"code.exe"},{"field":"device","op":"eq","value":"DECK"}]}`}
	if diagnostics := commandSettingsBindingDiagnostics(backendDeck); len(diagnostics) != 0 {
		t.Fatalf("Deck durable perdeu process/device: %+v", diagnostics)
	}
	backendVisual := commandconfig.Binding{ID: "deck-backend-visual", CommandID: &backendCommand, TriggerType: "streamdeck.key", Condition: `{"version":1,"clauses":[{"field":"surface.type","op":"eq","value":"chat"}]}`}
	if diagnostics := commandSettingsBindingDiagnostics(backendVisual); len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_origin_condition" {
		t.Fatalf("Deck durable anunciou fato visual como suportado: %+v", diagnostics)
	}
	deckSuppression := commandconfig.Binding{ID: "deck-suppress-no-target", TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"DECK","key":0}`, Effect: "suppress", Enabled: true, Condition: `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"dev"}]}`}
	if diagnostics := commandSettingsBindingDiagnostics(deckSuppression); len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_origin_condition" {
		t.Fatalf("suppression Deck sem alvo não falhou fechado: %+v", diagnostics)
	}
	deckSuppressionSnapshot := commandconfig.Snapshot{Bindings: []commandconfig.Binding{deckSuppression}}
	if diagnostics := commandSettingsLocalAdapterDiagnostics(deckSuppressionSnapshot, p.registry, configuration); len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_origin_condition" {
		t.Fatalf("effective suppression Deck sem alvo não falhou fechado: %+v", diagnostics)
	}
}
func TestCommandSettingsDeckMixedLocalUIAndDurableBindingFailsClosed(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	trigger := "streamdeck.key:DECK:key:1"
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		{ID: "deck-local-ui-profile-a", Trigger: trigger, CommandID: "help.shortcuts.show", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true, Condition: commandbindings.Facts{commandbindings.Profile: "profile-a", commandbindings.SurfaceType: "chat"}},
		{ID: "deck-durable-profile-b", Trigger: trigger, CommandID: "workspace.list", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true, Condition: commandbindings.Facts{commandbindings.Profile: "profile-b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if conditions := localDeckUIConditions(configuration, p.registry, trigger); len(conditions) == 0 {
		t.Fatal("fixture não produziu ramo local_ui do Deck")
	}
	command := "workspace.list"
	snapshot := commandconfig.Snapshot{Bindings: []commandconfig.Binding{{ID: "deck-durable-profile-b", CommandID: &command, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"DECK","key":1}`, Enabled: true, Condition: `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"profile-b"}]}`}}}
	diagnostics := commandSettingsLocalAdapterDiagnostics(snapshot, p.registry, configuration)
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_contextual_binding" {
		t.Fatalf("binding durable compartilhou projeção local_ui: %+v", diagnostics)
	}
}

func TestCommandSettingsDeckContextualUnionAndInheritedPhysicalFacts(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	for _, id := range []string{commandWorkspaceTabCloseID, commandEditorFormatBoldID, commandLayerActivateID, commandLayerToggleID, commandLayerBackID, commandEditorMermaidApplyID, commandEditorMermaidRemoveID} {
		for _, physical := range []commandbindings.Field{"", commandbindings.Process, commandbindings.Device} {
			local := deckConditionCandidate("local", "help.shortcuts.show", commandbindings.Facts{commandbindings.Profile: "local"})
			ledger := deckConditionCandidate("ledger", id, commandbindings.Facts{commandbindings.Profile: "ledger", commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-one"})
			ledger.ArgumentsKey = deckContextualTestArguments(id)
			if isMermaidMutation(id) {
				ledger.Condition[commandbindings.SurfaceType] = "editor"
				ledger.Condition[commandbindings.SurfaceID] = "editor-one"
			}
			local.Trigger, ledger.Trigger = "streamdeck.key:DECK:key:1", "streamdeck.key:DECK:key:1"
			if physical != "" {
				ledger.LayerConditions = []commandbindings.Facts{{physical: "editor.exe"}}
			}
			config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{local, ledger})
			if err != nil {
				t.Fatal(err)
			}
			row := commandconfig.Binding{ID: "ledger", CommandID: &id, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"DECK","key":1}`, Enabled: true, Condition: `{"version":1,"clauses":[{"field":"surface.type","op":"eq","value":"chat"}]}`}
			if got := commandSettingsBindingDiagnostics(row); len(got) != 0 {
				t.Fatalf("direct %s: %+v", id, got)
			}
			got := commandSettingsLocalAdapterDiagnostics(commandconfig.Snapshot{Bindings: []commandconfig.Binding{row}}, registry, config)
			if physical == "" && len(got) != 0 || physical != "" && (len(got) != 1 || got[0].Code != "unsupported_origin_condition") {
				t.Fatalf("%s/%s: %+v", id, physical, got)
			}
			if physical != "" && len(contextualDeckUIConditions(config, registry, ledger.Trigger)) != 0 {
				t.Fatal("mixed physical facts were published")
			}
		}
	}
}

func TestCommandSettingsDeckLayerNativeAndMixedFacts(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	for _, id := range []string{commandLayerActivateID, commandLayerToggleID, commandLayerBackID} {
		for _, physical := range []commandbindings.Field{commandbindings.Process, commandbindings.Device} {
			for _, profile := range []bool{false, true} {
				for _, visual := range []commandbindings.Field{"", commandbindings.AppFocused, commandbindings.SurfaceType, commandbindings.SurfaceID} {
					for _, inherited := range []bool{false, true} {
						facts := commandbindings.Facts{physical: "app.exe"}
						if profile {
							facts[commandbindings.Profile] = "dev"
						}
						visualFacts := commandbindings.Facts{}
						if visual == commandbindings.AppFocused {
							visualFacts[visual] = true
						}
						if visual == commandbindings.SurfaceType || visual == commandbindings.SurfaceID {
							visualFacts[commandbindings.SurfaceType] = "chat"
						}
						if visual == commandbindings.SurfaceID {
							visualFacts[visual] = "chat-one"
						}
						if !inherited {
							for field, value := range visualFacts {
								facts[field] = value
							}
						}
						clauses := []CommandSettingsConditionClause{}
						for field, value := range facts {
							clauses = append(clauses, CommandSettingsConditionClause{Field: string(field), Op: "eq", Value: value})
						}
						condition, err := json.Marshal(CommandSettingsCondition{Version: 1, Clauses: clauses})
						if err != nil {
							t.Fatal(err)
						}
						row := commandconfig.Binding{ID: "layer", CommandID: &id, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"DECK","key":1}`, Enabled: true, Condition: string(condition)}
						assertDiagnostics := func(got []CommandSettingsDiagnostic, denied bool) {
							t.Helper()
							if denied && (len(got) != 1 || got[0].Code != "unsupported_origin_condition") || !denied && len(got) != 0 {
								t.Fatalf("%s physical=%s profile=%v visual=%s inherited=%v: %+v", id, physical, profile, visual, inherited, got)
							}
						}
						assertDiagnostics(commandSettingsBindingDiagnostics(row), visual != "" && !inherited)
						candidate := deckConditionCandidate(row.ID, id, facts)
						candidate.ArgumentsKey = deckContextualTestArguments(id)
						candidate.Trigger = "streamdeck.key:DECK:key:1"
						if inherited {
							candidate.LayerConditions = []commandbindings.Facts{visualFacts}
						}
						config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
						if err != nil {
							t.Fatal(err)
						}
						assertDiagnostics(commandSettingsLocalAdapterDiagnostics(commandconfig.Snapshot{Bindings: []commandconfig.Binding{row}}, registry, config), visual != "")
						if got := contextualDeckUIConditions(config, registry, candidate.Trigger); len(got) != 0 {
							t.Fatalf("native/mixed physical facts projected: %+v", got)
						}
					}
				}
			}
		}
	}
}

func TestCommandSettingsDeckPageDirectAndInheritedFields(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	for _, id := range []string{"tasklists.duplicate", "tasklists.delete", "tasklists.clear", "profiles.duplicate", "profiles.delete", "profiles.activate"} {
		for _, field := range []commandbindings.Field{commandbindings.AppFocused, commandbindings.SurfaceType, commandbindings.Profile, commandbindings.SurfaceID, commandbindings.Process, commandbindings.Device} {
			value := any("dev")
			if field == commandbindings.AppFocused {
				value = true
			}
			if field == commandbindings.SurfaceType {
				value = "tasklists"
				if strings.HasPrefix(id, "profiles.") {
					value = "profiles"
				}
			}
			facts := commandbindings.Facts{field: value}
			clauses := []CommandSettingsConditionClause{{Field: string(field), Op: "eq", Value: value}}
			if field == commandbindings.SurfaceID {
				facts[commandbindings.SurfaceType] = "tasklists"
				clauses = append(clauses, CommandSettingsConditionClause{Field: "surface.type", Op: "eq", Value: "tasklists"})
			}
			condition, err := json.Marshal(CommandSettingsCondition{Version: 1, Clauses: clauses})
			if err != nil {
				t.Fatal(err)
			}
			row := commandconfig.Binding{ID: "page", CommandID: &id, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"DECK","key":1}`, Enabled: true, Condition: string(condition)}
			allowed := field == commandbindings.AppFocused || field == commandbindings.SurfaceType || field == commandbindings.Profile
			assert := func(got []CommandSettingsDiagnostic) {
				t.Helper()
				if allowed && len(got) != 0 || !allowed && (len(got) != 1 || got[0].Code != "unsupported_origin_condition") {
					t.Fatalf("%s/%s: %+v", id, field, got)
				}
			}
			assert(commandSettingsBindingDiagnostics(row))
			candidate := deckConditionCandidate(row.ID, id, nil)
			candidate.Trigger = "streamdeck.key:DECK:key:1"
			candidate.LayerConditions = []commandbindings.Facts{facts}
			config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
			if err != nil {
				t.Fatal(err)
			}
			row.Condition = ""
			assert(commandSettingsLocalAdapterDiagnostics(commandconfig.Snapshot{Bindings: []commandconfig.Binding{row}}, registry, config))
			row.CommandID = nil
			row.Effect = "suppress"
			if got := commandSettingsLocalAdapterDiagnostics(commandconfig.Snapshot{Bindings: []commandconfig.Binding{row}}, registry, config); len(got) != 1 || got[0].Code != "unsupported_origin_condition" {
				t.Fatalf("suppression guessed page target: %+v", got)
			}
		}
	}
}

func TestCommandSettingsProcessConditionOnlyAcceptsExecutableName(t *testing.T) {
	for _, invalid := range []string{"", `C:\\private\\app.exe`, "https://private", "app.exe\n", "app\x00.exe"} {
		_, err := commandSettingsConditionFacts(&CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "foreground.process", Value: invalid}}})
		if err == nil {
			t.Fatalf("nome de executável inválido aceito: %q", invalid)
		}
	}
	facts, err := commandSettingsConditionFacts(&CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "foreground.process", Value: " OBS64.EXE "}}})
	if err != nil || facts[commandbindings.Process] != "obs64.exe" {
		t.Fatalf("normalização divergente do provider: %+v %v", facts, err)
	}
}

func TestCommandSettingsOriginDiagnosticsIncludeLayerConditions(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	command := commandProductWorkspaceListID
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{ID: "binding", Trigger: "palette:workspace.list", CommandID: command, ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true, LayerConditions: []commandbindings.Facts{{commandbindings.SurfaceType: "chat"}}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := commandconfig.Snapshot{Bindings: []commandconfig.Binding{{ID: "binding", CommandID: &command, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"workspace.list"}`, Enabled: true, Condition: "{}"}}}
	diagnostics := commandSettingsLocalAdapterDiagnostics(snapshot, p.registry, configuration)
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_origin_condition" || diagnostics[0].ResourceID != "binding" {
		t.Fatalf("layer condition silently ignored: %+v", diagnostics)
	}
}

func TestCommandSettingsLocalProfileSurfaceAdministrativaIsUnsupported(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	command := commandProductWorkspaceListID
	trigger := "keyboard.local:Alt+KeyI"
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
		ID: "profile-admin", Trigger: trigger, CommandID: command, ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Surface,
		Condition: commandbindings.Facts{commandbindings.Profile: "dev", commandbindings.SurfaceType: "administrativa"}, Enabled: true, LayerActive: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	commandRef := command
	snapshot := commandconfig.Snapshot{Bindings: []commandconfig.Binding{{ID: "profile-admin", CommandID: &commandRef, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyI","modifiers":["Alt"]}`, Enabled: true, Condition: `{"version":1,"clauses":[]}`}}}
	diagnostics := commandSettingsLocalAdapterDiagnostics(snapshot, p.registry, configuration)
	if len(diagnostics) != 1 || diagnostics[0].Code != "unsupported_contextual_binding" || diagnostics[0].ResourceID != "profile-admin" {
		t.Fatalf("combinação administrativa foi anunciada como executável: %+v", diagnostics)
	}
}
