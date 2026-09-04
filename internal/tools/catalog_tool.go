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

	defaultCatalogToolLimit = 20
	maxCatalogToolPageSize  = 50

	catalogActionSearch     = "search"
	catalogActionLoad       = "load"
	catalogActionUnload     = "unload"
	catalogActionListLoaded = "list_loaded"

	loadedToolRejectWildcardLimit = "wildcard_limit_exceeded"
)

type CatalogToolStore interface {
	ListTools(ctx context.Context, filter ToolCatalogFilter) ([]ToolCatalogEntry, error)
}

type CatalogTool struct {
	store CatalogToolStore
}

type catalogToolRequest struct {
	Action             string   `json:"action,omitempty"`
	Tools              []string `json:"tools,omitempty"`
	Query              string   `json:"query,omitempty"`
	Origin             string   `json:"origin,omitempty"`
	Category           string   `json:"category,omitempty"`
	Class              string   `json:"class,omitempty"`
	Package            string   `json:"package,omitempty"`
	Risk               string   `json:"risk,omitempty"`
	AvailabilityStatus string   `json:"availability_status,omitempty"`
	IncludeUnavailable bool     `json:"include_unavailable,omitempty"`
	Limit              int      `json:"limit,omitempty"`
	Offset             int      `json:"offset,omitempty"`
}

type catalogToolResponse struct {
	Tools         []catalogToolItem      `json:"tools"`
	SelectedTools []string               `json:"selected_tools"`
	LoadedTools   []string               `json:"loaded_tools,omitempty"`
	UnloadedTools []string               `json:"unloaded_tools,omitempty"`
	RejectedTools []catalogToolRejection `json:"rejected_tools,omitempty"`
	Loaded        []LoadedToolRecord     `json:"loaded,omitempty"`
	Count         int                    `json:"count"`
	Limit         int                    `json:"limit"`
	Offset        int                    `json:"offset"`
	HasMore       bool                   `json:"has_more"`
	NextOffset    int                    `json:"next_offset,omitempty"`
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

type catalogToolRejection struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func NewCatalogTool(store CatalogToolStore) *CatalogTool {
	return &CatalogTool{store: store}
}

func (t *CatalogTool) Name() string { return ToolCatalogName }

func (t *CatalogTool) Description() string {
	return "Discover and manage authorized on-demand tools. Search once with a task query and optional filters; results rank task relevance, profile preferred packages, conversation recency, then stable name. Examples: {\"action\":\"search\",\"query\":\"read files\",\"category\":\"filesystem\",\"risk\":\"read\"}, {\"action\":\"search\",\"query\":\"search Jira\",\"package\":\"mcp:atlassian\"}, {\"action\":\"load\",\"tools\":[\"read_file\"]}, or {\"action\":\"load\",\"tools\":[\"mcp/atlassian/*\"]}. Wildcard load is capped at 20 matches and still obeys profile policy, availability, risk controls and schema budget. Disabled or opt-in tools are never elevated."
}

func (t *CatalogTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["search", "load", "unload", "list_loaded"], "description": "Control action. Defaults to search when omitted."},
    "tools": {"type": "array", "items": {"type": "string"}, "description": "Names for load/unload. Load also accepts the canonical policy selectors *, mcp/*, mcp/<server>/*, package/* and package/<package>/*; each wildcard is capped at 20 authorized matches."},
    "query": {"type": "string", "description": "Task-oriented text search over name, description, tags, category, class and package. Example: read files, search the web, or Jira issues."},
    "origin": {"type": "string", "description": "Optional origin filter: builtin, mcp_bridge, or mcp_native."},
    "category": {"type": "string", "description": "Optional category filter, for example filesystem, web, tasklist, or mcp:<server>."},
    "class": {"type": "string", "description": "Optional capability class, for example read_context, edit_files, web_lookup, task_management, mcp_tool."},
    "package": {"type": "string", "description": "Optional package filter, for example coding_readonly, coding_edit, web, tasks, or mcp:<server>."},
    "risk": {"type": "string", "description": "Optional risk filter: read, write, destructive, network, shell."},
    "availability_status": {"type": "string", "description": "Optional availability filter: available or unavailable."},
    "include_unavailable": {"type": "boolean", "description": "Whether unavailable tools should be included."},
    "limit": {"type": "integer", "description": "Maximum number of tools to return in this page. Defaults to 20 and cannot exceed 50.", "minimum": 1, "maximum": 50},
    "offset": {"type": "integer", "description": "Zero-based offset for pagination. When has_more is true, call again with next_offset to access the next page.", "minimum": 0}
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
	action := strings.TrimSpace(req.Action)
	if action == "" {
		action = catalogActionSearch
	}
	switch action {
	case catalogActionSearch:
		return t.executeSearch(ctx, req)
	case catalogActionLoad:
		return t.executeLoad(ctx, req)
	case catalogActionUnload:
		return t.executeUnload(ctx, req)
	case catalogActionListLoaded:
		return t.executeListLoaded(ctx)
	default:
		return ToolResult{Content: fmt.Sprintf("ação inválida para tool_catalog: %s", action), IsError: true}, nil
	}
}

func (t *CatalogTool) executeSearch(ctx context.Context, req catalogToolRequest) (ToolResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = defaultCatalogToolLimit
	} else if limit > maxCatalogToolPageSize {
		limit = maxCatalogToolPageSize
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	visibleNames := toolCatalogVisibleNames(ctx)
	if visibleNames != nil && len(visibleNames) == 0 {
		data, err := json.Marshal(catalogToolResponse{
			Tools:         []catalogToolItem{},
			SelectedTools: []string{},
			Count:         0,
			Limit:         limit,
			Offset:        offset,
			HasMore:       false,
		})
		if err != nil {
			return ToolResult{Content: fmt.Sprintf("erro ao serializar resposta do catálogo de tools: %v", err), IsError: true}, nil
		}
		return ToolResult{Content: string(data)}, nil
	}
	runtime, hasRuntime := ToolCatalogRuntimeFromContext(ctx)
	recentNames := []string(nil)
	preferredPackages := []string(nil)
	if hasRuntime {
		preferredPackages = runtime.PreferredPackages
		if runtime.Store != nil {
			recentNames = runtime.Store.RecentNames(runtime.ConversationID, runtime.ProfileSlug)
		}
	}
	rankedSearch := strings.TrimSpace(req.Query) != "" || len(preferredPackages) > 0 || len(recentNames) > 0
	storeLimit, storeOffset := limit+1, offset
	if rankedSearch {
		storeLimit, storeOffset = MaxCatalogSearchCandidates+1, 0
	}
	filter := ToolCatalogFilter{
		NameIn:             visibleNames,
		Origin:             strings.TrimSpace(req.Origin),
		Category:           strings.TrimSpace(req.Category),
		Class:              strings.TrimSpace(req.Class),
		Package:            strings.TrimSpace(req.Package),
		Risk:               strings.TrimSpace(req.Risk),
		AvailabilityStatus: strings.TrimSpace(req.AvailabilityStatus),
		IncludeUnavailable: req.IncludeUnavailable,
		Limit:              storeLimit,
		Offset:             storeOffset,
	}
	entries, err := t.store.ListTools(ctx, filter)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("erro ao consultar catálogo de tools: %v", err), IsError: true}, nil
	}
	entries = filterCatalogEntriesByVisibleNames(entries, filter.NameIn)
	if rankedSearch {
		entries = RankCatalogEntries(entries, CatalogDiscoveryOptions{
			Query:             req.Query,
			PreferredPackages: preferredPackages,
			RecentNames:       recentNames,
		})
		if offset >= len(entries) {
			entries = nil
		} else if offset > 0 {
			entries = entries[offset:]
		}
	}
	hasMore := len(entries) > limit
	if len(entries) > limit {
		entries = entries[:limit]
	}
	resp := catalogToolResponse{
		Tools:         make([]catalogToolItem, 0, len(entries)),
		SelectedTools: []string{},
		Count:         len(entries),
		Limit:         limit,
		Offset:        offset,
		HasMore:       hasMore,
	}
	if hasMore {
		resp.NextOffset = offset + len(entries)
	}
	for _, entry := range entries {
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

func (t *CatalogTool) executeLoad(ctx context.Context, req catalogToolRequest) (ToolResult, error) {
	runtime, ok := ToolCatalogRuntimeFromContext(ctx)
	if !ok || runtime.Store == nil {
		return ToolResult{Content: "estado runtime do catálogo de tools não configurado", IsError: true}, nil
	}
	requested := normalizeRequestedToolNames(req.Tools)
	if len(requested) == 0 {
		return marshalCatalogToolResponse(catalogToolResponse{LoadedTools: []string{}, SelectedTools: []string{}, RejectedTools: []catalogToolRejection{}})
	}
	requested, selectorRejected, err := t.expandLoadSelectors(ctx, requested, runtime)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("erro ao expandir seletores do catálogo de tools: %v", err), IsError: true}, nil
	}
	available, loadRejected, err := t.partitionAvailableTools(ctx, requested, runtime.VisibleNames)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("erro ao consultar catálogo de tools: %v", err), IsError: true}, nil
	}
	loaded, rejected := runtime.Store.Load(runtime.ConversationID, runtime.ProfileSlug, available, runtime.VisibleNames, runtime.PreloadedNames, runtime.ControlPlane)
	rejected = append(rejected, selectorRejected...)
	rejected = append(rejected, loadRejected...)
	resp := catalogToolResponse{
		SelectedTools: loadedToolChangeNames(loaded),
		LoadedTools:   loadedToolChangeNames(loaded),
		RejectedTools: catalogToolRejections(rejected),
		Count:         len(loaded),
	}
	return marshalCatalogToolResponse(resp)
}

func (t *CatalogTool) expandLoadSelectors(ctx context.Context, requested []string, runtime ToolCatalogRuntime) ([]string, []LoadedToolChange, error) {
	if runtime.MatchSelector == nil {
		return requested, nil, nil
	}
	needsCatalog := false
	for _, raw := range requested {
		_, wildcard := runtime.MatchSelector(raw, ToolCatalogEntry{Name: raw})
		if wildcard {
			needsCatalog = true
			break
		}
	}
	if !needsCatalog {
		return requested, nil, nil
	}
	entries, err := t.store.ListTools(ctx, ToolCatalogFilter{
		NameIn:             runtime.VisibleNames,
		AvailabilityStatus: ToolAvailabilityAvailable,
		Limit:              MaxCatalogSearchCandidates + 1,
	})
	if err != nil {
		return nil, nil, err
	}
	entries = filterCatalogEntriesByVisibleNames(entries, runtime.VisibleNames)
	entries = RankCatalogEntries(entries, CatalogDiscoveryOptions{
		PreferredPackages: runtime.PreferredPackages,
		RecentNames:       runtime.Store.RecentNames(runtime.ConversationID, runtime.ProfileSlug),
	})
	expanded := make([]string, 0, len(requested))
	var rejected []LoadedToolChange
	for _, raw := range requested {
		_, wildcard := runtime.MatchSelector(raw, ToolCatalogEntry{Name: raw})
		if !wildcard {
			expanded = append(expanded, raw)
			continue
		}
		matches := 0
		total := 0
		for _, entry := range entries {
			match, _ := runtime.MatchSelector(raw, entry)
			if !match {
				continue
			}
			total++
			if matches < MaxCatalogWildcardMatches {
				expanded = append(expanded, entry.Name)
				matches++
			}
		}
		switch {
		case total == 0:
			rejected = append(rejected, LoadedToolChange{Name: raw, Reason: LoadedToolRejectUnavailable})
		case total > MaxCatalogWildcardMatches:
			rejected = append(rejected, LoadedToolChange{Name: raw, Reason: loadedToolRejectWildcardLimit})
		}
	}
	return normalizeRequestedToolNames(expanded), rejected, nil
}

func (t *CatalogTool) executeUnload(ctx context.Context, req catalogToolRequest) (ToolResult, error) {
	runtime, ok := ToolCatalogRuntimeFromContext(ctx)
	if !ok || runtime.Store == nil {
		return ToolResult{Content: "estado runtime do catálogo de tools não configurado", IsError: true}, nil
	}
	unloaded, rejected := runtime.Store.Unload(runtime.ConversationID, runtime.ProfileSlug, normalizeRequestedToolNames(req.Tools), runtime.PreloadedNames, runtime.ControlPlane)
	resp := catalogToolResponse{
		UnloadedTools: loadedToolChangeNames(unloaded),
		RejectedTools: catalogToolRejections(rejected),
		Count:         len(unloaded),
	}
	return marshalCatalogToolResponse(resp)
}

func (t *CatalogTool) executeListLoaded(ctx context.Context) (ToolResult, error) {
	runtime, ok := ToolCatalogRuntimeFromContext(ctx)
	if !ok || runtime.Store == nil {
		return ToolResult{Content: "estado runtime do catálogo de tools não configurado", IsError: true}, nil
	}
	loaded := runtime.Store.List(runtime.ConversationID, runtime.ProfileSlug, runtime.PreloadedNames, runtime.ControlPlane, runtime.VisibleNames)
	resp := catalogToolResponse{
		Loaded: loaded,
		Count:  len(loaded),
	}
	return marshalCatalogToolResponse(resp)
}

func (t *CatalogTool) partitionAvailableTools(ctx context.Context, requested, visible []string) (available []string, rejected []LoadedToolChange, err error) {
	visibleSet, constrained := nameSet(visible)
	checkNames := requested
	if constrained {
		checkNames = make([]string, 0, len(requested))
		for _, name := range requested {
			if _, ok := visibleSet[name]; !ok {
				rejected = append(rejected, LoadedToolChange{Name: name, Reason: LoadedToolRejectDisabled})
				continue
			}
			checkNames = append(checkNames, name)
		}
	}
	if len(checkNames) == 0 {
		return nil, rejected, nil
	}
	entries, err := t.store.ListTools(ctx, ToolCatalogFilter{
		NameIn:             checkNames,
		AvailabilityStatus: ToolAvailabilityAvailable,
		IncludeUnavailable: false,
		Limit:              len(checkNames),
	})
	if err != nil {
		return nil, nil, err
	}
	found := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		found[entry.Name] = struct{}{}
	}
	for _, name := range checkNames {
		if _, ok := found[name]; ok {
			available = append(available, name)
			continue
		}
		rejected = append(rejected, LoadedToolChange{Name: name, Reason: LoadedToolRejectUnavailable})
	}
	return available, rejected, nil
}

func toolCatalogVisibleNames(ctx context.Context) []string {
	names, ok := ToolCatalogVisibleNamesFromContext(ctx)
	if !ok {
		return nil
	}
	return names
}

func filterCatalogEntriesByVisibleNames(entries []ToolCatalogEntry, visible []string) []ToolCatalogEntry {
	if visible == nil {
		return entries
	}
	if len(visible) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(visible))
	for _, name := range visible {
		name = strings.TrimSpace(name)
		if name != "" {
			allowed[name] = struct{}{}
		}
	}
	filtered := make([]ToolCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if _, ok := allowed[entry.Name]; ok {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func normalizeRequestedToolNames(names []string) []string {
	normalized := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	return normalized
}

func loadedToolChangeNames(changes []LoadedToolChange) []string {
	names := make([]string, 0, len(changes))
	for _, change := range changes {
		names = append(names, change.Name)
	}
	return names
}

func catalogToolRejections(changes []LoadedToolChange) []catalogToolRejection {
	rejections := make([]catalogToolRejection, 0, len(changes))
	for _, change := range changes {
		rejections = append(rejections, catalogToolRejection(change))
	}
	return rejections
}

func marshalCatalogToolResponse(resp catalogToolResponse) (ToolResult, error) {
	if resp.Tools == nil {
		resp.Tools = []catalogToolItem{}
	}
	if resp.SelectedTools == nil {
		resp.SelectedTools = []string{}
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("erro ao serializar resposta do catálogo de tools: %v", err), IsError: true}, nil
	}
	return ToolResult{Content: string(data)}, nil
}
