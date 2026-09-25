package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

const (
	interruptOperationPrepared uint32 = iota
	interruptOperationExecuting
	interruptOperationDone
	interruptOperationCanceled
)

// InterruptOperation é o handoff curto entre a admissão protegida pelos
// owners da aplicação e o efeito no PTY. PrepareInterrupt já adquiriu o ioMu
// da sessão; Execute não deve tentar adquiri-lo novamente.
//
// A operação não é serializável nem deve escapar do runtime do backend.
type InterruptOperation struct {
	session   *Session
	sessionID string
	writer    io.Writer

	state       atomic.Uint32
	releaseOnce sync.Once
}

func (op *InterruptOperation) release(state uint32) {
	if op == nil {
		return
	}
	op.releaseOnce.Do(func() {
		op.state.Store(state)
		if op.session != nil {
			op.session.ioMu.Unlock()
		}
	})
}

// Execute verifica o contexto antes do primeiro byte e escreve Ctrl+C de
// modo síncrono. Cancelamento posterior ao início do Write não transforma um
// erro do PTY em stale: nesse ponto o efeito já pode ter começado.
func (op *InterruptOperation) Execute(ctx context.Context) error {
	if op == nil || op.session == nil {
		return ErrInterruptStale
	}
	if !op.state.CompareAndSwap(interruptOperationPrepared, interruptOperationExecuting) {
		return ErrInterruptStale
	}
	defer op.release(interruptOperationDone)

	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return errors.Join(ErrInterruptStale, ctx.Err())
	default:
	}

	return writeInterruptByte(op.writer, op.sessionID)
}

// Cancel abandona uma operação preparada antes de Execute e libera a reserva
// da sessão. É seguro chamar em defer depois de Execute.
func (op *InterruptOperation) Cancel() {
	if op == nil || op.session == nil || !op.state.CompareAndSwap(interruptOperationPrepared, interruptOperationCanceled) {
		return
	}
	op.release(interruptOperationCanceled)
}

func writeInterruptByte(writer io.Writer, sessionID string) error {
	if writer == nil {
		return fmt.Errorf("sessão %s não possui writer PTY", sessionID)
	}
	n, err := writer.Write([]byte{0x03})
	if err != nil {
		return fmt.Errorf("falha ao enviar Ctrl+C para sessão %s: %w", sessionID, err)
	}
	if n != 1 {
		return fmt.Errorf("falha ao enviar Ctrl+C para sessão %s: %w", sessionID, io.ErrShortWrite)
	}
	return nil
}
