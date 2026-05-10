package app

// ============================================================================
// Database Management API
// ============================================================================

func (a *App) ResetDatabase() error {
	return a.settingsCtrl.ResetDatabase()
}

func (a *App) ClearMessages() error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.settingsCtrl.ClearMessages(ctx)
}
