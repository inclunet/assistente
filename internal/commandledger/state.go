package commandledger

// validTransition não permite concluir antes do handoff nem repetir terminais.
// Reconciliação de outcome_unknown depende de um contrato futuro separado.
func validTransition(from, to Status) bool {
	switch from {
	case Evaluating:
		return to == Queued || to == Denied || to == Failed || to == Cancelled || to == CancelledStale || to == TimedOut
	case Queued:
		return to == Running || to == Cancelled || to == CancelledStale || to == TimedOut
	case Running:
		return to == Succeeded || to == Failed || to == Cancelled || to == OutcomeUnknown
	}
	return false
}

func terminal(status Status) bool {
	switch status {
	case Succeeded, Failed, Denied, Cancelled, CancelledStale, TimedOut, OutcomeUnknown:
		return true
	}
	return false
}
