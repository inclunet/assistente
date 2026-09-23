package commandjobactivation

import (
	"context"
	"time"

	"assistente/internal/commandmaintenance"
)

// O coordenador fornece um snapshot imutável por passagem. Chamadas diretas
// fora da manutenção usam os valores validados na construção do Consumer.
// Não altera TTLs compartilhados nem recalcula deadlines de fatos persistidos.
func (c *Consumer) durations(ctx context.Context) (lease, retention time.Duration) {
	if policy, ok := commandmaintenance.PolicyFromContext(ctx); ok {
		return policy.LeaseDuration, policy.ActivationRetention
	}
	return c.lease, c.retention
}
