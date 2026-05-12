package app

import (
	"assistente/controllers"
	"assistente/internal/tools"
)

// ============================================================================
// Tools API
// ============================================================================

// ToolInfo é alias de controllers.ToolInfo para compatibilidade com o frontend Wails.
type ToolInfo = controllers.ToolInfo

// GetAvailableTools retorna a lista de ferramentas registradas no registry.
func (a *App) GetAvailableTools() []ToolInfo {
	return a.toolsCtrl.GetAvailableTools()
}

func (a *App) GetRuntimeToolCatalog(filter tools.ToolCatalogFilter) ([]tools.ToolCatalogEntry, error) {
	if a.mcpMgr == nil {
		return []tools.ToolCatalogEntry{}, nil
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.mcpMgr.ListToolCatalog(ctx, filter)
}
