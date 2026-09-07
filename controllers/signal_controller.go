package controllers

import (
	"assistente/internal/apidto"
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
func (c *SignalController) SignalCheckAPI(apiURL, apiToken string) (apidto.SignalAPIStatus, error) {
	raw, err := signal.CheckAPI(apiURL, apiToken)
	if err != nil {
		return apidto.SignalAPIStatus{}, err
	}
	return signalAPIStatusFromRaw(raw), nil
}

// signalAPIStatusFromRaw converte a resposta bruta de /v1/about (decodificada
// como map genérico) para o DTO tipado da borda (D3). Campos ausentes ou com
// tipo inesperado ficam no zero value.
func signalAPIStatusFromRaw(raw map[string]interface{}) apidto.SignalAPIStatus {
	status := apidto.SignalAPIStatus{}
	if version, ok := raw["version"].(string); ok {
		status.Version = version
	}
	if mode, ok := raw["mode"].(string); ok {
		status.Mode = mode
	}
	if build, ok := raw["build"].(float64); ok {
		status.Build = int(build)
	}
	if versions, ok := raw["versions"].([]interface{}); ok {
		status.Versions = make([]string, 0, len(versions))
		for _, v := range versions {
			if s, ok := v.(string); ok {
				status.Versions = append(status.Versions, s)
			}
		}
	}
	if capabilities, ok := raw["capabilities"].(map[string]interface{}); ok {
		status.Capabilities = make(map[string][]string, len(capabilities))
		for key, val := range capabilities {
			items, ok := val.([]interface{})
			if !ok {
				continue
			}
			list := make([]string, 0, len(items))
			for _, item := range items {
				if s, ok := item.(string); ok {
					list = append(list, s)
				}
			}
			status.Capabilities[key] = list
		}
	}
	return status
}

// SignalListAccounts retorna as contas já registradas na signal-cli-rest-api.
func (c *SignalController) SignalListAccounts(apiURL, apiToken string) ([]string, error) {
	return signal.ListAccounts(apiURL, apiToken)
}
