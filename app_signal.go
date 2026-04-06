package main

import (
	"assistente/internal/messaging/signal"
)

// ============================================================================
// Signal Wizard API (signal-cli-rest-api provisioning)
// ============================================================================

// SignalRegister inicia o registro de uma conta Signal via signal-cli-rest-api.
// mode: "sms" (padrão) ou "voice" para receber o código por ligação.
// captcha: token de verificação exigido pelo Signal (signalcaptcha://...).
func (a *App) SignalRegister(apiURL, number, mode, captcha, apiToken string) error {
	return signal.Register(apiURL, number, mode, captcha, apiToken)
}

// SignalVerify verifica o código recebido via SMS/ligação.
func (a *App) SignalVerify(apiURL, number, code, apiToken string) error {
	return signal.Verify(apiURL, number, code, apiToken)
}

// SignalLink gera o QR code para vincular como dispositivo secundário.
// Retorna a imagem QR code em base64 (PNG).
func (a *App) SignalLink(apiURL, deviceName, apiToken string) (string, error) {
	return signal.GetLinkQRCode(apiURL, deviceName, apiToken)
}

// SignalLinkRaw gera a URI texto para vincular como dispositivo secundário.
// Alternativa acessível ao QR code.
func (a *App) SignalLinkRaw(apiURL, deviceName, apiToken string) (string, error) {
	return signal.GetLinkRawURI(apiURL, deviceName, apiToken)
}

// SignalUnregister remove uma conta da signal-cli-rest-api.
func (a *App) SignalUnregister(apiURL, number string, deleteLocalData bool, apiToken string) error {
	return signal.Unregister(apiURL, number, deleteLocalData, apiToken)
}

// SignalCheckAPI verifica se a signal-cli-rest-api está acessível na URL informada.
func (a *App) SignalCheckAPI(apiURL, apiToken string) (map[string]interface{}, error) {
	return signal.CheckAPI(apiURL, apiToken)
}

// SignalListAccounts retorna as contas já registradas na signal-cli-rest-api.
func (a *App) SignalListAccounts(apiURL, apiToken string) ([]string, error) {
	return signal.ListAccounts(apiURL, apiToken)
}
