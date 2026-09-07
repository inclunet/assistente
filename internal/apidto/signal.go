package apidto

// SignalAPIStatus descreve a resposta de /v1/about da signal-cli-rest-api
// (D3). Substitui o antigo map[string]any na borda Wails.
type SignalAPIStatus struct {
	Versions     []string            `json:"versions"`
	Build        int                 `json:"build"`
	Mode         string              `json:"mode"`
	Version      string              `json:"version"`
	Capabilities map[string][]string `json:"capabilities"`
}
