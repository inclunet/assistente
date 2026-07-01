package app

import (
	"encoding/json"
	"strings"
	"time"

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

type RuntimeToolCatalogFilter struct {
	Origin             string `json:"origin,omitempty"`
	MCPServerID        string `json:"mcpServerId,omitempty"`
	Category           string `json:"category,omitempty"`
	Class              string `json:"class,omitempty"`
	Package            string `json:"package,omitempty"`
	Risk               string `json:"risk,omitempty"`
	AvailabilityStatus string `json:"availabilityStatus,omitempty"`
	IncludeUnavailable bool   `json:"includeUnavailable,omitempty"`
	Limit              int    `json:"limit,omitempty"`
	Offset             int    `json:"offset,omitempty"`
}

type RuntimeToolCatalogEntry struct {
	ID                 string          `json:"id"`
	UserID             string          `json:"userId,omitempty"`
	MCPServerID        string          `json:"mcpServerId,omitempty"`
	Name               string          `json:"name"`
	DisplayName        string          `json:"displayName"`
	Description        string          `json:"description,omitempty"`
	Origin             string          `json:"origin"`
	Category           string          `json:"category,omitempty"`
	Class              string          `json:"class,omitempty"`
	Package            string          `json:"package,omitempty"`
	Risk               string          `json:"risk,omitempty"`
	Schema             json.RawMessage `json:"schema,omitempty"`
	SchemaHash         string          `json:"schemaHash,omitempty"`
	SchemaBytes        int             `json:"schemaBytes,omitempty"`
	Tags               []string        `json:"tags,omitempty"`
	AvailabilityStatus string          `json:"availabilityStatus"`
	AvailabilityReason string          `json:"availabilityReason,omitempty"`
	LastSeenAt         *time.Time      `json:"lastSeenAt,omitempty"`
	LastAvailableAt    *time.Time      `json:"lastAvailableAt,omitempty"`
	LastUnavailableAt  *time.Time      `json:"lastUnavailableAt,omitempty"`
	LastTestedAt       *time.Time      `json:"lastTestedAt,omitempty"`
	LastTestStatus     string          `json:"lastTestStatus,omitempty"`
	LastTestError      string          `json:"lastTestError,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
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

func (a *App) GetRuntimeToolCatalog(filter RuntimeToolCatalogFilter) ([]RuntimeToolCatalogEntry, error) {
	if a.mcpMgr == nil {
		return []RuntimeToolCatalogEntry{}, nil
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	entries, err := a.mcpMgr.ListToolCatalog(ctx, runtimeToolCatalogFilterToTools(filter))
	if err != nil {
		return nil, err
	}
	result := make([]RuntimeToolCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, runtimeToolCatalogEntryFromTools(entry))
	}
	return result, nil
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
