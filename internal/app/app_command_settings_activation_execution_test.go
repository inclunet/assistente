package app

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"gorm.io/gorm"
)

// Exercita as mesmas fachadas da tela e o ingresso real da paleta. Não
// instala claims diretamente nem substitui a resolução por um mock.
func TestCommandSettingsManualActivationChangesExecutedBinding(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	confirmed := func(call func() (CommandSettingsMutation, error)) CommandSettingsMutation {
		t.Helper()
		done := settingsSecurityStart(t, a, call)
		appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
		outcome := settingsSecurityFinish(t, done)
		if outcome.err != nil || !outcome.result.Committed || !outcome.result.Published {
			t.Fatalf("mutação confirmada não publicou: %+v", outcome)
		}
		return outcome.result
	}
	layer := confirmed(func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Paleta pessoal", Enabled: true})
	})
	binding := confirmed(func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer.ID, CommandID: commandProductWorkspaceListID,
			TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"workspace.list"}`, Enabled: true})
	})
	assertExecution := func(wantPersonal bool) {
		t.Helper()
		result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
		if err != nil || result.Status != "succeeded" {
			t.Fatalf("paleta não executou: %+v err=%v", result, err)
		}
		var row struct{ BindingIDs string }
		if err := database.DB().Table("command_invocations").Select("binding_ids").Where("invocation_id = ?", result.InvocationID).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		var ids []string
		if err := json.Unmarshal([]byte(row.BindingIDs), &ids); err != nil {
			t.Fatal(err)
		}
		if slices.Contains(ids, binding.ID) != wantPersonal {
			t.Fatalf("personal=%v: bindings executados=%v", wantPersonal, ids)
		}
		if !wantPersonal && !slices.Contains(ids, "builtin.palette.workspace.list") {
			t.Fatalf("default não voltou após desativação: %v", ids)
		}
	}
	assertExecution(false) // Habilitar/salvar não ativa.
	confirmed(func() (CommandSettingsMutation, error) { return a.PrepareManualCommandLayer(layer.ID) })
	assertExecution(false) // Preparar a regra também não ativa.
	for _, active := range []bool{true, true, false, false} {
		result, err := a.SetCommandLayerActive(layer.ID, active)
		if err != nil || !result.Committed || !result.Published {
			t.Fatalf("active=%v: %+v err=%v", active, result, err)
		}
		assertExecution(active)
		if err := a.rebuildCommandLifecyclePersistedConfiguration(a.ctx); err != nil {
			t.Fatal(err)
		}
		assertExecution(active) // Rebuild não pode ressuscitar uma claim retirada.
	}
	if result, err := a.SetCommandLayerActive(layer.ID, true); err != nil || !result.Published {
		t.Fatalf("pin antes da restauração: %+v err=%v", result, err)
	}
	if err := a.commandEpochs.InvalidateSecurity(a.ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(a.ctx); err != nil {
		t.Fatal(err)
	}
	assertExecution(true)
	if result, err := a.SetCommandLayerActive(layer.ID, false); err != nil || !result.Published {
		t.Fatalf("claim restaurada não ficou removível: %+v err=%v", result, err)
	}
	assertExecution(false)
}

func TestCommandSettingsManualActivationCommittedWithoutPublicationClosesOldMap(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	confirmed := func(call func() (CommandSettingsMutation, error)) CommandSettingsMutation {
		t.Helper()
		done := settingsSecurityStart(t, a, call)
		appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
		outcome := settingsSecurityFinish(t, done)
		if outcome.err != nil || !outcome.result.Published {
			t.Fatalf("preparação falhou: %+v", outcome)
		}
		return outcome.result
	}
	layer := confirmed(func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Falha de publicação", Enabled: true})
	})
	confirmed(func() (CommandSettingsMutation, error) { return a.PrepareManualCommandLayer(layer.ID) })
	inserted := false
	const createCallback = "test:settings-activation-inserted"
	const queryCallback = "test:settings-activation-publication-failure"
	db := database.DB()
	if err := db.Callback().Create().After("gorm:create").Register(createCallback, func(tx *gorm.DB) {
		if tx.Statement.Table == "command_layer_activation_state" && tx.Error == nil {
			inserted = true
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(createCallback) })
	if err := db.Callback().Query().Before("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
		if inserted && tx.Statement.Table == "command_layers" {
			_ = tx.AddError(errors.New("publicação indisponível no teste"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(queryCallback) })
	result, err := a.SetCommandLayerActive(layer.ID, true)
	if err != nil || !result.Committed || result.Published {
		t.Fatalf("commit e publicação foram confundidos: %+v err=%v", result, err)
	}
	var claims int64
	if err := db.Model(&commandactivation.Claim{}).Where("layer_ref = ? AND state = ?", layer.ID, commandactivation.StateActive).Count(&claims).Error; err != nil || claims != 1 {
		t.Fatalf("claim committed não preservada: count=%d err=%v", claims, err)
	}
	execution, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err == nil && execution.Status == "succeeded" {
		t.Fatal("mapa antigo continuou executável após commit sem publicação")
	}
	if err := db.Callback().Query().Remove(queryCallback); err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecycleProjection(a.ctx, false); err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range snapshot.Layers {
		if row.ID == layer.ID {
			if !row.Active || !row.ManualActive {
				t.Fatalf("rebuild perdeu ativação: %+v", row)
			}
			return
		}
	}
	t.Fatal("camada não encontrada após rebuild")
}
