package toolcatalog

import (
	"context"
	"encoding/json"

	"assistente/internal/database"
	"assistente/internal/tools"
)

// Service orquestra a sincronização do catálogo de tools (builtins + MCP) sobre
// um Repository de persistência. É o ponto único consumido por MCP, builtins e
// (futuro) planner.
//
// Além da sincronização, reexpõe as operações de persistência do Repository por
// delegação, de modo que um único objeto satisfaça tanto os consumidores de
// sync quanto os de leitura/escrita (ex.: a tool de catálogo e o MCP Manager).
type Service struct {
	repo Repository
}

// NewService cria um Service sobre o Repository informado.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// UpsertTool delega ao Repository subjacente.
func (s *Service) UpsertTool(ctx context.Context, entry *tools.ToolCatalogEntry) error {
	return s.repo.UpsertTool(ctx, entry)
}

// ListTools delega ao Repository subjacente.
func (s *Service) ListTools(ctx context.Context, filter tools.ToolCatalogFilter) ([]tools.ToolCatalogEntry, error) {
	return s.repo.ListTools(ctx, filter)
}

// MarkServerToolsUnavailable delega ao Repository subjacente.
func (s *Service) MarkServerToolsUnavailable(ctx context.Context, serverID string, seenNames []string, reason string) (int, error) {
	return s.repo.MarkServerToolsUnavailable(ctx, serverID, seenNames, reason)
}

// RecordToolTest delega ao Repository subjacente.
func (s *Service) RecordToolTest(ctx context.Context, toolName, status, errorMessage string) error {
	return s.repo.RecordToolTest(ctx, toolName, status, errorMessage)
}

// RecordToolTestByID delega ao Repository subjacente.
func (s *Service) RecordToolTestByID(ctx context.Context, toolCatalogID, status, errorMessage string) error {
	return s.repo.RecordToolTestByID(ctx, toolCatalogID, status, errorMessage)
}

// SyncBuiltins cataloga as tools builtin descobríveis do registry. Tools com
// nome namespaced de MCP são ignoradas (são sincronizadas via SyncMCPServerTools
// com escopo de servidor/usuário).
func (s *Service) SyncBuiltins(ctx context.Context, registry *tools.Registry) error {
	if s == nil || s.repo == nil || registry == nil {
		return nil
	}
	for _, tool := range registry.Discoverable() {
		if isMCPNamespacedName(tool.Name()) {
			continue
		}
		entry := tools.CatalogEntryFromTool(tool)
		if err := s.repo.UpsertTool(ctx, &entry); err != nil {
			return err
		}
	}
	return nil
}

// MCPToolDescriptor descreve uma tool MCP a catalogar, num formato neutro que
// não depende dos tipos do pacote MCP (evita ciclo de import).
type MCPToolDescriptor struct {
	FullName    string
	Name        string
	Description string
	Schema      json.RawMessage
}

// SyncMCPServerTools cataloga as tools de um servidor MCP (origem bridge) e
// marca como indisponíveis as que não foram mais vistas na descoberta atual.
// Requer contexto autenticado.
func (s *Service) SyncMCPServerTools(ctx context.Context, slug, serverID, ownerUserID string, descriptors []MCPToolDescriptor) error {
	if s == nil || s.repo == nil {
		return nil
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	seen := make([]string, 0, len(descriptors))
	for _, info := range descriptors {
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
		if err := s.repo.UpsertTool(ctx, &entry); err != nil {
			return err
		}
	}
	if _, err := s.repo.MarkServerToolsUnavailable(ctx, serverID, seen, "not discovered"); err != nil {
		return err
	}
	return nil
}
