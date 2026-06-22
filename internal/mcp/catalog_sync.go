package mcp

import (
	"context"
	"log"
	"strings"

	"assistente/internal/database"
	"assistente/internal/toolcatalog"
)

// SyncBuiltinTools delega a catalogação das builtins ao serviço de catálogo.
// O MCP consome o catálogo (internal/toolcatalog); não o possui.
func (m *Manager) SyncBuiltinTools(ctx context.Context) error {
	catalog := m.toolCatalog()
	if catalog == nil || m.registry == nil {
		return nil
	}
	return catalog.SyncBuiltins(ctx, m.registry)
}

func (m *Manager) syncMCPTools(ctx context.Context, slug string, toolInfos []MCPToolInfo) error {
	catalog := m.toolCatalog()
	if catalog == nil {
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
	descriptors := make([]toolcatalog.MCPToolDescriptor, 0, len(toolInfos))
	for _, info := range toolInfos {
		descriptors = append(descriptors, toolcatalog.MCPToolDescriptor{
			FullName:    info.FullName,
			Name:        info.Name,
			Description: info.Description,
			Schema:      info.Schema,
		})
	}
	return catalog.SyncMCPServerTools(ctx, slug, serverID, ownerUserID, descriptors)
}

func (m *Manager) syncMCPToolsBestEffort(ctx context.Context, slug string, toolInfos []MCPToolInfo) {
	if err := m.syncMCPTools(ctx, slug, toolInfos); err != nil {
		log.Printf("[MCP:%s] Erro ao sincronizar catálogo de tools: %v", slug, err)
	}
}
