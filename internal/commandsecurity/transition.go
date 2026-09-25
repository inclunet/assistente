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
		return s.beginTransitionLocked(ctx, nil, nil, nil)
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

// BeginTransitionFromSnapshot começa uma transição que foi iniciada por uma
// operação de capability capaz de invalidar o próprio contexto de execução.
// O preflight do host ocorre fora do gate. Sob exclusão, o snapshot de epochs
// é conferido novamente e a geração é alocada antes do claim. O callback de
// claim deve revalidar/cercar os fatos locais mutáveis, além de reivindicar a
// ownership, antes de qualquer watch ser cancelado.
//
// revalidate roda fora do gate e pode consultar portas confiáveis; claim roda
// dentro do gate, depois da alocação, e deve ser local/curto, sem I/O nem
// reentrada. finish é idempotente e usa contexto independente do caller.
func (s *EpochService) BeginTransitionFromSnapshot(
	ctx context.Context,
	snapshot EpochSnapshot,
	revalidate func(context.Context) error,
	claim func(context.Context) error,
) (func(), error) {
	if !s.valid() || revalidate == nil || claim == nil {
		return nil, ErrInvalidEpochInput
	}
	// A consulta confiável do host pode fazer I/O (por exemplo, reler o
	// snapshot de configuração/workspace). Ela não pode ocorrer sob o gate:
	// admissões e finalizações precisam continuar capazes de observar a
	// invalidação enquanto a consulta está em andamento.
	err := s.gate.WithAdmission(ctx, func() error {
		return s.validateTransitionSnapshotLocked(ctx, snapshot)
	})
	if err != nil {
		return nil, err
	}
	if err := revalidate(ctx); err != nil {
		return nil, err
	}
	err = s.gate.WithMutation(ctx, func() error {
		if err := s.validateTransitionSnapshotLocked(ctx, snapshot); err != nil {
			return err
		}
		if s.transitions != 0 {
			return ErrStaleEpoch
		}
		return s.beginTransitionLocked(ctx, nil, claim, nil)
	})
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = s.gate.WithMutation(context.Background(), func() error {
				if s.transitions == 0 {
					return nil
				}
				s.transitions--
				return nil
			})
		})
	}, nil
}

func (s *EpochService) validateTransitionSnapshotLocked(ctx context.Context, snapshot EpochSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !epochID(snapshot.UserID) || !epochID(snapshot.SessionID) || snapshot.AuthGeneration == "" || snapshot.SecurityGeneration == "" {
		return ErrInvalidEpochInput
	}
	current, ok := s.sessions[snapshot.SessionID]
	if !ok || current.user != snapshot.UserID || current.generation != snapshot.AuthGeneration || s.security != snapshot.SecurityGeneration {
		return ErrStaleEpoch
	}
	if s.disabled || s.closing || s.transitions != 0 {
		return ErrStaleEpoch
	}
	return nil
}

// beginTransitionLocked contém a mutação comum às duas APIs. O chamador já
// está sob o gate; callbacks continuam antes da publicação e nunca são
// executados depois de uma transição parcial.
func (s *EpochService) beginTransitionLocked(
	ctx context.Context,
	revalidate func(context.Context) error,
	claim func(context.Context) error,
	precondition func() error,
) error {
	if s.disabled || s.closing {
		return ErrStaleEpoch
	}
	if precondition != nil {
		if err := precondition(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if revalidate != nil {
		if err := revalidate(ctx); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
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
	if claim != nil {
		if err := claim(ctx); err != nil {
			return err
		}
	}
	s.security = generation
	s.issuedSecurity[generation] = struct{}{}
	clear(s.sessions)
	s.cancelExecutions("", true)
	s.transitions++
	return nil
}
