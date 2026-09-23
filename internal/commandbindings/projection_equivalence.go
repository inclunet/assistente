package commandbindings

import "reflect"

// Equivalent compara somente o estado imutável da configuração. A instância
// é publicada como snapshot e nunca deve ser mutada depois da construção;
// por isso a comparação pode ocorrer fora de qualquer gate.
func (c *Configuration) Equivalent(other *Configuration) bool {
	return reflect.DeepEqual(c, other)
}
