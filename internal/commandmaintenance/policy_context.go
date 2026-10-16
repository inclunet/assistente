package commandmaintenance

import "context"

type policyContextKey struct{}

// PolicyFromContext retorna uma cópia da política validada da passagem atual.
// Somente Coordinator.Run publica esse valor. Domínios que não recebem Policy
// em sua assinatura podem consumir o mesmo snapshot sem reler configuração ou
// alterar estado compartilhado. Não é identidade nem capacidade de autorização.
func PolicyFromContext(ctx context.Context) (Policy, bool) {
	if ctx == nil {
		return Policy{}, false
	}
	policy, ok := ctx.Value(policyContextKey{}).(Policy)
	return policy, ok
}
