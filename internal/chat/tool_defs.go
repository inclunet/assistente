package chat

import (
	"context"
	"log"
	"reflect"

	"assistente/internal/llm"
	mcplib "assistente/internal/mcp"
	"assistente/internal/tools"
)

// ChatProviderIsNil reports whether c is nil or holds a nil concrete pointer (typed nil).
// Calling methods on a typed-nil ChatProvider panics (e.g. (*OpenAIProvider)(nil).SupportsNativeMCP()).
func ChatProviderIsNil(c llm.ChatProvider) bool {
	if c == nil {
		return true
	}
	v := reflect.ValueOf(c)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return false
	}
}

// NativeMCPManagerIsNil reports whether m is nil or holds a nil concrete pointer.
func NativeMCPManagerIsNil(m NativeMCPManager) bool {
	if m == nil {
		return true
	}
	v := reflect.ValueOf(m)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return false
	}
}

// NativeMCPManager abstrai a consulta de servidores MCP elegíveis para passthrough nativo.
// Implementado por *mcp.Manager; pode ser mockado em testes.
type NativeMCPManager interface {
	GetEligibleNativeMCPServers() []mcplib.NativeMCPServer
	RecoverServerBestEffort(ctx context.Context, slug string) mcplib.RecoveryResult
}

// BuildLLMToolDefs constrói a lista de tool definitions para o LLM.
// Se disableTools for true, retorna nil. Se enabledTools for nil, inclui todas.
func BuildLLMToolDefs(registry *tools.Registry, enabledTools []string, disableTools bool) []llm.ToolDefinition {
	if disableTools || registry == nil || registry.Count() == 0 {
		return nil
	}

	var toolDefs []tools.ToolDefinition
	if enabledTools != nil {
		toolDefs = registry.FilterByNames(enabledTools)
	} else {
		toolDefs = registry.ToDefinitions()
	}

	result := make([]llm.ToolDefinition, len(toolDefs))
	for i, td := range toolDefs {
		result[i] = llm.ToolDefinition{
			Type: td.Type,
			Function: llm.FunctionDefinition{
				Name:        td.Function.Name,
				Description: td.Function.Description,
				Parameters:  td.Function.Parameters,
			},
		}
	}
	return result
}

func ResolveInitialEnabledTools(registry *tools.Registry, enabledTools []string, disableTools bool) []string {
	if disableTools || enabledTools != nil || registry == nil {
		return enabledTools
	}
	if registry.Has(tools.ToolCatalogName) {
		return []string{tools.ToolCatalogName}
	}
	return nil
}

func BuildLLMToolDefsByNames(registry *tools.Registry, names []string, disableTools bool) []llm.ToolDefinition {
	if disableTools || registry == nil || len(names) == 0 {
		return nil
	}
	toolDefs := registry.FilterByNames(names)
	result := make([]llm.ToolDefinition, len(toolDefs))
	for i, td := range toolDefs {
		result[i] = llm.ToolDefinition{
			Type: td.Type,
			Function: llm.FunctionDefinition{
				Name:        td.Function.Name,
				Description: td.Function.Description,
				Parameters:  td.Function.Parameters,
			},
		}
	}
	return result
}

func FilterToolNamesForNativeMCP(streamer llm.ChatProvider, mcpMgr NativeMCPManager, names []string, disableTools bool) []string {
	if disableTools || len(names) == 0 || NativeMCPManagerIsNil(mcpMgr) || ChatProviderIsNil(streamer) {
		return names
	}
	if !streamer.SupportsNativeMCP() {
		return names
	}
	nativeServers := mcpMgr.GetEligibleNativeMCPServers()
	if len(nativeServers) == 0 {
		return names
	}
	nativeToolNames := make(map[string]struct{})
	for _, srv := range nativeServers {
		for _, name := range srv.ToolNames {
			nativeToolNames[name] = struct{}{}
		}
	}
	if len(nativeToolNames) == 0 {
		return names
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if _, native := nativeToolNames[name]; native {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

// ApplyNativeMCP configura servidores MCP HTTP nativos no ChatProvider e remove
// as bridge tools correspondentes do toolDefs para evitar duplicatas.
func ApplyNativeMCP(
	streamer llm.ChatProvider,
	toolDefs []llm.ToolDefinition,
	mcpMgr NativeMCPManager,
	enabledTools []string,
	disableTools bool,
) (llm.ChatProvider, []llm.ToolDefinition) {
	if disableTools || NativeMCPManagerIsNil(mcpMgr) || ChatProviderIsNil(streamer) {
		return streamer, toolDefs
	}
	if !streamer.SupportsNativeMCP() {
		return streamer, toolDefs
	}

	nativeServers := mcpMgr.GetEligibleNativeMCPServers()
	if len(nativeServers) == 0 {
		return streamer, toolDefs
	}

	var enabledSet map[string]bool
	if enabledTools != nil {
		enabledSet = make(map[string]bool, len(enabledTools))
		for _, n := range enabledTools {
			enabledSet[n] = true
		}
	}

	var mcpConfigs []llm.MCPServerConfig
	nativeToolNames := make(map[string]bool)

	for _, srv := range nativeServers {
		cfg := llm.MCPServerConfig{
			Slug:      srv.Slug,
			Name:      srv.Name,
			URL:       srv.URL,
			AuthToken: srv.AuthToken,
			ToolNames: srv.ToolNames,
			Recover: func(slug string) func(context.Context) error {
				return func(ctx context.Context) error {
					return mcpMgr.RecoverServerBestEffort(ctx, slug).Err
				}
			}(srv.Slug),
		}

		if enabledSet != nil {
			var allowed []string
			var allowedFull []string
			for _, fullName := range srv.ToolNames {
				if enabledSet[fullName] {
					if _, originalName, ok := mcplib.ParseToolName(fullName); ok {
						allowed = append(allowed, originalName)
					}
					allowedFull = append(allowedFull, fullName)
				}
			}
			if len(allowed) == 0 {
				log.Printf("[chat] MCP nativo: servidor %q excluído (nenhuma tool habilitada no perfil)", srv.Name)
				continue
			}
			cfg.AllowedTools = allowed
			cfg.ToolNames = allowedFull
		}

		mcpConfigs = append(mcpConfigs, cfg)
		for _, tn := range cfg.ToolNames {
			nativeToolNames[tn] = true
		}
	}

	if len(mcpConfigs) > 0 {
		streamer = streamer.WithMCPServers(mcpConfigs)
		log.Printf("[chat] MCP nativo: %d servidores HTTP configurados", len(mcpConfigs))
	}

	if len(nativeToolNames) > 0 {
		filtered := make([]llm.ToolDefinition, 0, len(toolDefs))
		for _, td := range toolDefs {
			if !nativeToolNames[td.Function.Name] {
				filtered = append(filtered, td)
			}
		}
		removed := len(toolDefs) - len(filtered)
		if removed > 0 {
			log.Printf("[chat] MCP nativo: %d bridge tools removidas (nativas agora)", removed)
		}
		toolDefs = filtered
	}

	return streamer, toolDefs
}
