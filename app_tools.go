package main

import (
	mcpmgr "assistente/internal/mcp"
)

// ============================================================================
// Tools API
// ============================================================================

// ToolInfo é um resumo de uma ferramenta para listagem no frontend.
type ToolInfo struct {
	Name        string `json:"name"`         // id interno (namespaced para MCP, ex: mcp_github__create_issue)
	DisplayName string `json:"display_name"` // nome curto para exibição (ex: create_issue)
	Description string `json:"description"`
	SourceType  string `json:"source_type"`  // "local" | "mcp"
	SourceLabel string `json:"source_label"` // "Local" | nome amigável do servidor MCP
}

// GetAvailableTools retorna a lista de ferramentas registradas no registry.
// Usado pelo frontend para exibir checkboxes no editor de perfis.
func (a *App) GetAvailableTools() []ToolInfo {
	if a.toolRegistry == nil {
		return []ToolInfo{}
	}

	allTools := a.toolRegistry.All()
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
			if a.mcpMgr != nil {
				if cfg, err := a.mcpMgr.GetConfig(slug); err == nil && cfg.Name != "" {
					info.SourceLabel = cfg.Name
				}
			}
		}

		result[i] = info
	}
	return result
}
