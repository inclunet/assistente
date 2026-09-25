package commandbindings

import (
	"reflect"
	"time"
)

// Equivalent compara somente o estado imutável da configuração. A instância
// é publicada como snapshot e nunca deve ser mutada depois da construção;
// por isso a comparação pode ocorrer fora de qualquer gate.
func (c *Configuration) Equivalent(other *Configuration) bool {
	return reflect.DeepEqual(c, other)
}

// EquivalentExceptValidityDeadline compares a refreshed projection while
// ignoring only the deadline derived from a renewable external lease. It is
// deliberately narrower than a general semantic equivalence: all bindings,
// provenance, persisted baseline, and other configuration state still match.
func (c *Configuration) EquivalentExceptValidityDeadline(other *Configuration) bool {
	if c == nil || other == nil {
		return c == other
	}
	left, right := *c, *other
	left.validUntil = time.Time{}
	right.validUntil = time.Time{}
	return reflect.DeepEqual(&left, &right)
}
