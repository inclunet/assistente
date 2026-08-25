package invocationctx

import "context"

// InvocationContext carrega informações sobre o contexto de onde a tool foi invocada.
// É propagado via context.Context pelo agentic loop.
type InvocationContext struct {
	TabType        string // "editor", "chat", etc.
	ActiveFilePath string // caminho absoluto do arquivo ativo (apenas para editor tabs)
	SurfaceState   map[string]any
	SurfaceContext map[string]any

	// Identidade da conversa/turno corrente (AEP-0068). Permite que a tool
	// `subagent` saiba qual é a conversa-pai e o turno que a invocou, e qual
	// profile o pai está usando (para herança de profile por padrão).
	ConversationID string
	TurnID         string
	ProfileSlug    string
	Source         string
	SurfaceTabID   string
}

type ctxKey struct{}

// With retorna um novo context.Context com o InvocationContext embutido.
func With(ctx context.Context, inv InvocationContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, inv)
}

// Get extrai o InvocationContext do context.Context.
// Retorna false se não houver contexto de invocação.
func Get(ctx context.Context) (InvocationContext, bool) {
	v, ok := ctx.Value(ctxKey{}).(InvocationContext)
	return v, ok
}
