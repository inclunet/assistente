package commandexecution

import (
	"context"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
)

// RebuildUserConfiguration autentica, constrói fora do gate e publica somente
// se a mesma sessão, segurança e estado do host ainda forem atuais. Callbacks
// pertencem ao host confiável, nunca à UI. authenticate consulta a sessão local
// autoritativa e é curto; build pode fazer I/O cancelável, sem efeitos externos.
// Nenhuma tentativa automática: uma reconstrução obsoleta deve ser descartada.
func (s *HostState) RebuildUserConfiguration(ctx context.Context,
	authenticate func(context.Context) (auth.LocalSessionPrincipal, error),
	build func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error),
) error {
	return s.RebuildUserConfigurationGuarded(ctx, authenticate, build, nil)
}

// RebuildUserConfigurationGuarded é a variante usada por fontes confiáveis
// que publicam uma projeção dependente de outro estado local. O guard não é
// uma porta de I/O nem de gate: deve apenas validar memória/estado já
// montado. Ele é executado fora de s.mu durante snapshots e antes do lock no
// commit, evitando reentrada e deadlock com managers externos.
func (s *HostState) RebuildUserConfigurationGuarded(ctx context.Context,
	authenticate func(context.Context) (auth.LocalSessionPrincipal, error),
	build func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error),
	guard func(context.Context) error,
) error {
	if s == nil || s.epochs == nil || ctx == nil || authenticate == nil || build == nil {
		return ErrInvalidHostState
	}
	var principal auth.LocalSessionPrincipal
	var revision uint64
	epoch, err := s.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		var err error
		principal, err = authenticate(ctx)
		if err != nil {
			return "", "", err
		}
		s.mu.RLock()
		defer s.mu.RUnlock()
		if s.disabled || !s.vaultUnlocked || !s.osKnown || s.osLocked {
			return "", "", ErrDenied
		}
		revision = s.counter
		return principal.UserID, principal.SessionID, nil
	})
	if err != nil {
		return err
	}
	// Inscrever depois da captura revalida também a janela entre as etapas.
	// Lock/logout cancela I/O cooperativo do carregador, não apenas a publicação.
	buildCtx, release, err := s.epochs.WatchEpoch(ctx, epoch)
	if err != nil {
		return err
	}
	defer release()
	configuration, layers, err := build(buildCtx, principal)
	if err != nil {
		return err
	}
	if configuration == nil {
		return ErrInvalidHostState
	}
	if err := validateHostLayers(layers); err != nil {
		return err
	}
	layers = cloneStrings(layers)
	s.mu.RLock()
	current, hasCurrent := s.users[principal.UserID]
	var capturedConfiguration *commandbindings.Configuration
	var capturedLayers []string
	var capturedSession string
	var capturedCounter uint64
	if hasCurrent {
		capturedConfiguration = current.configuration
		capturedLayers = cloneStrings(current.activeLayers)
		capturedSession = current.readySession
		capturedCounter = s.counter
	}
	s.mu.RUnlock()
	equivalent := hasCurrent && capturedConfiguration != nil && capturedSession == principal.SessionID && slicesEqual(capturedLayers, layers) && capturedConfiguration.Equivalent(configuration)
	// A publicação cancela watches do usuário. Encerrar o nosso primeiro e
	// usar ctx original evita autocancelamento; o epoch continua revalidado.
	release()
	revalidate := func(ctx context.Context) error {
		current, err := authenticate(ctx)
		if err != nil {
			return err
		}
		if current.UserID != principal.UserID || current.SessionID != principal.SessionID {
			return ErrDenied
		}
		return nil
	}
	guardCommit := func() error {
		if guard != nil {
			if err := guard(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	if equivalent && guard != nil {
		return s.epochs.RefreshAuthenticatedProjection(ctx, epoch, revalidate, func() error {
			if err := guardCommit(); err != nil {
				return err
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.disabled || !s.vaultUnlocked || !s.osKnown || s.osLocked || s.counter != revision {
				return ErrDenied
			}
			user, ok := s.users[principal.UserID]
			if !ok || user.configuration != capturedConfiguration || user.readySession != capturedSession || s.counter != capturedCounter || !slicesEqual(user.activeLayers, capturedLayers) {
				return ErrDenied
			}
			if _, err := s.reserveGenerationsLocked(1); err != nil {
				return err
			}
			user.projectionGuard = guard
			s.users[principal.UserID] = user
			return nil
		})
	}
	return s.epochs.PublishAuthenticatedConfiguration(ctx, epoch, revalidate, func() error {
		if err := guardCommit(); err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.disabled || !s.vaultUnlocked || !s.osKnown || s.osLocked || s.counter != revision {
			return ErrDenied
		}
		generations, err := s.reserveGenerationsLocked(2)
		if err != nil {
			return err
		}
		s.users[principal.UserID] = hostUserState{
			configuration: configuration, activeLayers: layers,
			globalConfig: generations[0], activeLayersVersion: generations[1],
			readySession:    principal.SessionID,
			projectionGuard: guard,
		}
		return nil
	})
}

// RebuildUserConfigurationForJobClaimProjection is the narrow publication
// path for a newly loaded job-claim projection. It preserves only admitted
// executions carrying a per-invocation resolution witness, and only when the
// persisted non-job configuration baseline is unchanged. The external guard
// runs before entering the dispatch gate; current must be a pure, lock-free
// memory check suitable for the short in-gate revalidation.
func (s *HostState) RebuildUserConfigurationForJobClaimProjection(ctx context.Context,
	authenticate func(context.Context) (auth.LocalSessionPrincipal, error),
	build func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error),
	guard func(context.Context) error,
	current func() bool,
) error {
	if s == nil || s.epochs == nil || ctx == nil || authenticate == nil || build == nil || current == nil {
		return ErrInvalidHostState
	}
	var principal auth.LocalSessionPrincipal
	var revision uint64
	epoch, err := s.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		var err error
		principal, err = authenticate(ctx)
		if err != nil {
			return "", "", err
		}
		s.mu.RLock()
		defer s.mu.RUnlock()
		if s.disabled || !s.vaultUnlocked || !s.osKnown || s.osLocked {
			return "", "", ErrDenied
		}
		revision = s.counter
		return principal.UserID, principal.SessionID, nil
	})
	if err != nil {
		return err
	}
	buildCtx, release, err := s.epochs.WatchEpoch(ctx, epoch)
	if err != nil {
		return err
	}
	defer release()
	configuration, layers, err := build(buildCtx, principal)
	if err != nil {
		return err
	}
	if configuration == nil {
		return ErrInvalidHostState
	}
	if err := validateHostLayers(layers); err != nil {
		return err
	}
	layers = cloneStrings(layers)
	if guard != nil {
		if err := guard(ctx); err != nil {
			return err
		}
	}
	s.mu.RLock()
	captured, hasCurrent := s.users[principal.UserID]
	capturedConfiguration := captured.configuration
	capturedLayers := cloneStrings(captured.activeLayers)
	capturedSession := captured.readySession
	capturedCounter := s.counter
	s.mu.RUnlock()
	equivalent := hasCurrent && capturedConfiguration != nil && capturedSession == principal.SessionID &&
		slicesEqual(capturedLayers, layers) && capturedConfiguration.EquivalentExceptValidityDeadline(configuration)
	release()
	revalidate := func(ctx context.Context) error {
		currentPrincipal, err := authenticate(ctx)
		if err != nil {
			return err
		}
		if currentPrincipal != principal || !current() {
			return ErrStale
		}
		return nil
	}
	if equivalent && guard != nil {
		return s.epochs.RefreshAuthenticatedProjection(ctx, epoch, revalidate, func() error {
			if !current() {
				return ErrStale
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.disabled || !s.vaultUnlocked || !s.osKnown || s.osLocked || s.counter != revision {
				return ErrDenied
			}
			user, ok := s.users[principal.UserID]
			if !ok || user.configuration != capturedConfiguration || user.readySession != capturedSession || s.counter != capturedCounter || !slicesEqual(user.activeLayers, capturedLayers) {
				return ErrDenied
			}
			// A renewed runtime lease may extend only the projection validity
			// deadline. Refresh that immutable snapshot and its local guard without
			// treating the heartbeat as a configuration/layer generation change.
			user.configuration = configuration
			user.projectionGuard = guard
			s.users[principal.UserID] = user
			return nil
		})
	}
	return s.epochs.PublishAuthenticatedProjectionForExecutions(ctx, epoch, configuration, revalidate, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.disabled || !s.vaultUnlocked || !s.osKnown || s.osLocked || s.counter != revision {
			return ErrDenied
		}
		user, ok := s.users[principal.UserID]
		if !ok || user.configuration == nil || user.readySession != principal.SessionID {
			return ErrDenied
		}
		if user.configuration != capturedConfiguration || capturedSession != principal.SessionID || s.counter != capturedCounter || !slicesEqual(user.activeLayers, capturedLayers) {
			return ErrDenied
		}
		if user.configuration.PersistedBaseline() == "" || user.configuration.PersistedBaseline() != configuration.PersistedBaseline() {
			return ErrJobProjectionBaseChanged
		}
		if !current() {
			return ErrStale
		}
		generations, err := s.reserveGenerationsLocked(1)
		if err != nil {
			return err
		}
		user.configuration = configuration
		user.activeLayers = layers
		user.activeLayersVersion = generations[0]
		user.projectionGuard = guard
		s.users[principal.UserID] = user
		return nil
	})
}
