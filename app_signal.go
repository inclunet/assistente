package main

// ============================================================================
// Signal Wizard API (signal-cli-rest-api provisioning)
// ============================================================================

func (a *App) SignalRegister(apiURL, number, mode, captcha, apiToken string) error {
	return a.signalCtrl.SignalRegister(apiURL, number, mode, captcha, apiToken)
}

func (a *App) SignalVerify(apiURL, number, code, apiToken string) error {
	return a.signalCtrl.SignalVerify(apiURL, number, code, apiToken)
}

func (a *App) SignalLink(apiURL, deviceName, apiToken string) (string, error) {
	return a.signalCtrl.SignalLink(apiURL, deviceName, apiToken)
}

func (a *App) SignalLinkRaw(apiURL, deviceName, apiToken string) (string, error) {
	return a.signalCtrl.SignalLinkRaw(apiURL, deviceName, apiToken)
}

func (a *App) SignalUnregister(apiURL, number string, deleteLocalData bool, apiToken string) error {
	return a.signalCtrl.SignalUnregister(apiURL, number, deleteLocalData, apiToken)
}

func (a *App) SignalCheckAPI(apiURL, apiToken string) (map[string]interface{}, error) {
	return a.signalCtrl.SignalCheckAPI(apiURL, apiToken)
}

func (a *App) SignalListAccounts(apiURL, apiToken string) ([]string, error) {
	return a.signalCtrl.SignalListAccounts(apiURL, apiToken)
}

