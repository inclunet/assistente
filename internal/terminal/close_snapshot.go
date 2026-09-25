package terminal

import (
	"errors"
	"fmt"
	"sync/atomic"
)

// ErrCloseStale indica que a captura de encerramento deixou de identificar
// exatamente a mesma sessão e geração gerenciada. Nenhum efeito destrutivo é
// iniciado quando essa condição ocorre.
var ErrCloseStale = errors.New("captura de encerramento obsoleta")

// CloseSnapshot é um token de runtime opaco para encerrar uma sessão. Ele não
// é serializável e não pode ser usado para selecionar outra sessão.
type CloseSnapshot struct {
	token *closeSnapshotToken
}

// SessionID returns the exact runtime session identity captured by the token.
// It is used only to compare a visible binding during command preparation.
func (s CloseSnapshot) SessionID() string {
	if s.token == nil {
		return ""
	}
	return s.token.sessionID
}

type closeSnapshotToken struct {
	sessionID         string
	session           *Session
	sessionVersion    uint64
	commandGeneration uint64
	commandPending    bool
	commandID         string
	state             SessionState
	consumed          atomic.Uint32
}

func (s *Session) captureCloseSnapshot() (CloseSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateClosing || s.state == StateExited {
		return CloseSnapshot{}, fmt.Errorf("%w: sessão %s está fechada", ErrCloseStale, s.id)
	}
	if s.state != StateIdle && s.state != StateRunning {
		return CloseSnapshot{}, fmt.Errorf("%w: estado da sessão não pode ser encerrado", ErrCloseStale)
	}
	if s.sessionVersion == 0 {
		s.sessionVersion = newSessionVersion()
	}

	return CloseSnapshot{token: &closeSnapshotToken{
		sessionID:         s.id,
		session:           s,
		sessionVersion:    s.sessionVersion,
		commandGeneration: s.commandGeneration,
		commandPending:    s.commandPending,
		commandID:         s.managedCommandID,
		state:             s.state,
	}}, nil
}

func (s *Session) matchesCloseSnapshot(token *closeSnapshotToken) bool {
	return token != nil && s.id == token.sessionID &&
		s.sessionVersion == token.sessionVersion &&
		s.commandGeneration == token.commandGeneration &&
		s.commandPending == token.commandPending &&
		s.managedCommandID == token.commandID && s.state == token.state &&
		(s.state == StateIdle || s.state == StateRunning)
}
