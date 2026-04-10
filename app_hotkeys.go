package main

import (
	"assistente/controllers"
)

// ============================================================================
// Global Hotkeys
// ============================================================================

// HotkeyInfo — type alias para controllers.
type HotkeyInfo = controllers.HotkeyInfo

// IsGlobalHotkeySupported verifica se hotkeys globais são suportados.
func (a *App) IsGlobalHotkeySupported() bool {
	return a.hotkeyCtrl.IsGlobalHotkeySupported()
}

// initGlobalHotkeys inicializa o gerenciador de hotkeys.
func (a *App) initGlobalHotkeys() {
	a.hotkeyCtrl.Init()
}

// registerActiveProfileHotkeys delega para o HotkeysController.
func (a *App) registerActiveProfileHotkeys() {
	a.hotkeyCtrl.RegisterActiveProfileHotkeys()
}
