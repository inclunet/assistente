package chat

import (
	"context"
	"sync"
)

// projectionMutex mantém uma única exclusão para writers e consultas canceláveis.
// O valor zero funciona; não copiar depois do primeiro uso.
type projectionMutex struct {
	once sync.Once
	gate chan struct{}
}

func (m *projectionMutex) channel() chan struct{} {
	m.once.Do(func() { m.gate = make(chan struct{}, 1) })
	return m.gate
}

func (m *projectionMutex) Lock() { m.channel() <- struct{}{} }

func (m *projectionMutex) LockContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case m.channel() <- struct{}{}:
		if err := ctx.Err(); err != nil {
			m.Unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *projectionMutex) Unlock() {
	select {
	case <-m.channel():
	default:
		panic("unlock de barreira de projeção livre")
	}
}
