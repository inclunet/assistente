package app

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"assistente/internal/commanddeck"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func waitDeckCaptureEvent(t *testing.T, events <-chan commandDeckCaptureEvent, requestID, status string) commandDeckCaptureEvent {
	t.Helper()
	deadline := time.NewTimer(6 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-events:
			if event.RequestID == requestID && (status == "" || event.Status == status) {
				return event
			}
			if event.RequestID == requestID && event.Status == "captured" && status != "captured" {
				t.Fatalf("captura inesperada para request=%s enquanto aguardava %s: %+v", requestID, status, event)
			}
		case <-deadline.C:
			t.Fatalf("evento de captura ausente: request=%s status=%s", requestID, status)
			return commandDeckCaptureEvent{}
		}
	}
}

func waitDeckHandle(t *testing.T, opened <-chan *appDeckHandle) *appDeckHandle {
	t.Helper()
	select {
	case handle := <-opened:
		return handle
	case <-time.After(6 * time.Second):
		t.Fatal("Stream Deck não foi aberto pelo runtime de captura")
		return nil
	}
}

func assertNoCapturedDeckEvent(t *testing.T, events <-chan commandDeckCaptureEvent, requestID string, duration time.Duration) {
	t.Helper()
	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	for {
		select {
		case event := <-events:
			if event.RequestID == requestID && event.Status == "captured" {
				t.Fatalf("captura proibida publicada: %+v", event)
			}
		case <-deadline.C:
			return
		}
	}
}

func waitDeckHeld(t *testing.T, p *commandProductRuntime, key string) {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
	held := p.deckHeld[key]
		p.mu.Unlock()
		if held {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("estado held não foi observado: %s", key)
}

func installDeckCaptureDriver(t *testing.T, a *App, driver commanddeck.Driver) (context.CancelFunc, *commandProductRuntime) {
	t.Helper()
	p := a.commandProduct.Load()
	p.deckDriver = driver
	ctx, cancel := context.WithCancel(context.Background())
	p.startDeck(ctx, driver)
	t.Cleanup(cancel)
	return cancel, p
}

func captureEmitter(events chan<- commandDeckCaptureEvent, reservations chan<- commandui.Reservation) func(string, any) {
	return func(name string, value any) {
		switch name {
		case "command:deck-capture":
			events <- value.(commandDeckCaptureEvent)
		case "command:deck-ui-reservation":
			reservations <- value.(commandui.Reservation)
		}
	}
}

func TestCommandDeckCaptureWithoutBindingCapturesPhysicalSpecWithoutExecution(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 4)}
	events := make(chan commandDeckCaptureEvent, 8)
	reservations := make(chan commandui.Reservation, 8)
	a.emitter = commandOSBootstrapEmitter(captureEmitter(events, reservations))
	_, p := installDeckCaptureDriver(t, a, driver)
	requestID := uuid.Must(uuid.NewV7()).String()
	if err := a.BeginCommandDeckCapture(requestID); err != nil {
		t.Fatal(err)
	}
	handle := waitDeckHandle(t, driver.opened)
	handle.events <- commanddeck.PhysicalKeyEvent{Index: 1, Down: true}
	handle.events <- commanddeck.PhysicalKeyEvent{Index: 1, Down: false}
	captured := waitDeckCaptureEvent(t, events, requestID, "captured")
	if captured.Model != "Test" || captured.Key != 1 || captured.TriggerSpec != `{"version":1,"device":"test-deck","key":1}` {
		t.Fatalf("captura física incompleta: %+v", captured)
	}
	select {
	case reservation := <-reservations:
		t.Fatalf("captura sem binding criou reserva: %+v", reservation)
	default:
	}
	p.mu.Lock()
	uiRuns := len(p.uiRuns)
	p.mu.Unlock()
	if uiRuns != 0 {
		t.Fatalf("captura sem binding criou UI run: %d", uiRuns)
	}
	var invocations int64
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", captured.RequestID).Count(&invocations).Error; err != nil {
		t.Fatal(err)
	}
	if invocations != 0 {
		t.Fatalf("captura sem binding gravou invocation: %d", invocations)
	}
	if err := a.CancelCommandDeckCapture(requestID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-handle.closed:
	case <-time.After(6 * time.Second):
		t.Fatal("cancelamento não fechou o handle físico")
	}
}

func TestCommandDeckCaptureSuppressesActiveBindingUntilPhysicalRelease(t *testing.T) {
	a := deckConfiguredFixture(t)
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 4)}
	events := make(chan commandDeckCaptureEvent, 8)
	localEvents := make(chan CommandDeckLocalUIEvent, 8)
	a.emitter = commandOSBootstrapEmitter(func(name string, value any) {
		switch name {
		case "command:deck-capture":
			events <- value.(commandDeckCaptureEvent)
		case "command:deck-local-ui":
			localEvents <- value.(CommandDeckLocalUIEvent)
		}
	})
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	_, _ = installDeckCaptureDriver(t, a, driver)
	requestID := uuid.Must(uuid.NewV7()).String()
	if err := a.BeginCommandDeckCapture(requestID); err != nil {
		t.Fatal(err)
	}
	handle := waitDeckHandle(t, driver.opened)
	handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	waitDeckHeld(t, a.commandProduct.Load(), "streamdeck.key:test-deck:key:0")
	if err := a.CancelCommandDeckCapture(requestID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-handle.closed:
	case <-time.After(6 * time.Second):
		t.Fatal("cancelamento não encerrou runtime de captura")
	}
	// O down original continua retido até o evento físico de release; portanto,
	// uma nova abertura não pode transformar o mesmo down em comando.
	newHandle := waitDeckHandle(t, driver.opened)
	newHandle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	waitDeckHeld(t, a.commandProduct.Load(), "streamdeck.key:test-deck:key:0")
	select {
	case event := <-localEvents:
		t.Fatalf("down retido disparou comando após cancelamento: %+v", event)
	default:
	}
	newHandle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: false}
	newHandle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	var event CommandDeckLocalUIEvent
	select {
	case event = <-localEvents:
		if event.CommandID != "navigation.settings.open" {
			t.Fatalf("binding físico incorreto após release: %+v", event)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("release não rearmou a tecla física")
	}
}

func TestCommandDeckCaptureCancelLockAndReplacementDoNotCaptureOldRequest(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	driver := &appDeckDriver{opened: make(chan *appDeckHandle, 8)}
	events := make(chan commandDeckCaptureEvent, 16)
	reservations := make(chan commandui.Reservation, 8)
	a.emitter = commandOSBootstrapEmitter(captureEmitter(events, reservations))
	_, p := installDeckCaptureDriver(t, a, driver)
	first := uuid.Must(uuid.NewV7()).String()
	if err := a.BeginCommandDeckCapture(first); err != nil {
		t.Fatal(err)
	}
	firstHandle := waitDeckHandle(t, driver.opened)
	second := uuid.Must(uuid.NewV7()).String()
	if err := a.BeginCommandDeckCapture(second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstHandle.closed:
	case <-time.After(6 * time.Second):
		t.Fatal("substituição não aposentou o handle antigo")
	}
	secondHandle := waitDeckHandle(t, driver.opened)
	firstHandle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	assertNoCapturedDeckEvent(t, events, first, 500*time.Millisecond)

	if err := p.host.SetOSSessionState(context.Background(), true, true); err != nil {
		t.Fatal(err)
	}
	secondHandle.events <- commanddeck.PhysicalKeyEvent{Index: 1, Down: true}
	select {
	case <-secondHandle.closed:
	case <-time.After(6 * time.Second):
		t.Fatal("lock não fechou o handle de captura")
	}
	waitDeckCaptureEvent(t, events, second, "cancelled")
	third := uuid.Must(uuid.NewV7()).String()
	if err := a.BeginCommandDeckCapture(third); err == nil {
		t.Fatal("captura deveria recusar início durante lock")
	}
}

type captureMultiDevice struct {
	id     commanddeck.DeviceID
	model  commanddeck.Model
	handle *appDeckHandle
}

type captureMultiDriver struct {
	opened chan captureMultiDevice
	mu     sync.Mutex
	items  map[commanddeck.DeviceID]captureMultiDevice
}

func (d *captureMultiDriver) Enumerate(context.Context) ([]commanddeck.PhysicalDevice, error) {
	return []commanddeck.PhysicalDevice{
		{ID: "deck-a", Model: commanddeck.Model{ID: "a", Name: "Model A", Rows: 1, Columns: 1, KeyImageW: 72, KeyImageH: 72, SupportsHID: true}},
		{ID: "deck-b", Model: commanddeck.Model{ID: "b", Name: "Model B", Rows: 1, Columns: 1, KeyImageW: 72, KeyImageH: 72, SupportsHID: true}},
	}, nil
}

func (d *captureMultiDriver) Open(_ context.Context, device commanddeck.PhysicalDevice) (commanddeck.Handle, error) {
	handle := &appDeckHandle{events: make(chan commanddeck.PhysicalKeyEvent, 8), writes: make(chan commanddeck.RenderPlan, 8), closed: make(chan struct{})}
	d.mu.Lock()
	item := captureMultiDevice{id: device.ID, model: device.Model, handle: handle}
	d.items[device.ID] = item
	d.mu.Unlock()
	d.opened <- item
	return handle, nil
}

func TestCommandDeckCaptureUsesSerialFromNativeSecondDevice(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	driver := &captureMultiDriver{opened: make(chan captureMultiDevice, 4), items: make(map[commanddeck.DeviceID]captureMultiDevice)}
	events := make(chan commandDeckCaptureEvent, 8)
	a.emitter = commandOSBootstrapEmitter(captureEmitter(events, make(chan commandui.Reservation, 2)))
	installDeckCaptureDriver(t, a, driver)
	requestID := uuid.Must(uuid.NewV7()).String()
	if err := a.BeginCommandDeckCapture(requestID); err != nil {
		t.Fatal(err)
	}
	var second captureMultiDevice
	for i := 0; i < 2; i++ {
		select {
		case item := <-driver.opened:
			if item.id == "deck-b" {
				second = item
			}
		case <-time.After(6 * time.Second):
			t.Fatal("segundo dispositivo não foi aberto")
		}
	}
	if second.handle == nil {
		t.Fatal("handle do segundo dispositivo ausente")
	}
	second.handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: true}
	second.handle.events <- commanddeck.PhysicalKeyEvent{Index: 0, Down: false}
	captured := waitDeckCaptureEvent(t, events, requestID, "captured")
	if captured.Model != "Model B" || captured.TriggerSpec != `{"version":1,"device":"deck-b","key":0}` {
		t.Fatalf("serial/model do segundo dispositivo não preservados: %+v", captured)
	}
}
