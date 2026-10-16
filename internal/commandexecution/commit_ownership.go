package commandexecution

import (
	"context"
	"errors"
	"sync"
	"time"
)

const maxCommitOwnershipTimeout = 35 * time.Second

var (
	ErrInvalidCommitOwnership = errors.New("ownership de commit inválido")
	ErrCommitOwnershipClaimed = errors.New("ownership de commit já reivindicado")
	ErrCommitOwnershipAborted = errors.New("ownership de commit abortado")
)

type commitOwnershipState uint8

const (
	commitOwnershipPending commitOwnershipState = iota + 1
	commitOwnershipClaimed
	commitOwnershipAborted
)

// CommitOwnership é uma reserva efêmera para uma operação de capability que
// pode invalidar o próprio contexto ao começar uma transição. A reserva nasce
// pending; somente o host confiável pode reivindicá-la. O prazo começa no
// instante da reivindicação e não depende do contexto usado para a claim.
type CommitOwnership struct {
	mu       sync.Mutex
	state    commitOwnershipState
	timeout  time.Duration
	deadline time.Time
}

func NewCommitOwnership(timeout time.Duration) (*CommitOwnership, error) {
	if timeout <= 0 || timeout > maxCommitOwnershipTimeout {
		return nil, ErrInvalidCommitOwnership
	}
	return &CommitOwnership{state: commitOwnershipPending, timeout: timeout}, nil
}

// Claim fecha a janela de corrida entre a observação do cancelamento e a
// transição que o causou. Um contexto cancelado nunca pode escrever claimed
// depois que o executor já decidiu abortar a reserva.
func (o *CommitOwnership) Claim(ctx context.Context) error {
	if o == nil || ctx == nil {
		return ErrInvalidCommitOwnership
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	switch o.state {
	case commitOwnershipPending:
		o.state = commitOwnershipClaimed
		o.deadline = time.Now().Add(o.timeout)
		return nil
	case commitOwnershipClaimed:
		return ErrCommitOwnershipClaimed
	case commitOwnershipAborted:
		return ErrCommitOwnershipAborted
	default:
		return ErrInvalidCommitOwnership
	}
}

// abortIfPending é a única transição usada pelo caminho de cancelamento. A
// mesma mutex protege abort e claim, portanto nunca há claim tardia depois de
// o executor ter decidido que a operação não possui o commit.
func (o *CommitOwnership) abortIfPending() bool {
	if o == nil {
		return true
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state != commitOwnershipPending {
		return false
	}
	o.state = commitOwnershipAborted
	return true
}

func (o *CommitOwnership) claimedDeadline() (time.Time, bool) {
	if o == nil {
		return time.Time{}, false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state != commitOwnershipClaimed || o.deadline.IsZero() {
		return time.Time{}, false
	}
	return o.deadline, true
}
