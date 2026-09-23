package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commanddeck"
	"assistente/internal/database"
)

type multiPhysicalDeckDriver struct{ opened chan *multiPhysicalDeckHandle }
type multiPhysicalDeckHandle struct {
	serial string
	events chan commanddeck.PhysicalKeyEvent
	writes chan commanddeck.RenderPlan
	closed chan struct{}
}

func (d *multiPhysicalDeckDriver) Enumerate(context.Context) ([]commanddeck.PhysicalDevice, error) {
	model := commanddeck.Model{ID: "fixture", Name: "Fixture", Rows: 1, Columns: 2, KeyImageW: 72, KeyImageH: 72, SupportsHID: true}
	return []commanddeck.PhysicalDevice{{ID: "DECKA123456", Model: model}, {ID: "DECKB123456", Model: model}}, nil
}
func (d *multiPhysicalDeckDriver) Open(_ context.Context, device commanddeck.PhysicalDevice) (commanddeck.Handle, error) {
	h := &multiPhysicalDeckHandle{serial: string(device.ID), events: make(chan commanddeck.PhysicalKeyEvent, 8), writes: make(chan commanddeck.RenderPlan, 16), closed: make(chan struct{})}
	d.opened <- h
	return h, nil
}
func (h *multiPhysicalDeckHandle) Write(ctx context.Context, plan commanddeck.RenderPlan) error {
	select {
	case h.writes <- plan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (h *multiPhysicalDeckHandle) Read(ctx context.Context) (commanddeck.PhysicalKeyEvent, error) {
	select {
	case event := <-h.events:
		return event, nil
	case <-h.closed:
		return commanddeck.PhysicalKeyEvent{}, errors.New("closed")
	case <-ctx.Done():
		return commanddeck.PhysicalKeyEvent{}, ctx.Err()
	}
}
func (h *multiPhysicalDeckHandle) Close(context.Context) error {
	select {
	case <-h.closed:
	default:
		close(h.closed)
	}
	return nil
}

// TestCommandDeckLayerActionsRealMapPressAndBack proves the complete physical
// route: persisted bindings -> deckMap -> native transport -> durable executor
// -> activation state. A and B have independent serials; A also exercises two
// keys sharing the same device serial for activate/back.
func TestCommandDeckLayerActionsRealMapPressAndBack(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	target := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Deck target", Enabled: true}})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: target.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true}})
	for _, binding := range []CommandSettingsBindingInput{
		{LayerID: layer, CommandID: commandLayerActivateID, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"DECKA123456","key":0}`, Arguments: map[string]any{"scope": "global", "rule_id": rule.ID, "duration_seconds": 0}, Effect: "execute", Enabled: true},
		{LayerID: layer, CommandID: commandLayerBackID, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"DECKA123456","key":1}`, Arguments: map[string]any{"scope": "global", "rule_id": "", "duration_seconds": 0}, Effect: "execute", Enabled: true},
		{LayerID: layer, CommandID: commandLayerActivateID, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"DECKB123456","key":0}`, Arguments: map[string]any{"scope": "global", "rule_id": rule.ID, "duration_seconds": 0}, Effect: "execute", Enabled: true},
	} {
		settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &binding})
	}
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	bindings, _, err := p.deckMap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bindings["DECKA123456"][0].commandID != commandLayerActivateID || bindings["DECKA123456"][1].commandID != commandLayerBackID || bindings["DECKB123456"][0].commandID != commandLayerActivateID {
		t.Fatalf("map não publicou as três ações por serial: %#v", bindings)
	}

	driver := &multiPhysicalDeckDriver{opened: make(chan *multiPhysicalDeckHandle, 4)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver)
	handles := map[string]*multiPhysicalDeckHandle{}
	deadline := time.After(5 * time.Second)
	for len(handles) < 2 {
		select {
		case h := <-driver.opened:
			handles[h.serial] = h
		case <-deadline:
			t.Fatalf("Decks não abriram: %#v", handles)
		}
	}

	handles["DECKA123456"].events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	waitDeckLayerClaim(t, rule.ID, true)
	// A layer mutation invalidates the current Deck session. Do not send the
	// next physical event to the retired handle: wait for the reopened HID
	// session published by runDeckEpoch.
	handles = waitLayerDeckPair(t, driver.opened, handles)
	initialInvocations := countDeckInvocations(t)
	// A reopened native session may repeat Down while the physical key is
	// still held. The product-wide edge ledger must suppress that edge.
	handles["DECKA123456"].events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	assertNoDeckInvocation(t, initialInvocations)
	handles["DECKA123456"].events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: false}
	handles["DECKA123456"].events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	waitDeckSucceededCount(t, 2)
	waitDeckLayerClaim(t, rule.ID, true)
	handles["DECKA123456"].events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: false}
	handles["DECKA123456"].events <- commanddeck.PhysicalKeyEvent{Index: 1, Down: true}
	waitDeckLayerClaim(t, rule.ID, false)
	handles["DECKA123456"].events <- commanddeck.PhysicalKeyEvent{Index: 1, Down: false}

	// The same key on the other physical serial resolves independently.
	handles = waitLayerDeckPair(t, driver.opened, handles)
	handles["DECKB123456"].events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	waitDeckLayerClaim(t, rule.ID, true)
	sourceInstances := countDeckInvocations(t)
	if sourceInstances < 3 {
		t.Fatalf("invocações físicas=%d, want >=3", sourceInstances)
	}
}

func countDeckInvocations(t *testing.T) int64 {
	t.Helper()
	var count int64
	if err := database.DB().Table("command_invocations").Where("source_type = ?", "streamdeck.key").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func waitDeckSucceededCount(t *testing.T, want int64) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		if err := database.DB().Table("command_invocations").Where("source_type = ? AND status = ?", "streamdeck.key", "succeeded").Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("execuções físicas succeeded não chegaram a %d", want)
}

func assertNoDeckInvocation(t *testing.T, want int64) {
	t.Helper()
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		got := countDeckInvocations(t)
		if got != want {
			t.Fatalf("Down repetido criou invocação física: want %d, got %d", want, got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitLayerDeckPair(t *testing.T, opened <-chan *multiPhysicalDeckHandle, retired map[string]*multiPhysicalDeckHandle) map[string]*multiPhysicalDeckHandle {
	t.Helper()
	result := map[string]*multiPhysicalDeckHandle{}
	deadline := time.After(5 * time.Second)
	for len(result) < 2 {
		select {
		case handle := <-opened:
			if handle != nil && (handle.serial == "DECKA123456" || handle.serial == "DECKB123456") && handle != retired[handle.serial] {
				result[handle.serial] = handle
			}
		case <-deadline:
			t.Fatalf("Deck não publicou as duas novas sessões após a mutação: %#v", result)
		}
	}
	return result
}

func waitDeckLayerClaim(t *testing.T, ruleID string, active bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var count int64
		if err := database.DB().Table("command_layer_activation_state").Where("rule_ref = ? AND state = ?", ruleID, "active").Count(&count).Error; err == nil && (count > 0) == active {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	var rows []struct {
		CommandID        string
		Status           string
		ErrorCode        string
		SourceType       string
		TriggerType      string
		ArgumentsSummary string
	}
	_ = database.DB().Table("command_invocations").Select("command_id, status, error_code, source_type, trigger_type, arguments_summary").Limit(8).Scan(&rows).Error
	t.Fatalf("claim %s não alcançou active=%v; invocações recentes=%+v", ruleID, active, rows)
}
