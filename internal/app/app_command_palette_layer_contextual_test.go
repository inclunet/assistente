package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/wailsapi"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

type contextualLayerPaletteFixture struct {
	a        *App
	events   <-chan map[string]any
	ruleID   string
	bindings map[string]string
	view     LocalCommandKeyboardMap
	observed LocalCommandKeyboardContext
}

// The API is synchronous; exercise its private proof at the asynchronous
// handler boundary as well. Hold the real gate so ABA happens after capture
// and before the authoritative mutation, without relying on timing sleeps.
func TestContextualPaletteLayerProfileABABeforeClaim(t *testing.T) {
	for _, scenario := range []string{"unchanged", "profile-ABA", "profile-ABA-after-validation"} {
		t.Run(scenario, func(t *testing.T) {
			f := newContextualLayerPaletteFixture(t)
			p := f.a.commandProduct.Load()
			proof, err := f.a.captureLocalKeyboardContext(f.observed)
			if err != nil {
				t.Fatal(err)
			}
			id, err := uuid.NewV7()
			if err != nil {
				t.Fatal(err)
			}
			versions, err := p.host.Snapshot(context.Background(), p.principal)
			if err != nil || !versions.Unlocked {
				t.Fatalf("capture published versions: %+v %v", versions, err)
			}
			occurrence := &contextualPaletteLayerOccurrence{product: p, invocationID: id.String(), commandID: commandLayerActivateID, proof: proof, versions: versions, valid: func() bool { return p.localKeyboardContextCurrent(proof) }}
			if scenario == "profile-ABA-after-validation" {
				// Force drift after the optimistic check returned true. Only the
				// snapshot comparison protected through the claim can reject it.
				occurrence.valid = func() bool {
					current := p.localKeyboardContextCurrent(proof)
					if f.a.workspaceMgr.SetProfile("other") != nil || f.a.workspaceMgr.SetProfile("dev") != nil {
						return false
					}
					return current
				}
			}
			ctx := context.WithValue(context.Background(), contextualPaletteLayerKey{}, occurrence)
			args := json.RawMessage(`{"scope":"global","rule_id":"` + f.ruleID + `","duration_seconds":0}`)
			var handle commandexecution.ExecutionHandle
			err = f.a.commandGate.WithMutation(context.Background(), func() error {
				var startErr error
				handle, startErr = f.a.startCommandLayerAction(ctx, commandexecution.Invocation{ID: id.String(), CommandID: commandLayerActivateID, Principal: p.principal, Source: commandcatalog.Palette, Envelope: &commandcontract.Envelope{Arguments: &args}})
				if startErr != nil {
					return startErr
				}
				if scenario != "profile-ABA" {
					return nil
				}
				if err := f.a.workspaceMgr.SetProfile("other"); err != nil {
					return err
				}
				return f.a.workspaceMgr.SetProfile("dev")
			})
			if err != nil {
				t.Fatal(err)
			}
			defer handle.Cancel()
			if scenario == "profile-ABA" && occurrence.valid() {
				t.Fatal("ABA did not invalidate captured snapshot")
			}
			select {
			case outcome := <-handle.Done:
				if (outcome.Status == "succeeded") != (scenario == "unchanged") {
					t.Fatalf("unexpected claim outcome for %s: %+v", scenario, outcome)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("handler did not finish")
			}
			view, err := f.a.GetLocalCommandKeyboardMap()
			if err != nil {
				t.Fatal(err)
			}
			assertContextualLayerState(t, f, view, scenario == "unchanged")
			if scenario == "unchanged" && view.Generation == f.view.Generation {
				t.Fatal("successful control did not republish map")
			}
		})
	}
}

func newContextualLayerPaletteFixture(t *testing.T) contextualLayerPaletteFixture {
	t.Helper()
	a, events, _ := surfacePaletteFixture(t, workspace.TabTypeChat)
	a.commandCatalogAPI = wailsapi.NewCommandCatalog()
	a.wireCommandCatalog()
	for _, id := range []string{commandLayerActivateID, commandLayerToggleID, commandLayerBackID} {
		assertContextualLayerReady(t, a, id, false)
	}
	if err := a.workspaceMgr.SetProfile("dev"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	observed := LocalCommandKeyboardContext{SurfaceType: string(snapshot.Tab.Type), SurfaceID: snapshot.Tab.ID, Profile: "dev"}
	control := settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Contextual palette control", Enabled: true}})
	settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: control.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	target := settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Contextual palette manual target", Enabled: true}})
	rule := settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: target.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true}})
	settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: target.ID, CommandID: commandProductWorkspaceListID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"F12","modifiers":["Control","Alt"]}`, Arguments: map[string]any{}, Effect: "execute", Enabled: true}})
	bindings := map[string]string{}
	for _, id := range []string{commandLayerActivateID, commandLayerToggleID, commandLayerBackID} {
		ruleID := rule.ID
		if id == commandLayerBackID {
			ruleID = ""
		}
		binding := settingsContractApply(t, a, events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: control.ID, CommandID: id, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"` + id + `"}`, Arguments: map[string]any{"scope": "global", "rule_id": ruleID, "duration_seconds": 0}, Effect: "execute", Enabled: true, Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{
			{Field: "app.focused", Value: true}, {Field: "surface.type", Value: observed.SurfaceType}, {Field: "surface.id", Value: observed.SurfaceID}, {Field: "profile", Value: observed.Profile},
		}}}})
		bindings[id] = binding.ID
	}
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	return contextualLayerPaletteFixture{a: a, events: events, ruleID: rule.ID, bindings: bindings, view: view, observed: observed}
}

func assertContextualLayerReady(t *testing.T, a *App, id string, want bool) {
	t.Helper()
	items, err := a.commandCatalogAPI.ListCommands(apidto.CommandCatalogFilter{Locale: "pt-BR", Source: "palette"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == id {
			if item.Available != want {
				t.Fatalf("readiness %s: %+v; want %v", id, item, want)
			}
			return
		}
	}
	t.Fatalf("missing catalog command %s", id)
}

func assertContextualLayerState(t *testing.T, f contextualLayerPaletteFixture, view LocalCommandKeyboardMap, want bool) {
	t.Helper()
	var active int64
	if err := database.DB().Table("command_layer_activation_state").Where("rule_ref = ? AND state = ?", f.ruleID, "active").Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if (active == 1) != want || active > 1 {
		t.Fatalf("manual claims=%d; want active=%v", active, want)
	}
	found := false
	for _, binding := range view.Bindings {
		if binding.CommandID == commandProductWorkspaceListID && binding.Shortcut.Code == "F12" {
			found = true
		}
	}
	if found != want {
		t.Fatalf("published target binding=%v; want %v", found, want)
	}
}

func TestContextualPaletteLayerActionsPublishAndAudit(t *testing.T) {
	f := newContextualLayerPaletteFixture(t)
	for _, id := range []string{commandLayerActivateID, commandLayerToggleID, commandLayerBackID} {
		assertContextualLayerReady(t, f.a, id, true)
	}
	assertContextualLayerState(t, f, f.view, false)
	view := f.view
	for _, step := range []struct {
		id     string
		active bool
	}{{commandLayerActivateID, true}, {commandLayerToggleID, false}, {commandLayerToggleID, true}, {commandLayerBackID, false}} {
		result, err := f.a.ExecuteContextualPaletteLayerCommand(view.Generation, step.id, f.observed)
		if err != nil || result.Status != "succeeded" || result.InvocationID == "" || result.Output != nil {
			t.Fatalf("%s: %+v %v", step.id, result, err)
		}
		var count int64
		if err = database.DB().Table("command_invocations").Where("invocation_id = ? AND source_type = ? AND command_id = ? AND status = ?", result.InvocationID, "palette", step.id, "succeeded").Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("ledger=%d %v", count, err)
		}
		fresh, err := f.a.GetLocalCommandKeyboardMap()
		if err != nil || fresh.Generation == "" || fresh.Generation == view.Generation {
			t.Fatalf("map not republished: %+v %v", fresh, err)
		}
		assertContextualLayerState(t, f, fresh, step.active)
		if stale, err := f.a.ExecuteContextualPaletteLayerCommand(view.Generation, commandLayerActivateID, f.observed); err == nil || stale.Status == "succeeded" {
			t.Fatalf("old map admitted: %+v %v", stale, err)
		}
		view = fresh
	}
}

func TestContextualPaletteLayerRejectsInvalidAdmission(t *testing.T) {
	for _, scenario := range []string{"generation", "profile", "missing-profile", "surface-type", "surface-id", "outside-allowlist", "expiry", "session", "ordinary", "ordinary-args"} {
		t.Run(scenario, func(t *testing.T) {
			f := newContextualLayerPaletteFixture(t)
			id, generation, observed := commandLayerActivateID, f.view.Generation, f.observed
			switch scenario {
			case "generation":
				generation = "stale"
			case "profile":
				observed.Profile = "other"
			case "missing-profile":
				observed.Profile = ""
			case "surface-type":
				observed.SurfaceType = "editor"
			case "surface-id":
				observed.SurfaceID = "different-tab"
			case "outside-allowlist":
				id = commandWorkspaceCreateID
			case "expiry":
				p := f.a.commandProduct.Load()
				p.keyboardMu.Lock()
				p.keyboardMap.view.ValidUntil = time.Now().Add(-time.Second).UnixMilli()
				p.keyboardMu.Unlock()
			case "session":
				if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", f.a.commandProduct.Load().principal.SessionID).Error; err != nil {
					t.Fatal(err)
				}
			}
			var result CommandExecutionResult
			var err error
			if scenario == "ordinary" || scenario == "ordinary-args" {
				args := json.RawMessage(`{}`)
				if scenario == "ordinary-args" {
					args = json.RawMessage(`{"scope":"global","rule_id":"` + f.ruleID + `","duration_seconds":0}`)
				}
				result, err = f.a.ExecutePaletteCommand(id, args)
			} else {
				result, err = f.a.ExecuteContextualPaletteLayerCommand(generation, id, observed)
			}
			if err == nil && result.Status == "succeeded" {
				t.Fatalf("invalid admission succeeded: %+v", result)
			}
			assertContextualLayerState(t, f, f.view, false)
			var succeeded int64
			if err = database.DB().Table("command_invocations").Where("command_id = ? AND status = ?", commandLayerActivateID, "succeeded").Count(&succeeded).Error; err != nil || succeeded != 0 {
				t.Fatalf("invalid success ledger=%d %v", succeeded, err)
			}
		})
	}
}

func TestContextualPaletteLayerReadinessTracksConfiguredTarget(t *testing.T) {
	f := newContextualLayerPaletteFixture(t)
	assertContextualLayerReady(t, f.a, commandLayerActivateID, true)
	assertContextualLayerReady(t, f.a, commandLayerBackID, true)
	settingsContractApply(t, f.a, f.events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_delete", ID: f.ruleID})
	assertContextualLayerReady(t, f.a, commandLayerActivateID, false)
	assertContextualLayerReady(t, f.a, commandLayerToggleID, false)
	assertContextualLayerReady(t, f.a, commandLayerBackID, true)
	settingsContractApply(t, f.a, f.events, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_delete", ID: f.bindings[commandLayerBackID]})
	assertContextualLayerReady(t, f.a, commandLayerBackID, false)
}
