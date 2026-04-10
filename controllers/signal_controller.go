package controllers

import (
	"assistente/internal/messaging/signal"
)

// SignalController é o adapter primário (Inbound) para operações de provisionamento Signal.
// Não possui estado — todos os métodos delegam diretamente ao pacote signal.
type SignalController struct{}

// NewSignalController cria um SignalController.
func NewSignalController() *SignalController {
	return &SignalController{}
}

// SignalRegister inicia o registro de uma conta Signal via signal-cli-rest-api.
func (c *SignalController) SignalRegister(apiURL, number, mode, captcha, apiToken string) error {
	return signal.Register(apiURL, number, mode, captcha, apiToken)
}

// SignalVerify verifica o código recebido via SMS/ligação.
func (c *SignalController) SignalVerify(apiURL, number, code, apiToken string) error {
	return signal.Verify(apiURL, number, code, apiToken)
}

// SignalLink gera o QR code para vincular como dispositivo secundário.
func (c *SignalController) SignalLink(apiURL, deviceName, apiToken string) (string, error) {
	return signal.GetLinkQRCode(apiURL, deviceName, apiToken)
}

// SignalLinkRaw gera a URI texto para vincular como dispositivo secundário.
func (c *SignalController) SignalLinkRaw(apiURL, deviceName, apiToken string) (string, error) {
	return signal.GetLinkRawURI(apiURL, deviceName, apiToken)
}

// SignalUnregister remove uma conta da signal-cli-rest-api.
func (c *SignalController) SignalUnregister(apiURL, number string, deleteLocalData bool, apiToken string) error {
	return signal.Unregister(apiURL, number, deleteLocalData, apiToken)
}

// SignalCheckAPI verifica se a signal-cli-rest-api está acessível na URL informada.
func (c *SignalController) SignalCheckAPI(apiURL, apiToken string) (map[string]interface{}, error) {
	return signal.CheckAPI(apiURL, apiToken)
}

// SignalListAccounts retorna as contas já registradas na signal-cli-rest-api.
func (c *SignalController) SignalListAccounts(apiURL, apiToken string) ([]string, error) {
	return signal.ListAccounts(apiURL, apiToken)
}
