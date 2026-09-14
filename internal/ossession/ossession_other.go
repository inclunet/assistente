//go:build !windows

package ossession

import "context"

func watchPlatform(_ context.Context, observe func(State) error) error {
	// Não há inferência por API alternativa: plataformas sem o adapter ficam
	// explicitamente desconhecidas e fechadas.
	_ = observe
	return errUnsupported
}
