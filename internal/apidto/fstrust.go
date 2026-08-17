package apidto

// PathAllowlistView é a projeção de uma entrada de allowlist/denylist de path
// para a borda Wails (AEP-0092 Fase 1b/2).
type PathAllowlistView struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Operation string `json:"operation"`
	Effect    string `json:"effect"`
	Scope     string `json:"scope"`
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt string `json:"createdAt"`
	Reason    string `json:"reason,omitempty"`
}
