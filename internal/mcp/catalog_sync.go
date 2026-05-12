package mcp

import (
	"context"
	"log"
	"strings"

	"assistente/internal/database"
	"assistente/internal/tools"
)

func (m *Manager) SyncBuiltinTools(ctx context.Context) error {
	repo := m.repository()
	if repo == nil || m.registry == nil {
		return nil
	}
	for _, name := range m.registry.Names() {
		if _, _, ok := ParseToolName(name); ok {
			continue
		}
		tool, ok := m.registry.Get(name)
		if !ok {
			continue
		}
		entry := tools.CatalogEntryFromTool(tool)
		if err := repo.UpsertTool(ctx, &entry); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) syncMCPTools(ctx context.Context, slug string, toolInfos []MCPToolInfo) error {
	repo := m.repository()
	if repo == nil {
		return nil
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	m.mu.RLock()
	status := m.servers[slug]
	m.mu.RUnlock()
	if status == nil || strings.TrimSpace(status.ID) == "" {
		return nil
	}
	seen := make([]string, 0, len(toolInfos))
	for _, info := range toolInfos {
		seen = append(seen, info.FullName)
		entry := tools.ToolCatalogEntry{
			UserID:             status.Config.UserID,
			MCPServerID:        status.ID,
			Name:               info.FullName,
			DisplayName:        info.Name,
			Description:        info.Description,
			Origin:             tools.ToolOriginMCPBridge,
			Category:           "mcp:" + slug,
			Class:              "mcp_tool",
			Package:            "mcp:" + slug,
			Risk:               "network",
			Schema:             info.Schema,
			SchemaHash:         tools.SchemaHash(info.Schema),
			SchemaBytes:        len(info.Schema),
			AvailabilityStatus: tools.ToolAvailabilityAvailable,
		}
		if err := repo.UpsertTool(ctx, &entry); err != nil {
			return err
		}
	}
	if _, err := repo.MarkServerToolsUnavailable(ctx, status.ID, seen, "not discovered"); err != nil {
		return err
	}
	return nil
}

func (m *Manager) syncMCPToolsBestEffort(ctx context.Context, slug string, toolInfos []MCPToolInfo) {
	if err := m.syncMCPTools(ctx, slug, toolInfos); err != nil {
		log.Printf("[MCP:%s] Erro ao sincronizar catálogo de tools: %v", slug, err)
	}
}
