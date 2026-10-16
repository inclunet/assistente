package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/commanddeck"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"github.com/google/uuid"
)

type appDeckHandle struct {
	events chan commanddeck.PhysicalKeyEvent
	writes chan commanddeck.RenderPlan
	closed chan struct{}
	once   sync.Once
}

func (h *appDeckHandle) Write(ctx context.Context, p commanddeck.RenderPlan) error {
	select {
	case h.writes <- p:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (h *appDeckHandle) Read(ctx context.Context) (commanddeck.PhysicalKeyEvent, error) {
	select {
	case e := <-h.events:
		return e, nil
	case <-h.closed:
		return commanddeck.PhysicalKeyEvent{}, errors.New("unplugged")
	case <-ctx.Done():
		return commanddeck.PhysicalKeyEvent{}, ctx.Err()
	}
}
func (h *appDeckHandle) Close(context.Context) error {
	h.once.Do(func() { close(h.closed) })
	return nil
}

type appDeckDriver struct{ opened chan *appDeckHandle }

func (d *appDeckDriver) Enumerate(context.Context) ([]commanddeck.PhysicalDevice, error) {
	return []commanddeck.PhysicalDevice{{ID: "test-deck", Model: commanddeck.Model{ID: "test", Name: "Test", Rows: 1, Columns: 2, KeyImageW: 72, KeyImageH: 72, SupportsHID: true}}}, nil
}
func (d *appDeckDriver) Open(context.Context, commanddeck.PhysicalDevice) (commanddeck.Handle, error) {
	h := &appDeckHandle{events: make(chan commanddeck.PhysicalKeyEvent, 8), writes: make(chan commanddeck.RenderPlan, 16), closed: make(chan struct{})}
	d.opened <- h
	return h, nil
}

func deckConfiguredFixture(t *testing.T) *App {
	t.Helper()
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: "navigation.settings.open", TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Enabled: true})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	return a
}

func deckChatPickerFixture(t *testing.T, commandID string) *App {
	t.Helper()
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: commandID, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Enabled: true})
	})
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestCommandDeckChatPickersEmitLocalUIWithoutLedger(t *testing.T) {
	for _, commandID := range []string{commandChatModelOpenID, commandChatHistoryOpenID, commandChatProfileOpenID, commandChatPinnedOpenID, commandChatTokensOpenID} {
		t.Run(commandID, func(t *testing.T) {
			a := deckChatPickerFixture(t, commandID)
			p := a.commandProduct.Load()
			localEvents := make(chan CommandDeckLocalUIEvent, 2)
			a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
				if name == "command:deck-local-ui" {
					localEvents <- value.(CommandDeckLocalUIEvent)
				}
			})
			if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
				t.Fatal(err)
			}
			driver := &appDeckDriver{opened: make(chan *appDeckHandle, 2)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p.startDeck(ctx, driver)
			var handle *appDeckHandle
			select {
			case handle = <-driver.opened:
			case <-time.After(4 * time.Second):
				t.Fatal("native adapter did not open configured device")
			}
			for i := 0; i < 2; i++ {
				select {
				case <-handle.writes:
				case <-time.After(time.Second):
					t.Fatal("missing Deck frame")
				}
			}
			handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
			select {
			case event := <-localEvents:
				if event.CommandID != commandID || event.Generation == "" || event.UserID == "" || event.SessionID == "" || event.WorkspaceID == "" {
					t.Fatalf("evento local_ui inesperado: %+v", event)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("missing local Deck picker event")
			}
			var count int64
			if err := database.DB().Table("command_invocations").Where("command_id = ?", commandID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("picker local criou ledger/invocação: %d", count)
			}
		})
	}
}

func TestCommandDeckNativeDriverExecutesUIAndBlanksOnLock(t *testing.T) {
	a := deckConfiguredFixture(t)
	p := a.commandProduct.Load()
	localEvents := make(chan CommandDeckLocalUIEvent, 8)
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-local-ui" {
			localEvents <- value.(CommandDeckLocalUIEvent)
		}
	})
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver)
	var h *appDeckHandle
	select {
	case h = <-driver.opened:
	case <-time.After(4 * time.Second):
		t.Fatal("native adapter did not open configured device")
	}
	select {
	case <-h.writes:
	case <-time.After(time.Second):
		t.Fatal("missing safe frame")
	}
	select {
	case frame := <-h.writes:
		if len(frame.Updates) != 1 || frame.Updates[0].View.Title == "" {
			t.Fatalf("missing title: %+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("missing user frame")
	}
	h.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	var event CommandDeckLocalUIEvent
	select {
	case event = <-localEvents:
	case <-time.After(4 * time.Second):
		t.Fatal("missing local Deck event")
	}
	if event.CommandID != "navigation.settings.open" || event.Generation == "" || event.UserID == "" || event.SessionID == "" || event.WorkspaceID == "" {
		t.Fatalf("evento Deck inesperado: %+v", event)
	}
	// Repeated down must not generate another occurrence before a release.
	h.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true, Repeat: true}
	h.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	if err := p.host.SetOSSessionState(context.Background(), true, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.closed:
	case <-time.After(4 * time.Second):
		t.Fatal("lock did not retire handle")
	}
	select {
	case duplicate := <-localEvents:
		t.Fatalf("duplicate physical press: %+v", duplicate)
	default:
	}
	var count int64
	if err := database.DB().Table("command_invocations").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("navegação local criou invocations: %d, %v", count, err)
	}
	var safe bool
	for len(h.writes) > 0 {
		plan := <-h.writes
		safe = len(plan.Updates) == 2
		for _, update := range plan.Updates {
			safe = safe && update.View.Title == ""
		}
	}
	if !safe {
		t.Fatal("lock did not clear physical labels")
	}
}

func TestCommandDeckEmptyStatusPublishesArray(t *testing.T) {
	a := deckConfiguredFixture(t)
	p := a.commandProduct.Load()
	var payload []byte
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-status" {
			payload, _ = json.Marshal(value)
		}
	})
	p.deckStatus("unconfigured", nil)
	var status struct {
		Devices []commandDeckDeviceStatus `json:"devices"`
	}
	if err := json.Unmarshal(payload, &status); err != nil {
		t.Fatal(err)
	}
	if status.Devices == nil || len(status.Devices) != 0 {
		t.Fatalf("empty status must clear frontend devices with an array: %s", payload)
	}
}

func TestCommandDeckMapExcludesOtherSerialAndInactiveLayer(t *testing.T) {
	a := deckConfiguredFixture(t)
	p := a.commandProduct.Load()
	bindings, _, err := p.deckMap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings["test-deck"][0].commandID != "navigation.settings.open" || len(bindings["other-deck"]) != 0 {
		t.Fatalf("map %+v", bindings)
	}
	settings, err := a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	for _, layer := range settings.Layers {
		if !layer.Builtin && layer.Active {
			if _, err := a.SetCommandLayerActive(layer.ID, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	bindings, _, err = p.deckMap(context.Background())
	if err != nil || len(bindings) != 0 {
		t.Fatalf("inactive map %+v %v", bindings, err)
	}
}

func TestCommandDeckReconnectRejectsOldReservationAndAllowsNewPress(t *testing.T) {
	a := deckConfiguredFixture(t)
	p := a.commandProduct.Load()
	localEvents := make(chan CommandDeckLocalUIEvent, 8)
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		if name == "command:deck-local-ui" {
			localEvents <- value.(CommandDeckLocalUIEvent)
		}
	})
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(ctx, driver)
	waitHandle := func() *appDeckHandle {
		t.Helper()
		select {
		case h := <-driver.opened:
			for i := 0; i < 2; i++ {
				select {
				case <-h.writes:
				case <-time.After(2 * time.Second):
					t.Fatal("missing reconnect frame")
				}
			}
			return h
		case <-time.After(5 * time.Second):
			t.Fatal("device failed to reconnect")
			return nil
		}
	}
	waitEvent := func() CommandDeckLocalUIEvent {
		t.Helper()
		select {
		case event := <-localEvents:
			return event
		case <-time.After(4 * time.Second):
			t.Fatal("missing press")
			return CommandDeckLocalUIEvent{}
		}
	}
	first := waitHandle()
	first.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	old := waitEvent()
	_ = first.Close(context.Background())
	second := waitHandle()
	second.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	current := waitEvent()
	if current.CommandID != old.CommandID || current.Generation == "" {
		t.Fatalf("evento após reconnect inesperado: old=%+v current=%+v", old, current)
	}
}

func TestCommandDeckExecutorRejectsCandidateWithoutNativeOccurrence(t *testing.T) {
	a := deckConfiguredFixture(t)
	p := a.commandProduct.Load()
	id := uuid.Must(uuid.NewV7()).String()
	_, err := p.deckExecution.service.ExecuteEnvelope(context.Background(), "", commandexecution.EnvelopeCandidate{InvocationID: id, CorrelationID: id, TriggerType: "streamdeck.key", TriggerSpec: json.RawMessage(`{"version":1,"device":"test-deck","key":0}`), Arguments: json.RawMessage(`{}`)})
	if err == nil {
		t.Fatal("forged physical event reached executor")
	}
	p.mu.Lock()
	runs := len(p.uiRuns)
	p.mu.Unlock()
	if runs != 0 {
		t.Fatal("forged event created UI handoff")
	}
}

func TestCommandDeckOldConnectionCleanupCannotCancelReconnection(t *testing.T) {
	c := &commandDeckController{ctx: context.Background(), instances: map[string]commandDeckInstance{}, pressed: map[string]bool{}}
	c.opened("serial")
	old := c.instances["serial"]
	c.opened("serial")
	current := c.instances["serial"]
	c.retire("serial", old.id)
	if old.ctx.Err() == nil || current.ctx.Err() != nil || c.instances["serial"].id != current.id {
		t.Fatal("stale cleanup affected current connection")
	}
	c.retire("serial", current.id)
	if current.ctx.Err() == nil || len(c.instances) != 0 {
		t.Fatal("current disconnect failed to revoke")
	}
}
