package commandsecurity

import "context"

// PublishAuthenticatedConfiguration revalida e publica uma configuração sob
// a seção exclusiva do DispatchGate. A revalidação deve ser curta e não pode
// chamar novamente APIs que adquiram este gate.
//
// A publicação não altera gerações de autenticação ou segurança. Antes do
// callback de publicação, cancela somente preparações/execuções de comandos
// associadas ao usuário do snapshot. Fontes observadas por WatchSecurityEpoch
// não são resoluções antigas e continuam vivas; outras contas não são afetadas.
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

// RefreshAuthenticatedProjection revalida o mesmo epoch e publica uma
// projeção já semanticamente equivalente sem cancelar watches de execução.
// É reservado para refresh de dependências locais da projeção; uma mudança
// de configuração continua usando PublishAuthenticatedConfiguration.
func (s *EpochService) RefreshAuthenticatedProjection(
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
		return publish()
	})
}

// cancelExecutionsForUser é distinto de cancelExecutions: uma publicação de
// configuração invalida resoluções em todas as sessões do usuário, mas não as
// fontes ligadas somente à segurança, nem sessões de outras contas.
func (s *EpochService) cancelExecutionsForUser(userID string) {
	s.watchesMu.Lock()
	defer s.watchesMu.Unlock()
	for watch := range s.watches {
		if watch.user == userID && watch.configurationSensitive {
			watch.cancel()
			delete(s.watches, watch)
		}
	}
}
