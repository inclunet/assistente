package commandsecurity

import (
	"context"
	"errors"

	"assistente/internal/commandinstance"
	"gorm.io/gorm"
)

var ErrDrainInProgress = errors.New("drenagem do core já em andamento")
var ErrDrainFailed = errors.New("executor falhou durante drenagem")

// DrainedGenerations contém uma prova opaca de fechamento: drain do core
// local ou exclusão interprocesso emitida pelo protocolo persistido.
// Nenhuma variante prova que um efeito externo não cooperativo terminou;
// recovery registra outcome_unknown, nunca reexecuta o handler.
type DrainedGenerations struct {
	issued  map[string]struct{}
	restart commandinstance.RecoveryProof
}

// FromRestartProof adapta a prova opaca de exclusão interprocesso aos mesmos
// writers de recovery. Não converte listas, PID, idade ou marker em prova.
func FromRestartProof(proof commandinstance.RecoveryProof) DrainedGenerations {
	return DrainedGenerations{restart: proof}
}

// Valid distingue uma prova emitida pelo core de um valor zero. Não permite
// construir, ampliar ou inferir gerações a partir de dados persistidos.
func (p DrainedGenerations) Valid() bool { return len(p.issued) != 0 || p.restart.Valid() }

// AllowsDatabase impede usar uma autoridade persistida na conexão de outra
// instância/cópia. O drain local continua limitado às gerações emitidas pelo core.
func (p DrainedGenerations) AllowsDatabase(db *gorm.DB) bool {
	return len(p.issued) != 0 || p.restart.UsesDatabase(db)
}

func (p DrainedGenerations) Includes(generation string) bool {
	_, ok := p.issued[generation]
	return ok || p.restart.Includes(generation)
}

// RegisterExecutorDrain é uma porta exclusiva de bootstrap, usada
// obrigatoriamente por New/NewComplete do executor. Nunca chamar sob o gate.
// drain deve esperar a operação inteira; não basta cancelar seus watches.
func (s *EpochService) RegisterExecutorDrain(ctx context.Context, drain func(context.Context) error) error {
	if !s.valid() || drain == nil {
		return ErrInvalidEpochInput
	}
	return s.gate.WithMutation(ctx, func() error {
		if s.disabled || s.closing || s.instanceBinding {
			return ErrStaleEpoch
		}
		s.executorDrains = append(s.executorDrains, drain)
		return nil
	})
}

// CloseAndDrain fecha permanentemente este domínio de execução. Timeout não
// reabre registro/admissão; uma nova chamada pode continuar a drenagem. Não
// chamar de um handler, porta ou callback do próprio executor/gate.
func (s *EpochService) CloseAndDrain(ctx context.Context) (DrainedGenerations, error) {
	if !s.valid() || ctx == nil {
		return DrainedGenerations{}, ErrInvalidEpochInput
	}
	if !s.drainRunning.CompareAndSwap(false, true) {
		return DrainedGenerations{}, ErrDrainInProgress
	}
	defer s.drainRunning.Store(false)
	var drains []func(context.Context) error
	var issued map[string]struct{}
	// A intenção de fechar não pode desaparecer se ctx expirar na espera pelo
	// gate. Essa espera já não é cancelável no DispatchGate; usamos contexto
	// independente somente para publicar a barreira curta, nunca para drenar.
	err := s.gate.WithMutation(context.Background(), func() error {
		s.closing, s.disabled = true, true
		s.cancelExecutions("", true)
		drains = append(drains, s.executorDrains...)
		issued = make(map[string]struct{}, len(s.issuedSecurity))
		for generation := range s.issuedSecurity {
			issued[generation] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return DrainedGenerations{}, err
	}
	// Mesmo após erro, tente fechar os demais serviços; nenhum resultado parcial
	// vira prova. Os callbacks devem respeitar ctx e são executados fora do gate.
	var failures []error
	for _, drain := range drains {
		if err := safelyDrain(ctx, drain); err != nil {
			failures = append(failures, err)
		}
	}
	if err := ctx.Err(); err != nil {
		failures = append(failures, err)
	}
	if err := errors.Join(failures...); err != nil {
		return DrainedGenerations{}, err
	}
	if err := s.gate.WithMutation(ctx, func() error { s.drained = true; return nil }); err != nil {
		return DrainedGenerations{}, err
	}
	return DrainedGenerations{issued: issued}, nil
}

func safelyDrain(ctx context.Context, drain func(context.Context) error) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrDrainFailed
		}
	}()
	return drain(ctx)
}
