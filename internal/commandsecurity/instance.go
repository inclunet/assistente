package commandsecurity

import (
	"context"
	"errors"

	"assistente/internal/commandinstance"
	"gorm.io/gorm"
)

// BindInstance registra o namespace privado deste core enquanto mantém a
// exclusão nativa do banco. Só o bootstrap pode chamar, antes de construir
// qualquer executor. Capturas de autenticação anteriores não executam efeitos.
// Não há espera por outro processo: contenção falha fechado.
func (s *EpochService) BindInstance(ctx context.Context, db *gorm.DB) error {
	if !s.valid() || ctx == nil || db == nil {
		return ErrInvalidEpochInput
	}
	if err := s.lockInstance(ctx); err != nil {
		return err
	}
	defer func() { <-s.instanceGate }()
	alreadyBound := false
	err := s.gate.WithMutation(ctx, func() error {
		if s.closing || s.disabled {
			return ErrStaleEpoch
		}
		if s.instance != nil {
			if !s.instance.UsesDatabase(db) {
				return ErrInvalidEpochInput
			}
			alreadyBound = true
			return nil
		}
		if len(s.executorDrains) != 0 {
			return ErrInvalidEpochInput
		}
		s.instanceBinding = true
		return nil
	})
	if err != nil || alreadyBound {
		return err
	}
	lease, openErr := commandinstance.Open(ctx, db, s.startup)
	published := false
	// A limpeza da barreira é obrigatória mesmo se Open cancelou. I/O permanece
	// fora do gate; shutdown pode fechar este core durante a aquisição.
	err = s.gate.WithMutation(context.Background(), func() error {
		s.instanceBinding = false
		if openErr != nil {
			return openErr
		}
		if s.closing || s.disabled {
			return ErrStaleEpoch
		}
		s.instance = lease
		published = true
		// Open já confirmou o registro durável. Um cancelamento tardio não
		// deve soltar a lease e tentar registrar o mesmo startup de novo.
		// A montagem continua cancelada; retry reutiliza esta posse ou shutdown
		// a encerra pelo caminho normal de drain.
		return ctx.Err()
	})
	if err != nil && lease != nil && !published {
		// Nunca houve executor registrado nessa lease.
		return errors.Join(err, lease.Close())
	}
	return err
}

// RestartProof nunca inclui este core, registros sem protocolo ou pertencentes
// a outro arquivo físico. Sem lease, retorna a prova zero, não infere um restart.
func (s *EpochService) RestartProof(ctx context.Context, db *gorm.DB) (commandinstance.RecoveryProof, error) {
	var proof commandinstance.RecoveryProof
	if !s.valid() || ctx == nil || db == nil {
		return proof, ErrInvalidEpochInput
	}
	err := s.gate.WithAdmission(ctx, func() error {
		if s.closing {
			return ErrStaleEpoch
		}
		if s.instance != nil {
			if !s.instance.UsesDatabase(db) {
				return ErrInvalidEpochInput
			}
			proof = s.instance.RecoveryProof()
		}
		return nil
	})
	return proof, err
}

// ReleaseInstance só libera exclusão depois de fechar admissões e terminar
// todos os drains. O App chama após reconciliar sua própria finalização.
// Falha de drain conserva o lock; não há release por timeout/cancelamento.
func (s *EpochService) ReleaseInstance(ctx context.Context) error {
	if !s.valid() || ctx == nil {
		return ErrInvalidEpochInput
	}
	if err := s.lockInstance(ctx); err != nil {
		return err
	}
	defer func() { <-s.instanceGate }()
	var lease *commandinstance.Lease
	err := s.gate.WithMutation(ctx, func() error {
		if !s.drained {
			return ErrDrainInProgress
		}
		lease = s.instance
		return nil
	})
	if err != nil {
		return err
	}
	return lease.Close()
}

func (s *EpochService) lockInstance(ctx context.Context) error {
	if s.instanceGate == nil {
		return ErrInvalidEpochInput
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.instanceGate <- struct{}{}:
		return nil
	}
}
