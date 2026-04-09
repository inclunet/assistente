package main

// ============================================================================
// Database Management API
// ============================================================================

func (a *App) ResetDatabase() error {
	return a.settingsCtrl.ResetDatabase()
}

func (a *App) ClearMessages() error {
	return a.settingsCtrl.ClearMessages()
}

