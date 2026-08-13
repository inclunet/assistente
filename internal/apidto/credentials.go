package apidto

// CredentialSummary descreve uma credencial para exibição (sem dados sensíveis).
type CredentialSummary struct {
	Pattern string `json:"pattern"`
	Type    string `json:"type"`
	Masked  string `json:"masked"`
	Managed bool   `json:"managed"`
}

// CredentialInput descreve a entrada para criar/atualizar credenciais.
type CredentialInput struct {
	Pattern     string `json:"pattern"`
	Type        string `json:"type"`
	Token       string `json:"token,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	HeaderName  string `json:"headerName,omitempty"`
	HeaderValue string `json:"headerValue,omitempty"`
}

// ExternalSourceSuggestion representa uma sugestão de fonte externa para autocomplete.
type ExternalSourceSuggestion struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
