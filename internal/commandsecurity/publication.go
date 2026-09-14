package commandsecurity

import "context"

// PublishAuthenticatedConfiguration revalida e publica uma configuração sob
// a seção exclusiva do DispatchGate. A revalidação deve ser curta e não pode
// chamar novamente APIs que adquiram este gate.
//
// A publicação não altera gerações de autenticação ou segurança. Antes do
// callback de publicação, cancela somente as execuções associadas ao usuário
// do snapshot; execuções de outras contas permanecem admitidas.
func (s *EpochService) PublishAuthenticatedConfiguration(
	ctx context.Context,
	snapshot EpochSnapshot,
	revalidate func(context.Context) error,
	publish func() error,
) error {
	if !s.valid() || revalidate == nil || publish == nil {
		return ErrInvalidEpochInput
	}

	return s.gate.WithMutation(ctx, func() error {
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

		s.cancelExecutionsForUser(snapshot.UserID)
		return publish()
	})
}

// cancelExecutionsForUser é distinto de cancelExecutions: uma publicação de
// configuração invalida todas as sessões do usuário, mas não as sessões de
// outras contas.
func (s *EpochService) cancelExecutionsForUser(userID string) {
	s.watchesMu.Lock()
	defer s.watchesMu.Unlock()
	for watch := range s.watches {
		if watch.user == userID {
			watch.cancel()
			delete(s.watches, watch)
		}
	}
}
