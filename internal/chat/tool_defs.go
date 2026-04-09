package chat

import (
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

// NativeMCPManager abstrai a consulta de servidores MCP elegíveis para passthrough nativo.
// Implementado por *mcp.Manager; pode ser mockado em testes.
type NativeMCPManager interface {
	GetEligibleNativeMCPServers() []mcplib.NativeMCPServer
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

// ApplyNativeMCP configura servidores MCP HTTP nativos no ChatProvider e remove
// as bridge tools correspondentes do toolDefs para evitar duplicatas.
func ApplyNativeMCP(
	streamer llm.ChatProvider,
	toolDefs []llm.ToolDefinition,
	mcpMgr NativeMCPManager,
	enabledTools []string,
	disableTools bool,
) (llm.ChatProvider, []llm.ToolDefinition) {
	if disableTools || mcpMgr == nil || ChatProviderIsNil(streamer) {
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
			Name:      srv.Name,
			URL:       srv.URL,
			AuthToken: srv.AuthToken,
			ToolNames: srv.ToolNames,
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
