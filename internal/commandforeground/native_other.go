//go:build !windows

package commandforeground

import "context"

type nativeReader struct{}

// NewNative retorna o adapter nativo da plataforma. Em plataformas sem
// implementação de foreground, Capture falha explicitamente.
func NewNative() Reader {
	return nativeReader{}
}

func (nativeReader) Capture(context.Context) (Snapshot, error) {
	return Snapshot{}, ErrUnavailable
}
