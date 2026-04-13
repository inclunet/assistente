//go:build !windows
// +build !windows

package speech

import (
	"errors"
	"sync"
)

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

var errSAPI5Unavailable = errors.New("SAPI5 indisponível nesta plataforma")

// GetSAPI5Manager retorna a instância singleton do manager
func GetSAPI5Manager() *SAPI5Manager {
	managerOnce.Do(func() {
		manager = &SAPI5Manager{}
	})
	return manager
}

// GetVoices retorna lista vazia em sistemas não-Windows
func (m *SAPI5Manager) GetVoices() []Voice {
	return []Voice{}
}

// Speak retorna erro em sistemas não-Windows
func (m *SAPI5Manager) Speak(text string, voiceID string) error {
	return errSAPI5Unavailable
}

// SynthesizeToBytes retorna erro em sistemas não-Windows (SAPI5 indisponível)
func (m *SAPI5Manager) SynthesizeToBytes(text, voiceName string, rate, volume int) ([]byte, error) {
	return nil, errSAPI5Unavailable
}

// Stop retorna erro em sistemas não-Windows
func (m *SAPI5Manager) Stop() error {
	return errSAPI5Unavailable
}

// SetVolume retorna erro em sistemas não-Windows
func (m *SAPI5Manager) SetVolume(volume int) error {
	return errSAPI5Unavailable
}

// SetRate retorna erro em sistemas não-Windows
func (m *SAPI5Manager) SetRate(rate int) error {
	return errSAPI5Unavailable
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
