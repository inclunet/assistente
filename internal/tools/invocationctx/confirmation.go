package invocationctx

import "sync/atomic"

// ConfirmationReceipt arbitra a entrega da resposta e o encerramento da espera.
// O valor zero representa uma pergunta aberta. O primeiro evento é definitivo.
type ConfirmationReceipt struct {
	state atomic.Uint32
}

const (
	confirmationOpen uint32 = iota
	confirmationAnswered
	confirmationRefused
	confirmationClosed
)

func (r *ConfirmationReceipt) TryAccept(refused bool) bool {
	next := confirmationAnswered
	if refused {
		next = confirmationRefused
	}
	return r.state.CompareAndSwap(confirmationOpen, next)
}

// Close encerra a espera e informa se uma negativa já havia sido aceita.
func (r *ConfirmationReceipt) Close() bool {
	r.state.CompareAndSwap(confirmationOpen, confirmationClosed)
	return r.state.Load() == confirmationRefused
}
