package commandsecurity

import "context"

import "assistente/internal/commandbindings"

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

// PublishAuthenticatedProjectionForExecutions publica uma nova projeção
// derivada sem transformar o refresh em uma isenção global. Cada execução
// sensível precisa provar, com witness imutável, que sua própria resolução
// continua equivalente na nova configuração. Execuções sem witness, com
// conflito, erro ou pânico são canceladas. O callback de prova roda sob o
// DispatchGate e deve ser puro de memória: sem I/O, readquire de gate ou
// mutexes externos. A elegibilidade desta API também depende do chamador
// provar que a base persistida não mudou. As testemunhas são copiadas sob o
// mutex, avaliadas sem ele e, ao cancelar, a associação é conferida novamente.
func (s *EpochService) PublishAuthenticatedProjectionForExecutions(
	ctx context.Context,
	snapshot EpochSnapshot,
	configuration *commandbindings.Configuration,
	revalidate func(context.Context) error,
	publish func() error,
) error {
	if !s.valid() || configuration == nil || publish == nil {
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
		if revalidate != nil {
			if err := revalidate(ctx); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		s.cancelExecutionsAffectedByProjection(snapshot.UserID, configuration)
		if err := publish(); err != nil {
			// A publication callback may have failed after a partial state change.
			// Do not leave any execution alive on a projection we cannot certify.
			s.cancelExecutionsForUser(snapshot.UserID)
			return err
		}
		return nil
	})
}

// cancelExecutionsAffectedByProjection cancels every configuration-sensitive
// execution whose private witness cannot prove exact equivalence. It never
// restores a watch already removed by an earlier cancellation.
func (s *EpochService) cancelExecutionsAffectedByProjection(userID string, configuration *commandbindings.Configuration) {
	s.watchesMu.Lock()
	watches := make([]*executionWatch, 0, len(s.watches))
	for watch := range s.watches {
		watches = append(watches, watch)
	}
	s.watchesMu.Unlock()

	for _, watch := range watches {
		if watch.user != userID || !watch.configurationSensitive {
			continue
		}
		current, ok := s.sessions[watch.session]
		if !ok || current.user != watch.user || current.generation != watch.authGeneration || s.security != watch.securityGeneration {
			s.cancelWatchIfCurrent(watch)
			continue
		}
		preserved := false
		if watch.projectionProof != nil {
			func() {
				defer func() { _ = recover() }()
				preserved = watch.projectionProof(configuration)
			}()
		}
		if !preserved {
			s.cancelWatchIfCurrent(watch)
		}
	}
}

func (s *EpochService) cancelWatchIfCurrent(watch *executionWatch) {
	s.watchesMu.Lock()
	defer s.watchesMu.Unlock()
	if _, current := s.watches[watch]; current {
		watch.cancel()
		delete(s.watches, watch)
	}
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
