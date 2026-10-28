package chat

import (
	"context"
	"reflect"

	"assistente/internal/llm"
	mcplib "assistente/internal/mcp"
)

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
	GetEligibleNativeMCPServers(context.Context, []string) []mcplib.NativeMCPServer
}
