package controllers

import (
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/tools"
)

// ToolInfo é um resumo de uma ferramenta para listagem no frontend.
type ToolInfo struct {
	Name        string `json:"name"`         // id interno (namespaced para MCP, ex: mcp_github__create_issue)
	DisplayName string `json:"display_name"` // nome curto para exibição (ex: create_issue)
	Description string `json:"description"`
	SourceType  string `json:"source_type"`  // "local" | "mcp"
	SourceLabel string `json:"source_label"` // "Local" | nome amigável do servidor MCP
}

// ToolsControllerConfig agrupa dependências do ToolsController.
type ToolsControllerConfig struct {
	ToolRegistry *tools.Registry
	MCPMgr       *mcpmgr.Manager
}

// ToolsController expõe operações sobre ferramentas disponíveis.
type ToolsController struct {
	toolRegistry *tools.Registry
	mcpMgr       *mcpmgr.Manager
}

// NewToolsController cria um ToolsController com as dependências fornecidas.
func NewToolsController(cfg ToolsControllerConfig) *ToolsController {
	return &ToolsController{
		toolRegistry: cfg.ToolRegistry,
		mcpMgr:       cfg.MCPMgr,
	}
}

// GetAvailableTools retorna a lista de ferramentas registradas no registry.
func (c *ToolsController) GetAvailableTools() []ToolInfo {
	if c.toolRegistry == nil {
		return []ToolInfo{}
	}

	allTools := c.toolRegistry.All()
	result := make([]ToolInfo, len(allTools))
	for i, t := range allTools {
		name := t.Name()
		info := ToolInfo{
			Name:        name,
			DisplayName: name,
			Description: t.Description(),
			SourceType:  "local",
			SourceLabel: "Local",
		}

		if slug, originalName, ok := mcpmgr.ParseToolName(name); ok {
			info.DisplayName = originalName
			info.SourceType = "mcp"
			info.SourceLabel = slug
			if c.mcpMgr != nil {
				if cfg, err := c.mcpMgr.GetConfig(slug); err == nil && cfg.Name != "" {
					info.SourceLabel = cfg.Name
				}
			}
		}

		result[i] = info
	}
	return result
}
