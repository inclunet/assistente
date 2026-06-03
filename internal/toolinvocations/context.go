package toolinvocations

import "context"

// Encadeamento de invocações pai↔filho via context (AEP-0068 + AEP-0063).
//
// O agentic loop de uma sub-conversa precisa que as tool_invocations geradas
// dentro dela apontem (ParentInvocationID) para a invocação da tool `subagent`
// que a originou na conversa-pai. Como tools não recebem o ID da própria
// invocação por parâmetro, propagamos esse vínculo pelo context:
//
//   - currentInvocationIDKey: o ID da invocação que está executando AGORA.
//     O Service carimba antes de chamar a tool; a tool `subagent` lê esse valor
//     e o injeta como ParentInvocationID no ctx da sub-conversa.
//   - parentInvocationIDKey: o ID da invocação-pai a ser herdado pelas próximas
//     invocações. Quando um ExecuteRequest não traz ParentInvocationID
//     explícito, o Service cai para este valor do ctx.
type currentInvocationIDKey struct{}

type parentInvocationIDKey struct{}

// WithCurrentInvocationID retorna um ctx carregando o ID da invocação corrente.
func WithCurrentInvocationID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, currentInvocationIDKey{}, id)
}

// CurrentInvocationID recupera o ID da invocação corrente carimbado no ctx.
func CurrentInvocationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(currentInvocationIDKey{}).(string)
	return id
}

// WithParentInvocationID retorna um ctx carregando o ID da invocação-pai que as
// próximas invocações devem herdar.
func WithParentInvocationID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, parentInvocationIDKey{}, id)
}

// ParentInvocationIDFromContext recupera o ID da invocação-pai carimbado no ctx.
func ParentInvocationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(parentInvocationIDKey{}).(string)
	return id
}
