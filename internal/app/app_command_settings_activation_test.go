package app

import (
	"errors"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
)

func TestCommandSettingsManualPrepareIdempotentAndDisabledClaimRemovable(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	confirmed := func(call func() (CommandSettingsMutation, error)) CommandSettingsMutation {
		t.Helper()
		done := settingsSecurityStart(t, a, call)
		appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
		outcome := settingsSecurityFinish(t, done)
		if outcome.err != nil || !outcome.result.Committed || !outcome.result.Published {
			t.Fatalf("mutação falhou: %+v", outcome)
		}
		return outcome.result
	}
	layer := confirmed(func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Camada manual", Enabled: true})
	})
	if result, err := a.SetCommandLayerActive(layer.ID, true); err == nil || result.Committed {
		t.Fatalf("ativou sem preparar regra: %+v err=%v", result, err)
	}
	rule := confirmed(func() (CommandSettingsMutation, error) { return a.PrepareManualCommandLayer(layer.ID) })
	repeated, err := a.PrepareManualCommandLayer(layer.ID)
	if err != nil || !repeated.Committed || repeated.ID != rule.ID {
		t.Fatalf("preparar repetidamente não é idempotente: %+v err=%v", repeated, err)
	}
	var stored commandactivation.Rule
	if err := database.DB().First(&stored, "id = ?", rule.ID).Error; err != nil || stored.LayerRef != layer.ID {
		t.Fatalf("ID retornado não identifica a regra: %+v err=%v", stored, err)
	}
	var count int64
	if err := database.DB().Model(&commandactivation.Rule{}).Where("layer_ref = ?", layer.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("regras duplicadas: count=%d err=%v", count, err)
	}
	assertLayer := func(active, manual bool) {
		t.Helper()
		snapshot, err := a.GetCommandSettings("pt-BR")
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range snapshot.Layers {
			if row.ID == layer.ID {
				if !row.ManualReady || row.Active != active || row.ManualActive != manual {
					t.Fatalf("estado incorreto: %+v", row)
				}
				return
			}
		}
		t.Fatal("camada ausente")
	}
	assertLayer(false, false)
	if result, err := a.SetCommandLayerActive(layer.ID, true); err != nil || !result.Published {
		t.Fatalf("pin: %+v %v", result, err)
	}
	assertLayer(true, true)
	confirmed(func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{ID: layer.ID, Name: "Camada manual", Enabled: false})
	})
	assertLayer(false, true)
	if result, err := a.SetCommandLayerActive(layer.ID, true); err == nil || result.Committed {
		t.Fatalf("ativou camada disabled: %+v %v", result, err)
	}
	if result, err := a.SetCommandLayerActive(layer.ID, false); err != nil || !result.Published {
		t.Fatalf("back disabled: %+v %v", result, err)
	}
	assertLayer(false, false)
	confirmed(func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{ID: layer.ID, Name: "Camada manual", Enabled: true})
	})
	assertLayer(false, false)
}

func TestCommandSettingsManualRuleCompatibilityAndDuplicateMutationGuard(t *testing.T) {
	rule := commandactivation.Rule{ID: "rule", UserID: "owner", LayerRefKind: commandactivation.UserRef, LayerRef: "layer", RuleRefKind: commandactivation.UserRef,
		RuleRef: "rule", Mode: commandactivation.ModeManual, Condition: `{}`, Lifecycle: commandactivation.LifecyclePersistent, Enabled: true, Source: "user", ReviewStatus: "active"}
	snapshot := commandconfig.Snapshot{Scope: commandconfig.Scope{UserID: rule.UserID}, ActivationRules: []commandactivation.Rule{rule}}
	if found, ok := commandSettingsCompatibleManualRule(snapshot, "layer"); !ok || found.ID != rule.ID {
		t.Fatal("regra válida recusada")
	}
	intent := commandconfig.MutationIntent{Operation: commandconfig.RuleCreate, Rule: &rule}
	diff := commandconfig.MutationDiff{Scope: snapshot.Scope, BeforeActivationRules: snapshot.ActivationRules}
	if err := validateCommandSettingsMutationDiff(intent, diff); !errors.Is(err, errCommandSettingsManualRuleAlreadyPrepared) {
		t.Fatalf("criação duplicada aceita: %v", err)
	}
	for _, change := range []func(*commandactivation.Rule){
		func(r *commandactivation.Rule) { r.UserID = "other" },
		func(r *commandactivation.Rule) { r.Condition = `{"advanced":true}` },
		func(r *commandactivation.Rule) { r.Enabled = false },
		func(r *commandactivation.Rule) { r.ReviewStatus = "needs_review" },
		func(r *commandactivation.Rule) { r.Mode = commandactivation.ModeEvent },
		func(r *commandactivation.Rule) { workspace := "workspace"; r.WorkspaceID = &workspace },
		func(r *commandactivation.Rule) { event := "event"; r.EventName = &event },
	} {
		copy := rule
		change(&copy)
		snapshot.ActivationRules = []commandactivation.Rule{copy}
		if _, ok := commandSettingsCompatibleManualRule(snapshot, "layer"); ok {
			t.Fatalf("regra incompatível aceita: %+v", copy)
		}
	}
	duplicate := rule
	duplicate.ID, duplicate.RuleRef = "second", "second"
	snapshot.ActivationRules = []commandactivation.Rule{rule, duplicate}
	if _, ok := commandSettingsCompatibleManualRule(snapshot, "layer"); ok {
		t.Fatal("escolheu regra ambígua")
	}
}
