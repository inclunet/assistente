package app

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/database"
)

func paletteConditionTestRegistry(t *testing.T) *commandcatalog.Registry {
	t.Helper()
	a := readyCommandProduct(t)
	return a.commandProduct.Load().registry
}

func paletteConditionCandidate(id, trigger string, condition commandbindings.Facts, scope commandbindings.Scope) commandbindings.Candidate {
	return commandbindings.Candidate{ID: id, Trigger: trigger, CommandID: commandProductShortcutsShowID, ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: scope, Condition: condition, Enabled: true, LayerActive: true}
}

func TestContextualLayerPaletteConditionsSchemaAndClosedClass(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	count := 0
	for _, definition := range registry.List() {
		if paletteConditionClassEligible(definition, commandExecutionDurable) || paletteConditionClassEligible(definition, commandExecutionAuditedUI) {
			count++
		}
	}
	if count != 81 {
		t.Fatalf("contextual IDs=%d, want 78+3", count)
	}
	for _, id := range []string{commandLayerActivateID, commandLayerToggleID, commandLayerBackID} {
		t.Run(id, func(t *testing.T) {
			definition, ok := registry.Lookup(id)
			if !ok || !paletteConditionClassEligible(definition, commandExecutionDurable) || paletteConditionClassEligible(definition, commandExecutionAuditedUI) || paletteConditionClassEligible(definition, commandExecutionLocalUI) {
				t.Fatalf("class eligibility: %+v", definition)
			}
			uiDefinition := definition
			uiDefinition.HandlerClassification = commandcatalog.HandlerUI
			if paletteConditionClassEligible(uiDefinition, commandExecutionAuditedUI) {
				t.Fatal("layer admitted as audited UI")
			}
			args := `{"scope":"workspace","rule_id":"configured-rule","duration_seconds":60}`
			if id == commandLayerBackID {
				args = `{"scope":"workspace","rule_id":"","duration_seconds":0}`
			}
			for _, raw := range []string{args, `{}`, `null`, `[]`, `{"scope":"other","rule_id":"r","duration_seconds":0}`, `{"scope":"global","rule_id":"r","duration_seconds":-1}`, `{"scope":"global","rule_id":"r","duration_seconds":1.5}`, `{"scope":"global","rule_id":"r","duration_seconds":86401}`, `{"scope":"global","rule_id":"r","duration_seconds":0,"forged":true}`} {
				candidate := contextualPaletteCandidate("layer", id, commandbindings.Facts{commandbindings.AppFocused: true, commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-one", commandbindings.Profile: "dev"})
				candidate.ArgumentsKey = raw
				config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
				if err != nil {
					t.Fatal(err)
				}
				got := contextualPaletteUIConditions(config, registry)
				if len(got) != 1 || got[0].ByProfile["dev"].BySurfaceID["chat"]["chat-one"] != (raw == args) || got[0].Fallback {
					t.Fatalf("args=%s projection=%+v", raw, got)
				}
				if len(localPaletteUIConditions(config, registry)) != 0 {
					t.Fatal("layer leaked into LOCAL_UI")
				}
			}
			for _, surface := range []string{"chat", "editor", "terminal", "tasklist", "tasklists", "profiles"} {
				base := contextualPaletteCandidate("base", id, commandbindings.Facts{commandbindings.SurfaceType: surface})
				base.ArgumentsKey = args
				config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{base})
				if err != nil {
					t.Fatal(err)
				}
				got := contextualPaletteUIConditions(config, registry)
				if len(got) != 1 || got[0].BySurface[surface] != localKeyboardWorkspaceSurface(surface) {
					t.Fatalf("surface=%s projection=%+v", surface, got)
				}
				config, err = commandbindings.NewConfiguration([]commandbindings.Default{{Version: "1", Fingerprint: "fp", Candidate: base}}, []commandbindings.Delta{{ID: "suppressed", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "fp", Trigger: base.Trigger, Effect: commandbindings.Suppress, Enabled: true, LayerActive: true, ReviewStatus: commandbindings.Active, Condition: commandbindings.Facts{commandbindings.SurfaceType: surface}}}, nil)
				if err != nil {
					t.Fatal(err)
				}
				if got := contextualPaletteUIConditions(config, registry); len(got) != 1 || got[0].BySurface[surface] {
					t.Fatalf("suppression bypass: %+v", got)
				}
			}
		})
	}
}

func TestLocalPaletteConditionsProjectClosedApplicationPages(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	const identity = "palette:" + commandProductShortcutsShowID
	candidate := commandbindings.Candidate{ID: "settings-only", Trigger: identity, CommandID: commandProductShortcutsShowID,
		ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Application, Enabled: true, LayerActive: true,
		Condition: commandbindings.Facts{commandbindings.AppPage: "settings"}}
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	conditions := localPaletteUIConditions(configuration, registry)
	if len(conditions) != 1 || conditions[0].ByPage["settings"].Fallback != true || conditions[0].ByPage["workspace"].Fallback {
		t.Fatalf("app.page availability projection = %+v", conditions)
	}
	clone := cloneLocalCommandPaletteConditions(conditions)
	clone[0].ByPage["settings"] = LocalCommandPaletteCondition{}
	if !conditions[0].ByPage["settings"].Fallback {
		t.Fatal("page branches share memory with cloned context projection")
	}
}

func TestContextualPagePaletteConditionsClosedScope(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	pageCount, workspaceCount := 0, 0
	for _, definition := range registry.List() {
		if isContextualPaletteWorkspaceCommand(definition.ID) {
			workspaceCount++
		}
		if !isContextualPagePaletteCommand(definition.ID) {
			continue
		}
		pageCount++
		if commandExecutionClassForDefinition(definition) != commandExecutionDurable || isContextualPaletteWorkspaceCommand(definition.ID) {
			t.Fatalf("page class/scope: %s", definition.ID)
		}
		t.Run(definition.ID, func(t *testing.T) {
			for _, surface := range []string{"tasklists", "profiles", "tasklist"} {
				for _, source := range []string{"direct", "layer", "suppression", "inherited-id", "suppression-id"} {
					facts := commandbindings.Facts{commandbindings.AppFocused: true, commandbindings.SurfaceType: surface, commandbindings.Profile: "dev"}
					base := contextualPaletteCandidate("base", definition.ID, facts)
					var config *commandbindings.Configuration
					var err error
					switch source {
					case "layer", "inherited-id":
						if source == "inherited-id" {
							facts[commandbindings.SurfaceID] = "untrusted-page-id"
						}
						base.Condition = nil
						base.LayerConditions = []commandbindings.Facts{facts}
						config, err = commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{base})
					case "suppression", "suppression-id":
						if source == "suppression-id" {
							facts[commandbindings.SurfaceID] = "untrusted-page-id"
						}
						base.Condition = nil
						config, err = commandbindings.NewConfiguration([]commandbindings.Default{{Version: "1", Fingerprint: "fp", Candidate: base}}, []commandbindings.Delta{{ID: "suppress", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "fp", Trigger: base.Trigger, Effect: commandbindings.Suppress, Enabled: true, LayerActive: true, ReviewStatus: commandbindings.Active, Condition: facts}}, nil)
					default:
						config, err = commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{base})
					}
					if err != nil {
						t.Fatal(err)
					}
					got := contextualPaletteUIConditions(config, registry)
					if source == "inherited-id" || source == "suppression-id" {
						if len(got) != 0 {
							t.Fatalf("inherited surface ID projected: %+v", got)
						}
						continue
					}
					if len(got) != 1 || got[0].ByProfile["dev"].BySurface[surface] != (source != "suppression") || got[0].BySurfaceID != nil {
						t.Fatalf("%s/%s: %+v", source, surface, got)
					}
					if source == "suppression" && !got[0].Fallback {
						t.Fatal("suppression erased unrelated fallback")
					}
				}
			}
		})
	}
	if pageCount != 6 || workspaceCount != 72 {
		t.Fatalf("pages=%d workspace=%d; want 6+72=78", pageCount, workspaceCount)
	}
}

func TestContextualPagePaletteConditionsRealSettings(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Page conditions", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "condition", Lifecycle: "persistent", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "profile", Value: "dev"}}}}})
	ids := []string{"tasklists.duplicate", "tasklists.delete", "tasklists.clear", "profiles.duplicate", "profiles.delete", "profiles.activate"}
	bindings := map[string]string{}
	for _, id := range ids {
		spec, _ := json.Marshal(map[string]any{"version": 1, "selection": id})
		binding := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: id, TriggerType: "palette", TriggerSpec: string(spec), Arguments: map[string]any{}, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: "tasklists"}, {Field: "app.focused", Value: true}}}}})
		bindings[binding.ID] = id
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, condition := range view.ContextualPaletteConditions {
		if !isContextualPagePaletteCommand(condition.CommandID) {
			continue
		}
		count++
		// The custom binding does not suppress the unrelated builtin fallback.
		if !condition.ByProfile["dev"].BySurface["tasklists"] || !condition.Fallback || condition.BySurfaceID != nil {
			t.Fatalf("real page projection: %+v", condition)
		}
	}
	if count != 6 {
		t.Fatalf("projected %d pages; want 6", count)
	}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "condition", Lifecycle: "persistent", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: "tasklists"}, {Field: "surface.id", Value: "not-a-page-authority"}}}}})
	view, err = a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, condition := range view.ContextualPaletteConditions {
		if isContextualPagePaletteCommand(condition.CommandID) {
			t.Fatalf("inherited ID survived publication: %+v", condition)
		}
	}
	settings, err := a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range settings.Diagnostics {
		if diagnostic.Code == "unsupported_origin_condition" {
			delete(bindings, diagnostic.ResourceID)
		}
	}
	if len(bindings) != 0 {
		t.Fatalf("missing effective diagnostics: %v", bindings)
	}
}

func TestContextualPaletteConditionsRegisteredScope(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	t.Run("unknown-command", func(t *testing.T) {
		const unknown = "unknown.contextual.palette"
		if _, exists := registry.Lookup(unknown); exists {
			t.Fatal("fixture desconhecida foi registrada")
		}
		config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{contextualPaletteCandidate("unknown", unknown, commandbindings.Facts{commandbindings.AppFocused: true})})
		if err != nil {
			t.Fatal(err)
		}
		if isContextualPaletteWorkspaceCommand(unknown) || len(contextualPaletteUIConditions(config, registry)) != 0 {
			t.Fatal("ID desconhecido foi admitido")
		}
	})
	groups := map[string]int{}
	classes := map[commandExecutionClass]int{}
	for _, definition := range registry.List() {
		if !isContextualPaletteWorkspaceCommand(definition.ID) {
			continue
		}
		class := commandExecutionClassForDefinition(definition)
		classes[class]++
		group := "workspace"
		switch {
		case definition.ID == commandConversationClearID:
			group = "clear"
		case definition.ID == commandTerminalInterruptID || definition.ID == commandTerminalSessionCreateID || definition.ID == commandTerminalSessionCloseID:
			group = "terminal"
		case isChatActionCommand(definition.ID) || isChatMessageCommand(definition.ID):
			group = "chat"
		case isEditorFormatCommand(definition.ID):
			group = "formats"
		case isEditorFileCommand(definition.ID):
			group = "files"
		}
		groups[group]++
		t.Run(definition.ID, func(t *testing.T) {
			facts := commandbindings.Facts{commandbindings.AppFocused: true, commandbindings.SurfaceType: "editor", commandbindings.SurfaceID: "editor-one", commandbindings.Profile: "dev"}
			candidate := contextualPaletteCandidate("binding", definition.ID, facts)
			for _, variant := range []string{"exact", "arguments", "scope", "command", "process", "device", "missing-type", "deck"} {
				t.Run(variant, func(t *testing.T) {
					row := candidate
					row.Condition = clonePaletteFacts(facts)
					switch variant {
					case "arguments":
						row.ArgumentsKey = `{"forged":true}`
					case "scope":
						row.ExecutionScopeKey = "workspace:other"
					case "command":
						row.CommandID = commandProductShortcutsShowID
					case "process":
						row.Condition[commandbindings.Process] = "editor.exe"
					case "device":
						row.Condition[commandbindings.Device] = "DECK"
					case "missing-type":
						delete(row.Condition, commandbindings.SurfaceType)
					case "deck":
						row.Trigger = "streamdeck.key:DECK:1"
					}
					config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{row})
					if variant == "missing-type" {
						if err == nil {
							t.Fatal("surface.id sem tipo passou pela validação canônica")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					conditions := contextualPaletteUIConditions(config, registry)
					selected := len(conditions) == 1 && conditions[0].ByProfile["dev"].BySurfaceID["editor"]["editor-one"]
					if selected != (variant == "exact") {
						t.Fatalf("class=%v projection=%+v", class, conditions)
					}
					if len(localPaletteUIConditions(config, registry)) != 0 {
						t.Fatal("comando contextual vazou para LOCAL_UI")
					}
				})
			}
		})
	}
	if want := map[string]int{"workspace": 10, "clear": 1, "terminal": 3, "chat": 10, "formats": 45, "files": 3}; !reflect.DeepEqual(groups, want) {
		t.Fatalf("registered contextual groups=%v; want=%v", groups, want)
	}
	if want := map[commandExecutionClass]int{commandExecutionDurable: 23, commandExecutionAuditedUI: 49}; !reflect.DeepEqual(classes, want) {
		t.Fatalf("registered contextual classes=%v; want=%v", classes, want)
	}
}

func TestLocalPaletteConditionsResolveSurfaceProfileAndUnknownFallback(t *testing.T) {
	trigger := "palette:" + commandProductShortcutsShowID
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{
		paletteConditionCandidate("base", trigger, nil, commandbindings.Global),
		paletteConditionCandidate("chat", trigger, commandbindings.Facts{commandbindings.SurfaceType: "chat"}, commandbindings.Surface),
		paletteConditionCandidate("chat-id", trigger, commandbindings.Facts{commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-1"}, commandbindings.Surface),
		paletteConditionCandidate("profile", trigger, commandbindings.Facts{commandbindings.Profile: "dev"}, commandbindings.Surface),
		paletteConditionCandidate("profile-chat", trigger, commandbindings.Facts{commandbindings.Profile: "dev", commandbindings.SurfaceType: "chat"}, commandbindings.Surface),
	})
	if err != nil {
		t.Fatal(err)
	}
	condition, ok := localPaletteUICondition(configuration, paletteConditionTestRegistry(t), trigger)
	if !ok {
		t.Fatal("condição local da paleta não foi projetada")
	}
	if !condition.Fallback || !condition.BySurface["chat"] || condition.BySurface["editor"] {
		t.Fatalf("fallback/surface inesperados: %+v", condition)
	}
	if !condition.BySurfaceID["chat"]["chat-1"] {
		t.Fatalf("surface.id não projetado: %+v", condition.BySurfaceID)
	}
	profile, ok := condition.ByProfile["dev"]
	if !ok || !profile.BySurface["chat"] || !profile.Fallback {
		t.Fatalf("ramo de perfil inesperado: %+v", condition.ByProfile)
	}
	if _, exists := condition.ByProfile[contextualFallbackProfile([]string{"dev"})]; exists {
		t.Fatal("perfil desconhecido virou ramo enumerado")
	}
}

func TestLocalPaletteConditionsRequireMatchingProfileAndFocusedApp(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	profileTrigger := "palette:" + commandProductShortcutsShowID
	profileConfig, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
		ID: "profile-only", Trigger: profileTrigger, CommandID: commandProductShortcutsShowID, ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: commandbindings.Surface,
		Condition: commandbindings.Facts{commandbindings.Profile: "dev"}, Enabled: true, LayerActive: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	profileCondition, ok := localPaletteUICondition(profileConfig, registry, profileTrigger)
	if !ok || profileCondition.Fallback || !profileCondition.ByProfile["dev"].Fallback {
		t.Fatalf("perfil sem fallback desconhecido seguro: %+v", profileCondition)
	}
	if localPaletteUISelection(profileConfig, registry, profileTrigger, commandProductShortcutsShowID, commandbindings.Facts{commandbindings.AppFocused: true, commandbindings.Profile: "prod"}) {
		t.Fatal("perfil não correspondente selecionou comando")
	}

	focusedTrigger := "palette:" + commandProductShortcutsShowID
	focusedConfig, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{
		ID: "focused", Trigger: focusedTrigger, CommandID: commandProductShortcutsShowID, ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: commandbindings.Application,
		Condition: commandbindings.Facts{commandbindings.AppFocused: true}, Enabled: true, LayerActive: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	focusedCondition, ok := localPaletteUICondition(focusedConfig, registry, focusedTrigger)
	if !ok || !focusedCondition.Fallback || localPaletteUISelection(focusedConfig, registry, focusedTrigger, commandProductShortcutsShowID, commandbindings.Facts{commandbindings.AppFocused: false}) {
		t.Fatalf("app.focused não foi tratado como fato obrigatório: %+v", focusedCondition)
	}
}

func TestLocalPaletteConditionsKeepBarriersAndSkipUnsupportedFacts(t *testing.T) {
	trigger := "palette:" + commandProductShortcutsShowID
	base := commandbindings.Default{Version: "1", Fingerprint: "base", Candidate: paletteConditionCandidate("base", trigger, nil, commandbindings.Global)}
	configuration, err := commandbindings.NewConfiguration([]commandbindings.Default{base}, []commandbindings.Delta{{
		ID: "suppressed-chat", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "base", Trigger: trigger,
		Effect: commandbindings.Suppress, Condition: commandbindings.Facts{commandbindings.SurfaceType: "chat"}, Enabled: true, LayerActive: true, ReviewStatus: commandbindings.Active,
	}}, []commandbindings.Candidate{{
		ID: "unsupported", Trigger: "palette:unsupported", CommandID: commandProductShortcutsShowID, ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: commandbindings.Global,
		Condition: commandbindings.Facts{commandbindings.Process: "editor.exe"}, Enabled: true, LayerActive: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	registry := paletteConditionTestRegistry(t)
	condition, ok := localPaletteUICondition(configuration, registry, trigger)
	if !ok || condition.BySurface["chat"] || !condition.Fallback {
		t.Fatalf("barreira de supressão ressuscitada: %+v ok=%v", condition, ok)
	}
	reviewConfiguration, err := commandbindings.NewConfiguration([]commandbindings.Default{base}, []commandbindings.Delta{{
		ID: "review-chat", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "base", Trigger: trigger,
		Effect: commandbindings.Execute, CommandID: commandProductShortcutsShowID, ArgumentsKey: `{}`, Condition: commandbindings.Facts{commandbindings.SurfaceType: "chat"}, Enabled: true, LayerActive: true, ReviewStatus: commandbindings.NeedsReview,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reviewCondition, ok := localPaletteUICondition(reviewConfiguration, registry, trigger)
	if !ok || reviewCondition.BySurface["chat"] {
		t.Fatalf("revisão pendente não permaneceu barreira: %+v ok=%v", reviewCondition, ok)
	}
	if _, ok := localPaletteUICondition(configuration, registry, "palette:unsupported"); ok {
		t.Fatal("fato de processo fora do conjunto finito foi enumerado")
	}
}

func TestLocalPaletteConditionsSuppressionIsSpecificAndCloneIsDeep(t *testing.T) {
	trigger := "palette:" + commandProductShortcutsShowID
	base := commandbindings.Default{Version: "1", Fingerprint: "base", Candidate: paletteConditionCandidate("base", trigger, nil, commandbindings.Global)}
	configuration, err := commandbindings.NewConfiguration([]commandbindings.Default{base}, []commandbindings.Delta{{
		ID: "specific-suppression", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "base", Trigger: trigger,
		Effect: commandbindings.Suppress, Enabled: true, LayerActive: true, ReviewStatus: commandbindings.Active,
		Condition: commandbindings.Facts{commandbindings.Profile: "dev", commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-1"},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	condition, ok := localPaletteUICondition(configuration, paletteConditionTestRegistry(t), trigger)
	profile, exists := condition.ByProfile["dev"]
	if !ok || !exists || profile.BySurfaceID["chat"]["chat-1"] || !profile.BySurface["chat"] || !condition.BySurfaceID["chat"]["chat-1"] {
		t.Fatalf("supressão perdeu a combinação exata de perfil/aba: %+v", condition)
	}
	clone := cloneLocalCommandPaletteCondition(condition)
	clone.ByProfile["dev"].BySurfaceID["chat"]["chat-1"] = true
	clone.BySurfaceID["chat"]["chat-1"] = false
	if condition.ByProfile["dev"].BySurfaceID["chat"]["chat-1"] || !condition.BySurfaceID["chat"]["chat-1"] {
		t.Fatal("clone compartilhou mapas aninhados de perfil/aba")
	}
}

func TestLocalPaletteConditionsFromRealSettingsMapWithoutInvocations(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	control := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Condições locais", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: control.ID, Mode: "condition", Lifecycle: "persistent", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Op: "eq", Value: "chat"}}}}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{
		LayerID: control.ID, CommandID: commandProductShortcutsShowID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"help.shortcuts.show"}`,
		Arguments: map[string]any{}, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "profile", Op: "eq", Value: "dev"}}}, Effect: "execute", Enabled: true,
	}})
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	var found *LocalCommandPaletteCondition
	foundIndex := -1
	for index := range view.LocalPaletteConditions {
		if view.LocalPaletteConditions[index].CommandID == commandProductShortcutsShowID {
			found = &view.LocalPaletteConditions[index]
			foundIndex = index
			break
		}
	}
	if found == nil || !found.ByProfile["dev"].BySurface["chat"] || found.ByProfile["dev"].BySurface["editor"] || foundIndex < 0 {
		t.Fatalf("condição real não projetada: %+v", view.LocalPaletteConditions)
	}
	for _, commandID := range view.LocalPaletteCommands {
		if commandID == commandProductShortcutsShowID {
			t.Fatal("binding contextual apareceu na lista incondicional")
		}
	}
	clone := cloneLocalCommandKeyboardMap(view)
	clone.LocalPaletteConditions[foundIndex].ByProfile["dev"].BySurface["chat"] = false
	if reflect.DeepEqual(clone.LocalPaletteConditions, view.LocalPaletteConditions) {
		t.Fatal("clone da projeção compartilhou mapas")
	}
	var invocations int64
	if err := database.DB().WithContext(context.Background()).Table("command_invocations").Where("user_id = ?", a.commandProduct.Load().principal.UserID).Count(&invocations).Error; err != nil {
		t.Fatal(err)
	}
	if invocations != 0 {
		t.Fatalf("projeção local criou invocações: %d", invocations)
	}
	encoded, err := json.Marshal(view.LocalPaletteConditions)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("DTO não serializável: %v", err)
	}
}
