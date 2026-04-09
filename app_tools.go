package main

import (
	"assistente/controllers"
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
