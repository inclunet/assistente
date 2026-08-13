package apidto

// NetworkAllowlistView é a projeção de uma entrada de allowlist de rede para a
// borda Wails (AEP-0088 / AEP-0082).
type NetworkAllowlistView struct {
	Host        string   `json:"host"`
	Port        string   `json:"port,omitempty"`
	Scope       string   `json:"scope"`
	Category    string   `json:"category,omitempty"`
	ResolvedIPs []string `json:"resolvedIps,omitempty"`
	CreatedBy   string   `json:"createdBy,omitempty"`
	CreatedAt   string   `json:"createdAt"`
	Reason      string   `json:"reason,omitempty"`
}
