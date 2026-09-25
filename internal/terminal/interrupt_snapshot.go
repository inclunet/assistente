package terminal

import (
	"errors"
	"fmt"
	"sync/atomic"
)

// ErrInterruptStale indica que a captura não identifica mais exatamente a
// sessão e a geração de input gerenciado a interromper. Nenhum byte foi
// escrito nesse caso.
var ErrInterruptStale = errors.New("captura de interrupção obsoleta")

// InterruptSnapshot é um token de runtime opaco para uma interrupção.
//
// O token não contém texto de entrada ou saída e não tem representação JSON;
// sua validade depende da instância viva de Session mantida pelo backend.
type InterruptSnapshot struct {
	token *interruptSnapshotToken
}

type interruptSnapshotToken struct {
	sessionID         string
	session           *Session
	sessionVersion    uint64
	commandGeneration uint64
	commandPending    bool
	commandID         string
	state             SessionState
	consumed          atomic.Uint32
}

func (s *Session) captureInterruptSnapshot() (InterruptSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateClosing || s.state == StateExited {
		return InterruptSnapshot{}, fmt.Errorf("%w: sessão %s está fechada", ErrInterruptStale, s.id)
	}
	if s.state != StateIdle && s.state != StateRunning {
		return InterruptSnapshot{}, fmt.Errorf("%w: estado da sessão não é interrompível", ErrInterruptStale)
	}
	if s.managedCommandID == "" {
		return InterruptSnapshot{}, fmt.Errorf("%w: sessão ainda não recebeu comando/input gerenciado", ErrInterruptStale)
	}
	if s.state == StateRunning && s.commandPending {
		return InterruptSnapshot{}, fmt.Errorf("%w: comando da sessão %s ainda não foi enviado", ErrInterruptStale, s.id)
	}
	if s.sessionVersion == 0 {
		s.sessionVersion = newSessionVersion()
	}

	return InterruptSnapshot{token: &interruptSnapshotToken{
		sessionID:         s.id,
		session:           s,
		sessionVersion:    s.sessionVersion,
		commandGeneration: s.commandGeneration,
		commandPending:    s.commandPending,
		commandID:         s.managedCommandID,
		state:             s.state,
	}}, nil
}

// MatchesTarget compara o alvo visível da integração com a identidade
// capturada no backend. Capturas sem comando/input gerenciado são recusadas.
func (snapshot InterruptSnapshot) MatchesTarget(sessionID, commandID string) bool {
	return snapshot.token != nil &&
		snapshot.token.sessionID == sessionID &&
		snapshot.token.commandID == commandID
}
