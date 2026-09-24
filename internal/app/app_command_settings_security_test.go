package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"gorm.io/gorm"
)

// Usa o produto, banco e confirmação reais; apenas o presenter visual é
// substituído por uma resposta explícita do teste.
func settingsSecurityFixture(t *testing.T) (*App, <-chan map[string]any) {
	t.Helper()
	a := readyCommandProduct(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	a.ctx = ctx
	if err := commandautomation.Migrate(ctx, database.DB()); err != nil {
		t.Fatal(err)
	}
	return a, appCommandImportWailsCopyDecisions(t, a)
}

type settingsSecurityOutcome struct {
	result CommandSettingsMutation
	err    error
}

func settingsSecurityStart(t *testing.T, a *App, operation func() (CommandSettingsMutation, error)) <-chan settingsSecurityOutcome {
	t.Helper()
	done := make(chan settingsSecurityOutcome, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		result, err := operation()
		done <- settingsSecurityOutcome{result, err}
	}()
	t.Cleanup(func() {
		select {
		case <-joined:
		case <-a.ctx.Done():
			select {
			case <-joined:
			case <-time.After(3 * time.Second):
				t.Error("mutação de configurações não encerrou")
			}
		}
	})
	return done
}

func settingsSecurityFinish(t *testing.T, done <-chan settingsSecurityOutcome) settingsSecurityOutcome {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("mutação de configurações não retornou após decisão")
		return settingsSecurityOutcome{}
	}
}

func TestCommandSettingsSecurityRevokedSessionCannotSaveAfterConfirmation(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Não deve persistir", Enabled: true})
	})
	var decision map[string]any
	select {
	case decision = <-decisions:
	case result := <-done:
		t.Fatalf("operação terminou antes da confirmação: %+v", result)
	case <-time.After(5 * time.Second):
		t.Fatal("confirmação não foi apresentada")
	}
	if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	finishCommandDecision(t, a.questionnaireMgr, decision, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	outcome := settingsSecurityFinish(t, done)
	if outcome.err == nil || outcome.result.Committed || outcome.result.Published {
		t.Fatalf("sessão revogada autorizou gravação: %+v", outcome)
	}
	var count int64
	if err := database.DB().Model(&commandconfig.Layer{}).Where("name = ?", "Não deve persistir").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("camada de sessão revogada persistida: count=%d err=%v", count, err)
	}
}

func TestCommandSettingsSecurityDefaultSuppressionChangesRealExecutionAndRestores(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	for _, suppressed := range []bool{true, false} {
		done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
			return a.SetDefaultCommandSuppressed("builtin.palette.workspace.list", suppressed)
		})
		appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
		outcome := settingsSecurityFinish(t, done)
		if outcome.err != nil || !outcome.result.Committed || !outcome.result.Published {
			t.Fatalf("suppress=%v não publicou: %+v", suppressed, outcome)
		}
		// Releitura e rebuild não podem depender do estado otimista da tela.
		if err := a.rebuildCommandLifecyclePersistedConfiguration(a.ctx); err != nil {
			t.Fatal(err)
		}
		result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
		want := "succeeded"
		if suppressed {
			want = "suppressed"
		}
		if err != nil || result.Status != want {
			t.Fatalf("suppress=%v: execução=%+v err=%v", suppressed, result, err)
		}
	}
	var count int64
	if err := database.DB().Model(&commandconfig.Binding{}).Where("replaces_default_id = ?", "builtin.palette.workspace.list").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("restore deixou delta persistido: count=%d err=%v", count, err)
	}
}

func TestCommandSettingsSecurityReadDoesNotOutliveAuthority(t *testing.T) {
	for _, scenario := range []string{"epoch", "os_lock", "product"} {
		t.Run(scenario, func(t *testing.T) {
			a, _ := settingsSecurityFixture(t)
			product := a.commandProduct.Load()
			t.Cleanup(func() { a.commandProduct.Store(product) })
			db := database.DB()
			layer := commandconfig.Layer{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, Name: "PRIVATE_SETTINGS_MARKER", Enabled: true, Source: "user", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			if err := db.Create(&layer).Error; err != nil {
				t.Fatal(err)
			}
			fired := false
			const callback = "test:command-settings-authority"
			if err := db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if fired || tx.Statement.Table != "command_layers" {
					return
				}
				fired = true
				switch scenario {
				case "epoch":
					_ = tx.AddError(a.commandEpochs.InvalidateSecurity(a.ctx))
				case "os_lock":
					_ = tx.AddError(a.commandHost.SetOSSessionState(a.ctx, true, true))
				case "product":
					a.commandProduct.Store(nil)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })
			result, err := a.GetCommandSettings("pt-BR")
			if !fired {
				t.Fatal("teste não alcançou leitura")
			}
			if err == nil || len(result.Layers) != 0 || len(result.Bindings) != 0 || strings.Contains(err.Error(), layer.Name) {
				t.Fatalf("leitura liberou dados após perder autoridade: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestCommandSettingsSecurityEditorPreservesContextualBinding(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := commandconfig.Layer{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, Name: "Contextual", Enabled: true, Source: "user", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := database.DB().Create(&layer).Error; err != nil {
		t.Fatal(err)
	}
	commandID := commandProductWorkspaceListID
	binding := commandconfig.Binding{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, LayerRefKind: "user", LayerRef: layer.ID, CommandID: &commandID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyW","modifiers":["Control"]}`, Arguments: `{}`, Condition: `{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true}]}`, Effect: "execute", Enabled: true, Source: "user", ReviewStatus: "active", Presentation: `{"version":1,"icon":"contextual"}`}
	if err := database.DB().Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range snapshot.Bindings {
		if row.ID == binding.ID {
			found = true
			if row.ReadOnly || row.Inherited {
				t.Fatal("binding contextual global exposto como herdado/read-only")
			}
			if row.Arguments == nil || row.Presentation["icon"] != "contextual" || len(row.Condition.Clauses) != 1 || row.Condition.Clauses[0].Field != "app.focused" || row.Condition.Clauses[0].Value != true {
				t.Fatalf("snapshot perdeu argumentos, apresentação ou condição contextual: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("binding contextual não apareceu para inspeção")
	}
	request := CommandBindingEdit{ID: binding.ID, LayerID: layer.ID, CommandID: commandID, TriggerType: binding.TriggerType, TriggerSpec: binding.TriggerSpec, Enabled: false}
	if result, err := a.SaveCommandBinding(request); err == nil || result.Committed {
		t.Fatalf("editor simplificou configuração contextual: %+v %v", result, err)
	}
	if result, err := a.DeleteCommandBinding(binding.ID); err == nil || result.Committed {
		t.Fatalf("editor excluiu configuração somente leitura: %+v %v", result, err)
	}
	var current commandconfig.Binding
	if err := database.DB().First(&current, "id = ?", binding.ID).Error; err != nil || current.Condition != binding.Condition || !current.Enabled {
		t.Fatalf("registro contextual alterado: %+v %v", current, err)
	}
	select {
	case <-decisions:
		t.Fatal("pedido inválido chegou à confirmação")
	default:
	}
}
