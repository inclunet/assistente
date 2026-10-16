package commandexecution

import (
	"context"

	"assistente/internal/auth"
	"assistente/internal/commandsecurity"
)

// ChangeUserConfiguration prepara uma mutação autenticada fora do gate e
// entrega seu commit somente depois de revalidar a mesma sessão e o estado
// atual do host. prepare é uma porta confiável: pode aguardar banco ou UI,
// mas não é uma fonte de autoridade do chamador e deve devolver um callback de
// commit curto, sem readquirir o DispatchGate.
//
// A publicação remove o mapa do usuário antes de chamar commit. Essa remoção
// é deliberadamente irreversível nesta operação: erro ou panic do escritor não
// restaura configuração em memória nem tenta publicar novamente.
func (s *HostState) ChangeUserConfiguration(ctx context.Context,
	authenticate func(context.Context) (auth.LocalSessionPrincipal, error),
	prepare func(context.Context, auth.LocalSessionPrincipal) (func(context.Context) error, error),
) error {
	if prepare == nil {
		return ErrInvalidHostState
	}
	return s.ChangeUserConfigurationWithEpoch(ctx, authenticate, func(ctx context.Context, principal auth.LocalSessionPrincipal, _ commandsecurity.EpochSnapshot) (func(context.Context) error, error) {
		return prepare(ctx, principal)
	})
}

// ChangeUserConfigurationWithEpoch é a variante que entrega à preparação o
// snapshot autenticado capturado antes de liberar o gate. A preparação pode
// aguardar banco ou UI, mas deve devolver um callback de commit curto, sem
// readquirir o DispatchGate.
func (s *HostState) ChangeUserConfigurationWithEpoch(ctx context.Context,
	authenticate func(context.Context) (auth.LocalSessionPrincipal, error),
	prepare func(context.Context, auth.LocalSessionPrincipal, commandsecurity.EpochSnapshot) (func(context.Context) error, error),
) error {
	if s == nil || s.epochs == nil || ctx == nil || authenticate == nil || prepare == nil {
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

	prepareCtx, release, err := s.epochs.WatchEpoch(ctx, epoch)
	if err != nil {
		return err
	}
	defer release()
	commit, err := prepare(prepareCtx, principal, epoch)
	if err != nil {
		return err
	}
	// Não manter o watch da preparação durante a própria publicação, que
	// cancela contextos antigos do usuário. O commit recebe o contexto original;
	// sua autorização continua dependendo da revalidação atômica abaixo.
	release()
	if commit == nil {
		return ErrInvalidHostState
	}

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
		// PublishAuthenticatedConfiguration já cancela os watches do usuário
		// imediatamente antes deste callback, sob o mesmo gate exclusivo.
		s.mu.Lock()
		if s.disabled || !s.vaultUnlocked || !s.osKnown || s.osLocked || s.counter != revision {
			s.mu.Unlock()
			return ErrDenied
		}
		if _, err := s.reserveGenerationsLocked(1); err != nil {
			s.mu.Unlock()
			return err
		}
		delete(s.users, principal.UserID)
		s.mu.Unlock()

		// O gate permanece adquirido, mas o mutex do HostState não. Não há
		// rollback se commit falhar ou entrar em panic.
		return commit(ctx)
	})
}
