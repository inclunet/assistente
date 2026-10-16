package commandexecution

import (
	"context"
	"errors"
	"sync"
)

var ErrServiceClosed = errors.New("executor de comandos encerrado")

// O registro cobre a operação inteira, inclusive preparação, reserva e
// finalização. Os watches de epoch cobrem apenas suas respectivas fases e
// portanto não podem ser usados como prova de drenagem do Service.
type executionLifecycle struct {
	mu      sync.Mutex
	closed  bool
	active  map[*executionOperation]struct{}
	drained chan struct{}
}

type executionOperation struct{ cancel context.CancelFunc }

// handoff compartilha a fronteira de fechamento com Shutdown. Somente Start,
// cujo contrato exige retorno imediato, roda aqui; nunca CAS, UI ou espera.
func (l *executionLifecycle) handoff(ctx context.Context, start func() error) error {
	if l == nil {
		return ErrInvalidRequest
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return ErrServiceClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return start()
}

func (l *executionLifecycle) enter(ctx context.Context) (context.Context, func(), error) {
	if l == nil {
		return nil, nil, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, nil, ErrServiceClosed
	}
	runCtx, cancel := context.WithCancel(ctx)
	op := &executionOperation{cancel: cancel}
	if l.active == nil {
		l.active = make(map[*executionOperation]struct{})
	}
	l.active[op] = struct{}{}
	var once sync.Once
	return runCtx, func() {
		once.Do(func() {
			cancel()
			l.mu.Lock()
			defer l.mu.Unlock()
			delete(l.active, op)
			if l.closed && len(l.active) == 0 {
				close(l.drained)
			}
		})
	}, nil
}

// Shutdown fecha a admissão antes de cancelar operações, e espera seu retorno
// (incluindo CAS finais) fora do DispatchGate. Timeout não reabre a instância:
// outra chamada pode continuar aguardando. Não chamar de dentro de um handler
// ou callback do próprio Service. O host deve encerrar todos os Services antes
// de recuperar uma geração compartilhada; este retorno não é prova de que um
// efeito externo não cooperativo terminou nem capability de recovery do banco.
func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil || ctx == nil || s.lifecycle == nil {
		return ErrInvalidRequest
	}
	return s.lifecycle.shutdown(ctx)
}

func (l *executionLifecycle) shutdown(ctx context.Context) error {
	if l == nil || ctx == nil {
		return ErrInvalidRequest
	}
	l.mu.Lock()
	if !l.closed {
		l.closed = true
		l.drained = make(chan struct{})
		for op := range l.active {
			op.cancel()
		}
		if len(l.active) == 0 {
			close(l.drained)
		}
	}
	done := l.drained
	l.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
