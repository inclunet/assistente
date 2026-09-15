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
	// A publicação cancela watches do usuário. Encerrar o nosso primeiro e
	// usar ctx original evita autocancelamento; o epoch continua revalidado.
	release()
	return s.epochs.PublishAuthenticatedConfiguration(ctx, epoch, func(ctx context.Context) error {
		current, err := authenticate(ctx)
		if err != nil {
			return err
		}
		if current.UserID != principal.UserID || current.SessionID != principal.SessionID {
			return ErrDenied
		}
		return nil
	}, func() error {
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
			readySession: principal.SessionID,
		}
		return nil
	})
}
