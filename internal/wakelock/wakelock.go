package wakelock

import "sync"

// Manager garante que Enable/Disable são idempotentes e thread-safe.
type Manager struct {
	mu      sync.Mutex
	enabled bool
}

func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabled == enabled {
		return
	}
	if enabled {
		enable()
	} else {
		disable()
	}
	m.enabled = enabled
}

func (m *Manager) Release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabled {
		disable()
		m.enabled = false
	}
}
