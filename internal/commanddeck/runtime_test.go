package commanddeck

import (
	"context"
	"errors"
	"testing"
)

type runtimeDriverFake struct {
	devices         []PhysicalDevice
	openErr         error
	openErrByDevice map[DeviceID]error
	handles         []*runtimeHandleFake
}

func (d *runtimeDriverFake) Enumerate(context.Context) ([]PhysicalDevice, error) {
	return d.devices, nil
}

func (d *runtimeDriverFake) Open(_ context.Context, device PhysicalDevice) (Handle, error) {
	if d.openErrByDevice != nil {
		if err := d.openErrByDevice[device.ID]; err != nil {
			return nil, err
		}
	}
	if d.openErr != nil {
		return nil, d.openErr
	}
	handle := &runtimeHandleFake{device: device.ID}
	d.handles = append(d.handles, handle)
	return handle, nil
}

func TestRuntimeDiscoverDetailedKeepsPartialFailuresObservable(t *testing.T) {
	controller := &adapterControllerSpy{}
	manager := NewManager(NewRenderer(), BackoffPolicy{})
	adapter, err := NewDeviceAdapter(manager, controller)
	if err != nil {
		t.Fatal(err)
	}
	blocked := errors.New("device busy")
	driver := &runtimeDriverFake{
		devices:         []PhysicalDevice{{ID: "deck-a", Model: testModel}, {ID: "deck-b", Model: testModel}},
		openErrByDevice: map[DeviceID]error{"deck-b": blocked},
	}
	runtime, err := NewRuntime(driver, adapter)
	if err != nil {
		t.Fatal(err)
	}
	results := runtime.DiscoverDetailed(context.Background())
	if len(results) != 2 {
		t.Fatalf("results=%+v", results)
	}
	if !results[0].Opened || results[0].Device != "deck-a" || results[0].Err != nil {
		t.Fatalf("deck-a deveria abrir: %+v", results[0])
	}
	if results[1].Opened || !errors.Is(results[1].Err, blocked) {
		t.Fatalf("deck-b deveria reportar falha parcial: %+v", results[1])
	}
	if _, err := manager.Snapshot("deck-a"); err != nil {
		t.Fatalf("deck-a deveria permanecer ativo: %v", err)
	}
	if _, err := manager.Snapshot("deck-b"); !errors.Is(err, ErrInvalidDevice) {
		t.Fatalf("deck-b não deveria criar estado: %v", err)
	}
}

type runtimeHandleFake struct {
	device  DeviceID
	writes  []RenderPlan
	events  []PhysicalKeyEvent
	readErr error
	closed  bool
}

func (h *runtimeHandleFake) Write(_ context.Context, plan RenderPlan) error {
	h.writes = append(h.writes, plan)
	return nil
}

func (h *runtimeHandleFake) Read(context.Context) (PhysicalKeyEvent, error) {
	if h.readErr != nil {
		return PhysicalKeyEvent{}, h.readErr
	}
	if len(h.events) == 0 {
		return PhysicalKeyEvent{}, errors.New("sem evento")
	}
	event := h.events[0]
	h.events = h.events[1:]
	return event, nil
}

func (h *runtimeHandleFake) Close(context.Context) error {
	h.closed = true
	return nil
}

func TestRuntimeDiscoverOpensSafeFrameAndActivates(t *testing.T) {
	controller := &adapterControllerSpy{}
	manager := NewManager(NewRenderer(), BackoffPolicy{})
	adapter, err := NewDeviceAdapter(manager, controller)
	if err != nil {
		t.Fatal(err)
	}
	driver := &runtimeDriverFake{devices: []PhysicalDevice{{ID: "deck-a", Model: testModel}}}
	runtime, err := NewRuntime(driver, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if errs := runtime.Discover(context.Background()); len(errs) != 0 {
		t.Fatalf("discover errs=%v", errs)
	}
	if len(driver.handles) != 1 || len(driver.handles[0].writes) != 1 {
		t.Fatalf("handle/writes inesperados: %+v", driver.handles)
	}
	if write := driver.handles[0].writes[0]; !write.FullFrame || len(write.Updates) != testModel.KeyCount() {
		t.Fatalf("abertura não escreveu frame seguro completo: %+v", write)
	}
	snapshot, err := manager.Snapshot("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != DeviceConnected {
		t.Fatalf("runtime não ativou dispositivo: %+v", snapshot)
	}
}

func TestRuntimePollForwardsEventsAndDisconnectsOnReadError(t *testing.T) {
	controller := &adapterControllerSpy{}
	manager := NewManager(NewRenderer(), BackoffPolicy{})
	adapter, err := NewDeviceAdapter(manager, controller)
	if err != nil {
		t.Fatal(err)
	}
	driver := &runtimeDriverFake{devices: []PhysicalDevice{{ID: "deck-a", Model: testModel}}}
	runtime, err := NewRuntime(driver, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if errs := runtime.Discover(context.Background()); len(errs) != 0 {
		t.Fatalf("discover errs=%v", errs)
	}
	handle := driver.handles[0]
	handle.events = append(handle.events, PhysicalKeyEvent{Index: 2, Down: true})
	if err := runtime.PollOne(context.Background(), "deck-a"); err != nil {
		t.Fatal(err)
	}
	if len(controller.events) != 1 || controller.events[0].SourceInstance != "streamdeck.key:deck-a" || controller.events[0].Key != "key:2" {
		t.Fatalf("evento encaminhado inválido: %+v", controller.events)
	}
	readErr := errors.New("hid removed")
	handle.readErr = readErr
	if err := runtime.PollOne(context.Background(), "deck-a"); !errors.Is(err, readErr) {
		t.Fatalf("erro de leitura err=%v", err)
	}
	if !handle.closed {
		t.Fatal("handle deveria fechar após erro de leitura")
	}
	snapshot, err := manager.Snapshot("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != DeviceDisconnected || snapshot.Reconnects != 1 {
		t.Fatalf("desconexão/backoff não registrada: %+v", snapshot)
	}
}

func TestRuntimeRenderSkipsEmptyDiffAndShutdownWritesSafeFrame(t *testing.T) {
	controller := &adapterControllerSpy{}
	manager := NewManager(NewRenderer(), BackoffPolicy{})
	adapter, err := NewDeviceAdapter(manager, controller)
	if err != nil {
		t.Fatal(err)
	}
	driver := &runtimeDriverFake{devices: []PhysicalDevice{{ID: "deck-a", Model: testModel}}}
	runtime, err := NewRuntime(driver, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if errs := runtime.Discover(context.Background()); len(errs) != 0 {
		t.Fatalf("discover errs=%v", errs)
	}
	handle := driver.handles[0]
	frame := Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{0: {Title: "A"}}}
	if err := runtime.Render(context.Background(), frame); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Render(context.Background(), frame); err != nil {
		t.Fatal(err)
	}
	if len(handle.writes) != 2 {
		t.Fatalf("segundo render com diff vazio não deveria escrever: writes=%d", len(handle.writes))
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !handle.closed {
		t.Fatal("shutdown deveria fechar handle")
	}
	last := handle.writes[len(handle.writes)-1]
	if !last.FullFrame || len(last.Updates) != testModel.KeyCount() {
		t.Fatalf("shutdown deveria escrever frame seguro completo: %+v", last)
	}
}

func TestRuntimeMultiDeviceKeepsRenderAndDisconnectIsolated(t *testing.T) {
	controller := &adapterControllerSpy{}
	manager := NewManager(NewRenderer(), BackoffPolicy{})
	adapter, err := NewDeviceAdapter(manager, controller)
	if err != nil {
		t.Fatal(err)
	}
	driver := &runtimeDriverFake{devices: []PhysicalDevice{{ID: "deck-a", Model: testModel}, {ID: "deck-b", Model: testModel}}}
	runtime, err := NewRuntime(driver, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if errs := runtime.Discover(context.Background()); len(errs) != 0 {
		t.Fatalf("discover errs=%v", errs)
	}
	if len(driver.handles) != 2 {
		t.Fatalf("handles=%d", len(driver.handles))
	}
	if err := runtime.Render(context.Background(), Frame{Device: "deck-b", Model: testModel, Keys: map[int]KeyView{1: {Title: "B"}}}); err != nil {
		t.Fatal(err)
	}
	if len(driver.handles[0].writes) != 1 || len(driver.handles[1].writes) != 2 {
		t.Fatalf("render de deck-b vazou: a=%d b=%d", len(driver.handles[0].writes), len(driver.handles[1].writes))
	}
	driver.handles[0].readErr = errors.New("deck-a removed")
	if err := runtime.PollOne(context.Background(), "deck-a"); err == nil {
		t.Fatal("esperava erro de remoção")
	}
	if err := runtime.Render(context.Background(), Frame{Device: "deck-b", Model: testModel, Keys: map[int]KeyView{2: {Title: "B2"}}}); err != nil {
		t.Fatalf("deck-b deveria seguir renderizando: %v", err)
	}
	if _, err := manager.Snapshot("deck-b"); err != nil {
		t.Fatalf("deck-b deveria seguir no manager: %v", err)
	}
}
