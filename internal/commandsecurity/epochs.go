package commandsecurity

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"assistente/internal/commandinstance"
	"github.com/google/uuid"
)

var ErrInvalidEpochInput = errors.New("entrada de geração inválida")
var ErrStaleEpoch = errors.New("geração de autenticação ou segurança obsoleta")

type EpochSnapshot struct{ UserID, SessionID, AuthGeneration, SecurityGeneration string }
type sessionEpoch struct{ user, group, generation string }

// EpochService coordena gerações locais pelo mesmo gate usado no handoff.
// O host deve criar uma única instância por domínio de invalidação e autenticar
// antes de Capture. Não registra sessões, não autentica, não autoriza e não
// representa o estado locked: o revalidador real deve recusar enquanto bloqueado.
type EpochService struct {
	gate            *DispatchGate
	startup         string
	sequence        uint64
	security        string
	sessions        map[string]sessionEpoch
	transitions     uint64
	disabled        bool
	watchesMu       sync.Mutex
	watches         map[*executionWatch]struct{}
	executorDrains  []func(context.Context) error // somente sob gate; registro obrigatório nos construtores
	closing         bool
	issuedSecurity  map[string]struct{}
	drainRunning    atomic.Bool
	instance        *commandinstance.Lease // gate; vinculada antes do primeiro executor
	drained         bool                   // gate; só após join bem-sucedido de todos os executores
	instanceGate    chan struct{}          // bootstrap/release; espera cancelável, nunca sob gate
	instanceBinding bool                   // gate; bloqueia registro de executor durante I/O de bind
}

func NewEpochService(gate *DispatchGate) (*EpochService, error) {
	if gate == nil {
		return nil, ErrInvalidEpochInput
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	generation := id.String() + ":0"
	return &EpochService{gate: gate, startup: id.String(), security: generation, sessions: map[string]sessionEpoch{}, issuedSecurity: map[string]struct{}{generation: {}}, instanceGate: make(chan struct{}, 1)}, nil
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
// dentro de outro callback do gate (o lock não é reentrante). Para preparar
// solicitações, preferir CaptureAuthenticated: autenticar fora deste método
// permite uma transição entre a leitura da identidade e a captura da geração.
func (s *EpochService) Capture(ctx context.Context, userID, sessionID string) (EpochSnapshot, error) {
	if !s.valid() || !epochID(userID) || !epochID(sessionID) {
		return EpochSnapshot{}, ErrInvalidEpochInput
	}
	return s.CaptureAuthenticated(ctx, func(context.Context) (string, string, error) {
		return userID, sessionID, nil
	})
}

// CaptureAuthenticated deriva identidade e gerações na mesma seção exclusiva.
// authenticate é fornecido pelo host confiável e deve consultar a sessão
// autoritativa, nunca aceitar IDs do payload. Não chamar APIs deste gate no
// callback, nem aguardar rede, cofre, UI ou handlers. A consulta local deve ser
// curta e respeitar ctx. Mutações de segurança devem usar o mesmo gate.
// O snapshot não é autorização: Admit ainda precisa revalidar identidade exata,
// contexto e política antes do handoff. Erro/cancelamento não retorna snapshot.
func (s *EpochService) CaptureAuthenticated(ctx context.Context, authenticate func(context.Context) (userID, sessionID string, err error)) (EpochSnapshot, error) {
	if !s.valid() || authenticate == nil {
		return EpochSnapshot{}, ErrInvalidEpochInput
	}
	var result EpochSnapshot
	err := s.gate.WithMutation(ctx, func() error {
		if s.disabled || s.transitions != 0 {
			return ErrStaleEpoch
		}
		userID, sessionID, err := authenticate(ctx)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !epochID(userID) || !epochID(sessionID) {
			return ErrInvalidEpochInput
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
		if s.closing {
			return ErrStaleEpoch
		}
		if current, ok := s.sessions[sessionID]; ok && current.user != userID {
			return ErrInvalidEpochInput
		}
		if security {
			generation, err := s.next()
			if err != nil {
				s.disabled = true
				s.cancelExecutions("", true)
				return err
			}
			s.security = generation
			s.issuedSecurity[generation] = struct{}{}
		}
		if sessionID != "" {
			delete(s.sessions, sessionID)
		}
		s.cancelExecutions(sessionID, security)
		if action != nil {
			return action()
		}
		return nil
	})
}

// MutateUserConfiguration publica configuração sob o mesmo gate exclusivo,
// sem invalidar a segurança global de usuários não afetados. O host deve
// avançar as gerações do escopo alterado antes de publicar o snapshot. Não é
// autorização nem transação de banco; callback curto e sem reentrada no gate.
// Contextos de preparação/execução de comandos desse usuário são cancelados
// antes do callback, mesmo se ele falhar; fontes de WatchSecurityEpoch e as
// demais contas permanecem intactas. Isso não invalida autenticação/segurança.
func (s *EpochService) MutateUserConfiguration(ctx context.Context, userID string, action func() error) error {
	if !s.valid() || action == nil || !epochID(userID) {
		return ErrInvalidEpochInput
	}
	return s.gate.WithMutation(ctx, func() error {
		if s.closing {
			return ErrStaleEpoch
		}
		s.cancelExecutionsForUser(userID)
		return action()
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
