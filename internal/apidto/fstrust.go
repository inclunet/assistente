package apidto

// PathAllowlistView é a projeção de uma entrada de allowlist de path para a
// borda Wails (AEP-0092 Fase 1b).
type PathAllowlistView struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Operation string `json:"operation"`
	Scope     string `json:"scope"`
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt string `json:"createdAt"`
	Reason    string `json:"reason,omitempty"`
}
