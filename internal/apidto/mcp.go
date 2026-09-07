package apidto

// MCPServerAuthInfo descreve se um servidor MCP tem autenticação configurada
// (sem expor segredos). Substitui o antigo map[string]any na borda Wails (D3).
type MCPServerAuthInfo struct {
	HasAuth  bool   `json:"hasAuth"`
	AuthType string `json:"authType"`
}
