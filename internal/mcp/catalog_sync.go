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
	for _, tool := range m.registry.All() {
		if _, _, ok := ParseToolName(tool.Name()); ok {
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
	serverID := ""
	ownerUserID := ""
	if status != nil {
		serverID = strings.TrimSpace(status.ID)
		ownerUserID = status.Config.UserID
	}
	m.mu.RUnlock()
	if serverID == "" {
		return nil
	}
	seen := make([]string, 0, len(toolInfos))
	for _, info := range toolInfos {
		seen = append(seen, info.FullName)
		entry := tools.ToolCatalogEntry{
			UserID:             ownerUserID,
			MCPServerID:        serverID,
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
	if _, err := repo.MarkServerToolsUnavailable(ctx, serverID, seen, "not discovered"); err != nil {
		return err
	}
	return nil
}

func (m *Manager) syncMCPToolsBestEffort(ctx context.Context, slug string, toolInfos []MCPToolInfo) {
	if err := m.syncMCPTools(ctx, slug, toolInfos); err != nil {
		log.Printf("[MCP:%s] Erro ao sincronizar catálogo de tools: %v", slug, err)
	}
}
