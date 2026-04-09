//go:build !windows
// +build !windows

package speech

import "sync"

// Voice representa uma voz SAPI5
type Voice struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Language    string `json:"language"`
	Gender      string `json:"gender"`
	Age         string `json:"age"`
	Vendor      string `json:"vendor"`
	Description string `json:"description"`
	Source      string `json:"source"` // "sapi5"
}

// SAPI5Manager é um stub para sistemas não-Windows
type SAPI5Manager struct{}

var (
	manager     *SAPI5Manager
	managerOnce sync.Once
)

// GetSAPI5Manager retorna a instância singleton do manager
func GetSAPI5Manager() *SAPI5Manager {
	managerOnce.Do(func() {
		manager = &SAPI5Manager{}
	})
	return manager
}

// Initialize não faz nada em sistemas não-Windows
func (m *SAPI5Manager) Initialize() error {
	return nil
}

// GetVoices retorna lista vazia em sistemas não-Windows
func (m *SAPI5Manager) GetVoices() []Voice {
	return []Voice{}
}

// Speak não faz nada em sistemas não-Windows
func (m *SAPI5Manager) Speak(text string, voiceID string) error {
	return nil
}

// Stop não faz nada em sistemas não-Windows
func (m *SAPI5Manager) Stop() error {
	return nil
}

// SetVolume não faz nada em sistemas não-Windows
func (m *SAPI5Manager) SetVolume(volume int) error {
	return nil
}

// SetRate não faz nada em sistemas não-Windows
func (m *SAPI5Manager) SetRate(rate int) error {
	return nil
}

// IsSpeaking retorna false em sistemas não-Windows
func (m *SAPI5Manager) IsSpeaking() bool {
	return false
}

// WaitUntilDone retorna true imediatamente em sistemas não-Windows
func (m *SAPI5Manager) WaitUntilDone(timeoutMs int) bool {
	return true
}

// Cleanup não faz nada em sistemas não-Windows
func (m *SAPI5Manager) Cleanup() {
}

// StopSpeaking não faz nada em sistemas não-Windows
func StopSpeaking() {
}
