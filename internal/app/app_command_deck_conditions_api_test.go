package app

import (
	"context"
	"testing"

	"assistente/internal/commandadapter"
	"assistente/internal/commandinput"
	"assistente/internal/database"
)

// Exercises persisted settings and the native input boundary together: one
// physical key can select different presentation commands without supplying
// any visual observations to the backend.
func TestCommandDeckVisualConditionsFromSettingsSelectDistinctCommands(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "layer_create",
		Layer: &CommandSettingsLayerInput{Name: "Deck contextual", Enabled: true},
	})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "rule_create",
		Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "condition", Lifecycle: "persistent", Enabled: true,
			Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "profile", Value: "dev"}}}},
	})
	commands := map[string]string{"chat": "navigation.settings.open", "editor": "navigation.menu.open"}
	for surface, command := range commands {
		settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
			Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
			Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: command, TriggerType: "streamdeck.key",
				TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Arguments: map[string]any{}, Effect: "execute", Enabled: true,
				Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: surface}}}},
		})
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	identities, versions, err := p.deckTriggerMap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	preview, _, err := p.deckMap(context.Background())
	if err != nil || preview["test-deck"][0].title == "" {
		t.Fatalf("conditional key has no presentation: %+v %v", preview, err)
	}
	events := make(chan CommandDeckLocalUIEvent, 4)
	settingsEmitter := a.emitter
	a.emitter = commandOSBootstrapEmitter(func(name string, payload any) {
		if name == "command:deck-local-ui" {
			events <- payload.(CommandDeckLocalUIEvent)
			return
		}
		if settingsEmitter != nil {
			settingsEmitter.Emit(name, payload)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &commandDeckController{p: p, ctx: ctx, versions: versions, identities: identities,
		pressed: map[string]bool{}, instances: map[string]commandDeckInstance{}, models: map[string]string{}, generation: p.deckInputGeneration()}
	c.opened("test-deck")
	press := commandadapter.Event{SourceInstance: "streamdeck.key:test-deck", Key: "key:0", Kind: commandinput.KeyDown}
	ack, err := c.Input(ctx, press)
	if err != nil || !ack.Accepted || ack.InvocationID != "" {
		t.Fatalf("press did not produce local presentation: %+v %v", ack, err)
	}
	var event CommandDeckLocalUIEvent
	select {
	case event = <-events:
	default:
		t.Fatal("missing conditional local event")
	}
	if event.CommandID != "" || len(event.Conditions) != len(commands) || event.Generation == "" || event.SessionID != p.principal.SessionID {
		t.Fatalf("invalid conditional event: %+v", event)
	}
	for surface, command := range commands {
		matches := 0
		for _, condition := range event.Conditions {
			if condition.ByProfile["dev"].BySurface[surface] {
				matches++
				if condition.CommandID != command {
					t.Fatalf("surface %s selected foreign command %s", surface, condition.CommandID)
				}
			}
			if condition.BySurface[surface] || condition.Fallback {
				t.Fatal("unknown profile gained a permissive fallback")
			}
		}
		if matches != 1 {
			t.Fatalf("surface %s has %d selected commands", surface, matches)
		}
	}
	if repeat, err := c.Input(ctx, press); err != nil || repeat.Accepted {
		t.Fatalf("held key repeated: %+v %v", repeat, err)
	}
	select {
	case <-events:
		t.Fatal("held key emitted twice")
	default:
	}
	// A configuration change invalidates the old physical controller; an
	// already-open connection cannot continue publishing the retired choices.
	if _, err := c.Input(ctx, commandadapter.Event{SourceInstance: press.SourceInstance, Key: press.Key, Kind: commandinput.KeyUp}); err != nil {
		t.Fatal(err)
	}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "layer_disable", ID: layer.ID,
	})
	if stale, err := c.Input(ctx, press); err == nil || stale.Accepted {
		t.Fatalf("retired controller accepted new press: %+v %v", stale, err)
	}
	select {
	case <-events:
		t.Fatal("retired configuration emitted a local event")
	default:
	}
	var count int64
	if err := database.DB().Table("command_invocations").Where("user_id = ?", p.principal.UserID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("local presentation wrote ledger: %d %v", count, err)
	}
}
