package app

import (
	"context"
	"errors"
	"time"

	"assistente/internal/commandexecution"
	"assistente/internal/commandsecurity"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/profiles"
	"assistente/internal/workspace"
)

// commitProfileMutation coordena os callers nativos do writer de perfis.
// A barreira invalida admissões antes de qualquer arquivo/grant e permanece
// fechada durante callbacks. Nenhum DispatchGate é mantido durante I/O.
func (a *App) commitProfileMutation(ctx context.Context, mutation *profiles.CommandMutation, publish func(string) error) (string, error) {
	return a.coordinateProfileMutation(ctx, mutation, publish, nil, nil)
}

type profileCommandAdmission struct {
	epoch        commandsecurity.EpochSnapshot
	revalidate   func(context.Context) error
	claim        func(context.Context) error
	transitioned bool
}

func (a *App) commitProfilePageMutation(ctx context.Context, p *commandProductRuntime, run *commandUIRun) error {
	p.mu.Lock()
	prep, admitted := run.pageMutation, run.admission
	if prep == nil || prep.profileMutation == nil || prep.ownership == nil || admitted == nil {
		p.mu.Unlock()
		return commandexecution.ErrDenied
	}
	prepared := prep.profileMutation
	admission := &profileCommandAdmission{epoch: admitted.epoch, revalidate: func(ctx context.Context) error { return a.validatePageMutationOrigin(ctx, p, run) }}
	admission.claim = func(ctx context.Context) error { return a.claimProfileCommand(ctx, p, run) }
	p.mu.Unlock()
	var result CommandPageMutationResult
	_, err := a.coordinateProfileMutation(database.WithUserID(ctx, p.principal.UserID), prepared.Mutation, func(slug string) error {
		if err := prepared.Publish(slug); err != nil {
			return err
		}
		result.ID = slug
		if run.reservation.CommandID != "profiles.delete" {
			value, err := prep.profileManager.Get(slug)
			if err != nil {
				return err
			}
			if value != nil {
				result.Title = value.Name
			}
		}
		p.mu.Lock()
		prep.result = result
		p.mu.Unlock()
		return nil
	}, admission, p)
	p.mu.Lock()
	prep.rebuildAfterCommit = admission.transitioned && !errors.Is(err, profiles.ErrCommandMutationOutcomeUnknown)
	p.mu.Unlock()
	if errors.Is(err, profiles.ErrCommandMutationOutcomeUnknown) {
		return errors.Join(commandui.ErrOutcomeUnknown, err)
	}
	return err
}

func (a *App) coordinateProfileMutation(ctx context.Context, mutation *profiles.CommandMutation, publish func(string) error, admission *profileCommandAdmission, product *commandProductRuntime) (string, error) {
	if a == nil || ctx == nil || mutation == nil || publish == nil {
		return "", commandexecution.ErrInvalidConfiguration
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return "", err
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil || principal.UserID != userID {
		return "", commandexecution.ErrDenied
	}
	// Serializa com login/logout e impede que callbacks sejam publicados em
	// nome de uma sessão diferente daquela que solicitou a mutação.
	a.authSessionMu.Lock()
	defer a.authSessionMu.Unlock()
	current, err := a.currentCommandPrincipal()
	if err != nil || current != principal {
		return "", commandexecution.ErrStale
	}
	a.authMu.RLock()
	if a.currentAuthUser == nil || a.currentAuthUser.UserID != principal.UserID || a.currentAuthUser.SessionID != principal.SessionID {
		a.authMu.RUnlock()
		return "", commandexecution.ErrStale
	}
	sessions := a.sessionSvc
	host := a.commandHost
	owner := *a.currentAuthUser
	a.authMu.RUnlock()
	if sessions == nil {
		return "", commandexecution.ErrDenied
	}
	verified, err := sessions.RevalidateLocalSession(ctx, principal)
	if err != nil || verified != principal {
		return "", commandexecution.ErrDenied
	}
	epochs, err := a.commandSecurityService()
	if err != nil {
		return "", err
	}
	var finish func()
	if admission == nil {
		finish, err = epochs.BeginTransition(ctx)
	} else {
		if product == nil || product.principal != principal {
			return "", commandexecution.ErrStale
		}
		finish, err = epochs.BeginTransitionFromSnapshot(ctx, admission.epoch, admission.revalidate, admission.claim)
	}
	if err != nil {
		return "", err
	}
	defer finish()
	if admission != nil {
		admission.transitioned = true
		// A partir do claim o backend é dono do efeito: a invalidação acima
		// cancela outras admissões, mas não pode transformar o próprio commit
		// confirmado em cancelamento. O prazo segue estritamente limitado.
		var cancelCommit context.CancelFunc
		ctx, cancelCommit = context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
		defer cancelCommit()
	}
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	err = a.resetCommandLifecycleIfConfigured(cleanup, "profile_mutation")
	if host != nil {
		err = errors.Join(err, host.ForgetUserConfiguration(cleanup, principal.UserID))
	}
	cancel()
	if err != nil {
		return "", err
	}
	result, commitErr := a.profileAccessService().CommitProfileMutation(ctx, mutation)
	if commitErr == nil {
		if err := publish(result); err != nil {
			commitErr = errors.Join(profiles.ErrCommandMutationOutcomeUnknown, err)
		}
	}
	finish()
	// Não republicar sobre estado de disco indeterminado nem repetir escrita.
	// Erro de reconstrução conserva o efeito confirmado e o mapa indisponível.
	if admission == nil && !errors.Is(commitErr, profiles.ErrCommandMutationOutcomeUnknown) && a.commandLifecycle.Load() != nil {
		reload, cancelReload := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		a.bootstrapCommandLifecycleAfterAuth(reload, &owner, nil)
		cancelReload()
	}
	return result, commitErr
}

// Chamado sob o gate exclusivo. A leitura completa de projectionGuard ficou
// fora dele; aqui somente estado em memória é relido e mantido estável até o
// claim, fechando a janela entre preflight e aquisição do gate sem I/O.
func (a *App) claimProfileCommand(ctx context.Context, p *commandProductRuntime, run *commandUIRun) error {
	if ctx.Err() != nil || a.commandProduct.Load() != p || !p.dependenciesMatch(a) || run.sourceValid != nil && !run.sourceValid() {
		return commandexecution.ErrStale
	}
	p.mu.Lock()
	prep, expected, closed := run.pageMutation, run.snapshot, p.closed
	p.mu.Unlock()
	if closed || prep == nil || prep.ownership == nil {
		return commandexecution.ErrDenied
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.profileManager != prep.profileManager || a.profilesCtrl != prep.profileController || a.workspaceMgr != p.workspaceMgr || a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
		return commandexecution.ErrStale
	}
	return p.host.WithPublishedVersions(ctx, p.principal, prep.versions, func() error {
		return p.withContextualDeckPageSource(ctx, run, func() error {
			return p.workspaceMgr.WithCommandSnapshot(ctx, func(current workspace.CommandSnapshot) error {
				if current != expected {
					return commandexecution.ErrStale
				}
				return prep.ownership.Claim(ctx)
			})
		})
	})
}

// A recuperação do mapa exige ledger terminal: não reconstruir dentro do
// callback de commit enquanto a própria invocação ainda aparece como running.
func (a *App) rebuildAfterProfileCommand(p *commandProductRuntime, run *commandUIRun) {
	p.mu.Lock()
	rebuild := run.pageMutation != nil && run.pageMutation.rebuildAfterCommit
	p.mu.Unlock()
	if !rebuild || run.err != nil || run.result.Status == "outcome_unknown" {
		return
	}
	a.authSessionMu.Lock()
	defer a.authSessionMu.Unlock()
	principal, err := a.currentCommandPrincipal()
	if err != nil || principal != p.principal || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
		return
	}
	a.authMu.RLock()
	if a.currentAuthUser == nil {
		a.authMu.RUnlock()
		return
	}
	owner := *a.currentAuthUser
	a.authMu.RUnlock()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(a.commandBridgeContext()), 15*time.Second)
	defer cancel()
	verified, err := p.sessionSvc.RevalidateLocalSession(ctx, principal)
	if err != nil || verified != principal {
		return
	}
	a.bootstrapCommandLifecycleAfterAuth(ctx, &owner, nil)
}
