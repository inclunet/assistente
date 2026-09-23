package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
)

func TestCommandLayerActionCatalogContract(t *testing.T) {
	registrations := commandLayerActionRegistrations()
	if _, err := commandcatalog.NewComplete(registrations); err != nil {
		t.Fatalf("catálogo de camadas inválido: %v", err)
	}
	registry, _ := commandcatalog.NewComplete(registrations)
	if _, err := registry.ValidateArguments(commandLayerActivateID, []byte(`{"scope":"global","rule_id":"rule","duration_seconds":0}`)); err != nil {
		t.Fatalf("argumentos layer.activate inválidos: %v", err)
	}
	if len(registrations) != 3 {
		t.Fatalf("quantidade de ações: %d", len(registrations))
	}
	for _, registration := range registrations {
		definition := registration.Definition
		if !isCommandLayerAction(definition.ID) || definition.HandlerClassification != commandcatalog.HandlerBackend ||
			definition.Effect != commandcatalog.Write || !definition.MutatesEffectiveCapability || !definition.HasMutableTarget ||
			definition.Decision != commandcatalog.NoDecision {
			t.Fatalf("contrato inesperado para %s: %+v", definition.ID, definition)
		}
		if len(definition.AllowedSources) != 4 || !definition.AllowsSource(commandcatalog.Palette) ||
			!definition.AllowsSource(commandcatalog.KeyboardLocal) || !definition.AllowsSource(commandcatalog.StreamDeck) || !definition.AllowsSource(commandcatalog.Chat) {
			t.Fatalf("origens inesperadas para %s: %v", definition.ID, definition.AllowedSources)
		}
		if definition.ArgumentsSchema == nil || definition.ResultSchema == nil {
			t.Fatalf("schemas ausentes para %s", definition.ID)
		}
	}
}

func TestCommandLayerActionOriginIsDerivedFromInvocation(t *testing.T) {
	instance := "trusted-instance"
	deckTrigger := json.RawMessage(`{"version":1,"device":"deck-A","key":7}`)
	invocation := commandexecution.Invocation{
		Principal: auth.LocalSessionPrincipal{UserID: "user", SessionID: "session"},
		Source:    commandcatalog.KeyboardLocal,
		Envelope:  &commandcontract.Envelope{SourceInstanceID: &instance},
	}
	origin, err := commandLayerOrigin(invocation)
	if err != nil || origin.Type != string(commandcatalog.KeyboardLocal) || origin.SessionID != "session" || origin.DeviceID != "local-keyboard" {
		t.Fatalf("origem não derivada corretamente: %+v err=%v", origin, err)
	}
	for _, source := range []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck} {
		invocation.Source = source
		invocation.ID = "occurrence-1"
		invocation.Envelope.SourceInstanceID = nil
		origin, err := commandLayerOrigin(invocation)
		if source == commandcatalog.StreamDeck {
			if err == nil {
				t.Fatalf("%s sem dispositivo foi aceito", source)
			}
			continue
		}
		if err != nil || origin.SessionID != "session" || origin.DeviceID == "" {
			t.Fatalf("origem %s inválida: %+v err=%v", source, origin, err)
		}
	}
	invocation.Source = commandcatalog.StreamDeck
	invocation.ID = "occurrence-A"
	invocation.Envelope.SourceInstanceID = &instance
	invocation.Envelope.TriggerSpec = &deckTrigger
	first, err := commandLayerOrigin(invocation)
	if err != nil || first.DeviceID != "deck-A" {
		t.Fatalf("serial do dispositivo Deck ausente: %+v err=%v", first, err)
	}
	invocation.ID = "occurrence-B"
	reconnected := "new-native-connection"
	invocation.Envelope.SourceInstanceID = &reconnected
	otherKey := json.RawMessage(`{"version":1,"device":"deck-A","key":2}`)
	invocation.Envelope.TriggerSpec = &otherKey
	second, err := commandLayerOrigin(invocation)
	if err != nil || second.DeviceID != first.DeviceID {
		t.Fatalf("dispositivo Deck instável: %+v / %+v err=%v", first, second, err)
	}
	firstStack, err := commandactivation.ManualStackKey(first)
	if err != nil {
		t.Fatal(err)
	}
	secondStack, err := commandactivation.ManualStackKey(second)
	if err != nil || secondStack != firstStack {
		t.Fatalf("reconexão/tecla separou a pilha do mesmo Deck: %v", err)
	}
	otherDevice := json.RawMessage(`{"version":1,"device":"deck-B","key":2}`)
	invocation.Envelope.TriggerSpec = &otherDevice
	third, err := commandLayerOrigin(invocation)
	if err != nil {
		t.Fatal(err)
	}
	thirdStack, err := commandactivation.ManualStackKey(third)
	if err != nil || thirdStack == firstStack {
		t.Fatalf("Decks distintos compartilharam a pilha manual: %v", err)
	}
	invocation.Source = commandcatalog.UI
	if _, err := commandLayerOrigin(invocation); err == nil {
		t.Fatal("origem UI não permitida para ativação durável")
	}
}

func TestCommandLayerActionsExecuteThroughPaletteAndBack(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	confirmed := func(call func() (CommandSettingsMutation, error)) CommandSettingsMutation {
		t.Helper()
		done := settingsSecurityStart(t, a, call)
		appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
		outcome := settingsSecurityFinish(t, done)
		if outcome.err != nil || !outcome.result.Committed || !outcome.result.Published {
			t.Fatalf("mutação não publicou: %+v", outcome)
		}
		return outcome.result
	}
	control := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Camada controladora always", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: control.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	target := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Camada manual alvo", Enabled: true}})
	rule := confirmed(func() (CommandSettingsMutation, error) { return a.PrepareManualCommandLayer(target.ID) })
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: control.ID, CommandID: commandLayerActivateID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"layer.activate"}`, Arguments: map[string]any{"scope": "global", "rule_id": rule.ID, "duration_seconds": 0}, Effect: "execute", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: control.ID, CommandID: commandLayerToggleID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"layer.toggle"}`, Arguments: map[string]any{"scope": "global", "rule_id": rule.ID, "duration_seconds": 0}, Effect: "execute", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: control.ID, CommandID: commandLayerBackID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"layer.back"}`, Arguments: map[string]any{"scope": "global", "rule_id": "", "duration_seconds": 0}, Effect: "execute", Enabled: true}})
	result, err := a.ExecutePaletteCommand(commandLayerActivateID, json.RawMessage(`{}`))
	if err != nil || result.Status != string(commandledger.Succeeded) {
		t.Fatalf("layer.activate falhou: %+v err=%v", result, err)
	}
	var active int64
	if err := database.DB().Table("command_layer_activation_state").Where("rule_ref = ? AND state = ?", rule.ID, "active").Count(&active).Error; err != nil || active != 1 {
		t.Fatalf("claim ativa ausente: count=%d err=%v", active, err)
	}
	result, err = a.ExecutePaletteCommand(commandLayerToggleID, json.RawMessage(`{}`))
	if err != nil || result.Status != string(commandledger.Succeeded) {
		t.Fatalf("layer.toggle falhou: %+v err=%v", result, err)
	}
	if err := database.DB().Table("command_layer_activation_state").Where("rule_ref = ? AND state = ?", rule.ID, "active").Count(&active).Error; err != nil || active != 0 {
		t.Fatalf("toggle não encerrou claim: count=%d err=%v", active, err)
	}
	result, err = a.ExecutePaletteCommand(commandLayerActivateID, json.RawMessage(`{}`))
	if err != nil || result.Status != string(commandledger.Succeeded) {
		t.Fatalf("layer.activate após toggle falhou: %+v err=%v", result, err)
	}
	result, err = a.ExecutePaletteCommand(commandLayerBackID, json.RawMessage(`{}`))
	if err != nil || result.Status != string(commandledger.Succeeded) {
		t.Fatalf("layer.back falhou: %+v err=%v", result, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := database.DB().Table("command_layer_activation_state").Where("rule_ref = ? AND state = ?", rule.ID, "active").Count(&active).Error; err == nil && active == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("layer.back não encerrou claim: %d", active)
}

func TestCommandLayerHandlerStartReturnsWithoutHoldingGate(t *testing.T) {
	app := &App{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := app.startCommandLayerAction(ctx, commandexecution.Invocation{}); err == nil {
		t.Fatal("handler aceitou invocation sem envelope/principal")
	}
}
