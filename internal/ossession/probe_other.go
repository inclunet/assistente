//go:build !windows

package ossession

import "context"

// Probe não infere o estado da sessão em plataformas sem consulta autoritativa.
func Probe(ctx context.Context) (State, error) {
	if ctx == nil {
		return unknownState(), errNilContext
	}
	if err := ctx.Err(); err != nil {
		return unknownState(), err
	}
	return unknownState(), errUnsupported
}
