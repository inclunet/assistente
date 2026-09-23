package app

import (
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/database"
)

func contextualPaletteCandidate(id, command string, facts commandbindings.Facts) commandbindings.Candidate {
	return commandbindings.Candidate{ID: id, Trigger: "palette:" + command, CommandID: command, ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: commandbindings.Application, Condition: facts, Enabled: true, LayerActive: true}
}

func TestContextualPaletteProjectionFocusProfileAndClassIsolation(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	for _, command := range []string{commandWorkspaceCreateID, commandWorkspaceChatOpenID, commandEditorModeMarkdownID, commandEditorModeRichID, commandEditorModeViewID, commandConversationClearID, commandTerminalInterruptID, commandTerminalSessionCreateID, commandTerminalSessionCloseID, commandChatSendID, commandMessageCopyID, commandMessageDeleteID, commandEditorFormatBoldID, commandEditorFileSaveID} {
		config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{contextualPaletteCandidate("supported", command, commandbindings.Facts{commandbindings.SurfaceType: "editor"})})
		if err != nil {
			t.Fatal(err)
		}
		if got := contextualPaletteUIConditions(config, registry); len(got) != 1 || !got[0].BySurface["editor"] {
			t.Fatalf("supported command %s absent: %+v", command, got)
		}
		if got := localPaletteUIConditions(config, registry); len(got) != 0 {
			t.Fatalf("contextual command %s leaked into LOCAL_UI: %+v", command, got)
		}
	}
	for _, field := range []commandbindings.Field{commandbindings.AppFocused, commandbindings.Profile, commandbindings.SurfaceType} {
		t.Run(string(field), func(t *testing.T) {
			value := any("dev")
			if field == commandbindings.AppFocused {
				value = true
			}
			if field == commandbindings.SurfaceType {
				value = "chat"
			}
			candidate := contextualPaletteCandidate("workspace", commandWorkspaceTabCloseID, commandbindings.Facts{field: value})
			config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
			if err != nil {
				t.Fatal(err)
			}
			conditions := contextualPaletteUIConditions(config, registry)
			if len(conditions) != 1 {
				t.Fatalf("missing projection: %+v", conditions)
			}
			c := conditions[0]
			switch field {
			case commandbindings.AppFocused:
				if !c.Fallback || paletteSelectionForClass(config, registry, candidate.Trigger, candidate.CommandID, commandbindings.Facts{field: false}, commandExecutionDurable) {
					t.Fatal("focus mismatch")
				}
			case commandbindings.Profile:
				if c.Fallback || !c.ByProfile["dev"].Fallback {
					t.Fatalf("profile mismatch: %+v", c)
				}
			case commandbindings.SurfaceType:
				if c.Fallback || !c.BySurface["chat"] {
					t.Fatalf("surface mismatch: %+v", c)
				}
			}
			if got := localPaletteUIConditions(config, registry); len(got) != 0 {
				t.Fatalf("durable leaked into local UI: %+v", got)
			}
		})
	}
	for _, command := range []string{"navigation.settings.open", "profiles.create", "profiles.update", "tasklists.create", "tasklists.update", commandProductWorkspaceListID} {
		if _, exists := registry.Lookup(command); !exists {
			t.Fatalf("negative fixture is not registered: %s", command)
		}
		candidate := contextualPaletteCandidate("excluded", command, commandbindings.Facts{commandbindings.AppFocused: true})
		config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
		if err != nil {
			t.Fatal(err)
		}
		if got := contextualPaletteUIConditions(config, registry); len(got) != 0 {
			t.Fatalf("unsupported command %s projected: %+v", command, got)
		}
	}
	for _, facts := range []commandbindings.Facts{nil, {commandbindings.Process: "editor.exe"}, {commandbindings.Device: "deck"}} {
		config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{contextualPaletteCandidate("excluded", commandWorkspaceTabCloseID, facts)})
		if err != nil {
			t.Fatal(err)
		}
		if got := contextualPaletteUIConditions(config, registry); len(got) != 0 {
			t.Fatalf("unsupported/unconditional facts projected: %+v", got)
		}
	}
}

func TestContextualPaletteProjectionBarriersAndArguments(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	base := commandbindings.Default{Version: "1", Fingerprint: "fp", Candidate: contextualPaletteCandidate("base", commandWorkspaceTabCloseID, nil)}
	for _, review := range []commandbindings.ReviewStatus{commandbindings.Active, commandbindings.NeedsReview} {
		config, err := commandbindings.NewConfiguration([]commandbindings.Default{base}, []commandbindings.Delta{{ID: "barrier", DefaultID: "base", DefaultVersion: "1", DefaultFingerprint: "fp", Trigger: base.Candidate.Trigger, Effect: commandbindings.Suppress, Enabled: true, LayerActive: true, ReviewStatus: review, Condition: commandbindings.Facts{commandbindings.Profile: "dev", commandbindings.SurfaceType: "chat", commandbindings.SurfaceID: "chat-1"}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := contextualPaletteUIConditions(config, registry)
		if len(got) != 1 {
			t.Fatalf("missing barrier projection: %+v", got)
		}
		c := got[0]
		if !c.Fallback || !c.BySurfaceID["chat"]["chat-1"] || !c.ByProfile["dev"].Fallback || !c.ByProfile["dev"].BySurface["chat"] || c.ByProfile["dev"].BySurfaceID["chat"]["chat-1"] {
			t.Fatalf("barrier/fallback mismatch: %+v", c)
		}
	}
	first := contextualPaletteCandidate("first", commandWorkspaceTabCloseID, commandbindings.Facts{commandbindings.SurfaceType: "chat"})
	second := first
	second.ID, second.CommandID = "second", commandWorkspaceCreateID
	config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{first, second})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := config.Resolve(first.Trigger, first.Condition, nil)
	if err != nil || resolved.Status != commandbindings.Conflict {
		t.Fatalf("fixture not ambiguous: %+v %v", resolved, err)
	}
	if got := contextualPaletteUIConditions(config, registry); len(got) != 1 || got[0].BySurface["chat"] {
		t.Fatalf("ambiguity authorized: %+v", got)
	}
	first.ArgumentsKey = `{"unexpected":true}`
	config, err = commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{first})
	if err != nil {
		t.Fatal(err)
	}
	if got := contextualPaletteUIConditions(config, registry); len(got) != 1 || got[0].BySurface["chat"] {
		t.Fatalf("arguments authorized: %+v", got)
	}
}

func TestContextualPaletteProjectionRealSettingsAndDeepClone(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Workspace palette", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "condition", Lifecycle: "persistent", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "profile", Value: "dev"}}}}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: commandWorkspaceTabCloseID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"workspace.tab.close"}`, Arguments: map[string]any{}, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: "chat"}, {Field: "surface.id", Value: "chat-1"}}}}})
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	index := -1
	for i, c := range view.ContextualPaletteConditions {
		if c.CommandID == commandWorkspaceTabCloseID {
			index = i
		}
	}
	if index < 0 || !view.ContextualPaletteConditions[index].ByProfile["dev"].BySurfaceID["chat"]["chat-1"] {
		t.Fatalf("missing configured projection: %+v", view.ContextualPaletteConditions)
	}
	for _, c := range view.LocalPaletteConditions {
		if c.CommandID == commandWorkspaceTabCloseID {
			t.Fatal("workspace command in local UI")
		}
	}
	clone := cloneLocalCommandKeyboardMap(view)
	originalSurface := view.ContextualPaletteConditions[index].BySurface["chat"]
	clone.ContextualPaletteConditions[index].ByProfile["dev"].BySurfaceID["chat"]["chat-1"] = false
	clone.ContextualPaletteConditions[index].BySurface["chat"] = !originalSurface
	if !view.ContextualPaletteConditions[index].ByProfile["dev"].BySurfaceID["chat"]["chat-1"] || view.ContextualPaletteConditions[index].BySurface["chat"] != originalSurface {
		t.Fatal("shared nested clone")
	}
	var count int64
	if err := database.DB().Table("command_invocations").Where("user_id = ?", a.commandProduct.Load().principal.UserID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("projection wrote ledger: %d %v", count, err)
	}
}
