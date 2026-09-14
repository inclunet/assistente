package commandsecurity

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
)

var ErrInvalidEpochInput = errors.New("entrada de geração inválida")
var ErrStaleEpoch = errors.New("geração de autenticação ou segurança obsoleta")

type EpochSnapshot struct{ UserID, SessionID, AuthGeneration, SecurityGeneration string }
type sessionEpoch struct{ user, generation string }

// EpochService coordena gerações locais pelo mesmo gate usado no handoff.
// O host deve criar uma única instância por domínio de invalidação e autenticar
// antes de Capture. Não registra sessões, não autentica, não autoriza e não
// representa o estado locked: o revalidador real deve recusar enquanto bloqueado.
type EpochService struct {
	gate        *DispatchGate
	startup     string
	sequence    uint64
	security    string
	sessions    map[string]sessionEpoch
	transitions uint64
	disabled    bool
}

func NewEpochService(gate *DispatchGate) (*EpochService, error) {
	if gate == nil {
		return nil, ErrInvalidEpochInput
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	return &EpochService{gate: gate, startup: id.String(), security: id.String() + ":0", sessions: map[string]sessionEpoch{}}, nil
}

func epochID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}
func (s *EpochService) valid() bool {
	return s != nil && s.gate != nil && s.sessions != nil && s.startup != ""
}
func (s *EpochService) next() (string, error) {
	if s.sequence == math.MaxUint64 {
		return "", ErrInvalidEpochInput
	}
	s.sequence++
	return fmt.Sprintf("%s:%d", s.startup, s.sequence), nil
}

// Capture só deve receber IDs já derivados do SessionService. Não chamar de
// dentro de outro callback do gate (o lock não é reentrante).
func (s *EpochService) Capture(ctx context.Context, userID, sessionID string) (EpochSnapshot, error) {
	if !s.valid() || !epochID(userID) || !epochID(sessionID) {
		return EpochSnapshot{}, ErrInvalidEpochInput
	}
	var result EpochSnapshot
	err := s.gate.WithMutation(ctx, func() error {
		if s.disabled || s.transitions != 0 {
			return ErrStaleEpoch
		}
		current, ok := s.sessions[sessionID]
		if ok && current.user != userID {
			return ErrInvalidEpochInput
		}
		if !ok {
			generation, err := s.next()
			if err != nil {
				return err
			}
			current = sessionEpoch{user: userID, generation: generation}
			s.sessions[sessionID] = current
		}
		result = EpochSnapshot{UserID: userID, SessionID: sessionID, AuthGeneration: current.generation, SecurityGeneration: s.security}
		return nil
	})
	if err != nil {
		return EpochSnapshot{}, err
	}
	return result, nil
}

// Admit revalida o estado autoritativo e faz somente handoff não bloqueante
// sob gate compartilhado. A espera do resultado deve ocorrer depois do retorno.
// Callbacks não podem readquirir este gate. Não fornece autorização por si só.
// revalidate deve confirmar exatamente o usuário/sessão do snapshot, além de
// lock, revogação e autorização atuais; aceitar qualquer sessão válida é incorreto.
func (s *EpochService) Admit(ctx context.Context, snapshot EpochSnapshot, revalidate func(context.Context) error, handoff func() error) error {
	if !s.valid() || revalidate == nil || handoff == nil {
		return ErrInvalidEpochInput
	}
	return s.gate.WithAdmission(ctx, func() error {
		if s.disabled || s.transitions != 0 {
			return ErrStaleEpoch
		}
		current, ok := s.sessions[snapshot.SessionID]
		if !ok || current.user != snapshot.UserID || current.generation != snapshot.AuthGeneration || s.security != snapshot.SecurityGeneration {
			return ErrStaleEpoch
		}
		if err := revalidate(ctx); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return handoff()
	})
}

func (s *EpochService) invalidate(ctx context.Context, userID, sessionID string, security bool) error {
	return s.mutate(ctx, userID, sessionID, security, nil)
}

func (s *EpochService) mutate(ctx context.Context, userID, sessionID string, security bool, action func() error) error {
	if !s.valid() {
		return ErrInvalidEpochInput
	}
	if sessionID != "" && (!epochID(sessionID) || !epochID(userID)) {
		return ErrInvalidEpochInput
	}
	return s.gate.WithMutation(ctx, func() error {
		if current, ok := s.sessions[sessionID]; ok && current.user != userID {
			return ErrInvalidEpochInput
		}
		if security {
			generation, err := s.next()
			if err != nil {
				return err
			}
			s.security = generation
		}
		if sessionID != "" {
			delete(s.sessions, sessionID)
		}
		if action != nil {
			return action()
		}
		return nil
	})
}

// MutateSession invalida a sessão e aplica uma mutação autoritativa curta sob
// o mesmo gate exclusivo. O host deve fornecer IDs autenticados e mutação
// correspondente ao escopo indicado; esta API não autentica o chamador.
// Erro/panic pode ocorrer após efeito parcial: a invalidação NÃO é revertida.
// O callback não pode chamar Capture/Admit/Invalidate/Mutate nem readquirir o
// gate; não deve aguardar UI, rede ou resultado de handler. Não há rollback DB
// implícito. A espera pelo gate herda o cancelamento limitado de DispatchGate.
func (s *EpochService) MutateSession(ctx context.Context, userID, sessionID string, action func() error) error {
	if action == nil || !epochID(userID) || !epochID(sessionID) {
		return ErrInvalidEpochInput
	}
	return s.mutate(ctx, userID, sessionID, false, action)
}

// MutatePrincipal invalida sessão e segurança antes de aplicar a mudança.
func (s *EpochService) MutatePrincipal(ctx context.Context, userID, sessionID string, action func() error) error {
	if action == nil || !epochID(userID) || !epochID(sessionID) {
		return ErrInvalidEpochInput
	}
	return s.mutate(ctx, userID, sessionID, true, action)
}

// MutateSecurity coordena lock/unlock ou outra mutação global de segurança.
// O callback mantém o estado autoritativo; o epoch não representa locked.
func (s *EpochService) MutateSecurity(ctx context.Context, action func() error) error {
	if action == nil {
		return ErrInvalidEpochInput
	}
	return s.mutate(ctx, "", "", true, action)
}

func (s *EpochService) InvalidateSession(ctx context.Context, userID, sessionID string) error {
	if !epochID(userID) || !epochID(sessionID) {
		return ErrInvalidEpochInput
	}
	return s.invalidate(ctx, userID, sessionID, false)
}

// InvalidatePrincipal invalida sessão e segurança na mesma seção exclusiva.
func (s *EpochService) InvalidatePrincipal(ctx context.Context, userID, sessionID string) error {
	if !epochID(userID) || !epochID(sessionID) {
		return ErrInvalidEpochInput
	}
	return s.invalidate(ctx, userID, sessionID, true)
}

func (s *EpochService) InvalidateSecurity(ctx context.Context) error {
	return s.invalidate(ctx, "", "", true)
}
