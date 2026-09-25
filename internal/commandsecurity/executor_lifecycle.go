package commandsecurity

import "context"

// RegisterExecutorDrainAction registra um drain e confirma a ação de montagem
// na mesma seção exclusiva. Se a ação falhar, o registro é removido antes de
// liberar o gate; assim um executor parcialmente montado nunca fica anexado à
// drenagem futura.
//
// A ação deve ser curta, não deve readquirir o gate e não deve aguardar o
// resultado de um executor. CloseAndDrain só pode observar o registro depois
// que a ação terminou com sucesso.
func (s *EpochService) RegisterExecutorDrainAction(ctx context.Context, drain func(context.Context) error, action func() error) error {
	if !s.valid() || drain == nil || action == nil {
		return ErrInvalidEpochInput
	}
	return s.gate.WithMutation(ctx, func() error {
		if s.disabled || s.closing || s.instanceBinding {
			return ErrStaleEpoch
		}
		before := len(s.executorDrains)
		s.executorDrains = append(s.executorDrains, drain)
		committed := false
		defer func() {
			if !committed {
				s.executorDrains = s.executorDrains[:before]
			}
		}()
		if err := action(); err != nil {
			return err
		}
		committed = true
		return nil
	})
}

// WithExecutorLifecycle serializa uma reconfiguração/publicação com o
// registro de fechamento. Não registra drain novo; serve para substituir uma
// montagem já registrada sem permitir que CloseAndDrain publique o fechamento
// no intervalo entre configurar o manager e publicar o mounted.
func (s *EpochService) WithExecutorLifecycle(ctx context.Context, action func() error) error {
	if !s.valid() || action == nil {
		return ErrInvalidEpochInput
	}
	return s.gate.WithMutation(ctx, func() error {
		if s.disabled || s.closing || s.instanceBinding {
			return ErrStaleEpoch
		}
		return action()
	})
}
