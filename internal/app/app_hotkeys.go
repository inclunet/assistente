package app

// ============================================================================
// Global Hotkeys
// ============================================================================

// initGlobalHotkeys inicializa o gerenciador de hotkeys.
func (a *App) initGlobalHotkeys() {
	a.hotkeyCtrl.Init()
}

// registerActiveProfileHotkeys delega para o HotkeysController.
func (a *App) registerActiveProfileHotkeys() {
	a.hotkeyCtrl.RegisterActiveProfileHotkeys()
}
