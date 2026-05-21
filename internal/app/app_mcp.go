package app

import (
	"log"
	"time"

	"assistente/controllers"
	"assistente/internal/database"
	mcpmgr "assistente/internal/mcp"
	toolpkg "assistente/internal/tools"
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

func (a *App) GetMCPServerLogs(slug string, limit int) ([]mcpmgr.MCPServerLog, error) {
	return a.mcpCtrl.GetMCPServerLogs(slug, limit)
}

// LLMSettings é alias de controllers.LLMSettings para compatibilidade com o frontend Wails.
type LLMSettings = controllers.LLMSettings

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
	// MCP Manager precisa existir tanto pré quanto pós-login. O contexto
	// propaga o userID quando existe e devolve ctx puro durante o boot.
	// Escritores reais dentro do MCP manager seguem usando RequireUserID.
	a.mcpMgr.SetAuthContextProvider(a.internalBootstrapCtx)
	if database.DB() != nil {
		repo := mcpmgr.NewDBRepository(database.DB())
		a.mcpMgr.SetRepository(repo)
		if a.toolRegistry != nil && !a.toolRegistry.Has(toolpkg.ToolCatalogName) {
			a.toolRegistry.MustRegister(toolpkg.NewCatalogTool(repo))
		}
		a.mcpMgr.StartLogRetention(24*time.Hour, 30*24*time.Hour)
		if err := a.mcpMgr.SyncBuiltinTools(database.WithBootstrap(a.internalBootstrapCtx())); err != nil {
			log.Printf("[MCP] Erro ao sincronizar catálogo de builtin tools: %v", err)
		}
		// Carrega configs somente do DB (NÃO importa filesystem e NÃO conecta).
		// Importações legadas e auto-connect rodam no reloadUserScopedRuntime
		// pós-login, quando as credenciais user-scoped já estão em memória.
		if err := a.mcpMgr.LoadConfigs(); err != nil {
			log.Printf("[MCP] Erro ao carregar configurações: %v", err)
		}
	}

	log.Printf("[MCP] Manager inicializado")
}
