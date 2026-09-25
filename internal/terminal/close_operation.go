package terminal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

const (
	closeOperationPrepared uint32 = iota
	closeOperationExecuting
	closeOperationDone
	closeOperationCanceled
)

// CloseOperation é o handoff curto entre a admissão protegida pelos owners
// da aplicação e o encerramento do PTY. PrepareClose reserva ioMu; Execute
// revalida a geração e somente então inicia o efeito destrutivo.
type CloseOperation struct {
	manager   *Manager
	session   *Session
	sessionID string
	token     *closeSnapshotToken

	state       atomic.Uint32
	releaseOnce sync.Once
}

func (op *CloseOperation) release(state uint32, unlock bool) {
	if op == nil {
		return
	}
	op.releaseOnce.Do(func() {
		op.state.Store(state)
		if unlock && op.session != nil {
			op.session.ioMu.Unlock()
		}
	})
}

// Execute verifica o contexto antes do primeiro efeito. Depois que o estado
// da sessão passa a closing, o cleanup continua mesmo que o caller cancele.
func (op *CloseOperation) Execute(ctx context.Context) error {
	if op == nil || op.session == nil || op.token == nil {
		return ErrCloseStale
	}
	if !op.state.CompareAndSwap(closeOperationPrepared, closeOperationExecuting) {
		return ErrCloseStale
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		op.release(closeOperationCanceled, true)
		return errors.Join(ErrCloseStale, ctx.Err())
	default:
	}

	// closeCapturedSession libera ioMu antes de esperar o processo e o leitor.
	// O manager só remove a identidade depois da segunda validação, evitando
	// apagar uma sessão diferente em caso de reuso do ID em outro runtime.
	err := op.session.closeCapturedSession(op.token)
	if err != nil {
		op.release(closeOperationDone, false)
		if !errors.Is(err, ErrCloseStale) && op.manager != nil {
			op.manager.removeIfSame(op.sessionID, op.session)
		}
		return err
	}
	if op.manager != nil {
		op.manager.removeIfSame(op.sessionID, op.session)
	}
	op.release(closeOperationDone, false)
	return nil
}

// Cancel abandona uma operação preparada antes de Execute e libera a sessão.
func (op *CloseOperation) Cancel() {
	if op == nil || !op.state.CompareAndSwap(closeOperationPrepared, closeOperationCanceled) {
		return
	}
	op.release(closeOperationCanceled, true)
}
