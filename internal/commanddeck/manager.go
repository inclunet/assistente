package commanddeck

import (
	"errors"
	"time"
)

var (
	ErrDeviceAlreadyOpen = errors.New("dispositivo de deck já aberto")
	ErrDeviceSafe        = errors.New("dispositivo de deck em estado seguro")
)

type DeviceStatus string

const (
	DeviceConnected    DeviceStatus = "connected"
	DeviceSafe         DeviceStatus = "safe"
	DeviceDisconnected DeviceStatus = "disconnected"
)

// BackoffPolicy define a cadência de reconexão após remoção/erro físico.
type BackoffPolicy struct {
	Initial time.Duration
	Max     time.Duration
}

func (p BackoffPolicy) validate() BackoffPolicy {
	if p.Initial <= 0 {
		p.Initial = 250 * time.Millisecond
	}
	if p.Max < p.Initial {
		p.Max = p.Initial
	}
	return p
}

// DeviceSnapshot é a visão observável do estado em memória do dispositivo.
type DeviceSnapshot struct {
	Device        DeviceID
	Model         Model
	Status        DeviceStatus
	Generation    uint64
	NextBackoff   time.Duration
	Reconnects    int
	SafeFrameSent bool
}

// Manager orquestra estado seguro e reconexão sem abrir HID. Um adapter real
// chama esses métodos depois de possuir/liberar handles físicos.
type Manager struct {
	renderer *Renderer
	backoff  BackoffPolicy
	now      func() time.Time
	devices  map[DeviceID]managedDevice
}

type managedDevice struct {
	model         Model
	status        DeviceStatus
	generation    uint64
	reconnects    int
	nextBackoff   time.Duration
	nextReconnect time.Time
	safeFrameSent bool
}

func NewManager(renderer *Renderer, policy BackoffPolicy) *Manager {
	if renderer == nil {
		renderer = NewRenderer()
	}
	return &Manager{
		renderer: renderer,
		backoff:  policy.validate(),
		now:      time.Now,
		devices:  make(map[DeviceID]managedDevice),
	}
}

func (m *Manager) SetClock(now func() time.Time) {
	if now != nil {
		m.now = now
	}
}

func (m *Manager) Open(device DeviceID, model Model) (RenderPlan, error) {
	if m == nil || !validText(string(device)) {
		return RenderPlan{}, ErrInvalidDevice
	}
	if err := model.validate(); err != nil {
		return RenderPlan{}, err
	}
	current, exists := m.devices[device]
	if exists && current.status != DeviceDisconnected {
		return RenderPlan{}, ErrDeviceAlreadyOpen
	}
	if err := m.renderer.OpenDevice(device, model); err != nil {
		return RenderPlan{}, err
	}
	nextBackoff := current.nextBackoff
	if nextBackoff <= 0 {
		nextBackoff = m.backoff.Initial
	}
	state := managedDevice{model: model, status: DeviceSafe, generation: current.generation + 1, reconnects: current.reconnects, nextBackoff: nextBackoff}
	m.devices[device] = state
	plan, err := m.RenderSafe(device)
	if err != nil {
		return RenderPlan{}, err
	}
	state = m.devices[device]
	state.safeFrameSent = true
	m.devices[device] = state
	return plan, nil
}

func (m *Manager) Activate(device DeviceID, generation uint64) error {
	state, err := m.state(device)
	if err != nil {
		return err
	}
	if generation != state.generation {
		return ErrInvalidDevice
	}
	state.status = DeviceConnected
	m.devices[device] = state
	return nil
}

func (m *Manager) Render(frame Frame) (RenderPlan, error) {
	state, err := m.state(frame.Device)
	if err != nil {
		return RenderPlan{}, err
	}
	if state.status != DeviceConnected {
		return RenderPlan{}, ErrDeviceSafe
	}
	if frame.Model.ID != state.model.ID {
		return RenderPlan{}, ErrInvalidModel
	}
	return m.renderer.Render(frame)
}

func (m *Manager) RenderSafe(device DeviceID) (RenderPlan, error) {
	state, err := m.state(device)
	if err != nil {
		return RenderPlan{}, err
	}
	plan, err := m.renderer.Render(Frame{Device: device, Model: state.model, Keys: map[int]KeyView{}})
	if err != nil {
		return RenderPlan{}, err
	}
	state.status = DeviceSafe
	state.safeFrameSent = true
	m.devices[device] = state
	return plan, nil
}

func (m *Manager) LockOrLogout() []RenderPlan {
	plans := make([]RenderPlan, 0, len(m.devices))
	for device, state := range m.devices {
		if state.status == DeviceDisconnected {
			continue
		}
		state.generation++
		m.devices[device] = state
		if plan, err := m.RenderSafe(device); err == nil {
			plans = append(plans, plan)
		}
	}
	return plans
}

func (m *Manager) Disconnect(device DeviceID) (time.Duration, error) {
	state, err := m.state(device)
	if err != nil {
		return 0, err
	}
	state.status = DeviceDisconnected
	state.generation++
	if state.nextBackoff <= 0 {
		state.nextBackoff = m.backoff.Initial
	}
	delay := state.nextBackoff
	state.nextReconnect = m.now().Add(delay)
	state.reconnects++
	state.nextBackoff *= 2
	if state.nextBackoff > m.backoff.Max {
		state.nextBackoff = m.backoff.Max
	}
	m.devices[device] = state
	m.renderer.RemoveDevice(device)
	return delay, nil
}

func (m *Manager) CanReconnect(device DeviceID) bool {
	state, err := m.state(device)
	if err != nil || state.status != DeviceDisconnected {
		return false
	}
	return !m.now().Before(state.nextReconnect)
}

func (m *Manager) Snapshot(device DeviceID) (DeviceSnapshot, error) {
	state, err := m.state(device)
	if err != nil {
		return DeviceSnapshot{}, err
	}
	return DeviceSnapshot{Device: device, Model: state.model, Status: state.status, Generation: state.generation, NextBackoff: state.nextBackoff, Reconnects: state.reconnects, SafeFrameSent: state.safeFrameSent}, nil
}

func (m *Manager) state(device DeviceID) (managedDevice, error) {
	if m == nil || !validText(string(device)) {
		return managedDevice{}, ErrInvalidDevice
	}
	state, ok := m.devices[device]
	if !ok {
		return managedDevice{}, ErrInvalidDevice
	}
	return state, nil
}
