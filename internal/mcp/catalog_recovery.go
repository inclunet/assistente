package mcp

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"

	"assistente/internal/database"
	"assistente/internal/tools"
)

// DiscoverableTools lê somente metadados de servidores habilitados do usuário.
// O schema persistido nunca é publicado como definição executável.
func (m *Manager) DiscoverableTools(ctx context.Context) ([]tools.ToolCatalogEntry, error) {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	servers := make(map[string]ServerConfig)
	for _, status := range m.servers {
		if status.Config.Enabled && status.Config.UserID == user {
			servers[status.ID] = status.Config
		}
	}
	m.mu.RUnlock()
	entries, err := m.ListToolCatalog(ctx, tools.ToolCatalogFilter{Origin: tools.ToolOriginMCPBridge, IncludeUnavailable: true})
	if err != nil {
		return nil, err
	}
	result := make([]tools.ToolCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		cfg, ok := servers[entry.MCPServerID]
		if !ok || entry.UserID != user || entry.AvailabilityReason == "not discovered" {
			continue
		}
		entry.Package = "mcp:" + cfg.Slug
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// PrepareCatalogTool conecta antes de carregar, sem executar nem repetir a tool.
// A confirmação usa a descoberta atual, mesmo se o sync do catálogo falhar.
func (m *Manager) PrepareCatalogTool(ctx context.Context, entry tools.ToolCatalogEntry) error {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if entry.Origin != tools.ToolOriginMCPBridge || entry.UserID != user {
		return fmt.Errorf("ferramenta MCP fora do escopo do usuário")
	}
	m.mu.RLock()
	var cfg ServerConfig
	for _, status := range m.servers {
		if status.ID == entry.MCPServerID && status.Config.UserID == user && status.Config.Enabled {
			cfg = status.Config
			break
		}
	}
	m.mu.RUnlock()
	if cfg.ID == "" {
		return fmt.Errorf("servidor da ferramenta MCP não está habilitado")
	}
	if err := m.ConnectWithContext(ctx, cfg.Slug); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	currentUser, err := database.RequireUserID(m.credentialContext())
	if err != nil || currentUser != user {
		return fmt.Errorf("sessão da ferramenta MCP mudou")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.servers[cfg.Slug]
	conn := m.connections[cfg.Slug]
	if status == nil || conn == nil || conn.sessionEnded || !status.Config.Enabled || !sameOAuthConsumer(status.Config, cfg) || status.Config.Transport != cfg.Transport || status.Config.Command != cfg.Command || !slices.Equal(status.Config.Args, cfg.Args) || !maps.Equal(status.Config.Env, cfg.Env) {
		return fmt.Errorf("configuração da ferramenta MCP mudou durante a conexão")
	}
	for _, bridge := range conn.bridges {
		if bridge.Name() == entry.Name {
			registered, ok := m.registry.Get(entry.Name)
			if ok && registered == bridge {
				return nil
			}
		}
	}
	return fmt.Errorf("ferramenta MCP '%s' não é mais oferecida pelo servidor", entry.Name)
}
