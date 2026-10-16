package commandcatalog

import (
	"errors"
	"fmt"
)

// ErrNotReady indica que a definição não passa no preflight estático atual.
//
// Este gate é temporário e deliberadamente restrito a comandos read-only. Ele
// não concede autenticação ou autorização, não consulta disponibilidade de
// runtime e não executa o comando; essas decisões pertencem ao fluxo posterior.
var ErrNotReady = errors.New("comando não está pronto")

// CheckReadiness faz o preflight estático inicial para o fluxo de preparação.
// A origem precisa ser explicitamente permitida pela definição canônica. O
// retorno é detached do snapshot do registry e pode ser alterado pelo chamador.
func (r *Registry) CheckReadiness(id string, source Source) (Definition, error) {
	if r == nil {
		return Definition{}, fmt.Errorf("%w: registry ausente", ErrNotReady)
	}

	d, ok := r.definitions[id]
	if !ok {
		return Definition{}, fmt.Errorf("%w: comando %q não encontrado por ID exato", ErrNotReady, id)
	}
	if !d.AllowsSource(source) {
		return Definition{}, fmt.Errorf("%w: origem %q não permitida para %q", ErrNotReady, source, id)
	}
	if d.Presentation == nil {
		return Definition{}, fmt.Errorf("%w: apresentação ausente para %q", ErrNotReady, id)
	}
	if d.Effect != Read {
		return Definition{}, fmt.Errorf("%w: efeito %q não é read", ErrNotReady, d.Effect)
	}
	if d.Decision != NoDecision {
		return Definition{}, fmt.Errorf("%w: decisão interativa não é aceita", ErrNotReady)
	}
	if d.HasMutableTarget {
		return Definition{}, fmt.Errorf("%w: alvo mutável não é aceito", ErrNotReady)
	}
	if d.MutatesEffectiveCapability {
		return Definition{}, fmt.Errorf("%w: mutação de capability efetiva não é aceita", ErrNotReady)
	}

	return clone(d), nil
}
