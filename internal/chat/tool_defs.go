package chat

import (
	"context"
	"reflect"

	"assistente/internal/llm"
	mcplib "assistente/internal/mcp"
	"assistente/internal/tools"
)

// Este arquivo expõe a API histórica de seleção de tools como WRAPPERS FINOS
// sobre ToolSelectionPolicy (AEP-0077 Fase 3, #119). A lógica de seleção vive
// num lugar só — tool_selection_policy.go. Estas funções permanecem para
// compatibilidade com call sites e testes existentes.

// ChatProviderIsNil reports whether c is nil or holds a nil concrete pointer (typed nil).
// Calling methods on a typed-nil ChatProvider panics (e.g. (*OpenAIProvider)(nil).NativeMCPCapable()).
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
	return NewToolSelectionPolicy(registry).buildLLMToolDefs(enabledTools, disableTools)
}

// ResolveInitialEnabledTools resolve a seleção inicial de tools do perfil.
func ResolveInitialEnabledTools(registry *tools.Registry, enabledTools []string, disableTools bool) []string {
	return NewToolSelectionPolicy(registry).resolveInitialEnabledTools(enabledTools, disableTools)
}

// ResolveInitialEnabledToolsWithRuntime resolve a seleção inicial somando runtime tools.
func ResolveInitialEnabledToolsWithRuntime(registry *tools.Registry, enabledTools []string, disableTools bool, runtimeTools []string) []string {
	return NewToolSelectionPolicy(registry).resolveInitialEnabledToolsWithRuntime(enabledTools, disableTools, runtimeTools)
}

// BuildLLMToolDefsByNames monta tool definitions a partir de uma lista de nomes.
func BuildLLMToolDefsByNames(registry *tools.Registry, names []string, disableTools bool) []llm.ToolDefinition {
	return NewToolSelectionPolicy(registry).buildLLMToolDefsByNames(names, disableTools)
}

// FilterToolNamesByEnabledTools restringe nomes ao allowlist do perfil.
func FilterToolNamesByEnabledTools(names []string, enabledTools []string, disableTools bool) []string {
	return filterToolNamesByEnabledTools(names, enabledTools, disableTools)
}

// ResolveNativeMCPEnabled resolve a política tri-state de MCP nativo (AEP-0021).
func ResolveNativeMCPEnabled(streamer llm.ChatProvider, override *bool) bool {
	return resolveNativeMCPEnabled(streamer, override)
}

// FilterToolNamesForNativeMCP remove nomes de bridge tools atendidas via MCP nativo.
func FilterToolNamesForNativeMCP(streamer llm.ChatProvider, mcpMgr NativeMCPManager, names []string, disableTools bool, nativeMCPOverride *bool) []string {
	return filterToolNamesForNativeMCP(streamer, mcpMgr, names, disableTools, nativeMCPOverride)
}

// ApplyNativeMCP configura servidores MCP HTTP nativos no ChatProvider e remove
// as bridge tools correspondentes do toolDefs para evitar duplicatas.
func ApplyNativeMCP(
	streamer llm.ChatProvider,
	toolDefs []llm.ToolDefinition,
	mcpMgr NativeMCPManager,
	enabledTools []string,
	disableTools bool,
	nativeMCPOverride *bool,
) (llm.ChatProvider, []llm.ToolDefinition) {
	return applyNativeMCP(streamer, toolDefs, mcpMgr, enabledTools, disableTools, nativeMCPOverride)
}
