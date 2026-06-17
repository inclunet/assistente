package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ToolCatalogName = "tool_catalog"
	LoadSkillName   = "load_skill"
)

type CatalogToolStore interface {
	ListTools(ctx context.Context, filter ToolCatalogFilter) ([]ToolCatalogEntry, error)
}

type CatalogTool struct {
	store CatalogToolStore
}

type catalogToolRequest struct {
	Origin             string `json:"origin,omitempty"`
	Category           string `json:"category,omitempty"`
	Class              string `json:"class,omitempty"`
	Package            string `json:"package,omitempty"`
	Risk               string `json:"risk,omitempty"`
	AvailabilityStatus string `json:"availability_status,omitempty"`
	IncludeUnavailable bool   `json:"include_unavailable,omitempty"`
	Limit              int    `json:"limit,omitempty"`
}

type catalogToolResponse struct {
	Tools         []catalogToolItem `json:"tools"`
	SelectedTools []string          `json:"selected_tools"`
	Count         int               `json:"count"`
}

type catalogToolItem struct {
	Name               string `json:"name"`
	DisplayName        string `json:"display_name"`
	Description        string `json:"description,omitempty"`
	Origin             string `json:"origin"`
	Category           string `json:"category,omitempty"`
	Class              string `json:"class,omitempty"`
	Package            string `json:"package,omitempty"`
	Risk               string `json:"risk,omitempty"`
	AvailabilityStatus string `json:"availability_status"`
}

func NewCatalogTool(store CatalogToolStore) *CatalogTool {
	return &CatalogTool{store: store}
}

func (t *CatalogTool) Name() string { return ToolCatalogName }

func (t *CatalogTool) Description() string {
	return "Discover and select tool capabilities from the persisted catalog (filter by origin, category, class, package, risk or availability); the tools you select only become available on the next turn. When tool access in the session is gated by the catalog, this may be the only tool available initially and you must call it first to unlock the tools you need. In sessions where other tools are already provided, use it only when you need a capability that is not yet available."
}

func (t *CatalogTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "origin": {"type": "string", "description": "Optional origin filter: builtin, mcp_bridge, or mcp_native."},
    "category": {"type": "string", "description": "Optional category filter, for example filesystem, web, tasklist, or mcp:<server>."},
    "class": {"type": "string", "description": "Optional capability class, for example read_context, edit_files, web_lookup, task_management, mcp_tool."},
    "package": {"type": "string", "description": "Optional package filter, for example coding_readonly, coding_edit, web, tasks, or mcp:<server>."},
    "risk": {"type": "string", "description": "Optional risk filter: read, write, destructive, network, shell."},
    "availability_status": {"type": "string", "description": "Optional availability filter: available or unavailable."},
    "include_unavailable": {"type": "boolean", "description": "Whether unavailable tools should be included."},
    "limit": {"type": "integer", "description": "Maximum number of tools to return.", "minimum": 1, "maximum": 50}
  }
}`)
}

func (t *CatalogTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	if t.store == nil {
		return ToolResult{Content: "catálogo de tools não configurado", IsError: true}, nil
	}
	var req catalogToolRequest
	if len(args) > 0 {
		if err := json.Unmarshal(args, &req); err != nil {
			return ToolResult{Content: fmt.Sprintf("argumentos inválidos para tool_catalog: %v", err), IsError: true}, nil
		}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 50 {
		limit = 50
	}
	filter := ToolCatalogFilter{
		Origin:             strings.TrimSpace(req.Origin),
		Category:           strings.TrimSpace(req.Category),
		Class:              strings.TrimSpace(req.Class),
		Package:            strings.TrimSpace(req.Package),
		Risk:               strings.TrimSpace(req.Risk),
		AvailabilityStatus: strings.TrimSpace(req.AvailabilityStatus),
		IncludeUnavailable: req.IncludeUnavailable,
		Limit:              limit,
	}
	entries, err := t.store.ListTools(ctx, filter)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("erro ao consultar catálogo de tools: %v", err), IsError: true}, nil
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	resp := catalogToolResponse{
		Tools:         make([]catalogToolItem, 0, len(entries)),
		SelectedTools: make([]string, 0, len(entries)),
		Count:         len(entries),
	}
	for _, entry := range entries {
		resp.SelectedTools = append(resp.SelectedTools, entry.Name)
		resp.Tools = append(resp.Tools, catalogToolItem{
			Name:               entry.Name,
			DisplayName:        entry.DisplayName,
			Description:        entry.Description,
			Origin:             entry.Origin,
			Category:           entry.Category,
			Class:              entry.Class,
			Package:            entry.Package,
			Risk:               entry.Risk,
			AvailabilityStatus: entry.AvailabilityStatus,
		})
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("erro ao serializar resposta do catálogo de tools: %v", err), IsError: true}, nil
	}
	return ToolResult{Content: string(data)}, nil
}
