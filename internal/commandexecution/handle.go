package commandexecution

import (
	"context"
	"strings"

	"assistente/internal/commandledger"
)

// awaitOutcome aguarda somente o resultado explícito do handoff. Ausência,
// perda ou invalidação do resultado não prova que nenhum efeito ocorreu.
func awaitOutcome(ctx context.Context, handle ExecutionHandle) commandledger.Status {
	if strings.TrimSpace(handle.ID) == "" || handle.Done == nil || handle.Cancel == nil {
		safeCancel(handle.Cancel)
		return commandledger.OutcomeUnknown
	}
	if ctx == nil || ctx.Err() != nil {
		safeCancel(handle.Cancel)
		return commandledger.OutcomeUnknown
	}

	select {
	case <-ctx.Done():
		safeCancel(handle.Cancel)
		return commandledger.OutcomeUnknown
	case outcome, ok := <-handle.Done:
		if !ok {
			safeCancel(handle.Cancel)
			return commandledger.OutcomeUnknown
		}
		switch outcome.Status {
		case commandledger.Succeeded, commandledger.Failed, commandledger.Cancelled:
			return outcome.Status
		default:
			safeCancel(handle.Cancel)
			return commandledger.OutcomeUnknown
		}
	}
}

// safeCancel preserva o limite do await mesmo quando um adapter malformado
// entra em panic. O contrato de Cancel exige que a chamada não bloqueie.
func safeCancel(cancel func()) {
	if cancel == nil {
		return
	}
	defer func() { _ = recover() }()
	cancel()
}
