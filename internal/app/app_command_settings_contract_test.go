package app

import (
	"errors"
	"testing"
	"time"

	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
)

func settingsContractApply(t *testing.T, a *App, decisions <-chan map[string]any, req CommandSettingsMutationRequest) CommandSettingsMutation {
	t.Helper()
	snapshot, err := a.GetCommandSettingsForScope("pt-BR", string(req.Scope))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision < 1 || snapshot.Fingerprint == "" {
		t.Fatalf("snapshot has no usable edit version: revision=%d fingerprint=%q", snapshot.Revision, snapshot.Fingerprint)
	}
	req.Locale, req.ExpectedRevision, req.ExpectedFingerprint = "pt-BR", snapshot.Revision, snapshot.Fingerprint
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) { return a.MutateCommandSettings(req) })
	select {
	case payload := <-decisions:
		finishCommandDecision(t, a.questionnaireMgr, payload, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	case early := <-done:
		t.Fatalf("%s terminou antes da decisão: resultado=%+v erro=%v", req.Operation, early.result, early.err)
	case <-time.After(5 * time.Second):
		t.Fatalf("%s não apresentou decisão", req.Operation)
	}
	out := settingsSecurityFinish(t, done)
	if out.err != nil || !out.result.Committed || !out.result.Published {
		t.Fatalf("%s: %+v", req.Operation, out)
	}
	return out.result
}

func TestCommandSettingsContractWorkspaceRestoreDoesNotRemoveGlobalLayer(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	global := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Global preservada", Enabled: true, ResolutionPriority: 7}})
	local := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Só neste workspace", Enabled: true, ResolutionPriority: 9}})
	snapshot, err := a.GetCommandSettingsForScope("pt-BR", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	foundGlobal, foundLocal := false, false
	for _, layer := range snapshot.Layers {
		if layer.ID == global.ID {
			foundGlobal = layer.WorkspaceID == "" && layer.ResolutionPriority == 7
		}
		if layer.ID == local.ID {
			foundLocal = layer.WorkspaceID != "" && layer.ResolutionPriority == 9
		}
	}
	if !foundGlobal || !foundLocal {
		t.Fatalf("wrong scope composition: %+v", snapshot.Layers)
	}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "config_restore"})
	snapshot, err = a.GetCommandSettingsForScope("pt-BR", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	foundGlobal = false
	for _, layer := range snapshot.Layers {
		if layer.ID == global.ID {
			foundGlobal = true
		}
		if layer.ID == local.ID {
			t.Fatal("workspace restore retained local customization")
		}
	}
	if !foundGlobal {
		t.Fatal("workspace restore deleted inherited global configuration")
	}
}

func TestCommandSettingsContractStaleEditorCannotOverwriteNewConfiguration(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	before, err := a.GetCommandSettingsForScope("en", "global")
	if err != nil {
		t.Fatal(err)
	}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Concurrent edit", Enabled: true}})
	_, err = a.MutateCommandSettings(CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, ExpectedRevision: before.Revision, ExpectedFingerprint: before.Fingerprint, Operation: "config_restore"})
	if !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("stale editor accepted: %v", err)
	}
	select {
	case <-decisions:
		t.Fatal("stale edit presented a decision")
	default:
	}
}

func TestCommandSettingsContractEditDuringConfirmationRejectsEarlierDecision(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	before, err := a.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatal(err)
	}
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		return a.MutateCommandSettings(CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, ExpectedRevision: before.Revision, ExpectedFingerprint: before.Fingerprint, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Decisão antiga", Enabled: true}})
	})
	var earlier map[string]any
	select {
	case earlier = <-decisions:
	case out := <-done:
		t.Fatalf("earlier mutation ended before decision: %+v", out)
	case <-a.ctx.Done():
		t.Fatal("no decision for earlier mutation")
	}
	// Simula outro escritor persistindo durante a confirmação. A UI mantém
	// uma única decisão bloqueante, portanto abrir uma segunda aqui testaria
	// a fila de diálogos, não o CAS da configuração. Não incrementamos a
	// geração: a fotografia do conteúdo também precisa recusar esse caso.
	concurrent := commandconfig.Layer{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, Name: "Edição concorrente", Enabled: true, Source: "user", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := database.DB().Create(&concurrent).Error; err != nil {
		t.Fatal(err)
	}
	finishCommandDecision(t, a.questionnaireMgr, earlier, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	out := settingsSecurityFinish(t, done)
	if out.err == nil || out.result.Committed || out.result.Published {
		t.Fatalf("earlier confirmation accepted after concurrent commit: %+v", out)
	}
	var after []commandconfig.Layer
	if err := database.DB().Where("user_id = ?", a.currentUserID).Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range after {
		if row.Name == "Decisão antiga" {
			t.Fatal("stale decision wrote configuration")
		}
		found = found || row.Name == "Edição concorrente"
	}
	if !found {
		t.Fatal("concurrent confirmed edit was lost")
	}
}

func TestCommandSettingsContractContextualRuleActuallyResolves(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Somente chat", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: "workspace.tab.next", TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyY","modifiers":["Control","Shift"]}`, Effect: "execute", Enabled: true}})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "condition", Lifecycle: "persistent", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: "chat"}}}}})
	p := a.commandProduct.Load()
	configuration, _, _, err := p.host.ResolutionSnapshot(a.ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{"chat", "editor"} {
		result, err := configuration.Resolve("keyboard.local:Control+Shift+KeyY", commandbindings.Facts{commandbindings.SurfaceType: surface, commandbindings.AppFocused: true}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if surface == "chat" && (result.Status != commandbindings.Selected || result.CommandID != "workspace.tab.next") {
			t.Fatalf("saved rule does not run in chat: %+v", result)
		}
		if surface != "chat" && result.Status != commandbindings.NoMatch {
			t.Fatalf("rule escaped its context: %+v", result)
		}
	}
	keyboard, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	foundContextual := false
	for _, row := range keyboard.ContextualBindings {
		if row.Shortcut.Code != "KeyY" {
			continue
		}
		foundContextual = true
		if row.BySurface["chat"] == nil || row.BySurface["chat"].CommandID != "workspace.tab.next" || row.Fallback != nil || row.BySurface["editor"] != nil {
			t.Fatalf("desktop map does not implement the configured surface gate: %+v", row)
		}
	}
	if !foundContextual {
		t.Fatal("saved condition was not published to desktop keyboard")
	}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_disable", ID: rule.ID})
	configuration, _, _, err = p.host.ResolutionSnapshot(a.ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	result, err := configuration.Resolve("keyboard.local:Control+Shift+KeyY", commandbindings.Facts{commandbindings.SurfaceType: "chat"}, nil)
	if err != nil || result.Status != commandbindings.NoMatch {
		t.Fatalf("disabled rule still runs: %+v %v", result, err)
	}
}

func TestCommandSettingsContractWorkspaceManualActivationPublishesAndReportsState(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Ativação local", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeWorkspace, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: "workspace.tab.next", TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyY","modifiers":["Control","Shift"]}`, Effect: "execute", Enabled: true}})
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		return a.PrepareManualCommandLayerForScope("workspace", layer.ID)
	})
	appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
	prepared := settingsSecurityFinish(t, done)
	if prepared.err != nil || !prepared.result.Committed || !prepared.result.Published {
		t.Fatalf("prepare: %+v", prepared)
	}
	for _, active := range []bool{true, false} {
		out, err := a.SetCommandLayerActiveForScope("workspace", layer.ID, active)
		if err != nil || !out.Committed || !out.Published {
			t.Fatalf("active=%v: %+v %v", active, out, err)
		}
		snapshot, err := a.GetCommandSettingsForScope("pt-BR", "workspace")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range snapshot.Layers {
			if row.ID == layer.ID {
				found = true
				if !row.ManualReady || row.ManualActive != active || row.Active != active {
					t.Fatalf("snapshot does not reflect manual claim active=%v: %+v", active, row)
				}
			}
		}
		if !found {
			t.Fatal("workspace layer disappeared")
		}
		keyboard, err := a.GetLocalCommandKeyboardMap()
		if err != nil {
			t.Fatal(err)
		}
		published := false
		for _, row := range keyboard.Bindings {
			if row.Shortcut.Code == "KeyY" && row.CommandID == "workspace.tab.next" {
				published = true
			}
		}
		if published != active {
			t.Fatalf("manual claim=%v but shortcut published=%v", active, published)
		}
	}
}

func TestCommandSettingsContractReportsUnsupportedNonWorkspaceContextualDurableShortcut(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Contexto auditado", Enabled: true}})
	binding := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: "workspace.tab.chat.create", TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyY","modifiers":["Control","Shift"]}`, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Op: "eq", Value: "profiles"}}}}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	snapshot, err := a.GetCommandSettingsForScope("pt-BR", "global")
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range snapshot.Diagnostics {
		if diagnostic.ResourceID == binding.ID && diagnostic.Code == "unsupported_contextual_binding" {
			return
		}
	}
	t.Fatal("saved but unpublished contextual durable shortcut has no diagnosis")
}
