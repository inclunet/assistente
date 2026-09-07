package app

import (
	"assistente/internal/logging"
	"context"
	"time"

	"assistente/internal/database"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/toolcatalog"
	toolpkg "assistente/internal/tools"
)

// initMCP inicializa o gerenciador de servidores MCP.
// Deve ser chamado após initToolRegistry (precisa do registry para registrar tools MCP).
// A superfície Wails do domínio MCP vive em wailsapi.MCP (AEP-0088).
func (a *App) initMCP() {
	emitEvent := func(event string, data any) {
		a.emitter.Emit(event, data)
		// Quando o set de tools MCP muda, regenera o catálogo de jobs
		if event == "mcp:tools_changed" && a.jobMgr != nil {
			go func() {
				if err := a.jobMgr.RegenerateCatalog(); err != nil {
					logging.Errorf(context.Background(), "app.app-mcp", "[Jobs] Catalog regeneration on MCP change failed: %v", err)
				} else {
					logging.Infof(context.Background(), "app.app-mcp", "[Jobs] Catalog regenerated after MCP tools change")
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
		// O catálogo de tools tem dono dedicado (internal/toolcatalog); o MCP e a
		// tool de catálogo apenas o consomem (AEP-0077, Fase 2 / #120).
		catalog := toolcatalog.NewService(toolcatalog.NewDBRepository(database.DB()))
		a.mcpMgr.SetCatalog(catalog)
		if a.toolRegistry != nil && !a.toolRegistry.Has(toolpkg.ToolCatalogName) {
			a.toolRegistry.MustRegister(toolpkg.NewCatalogTool(catalog))
		}
		a.mcpMgr.StartLogRetention(24*time.Hour, 30*24*time.Hour)
		if err := a.mcpMgr.SyncBuiltinTools(database.WithBootstrap(a.internalBootstrapCtx())); err != nil {
			logging.Errorf(context.Background(), "app.app-mcp", "[MCP] Erro ao sincronizar catálogo de builtin tools: %v", err)
		}
		// Carrega configs somente do DB (NÃO importa filesystem e NÃO conecta).
		// Importações legadas e auto-connect rodam no reloadUserScopedRuntime
		// pós-login, quando as credenciais user-scoped já estão em memória.
		if err := a.mcpMgr.LoadConfigs(); err != nil {
			logging.Errorf(context.Background(), "app.app-mcp", "[MCP] Erro ao carregar configurações: %v", err)
		}
	}

	logging.Infof(context.Background(), "app.app-mcp", "[MCP] Manager inicializado")
}
