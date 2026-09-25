package commanddeck

import (
	"errors"
	"sync"
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
	mu       sync.Mutex
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
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.openLocked(device, model)
}

func (m *Manager) openLocked(device DeviceID, model Model) (RenderPlan, error) {
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
	plan, err := m.renderSafeLocked(device)
	if err != nil {
		return RenderPlan{}, err
	}
	state = m.devices[device]
	state.safeFrameSent = true
	m.devices[device] = state
	return plan, nil
}

func (m *Manager) Activate(device DeviceID, generation uint64) error {
	if m == nil {
		return ErrInvalidDevice
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.stateLocked(device)
	if err != nil {
		return err
	}
	if state.status != DeviceSafe || generation != state.generation {
		return ErrInvalidDevice
	}
	state.status = DeviceConnected
	m.devices[device] = state
	return nil
}

func (m *Manager) Render(frame Frame) (RenderPlan, error) {
	if m == nil {
		return RenderPlan{}, ErrInvalidDevice
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.stateLocked(frame.Device)
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
	if m == nil {
		return RenderPlan{}, ErrInvalidDevice
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.renderSafeLocked(device)
}

func (m *Manager) renderSafeLocked(device DeviceID) (RenderPlan, error) {
	state, err := m.stateLocked(device)
	if err != nil {
		return RenderPlan{}, err
	}
	if state.status == DeviceDisconnected {
		return RenderPlan{}, ErrDeviceSafe
	}
	if err := m.renderer.InvalidateDevice(device); err != nil {
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
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	plans := make([]RenderPlan, 0, len(m.devices))
	for device, state := range m.devices {
		if state.status == DeviceDisconnected {
			continue
		}
		state.generation++
		m.devices[device] = state
		if plan, err := m.renderSafeLocked(device); err == nil {
			plans = append(plans, plan)
		}
	}
	return plans
}

func (m *Manager) Disconnect(device DeviceID) (time.Duration, error) {
	if m == nil {
		return 0, ErrInvalidDevice
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.stateLocked(device)
	if err != nil {
		return 0, err
	}
	if state.status == DeviceDisconnected {
		remaining := state.nextReconnect.Sub(m.now())
		if remaining < 0 {
			remaining = 0
		}
		return remaining, nil
	}
	state.status = DeviceDisconnected
	state.generation++
	if state.nextBackoff <= 0 {
		state.nextBackoff = m.backoff.Initial
	}
	delay := state.nextBackoff
	state.nextReconnect = m.now().Add(delay)
	state.reconnects++
	if state.nextBackoff > m.backoff.Max/2 {
		state.nextBackoff = m.backoff.Max
	} else {
		state.nextBackoff *= 2
	}
	m.devices[device] = state
	m.renderer.RemoveDevice(device)
	return delay, nil
}

func (m *Manager) CanReconnect(device DeviceID) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.stateLocked(device)
	if err != nil || state.status != DeviceDisconnected {
		return false
	}
	return !m.now().Before(state.nextReconnect)
}

func (m *Manager) Snapshot(device DeviceID) (DeviceSnapshot, error) {
	if m == nil {
		return DeviceSnapshot{}, ErrInvalidDevice
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.stateLocked(device)
	if err != nil {
		return DeviceSnapshot{}, err
	}
	return DeviceSnapshot{Device: device, Model: state.model, Status: state.status, Generation: state.generation, NextBackoff: state.nextBackoff, Reconnects: state.reconnects, SafeFrameSent: state.safeFrameSent}, nil
}

func (m *Manager) stateLocked(device DeviceID) (managedDevice, error) {
	if !validText(string(device)) {
		return managedDevice{}, ErrInvalidDevice
	}
	state, ok := m.devices[device]
	if !ok {
		return managedDevice{}, ErrInvalidDevice
	}
	return state, nil
}
