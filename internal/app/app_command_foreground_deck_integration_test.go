package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandadapter"
	"assistente/internal/commandbridge"
	"assistente/internal/commandforeground"
	"assistente/internal/commandinput"
	"assistente/internal/commandledger"
	"assistente/internal/database"
)

type controlledDeckForegroundReader struct {
	mu         sync.Mutex
	calls      int
	snapshot   commandforeground.Snapshot
	err        error
	blockCall  int
	captured   chan int
	continueAt chan struct{}
}

func (r *controlledDeckForegroundReader) Capture(ctx context.Context) (commandforeground.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return commandforeground.Snapshot{}, err
	}
	r.mu.Lock()
	r.calls++
	call, snapshot, captureErr := r.calls, r.snapshot, r.err
	block := r.blockCall == call
	r.mu.Unlock()
	if block {
		r.captured <- call
		select {
		case <-r.continueAt:
		case <-ctx.Done():
			return commandforeground.Snapshot{}, ctx.Err()
		}
	}
	return snapshot, captureErr
}

func (r *controlledDeckForegroundReader) callsMade() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *controlledDeckForegroundReader) setSnapshot(snapshot commandforeground.Snapshot) {
	r.mu.Lock()
	r.snapshot = snapshot
	r.mu.Unlock()
}

func TestCommandForegroundDeckLayerActivatesFromCapturedProcess(t *testing.T) {
	for _, scenario := range []struct {
		name              string
		previewProcess    string
		pressProcess      string
		captureErr        error
		wantPreviewAction bool
		wantInputAccepted bool
	}{
		{name: "preview editor, press editor", previewProcess: `C:\Apps\editor.exe`, pressProcess: `C:\Apps\editor.exe`, wantPreviewAction: true, wantInputAccepted: true},
		{name: "preview editor, press calc", previewProcess: `C:\Apps\editor.exe`, pressProcess: `C:\Apps\calc.exe`, wantPreviewAction: true},
		{name: "preview calc, press editor", previewProcess: `C:\Apps\calc.exe`, pressProcess: `C:\Apps\editor.exe`, wantInputAccepted: true},
		{name: "preview calc, press calc", previewProcess: `C:\Apps\calc.exe`, pressProcess: `C:\Apps\calc.exe`},
		{name: "capture error", captureErr: errors.New("foreground unavailable")},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a, decisions := settingsSecurityFixture(t)
			controllerLayer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
				Scope: CommandSettingsScopeGlobal, Operation: "layer_create",
				Layer: &CommandSettingsLayerInput{Name: "Editor foreground controller", Enabled: true},
			})
			condition := &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{
				Field: "foreground.process", Op: "eq", Value: "editor.exe",
			}}}
			contextRule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
				Scope: CommandSettingsScopeGlobal, Operation: "rule_create",
				Rule: &CommandSettingsRuleInput{LayerID: controllerLayer.ID, Mode: "context", Lifecycle: "persistent", Enabled: true, Condition: condition},
			})
			target := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
				Scope: CommandSettingsScopeGlobal, Operation: "layer_create",
				Layer: &CommandSettingsLayerInput{Name: "Manual foreground target", Enabled: true},
			})
			targetRule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
				Scope: CommandSettingsScopeGlobal, Operation: "rule_create",
				Rule: &CommandSettingsRuleInput{LayerID: target.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true},
			})
			settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
				Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
				Binding: &CommandSettingsBindingInput{
					LayerID: controllerLayer.ID, CommandID: commandLayerActivateID,
					TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"FOREGROUNDDECK01","key":0}`,
					Arguments: map[string]any{"scope": "global", "rule_id": targetRule.ID, "duration_seconds": 0},
					Effect:    "execute", Enabled: true,
				},
			})
			settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
				Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
				Binding: &CommandSettingsBindingInput{
					LayerID: target.ID, CommandID: commandLayerBackID,
					TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"FOREGROUNDDECK01","key":1}`,
					Arguments: map[string]any{"scope": "global", "rule_id": "", "duration_seconds": 0}, Effect: "execute", Enabled: true,
				},
			})

			persisted, err := a.GetCommandSettingsForScope("pt-BR", string(CommandSettingsScopeGlobal))
			if err != nil {
				t.Fatal(err)
			}
			persistedRule := false
			for _, rule := range persisted.Rules {
				if rule.ID == contextRule.ID && rule.Mode == "context" && len(rule.Condition.Clauses) == 1 &&
					rule.Condition.Clauses[0].Field == "foreground.process" && rule.Condition.Clauses[0].Value == "editor.exe" {
					persistedRule = true
				}
			}
			if !persistedRule {
				t.Fatalf("regra contextual não persistiu como foreground.process=editor.exe: %+v", persisted.Rules)
			}
			persistedAction, persistedTarget := false, false
			for _, binding := range persisted.Bindings {
				persistedAction = persistedAction || (binding.LayerID == controllerLayer.ID && binding.CommandID == commandLayerActivateID && binding.TriggerType == "streamdeck.key")
				persistedTarget = persistedTarget || (binding.LayerID == target.ID && binding.CommandID == commandLayerBackID && binding.TriggerType == "streamdeck.key")
			}
			if !persistedAction || !persistedTarget {
				t.Fatalf("bindings backend/contextual e alvo não persistiram: %+v", persisted.Bindings)
			}

			var reader *controlledDeckForegroundReader
			if scenario.captureErr != nil {
				reader = &controlledDeckForegroundReader{err: scenario.captureErr}
			} else {
				reader = &controlledDeckForegroundReader{snapshot: foregroundDeckSnapshot(t, scenario.previewProcess)}
			}
			p := a.commandProduct.Load()
			p.foregroundReader = reader
			preview, _, err := p.deckMap(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if scenario.wantPreviewAction {
				if binding := preview["FOREGROUNDDECK01"][0]; binding.commandID != commandLayerActivateID {
					t.Fatalf("mapa produtivo não resolveu a ação contextual para editor.exe: %+v", binding)
				}
			} else if preview["FOREGROUNDDECK01"][0].commandID == commandLayerActivateID {
				t.Fatal("mapa publicou a ação contextual sem captura correspondente")
			}

			identities, versions, err := p.deckTriggerMap(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(a.ctx)
			defer cancel()
			if scenario.captureErr == nil {
				reader.setSnapshot(foregroundDeckSnapshot(t, scenario.pressProcess))
			}
			reader.mu.Lock()
			if scenario.wantInputAccepted {
				reader.blockCall = reader.calls + 1
				reader.captured = make(chan int, 1)
				reader.continueAt = make(chan struct{})
			}
			reader.mu.Unlock()
			controller := &commandDeckController{
				p: p, ctx: ctx, versions: versions, identities: identities,
				pressed: map[string]bool{}, instances: map[string]commandDeckInstance{},
				models: map[string]string{}, generation: p.deckInputGeneration(),
			}
			controller.opened("FOREGROUNDDECK01")
			t.Cleanup(func() { controller.reset("") })
			if scenario.wantInputAccepted {
				type inputResult struct {
					ack commandbridge.InvocationAck
					err error
				}
				inputDone := make(chan inputResult, 1)
				go func() {
					ack, err := controller.Input(ctx, commandadapter.Event{
						SourceInstance: "streamdeck.key:FOREGROUNDDECK01", Key: "key:0", Kind: commandinput.KeyDown,
					})
					inputDone <- inputResult{ack: ack, err: err}
				}()
				select {
				case call := <-reader.captured:
					if call != reader.callsMade() {
						t.Fatalf("captura bloqueada inesperada: call=%d total=%d", call, reader.callsMade())
					}
				case <-time.After(3 * time.Second):
					t.Fatal("controller.Input não capturou foreground no ingresso")
				}
				// A captura do evento foi concluída; a mudança subsequente de foco
				// deve afetar somente eventos futuros, nunca esta ocorrência.
				reader.setSnapshot(foregroundDeckSnapshot(t, `C:\Apps\calc.exe`))
				close(reader.continueAt)
				var ack commandbridge.InvocationAck
				var inputErr error
				select {
				case result := <-inputDone:
					ack, inputErr = result.ack, result.err
				case <-time.After(3 * time.Second):
					t.Fatal("controller.Input não concluiu depois da liberação da captura")
				}
				if inputErr != nil || !ack.Accepted || ack.InvocationID == "" {
					t.Fatalf("controller.Input não admitiu a ocorrência do Deck: ack=%+v err=%v", ack, inputErr)
				}
				waitForegroundDeckInvocation(t, a.currentUserID, ack.InvocationID)
				waitDeckLayerClaim(t, targetRule.ID, true)
				if got := reader.callsMade(); got != 2 {
					t.Fatalf("execução recapturou foreground após o ingresso: chamadas=%d, esperado=2", got)
				}
				var audit struct{ ForegroundSummary *string }
				if err := database.DB().Table("command_invocations").Select("foreground_summary").Where("invocation_id = ?", ack.InvocationID).Take(&audit).Error; err != nil {
					t.Fatal(err)
				}
				var summary commandforeground.Summary
				var summaryErr error
				if audit.ForegroundSummary == nil {
					summaryErr = errors.New("foreground_summary ausente")
				} else {
					summaryErr = json.Unmarshal([]byte(*audit.ForegroundSummary), &summary)
				}
				if summaryErr != nil || summary.Executable != "editor.exe" || summary.WindowClass != "ForegroundDeckFixture" || summary.ProviderVersion != commandforeground.ProviderVersion {
					raw := "<ausente>"
					if audit.ForegroundSummary != nil {
						raw = *audit.ForegroundSummary
					}
					t.Fatalf("ledger não preservou o foreground capturado para a ocorrência: summary=%+v raw=%q err=%v", summary, raw, summaryErr)
				}
				updatedMap, _, err := p.deckMap(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if binding := updatedMap["FOREGROUNDDECK01"][1]; binding.commandID != commandLayerBackID {
					t.Fatalf("claim manual não publicou binding da camada alvo no mapa Deck: %+v", binding)
				}
			} else {
				ack, inputErr := controller.Input(ctx, commandadapter.Event{
					SourceInstance: "streamdeck.key:FOREGROUNDDECK01", Key: "key:0", Kind: commandinput.KeyDown,
				})
				if inputErr == nil || ack.Accepted {
					t.Fatalf("ocorrência não correspondente/falha de captura foi aceita: ack=%+v err=%v", ack, inputErr)
				}
				var invocations int64
				if err := database.DB().Table("command_invocations").Where("user_id = ? AND source_type = ?", a.currentUserID, "streamdeck.key").Count(&invocations).Error; err != nil {
					t.Fatal(err)
				}
				if invocations != 0 {
					t.Fatalf("falha/não correspondência criou invocações no ledger: %d", invocations)
				}
				if active := countForegroundDeckClaims(t, targetRule.ID); active != 0 {
					t.Fatalf("falha/não correspondência ativou a camada manual: %d claims", active)
				}
				if got, want := reader.callsMade(), 2; got != want {
					t.Fatalf("capturas do reader controlado=%d, esperado=%d (preview+ingresso)", got, want)
				}
			}
		})
	}
}

func foregroundDeckSnapshot(t *testing.T, executablePath string) commandforeground.Snapshot {
	t.Helper()
	var window uintptr = 1
	var process uint32 = 2
	var created uint64 = 3
	switch executablePath {
	case `C:\Apps\calc.exe`:
		window, process, created = 4, 5, 6
	case `C:\Apps\Code.exe`:
		window, process, created = 7, 8, 9
	}
	snapshot, err := commandforeground.NewSnapshot(window, process, created, executablePath, "ForegroundDeckFixture")
	if err != nil {
		t.Fatalf("NewSnapshot(%q): %v", executablePath, err)
	}
	return snapshot
}

func waitForegroundDeckInvocation(t *testing.T, userID, invocationID string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		if err := database.DB().Table("command_invocations").Where("user_id = ? AND invocation_id = ? AND source_type = ? AND status = ?", userID, invocationID, "streamdeck.key", string(commandledger.Succeeded)).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("invocação %s não chegou a succeeded", invocationID)
}

func countForegroundDeckClaims(t *testing.T, ruleID string) int64 {
	t.Helper()
	var count int64
	if err := database.DB().Table("command_layer_activation_state").Where("rule_ref = ? AND state = ?", ruleID, "active").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}
