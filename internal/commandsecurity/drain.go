package commandsecurity

import (
	"context"
	"errors"
)

var ErrDrainInProgress = errors.New("drenagem do core já em andamento")
var ErrDrainFailed = errors.New("executor falhou durante drenagem")

// DrainedGenerations só é emitida depois de fechar o core e aguardar todos os
// executores registrados. Não prova encerramento de outro processo, nem que
// um efeito externo não cooperativo terminou; prova exclusão de novas
// admissões e retorno das operações locais, incluindo sua finalização.
type DrainedGenerations struct{ issued map[string]struct{} }

func (p DrainedGenerations) Includes(generation string) bool {
	_, ok := p.issued[generation]
	return ok
}

// RegisterExecutorDrain é uma porta exclusiva de bootstrap, usada
// obrigatoriamente por New/NewComplete do executor. Nunca chamar sob o gate.
// drain deve esperar a operação inteira; não basta cancelar seus watches.
func (s *EpochService) RegisterExecutorDrain(ctx context.Context, drain func(context.Context) error) error {
	if !s.valid() || drain == nil {
		return ErrInvalidEpochInput
	}
	return s.gate.WithMutation(ctx, func() error {
		if s.disabled || s.closing {
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
