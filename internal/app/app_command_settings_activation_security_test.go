package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"gorm.io/gorm"
)

func settingsActivationSecurityConfirmed(t *testing.T, a *App, decisions <-chan map[string]any, operation func() (CommandSettingsMutation, error)) CommandSettingsMutation {
	t.Helper()
	done := settingsSecurityStart(t, a, operation)
	appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
	outcome := settingsSecurityFinish(t, done)
	if outcome.err != nil || !outcome.result.Committed || !outcome.result.Published || outcome.result.ID == "" {
		t.Fatalf("mutação confirmada não publicou: %+v", outcome)
	}
	return outcome.result
}

func settingsActivationSecurityLayerAndRule(t *testing.T, a *App, decisions <-chan map[string]any) (string, string) {
	t.Helper()
	layer := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Camada de segurança", Enabled: true})
	})
	rule := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.PrepareManualCommandLayer(layer.ID)
	})
	return layer.ID, rule.ID
}

func TestCommandSettingsActivationSecurityRevokedSessionCannotPrepareAfterConfirmation(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Não deve preparar", Enabled: true})
	})
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		return a.PrepareManualCommandLayer(layer.ID)
	})
	var decision map[string]any
	select {
	case decision = <-decisions:
	case outcome := <-done:
		t.Fatalf("Prepare terminou antes da confirmação: %+v", outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("confirmação de Prepare não foi apresentada")
	}
	if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	finishCommandDecision(t, a.questionnaireMgr, decision, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	outcome := settingsSecurityFinish(t, done)
	if outcome.err == nil || outcome.result.Committed || outcome.result.Published {
		t.Fatalf("sessão revogada autorizou Prepare: %+v", outcome)
	}
	var count int64
	if err := database.DB().Table("command_layer_activation_rules").Where("layer_ref = ?", layer.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("regra de sessão revogada persistida: count=%d err=%v", count, err)
	}
}

func TestCommandSettingsActivationSecurityReadAuthorityInvalidationCannotCreateClaim(t *testing.T) {
	for _, scenario := range []string{"session", "os_lock"} {
		t.Run(scenario, func(t *testing.T) {
			a, decisions := settingsSecurityFixture(t)
			layerID, ruleID := settingsActivationSecurityLayerAndRule(t, a, decisions)
			db := database.DB()
			fired := false
			const callback = "test:command-settings-activation-authority"
			if err := db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if fired || tx.Statement.Table != "command_layer_activation_rules" {
					return
				}
				fired = true
				if scenario == "session" {
					_ = tx.AddError(tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error)
					return
				}
				_ = tx.AddError(a.commandHost.SetOSSessionState(a.ctx, true, true))
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })

			result, err := a.SetCommandLayerActive(layerID, true)
			if !fired {
				t.Fatal("teste não alcançou a leitura das activation rules")
			}
			if err == nil || result.Committed || result.Published {
				t.Fatalf("autoridade perdida permitiu Pin: %+v err=%v", result, err)
			}
			var count int64
			if queryErr := db.Table("command_layer_activation_state").Where("layer_ref = ? AND rule_ref = ?", layerID, ruleID).Count(&count).Error; queryErr != nil || count != 0 {
				t.Fatalf("claim criada após invalidação: count=%d err=%v", count, queryErr)
			}
		})
	}
}

func TestCommandSettingsActivationSecurityRevocationInsideCreateClaimRollsBackTransaction(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layerID, ruleID := settingsActivationSecurityLayerAndRule(t, a, decisions)
	db := database.DB()
	fired := false
	var insertedInsideTx int64
	const callback = "test:command-settings-activation-after-claim"
	if err := db.Callback().Create().After("gorm:create").Register(callback, func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "command_layer_activation_state" {
			return
		}
		fired = true
		if err := tx.Session(&gorm.Session{NewDB: true}).Model(&commandactivation.Claim{}).Where("layer_ref = ? AND rule_ref = ?", layerID, ruleID).Count(&insertedInsideTx).Error; err != nil {
			_ = tx.AddError(err)
			return
		}
		if err := tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error; err != nil {
			_ = tx.AddError(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callback) })

	result, err := a.SetCommandLayerActive(layerID, true)
	if !fired || insertedInsideTx != 1 {
		t.Fatalf("teste não inseriu a claim antes de revogar a sessão: fired=%v count=%d", fired, insertedInsideTx)
	}
	if err == nil || result.Committed || result.Published {
		t.Fatalf("revogação concorrente permitiu Pin: %+v err=%v", result, err)
	}
	var claims int64
	if queryErr := db.Table("command_layer_activation_state").Where("layer_ref = ? AND rule_ref = ?", layerID, ruleID).Count(&claims).Error; queryErr != nil || claims != 0 {
		t.Fatalf("claim sobreviveu ao rollback: count=%d err=%v", claims, queryErr)
	}
	var revoked int64
	if err := db.Table("sessions").Where("id = ? AND revoked_at IS NOT NULL", a.commandProduct.Load().principal.SessionID).Count(&revoked).Error; err != nil {
		t.Fatal(err)
	}
	if revoked != 0 {
		t.Fatal("revogação do callback vazou para fora da transação")
	}
}

func TestCommandSettingsActivationSecuritySetActiveFalsePreservesOtherOriginClaim(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layerID, ruleID := settingsActivationSecurityLayerAndRule(t, a, decisions)
	p := a.commandProduct.Load()
	epoch, err := p.epochs.CaptureAuthenticated(a.commandBridgeContext(), func(ctx context.Context) (string, string, error) {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil {
			return "", "", err
		}
		return current.UserID, current.SessionID, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	origin := commandactivation.Origin{Type: "ui_action", SessionID: p.principal.SessionID, DeviceID: "outra-origem"}
	stackKey, err := commandactivation.ManualStackKey(origin)
	if err != nil {
		t.Fatal(err)
	}
	device := origin.DeviceID
	claim := commandactivation.Claim{
		ActivationID:       appCommandPortabilityUUID(t),
		LayerRefKind:       commandactivation.UserRef,
		LayerRef:           layerID,
		RuleRefKind:        commandactivation.UserRef,
		RuleRef:            ruleID,
		UserID:             p.principal.UserID,
		AuthContextType:    "local_session",
		AuthContextID:      p.principal.SessionID,
		AuthGeneration:     epoch.AuthGeneration,
		SecurityGeneration: epoch.SecurityGeneration,
		SourceType:         "manual",
		SourceInstanceID:   &device,
		State:              commandactivation.StateActive,
		ManualStackKey:     &stackKey,
		ActivatedAt:        time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	if err := database.DB().Create(&claim).Error; err != nil {
		t.Fatalf("claim da outra origem não foi criada: %v", err)
	}

	result, err := a.SetCommandLayerActive(layerID, false)
	if err != nil || !result.Committed || !result.Published {
		t.Fatalf("desativação sem claim da origem atual falhou: %+v err=%v", result, err)
	}
	var state commandactivation.Claim
	if err := database.DB().Where("activation_id = ?", claim.ActivationID).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.State != commandactivation.StateActive || state.ManualStackKey == nil || *state.ManualStackKey != stackKey {
		t.Fatalf("SetActive(false) removeu ou alterou claim de outra origem: %+v", state)
	}
	snapshot, err := a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	for _, layer := range snapshot.Layers {
		if layer.ID == layerID {
			if !layer.Active || layer.ManualActive || !layer.ManualReady {
				t.Fatalf("atividade de outra origem confundida com ativação desta tela: %+v", layer)
			}
			return
		}
	}
	t.Fatal("camada da outra origem não apareceu na projeção")
}
