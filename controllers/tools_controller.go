package controllers

import (
	"context"
	"strings"

	"assistente/internal/apidto"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/tools"
)

// ToolInfo — alias estável durante a migração Strangler (AEP-0088 D5).
type ToolInfo = apidto.ToolInfo

// RuntimeToolCatalogFilter — alias estável da borda (AEP-0088 D5).
type RuntimeToolCatalogFilter = apidto.RuntimeToolCatalogFilter

// RuntimeToolCatalogEntry — alias estável da borda (AEP-0088 D5).
type RuntimeToolCatalogEntry = apidto.RuntimeToolCatalogEntry

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

	allTools := c.toolRegistry.Discoverable()
	result := make([]ToolInfo, len(allTools))
	for i, t := range allTools {
		name := t.Name()
		info := ToolInfo{
			Name:        name,
			DisplayName: name,
			Description: t.Description(),
			SourceType:  "local",
			SourceLabel: "Local",
			Package:     tools.CatalogMetadataForTool(t).Package,
			OptIn:       c.toolRegistry.IsOptIn(name),
		}

		if slug, originalName, ok := mcpmgr.ParseToolName(name); ok {
			info.DisplayName = originalName
			info.SourceType = "mcp"
			info.SourceLabel = slug
			info.Package = ""
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

// GetRuntimeToolCatalog lista entradas do catálogo persistido (filtro tipado).
func (c *ToolsController) GetRuntimeToolCatalog(ctx context.Context, filter RuntimeToolCatalogFilter) ([]RuntimeToolCatalogEntry, error) {
	if c.mcpMgr == nil {
		return []RuntimeToolCatalogEntry{}, nil
	}
	entries, err := c.mcpMgr.ListToolCatalog(ctx, runtimeToolCatalogFilterToTools(filter))
	if err != nil {
		return nil, err
	}
	result := make([]RuntimeToolCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, runtimeToolCatalogEntryFromTools(entry))
	}
	return result, nil
}

func normalizeRuntimeToolCatalogLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func normalizeRuntimeToolCatalogOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func runtimeToolCatalogFilterToTools(filter RuntimeToolCatalogFilter) tools.ToolCatalogFilter {
	return tools.ToolCatalogFilter{
		Origin:             strings.TrimSpace(filter.Origin),
		MCPServerID:        strings.TrimSpace(filter.MCPServerID),
		Category:           strings.TrimSpace(filter.Category),
		Class:              strings.TrimSpace(filter.Class),
		Package:            strings.TrimSpace(filter.Package),
		Risk:               strings.TrimSpace(filter.Risk),
		AvailabilityStatus: strings.TrimSpace(filter.AvailabilityStatus),
		IncludeUnavailable: filter.IncludeUnavailable,
		Limit:              normalizeRuntimeToolCatalogLimit(filter.Limit),
		Offset:             normalizeRuntimeToolCatalogOffset(filter.Offset),
	}
}

func runtimeToolCatalogEntryFromTools(entry tools.ToolCatalogEntry) RuntimeToolCatalogEntry {
	return RuntimeToolCatalogEntry{
		ID:                 entry.ID,
		UserID:             entry.UserID,
		MCPServerID:        entry.MCPServerID,
		Name:               entry.Name,
		DisplayName:        entry.DisplayName,
		Description:        entry.Description,
		Origin:             entry.Origin,
		Category:           entry.Category,
		Class:              entry.Class,
		Package:            entry.Package,
		Risk:               entry.Risk,
		Schema:             entry.Schema,
		SchemaHash:         entry.SchemaHash,
		SchemaBytes:        entry.SchemaBytes,
		Tags:               entry.Tags,
		AvailabilityStatus: entry.AvailabilityStatus,
		AvailabilityReason: entry.AvailabilityReason,
		LastSeenAt:         entry.LastSeenAt,
		LastAvailableAt:    entry.LastAvailableAt,
		LastUnavailableAt:  entry.LastUnavailableAt,
		LastTestedAt:       entry.LastTestedAt,
		LastTestStatus:     entry.LastTestStatus,
		LastTestError:      entry.LastTestError,
		CreatedAt:          entry.CreatedAt,
		UpdatedAt:          entry.UpdatedAt,
	}
}
