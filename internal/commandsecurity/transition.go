package commandsecurity

import (
	"context"
	"math"
	"sync"
)

// BeginTransition cerca uma transição longa do host sem manter o gate durante
// I/O. Invalida snapshots e impede Capture/Admit até todos os encerramentos.
// Cancela contextos associados por AdmitExecution, sem desfazer efeitos já
// admitidos. O host deve deferir finish imediatamente.
// finish é idempotente e usa contexto independente para não deixar a barreira
// ativa quando o contexto da operação for cancelado. Não chamar sob o gate.
func (s *EpochService) BeginTransition(ctx context.Context) (func(), error) {
	if !s.valid() {
		return nil, ErrInvalidEpochInput
	}
	err := s.gate.WithMutation(ctx, func() error {
		if s.disabled {
			return ErrStaleEpoch
		}
		if s.transitions == math.MaxUint64 {
			s.disabled = true
			s.cancelExecutions("", true)
			return ErrInvalidEpochInput
		}
		generation, err := s.next()
		if err != nil {
			s.disabled = true
			s.cancelExecutions("", true)
			return err
		}
		s.security = generation
		clear(s.sessions)
		s.cancelExecutions("", true)
		s.transitions++
		return nil
	})
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = s.gate.WithMutation(context.Background(), func() error { s.transitions--; return nil })
		})
	}, nil
}
