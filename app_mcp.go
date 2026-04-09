package main

import (
	"log"

	mcpmgr "assistente/internal/mcp"
)

// ============================================================================
// MCP Server Management API — delegação para MCPController
// Os métodos abaixo existem apenas para manter compatibilidade com o Wails Bind
// enquanto a migração para controllers/ está em andamento (Strangler Fig).
// ============================================================================

func (a *App) ListMCPServers() []mcpmgr.ServerInfo   { return a.mcpCtrl.ListMCPServers() }
func (a *App) ConnectMCPServer(slug string) error    { return a.mcpCtrl.ConnectMCPServer(slug) }
func (a *App) DisconnectMCPServer(slug string) error { return a.mcpCtrl.DisconnectMCPServer(slug) }
func (a *App) ReconnectMCPServer(slug string) error  { return a.mcpCtrl.ReconnectMCPServer(slug) }
func (a *App) SaveMCPServer(slug string, cfg mcpmgr.ServerConfig) error {
	return a.mcpCtrl.SaveMCPServer(slug, cfg)
}
func (a *App) DuplicateMCPServer(slug string) (string, error) {
	return a.mcpCtrl.DuplicateMCPServer(slug)
}
func (a *App) DeleteMCPServer(slug string) error { return a.mcpCtrl.DeleteMCPServer(slug) }
func (a *App) GetMCPServerTools(slug string) []mcpmgr.MCPToolInfo {
	return a.mcpCtrl.GetMCPServerTools(slug)
}
func (a *App) GetMCPServerConfig(slug string) (*mcpmgr.ServerConfig, error) {
	return a.mcpCtrl.GetMCPServerConfig(slug)
}
func (a *App) ReadMCPResource(slug, uri string) (string, error) {
	return a.mcpCtrl.ReadMCPResource(slug, uri)
}
func (a *App) GetMCPPrompt(slug, name string, arguments map[string]string) ([]string, error) {
	return a.mcpCtrl.GetMCPPrompt(slug, name, arguments)
}
func (a *App) SetMCPWorkspaceRoots(roots []mcpmgr.Root) error {
	return a.mcpCtrl.SetMCPWorkspaceRoots(roots)
}
func (a *App) GetMCPWorkspaceRoots() []mcpmgr.Root { return a.mcpCtrl.GetMCPWorkspaceRoots() }
func (a *App) SubscribeToMCPResource(slug, uri string) error {
	return a.mcpCtrl.SubscribeToMCPResource(slug, uri)
}
func (a *App) UnsubscribeFromMCPResource(slug, uri string) error {
	return a.mcpCtrl.UnsubscribeFromMCPResource(slug, uri)
}
func (a *App) SaveMCPServerAuth(slug, authType, token, username, password, clientSecret string) error {
	return a.mcpCtrl.SaveMCPServerAuth(slug, authType, token, username, password, clientSecret)
}
func (a *App) DeleteMCPServerAuth(slug string) error { return a.mcpCtrl.DeleteMCPServerAuth(slug) }
func (a *App) GetMCPServerAuthInfo(slug string) (map[string]any, error) {
	return a.mcpCtrl.GetMCPServerAuthInfo(slug)
}
func (a *App) DiscoverMCPServerAuth(serverURL string) mcpmgr.OAuthDiscoveryResult {
	return a.mcpCtrl.DiscoverMCPServerAuth(serverURL)
}

// LLMSettings contém configurações da API LLM (mantém compatibilidade com código legado).
type LLMSettings struct {
	APIKey  string
	BaseURL string
}

// initMCP inicializa o gerenciador de servidores MCP.
// Deve ser chamado após initToolRegistry (precisa do registry para registrar tools MCP).
func (a *App) initMCP() {
	emitEvent := func(event string, data any) {
		a.emitter.Emit(event, data)
		// Quando o set de tools MCP muda, regenera o catálogo de jobs
		if event == "mcp:tools_changed" && a.jobMgr != nil {
			go func() {
				if err := a.jobMgr.RegenerateCatalog(); err != nil {
					log.Printf("[Jobs] Catalog regeneration on MCP change failed: %v", err)
				} else {
					log.Printf("[Jobs] Catalog regenerated after MCP tools change")
				}
			}()
		}
	}

	a.mcpMgr = mcpmgr.NewManager(a.toolRegistry, a.credMgr, emitEvent)

	// Carrega configs e auto-conecta servidores habilitados
	if err := a.mcpMgr.LoadConfigs(); err != nil {
		log.Printf("[MCP] Erro ao carregar configurações: %v", err)
	}

	// Observa mudanças externas nos arquivos de config
	go a.mcpMgr.WatchConfigs()

	log.Printf("[MCP] Manager inicializado")
}
