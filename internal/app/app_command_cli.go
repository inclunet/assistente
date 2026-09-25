package app

import (
	"context"
	"fmt"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcli"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandruntime"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
)

// NewCommandCLIApp seleciona o bootstrap curto do entrypoint antes do Startup.
// Restaurar uma sessão para consultar comandos não inicia jobs, canais, MCP
// automático, monitor LLM ou servidor HTTP em segundo plano.
func NewCommandCLIApp() *App {
	a := NewApp()
	a.commandCLIOnly = true
	return a
}

// NewCommandCLI é uma composição interna de entrypoint, não um método Wails.
// Restaura somente a sessão local já persistida pelo fluxo comum de autenticação;
// não recebe tokens, owner, grants ou confirmação textual da linha de comando.
func NewCommandCLI(a *App) (*commandcli.Service, error) {
	if a == nil {
		return nil, commandexecution.ErrDenied
	}
	a.authMu.RLock()
	authenticated := a.currentAuthUser != nil && a.currentUserID != ""
	a.authMu.RUnlock()
	if !authenticated {
		if _, err := a.RefreshAuth(RefreshRequest{}); err != nil {
			return nil, fmt.Errorf("sessão local indisponível; autentique-se no aplicativo e desbloqueie o cofre: %w", commandexecution.ErrDenied)
		}
	}
	p := a.commandProduct.Load()
	if p == nil || !p.dependenciesMatch(a) {
		return nil, commandruntime.ErrNotReady
	}
	if !authenticated {
		// A primeira observação do SO e a publicação do mapa são assíncronas.
		// Aguarda somente o bootstrap já iniciado; nunca infere desbloqueio.
		ctx, cancel := context.WithTimeout(a.commandBridgeContext(), 5*time.Second)
		err := waitCommandCLIReady(ctx, a, p)
		cancel()
		if err != nil {
			return nil, err
		}
	}
	base := p.agentConfig
	if base.Envelope == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	prepared, err := a.prepareCommandExecutor(base, p.host)
	if err != nil {
		return nil, err
	}
	principal := p.principal
	owner := desktopOwner(principal)
	authenticate := func(ctx context.Context) error {
		if ctx == nil {
			return commandexecution.ErrDenied
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if a.commandProduct.Load() != p || !p.dependenciesMatch(a) || !a.commandPrincipalMatches(prepared.sessions, prepared.manager, principal) {
			return commandexecution.ErrDenied
		}
		current, err := prepared.sessions.RevalidateLocalSession(ctx, principal)
		if err != nil || current != principal {
			return commandexecution.ErrDenied
		}
		var count int64
		if err := database.DB().WithContext(ctx).Table("users").Where("id = ? AND is_active = ? AND role IN ?", principal.UserID, true, []string{database.UserRoleUser, database.UserRoleAdmin}).Count(&count).Error; err != nil || count != 1 {
			return commandexecution.ErrDenied
		}
		ready, err := p.host.SourceSecurityReady(ctx)
		if err != nil || !ready {
			return commandexecution.ErrDenied
		}
		return nil
	}
	if err := authenticate(a.commandBridgeContext()); err != nil {
		return nil, err
	}
	config := prepared.config
	config.Source = commandcatalog.CLI
	envelope := *base.Envelope
	// Nenhum presenter é instalado nesta origem. O executor também recusa
	// decisões interativas em CLI; a ausência não significa aprovação tácita.
	envelope.Decisions = nil
	envelope.DecisionBody = nil
	envelope.Context = p.facts
	envelope.Identity = &commandexecution.EnvelopeIdentityPorts{
		Authenticate: func(ctx context.Context, token string) (commandexecution.EnvelopeAuthenticatedIdentity, error) {
			if token != "" {
				return commandexecution.EnvelopeAuthenticatedIdentity{}, commandexecution.ErrDenied
			}
			if err := authenticate(ctx); err != nil {
				return commandexecution.EnvelopeAuthenticatedIdentity{}, err
			}
			return commandexecution.EnvelopeAuthenticatedIdentity{
				Ownership:        owner,
				ContextPrincipal: commandsecurity.ContextPrincipal{UserID: principal.UserID, Type: string(commandcontract.AuthLocalSession), ID: principal.SessionID},
				WireSessionID:    stringPointer(principal.SessionID),
			}, nil
		},
		Snapshot: func(ctx context.Context, got commandledger.FullOwnership, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
			if !sameDesktopOwner(got, owner) || candidate.TriggerType != "" {
				return commandcontract.Envelope{}, commandexecution.ErrDenied
			}
			if err := authenticate(ctx); err != nil {
				return commandcontract.Envelope{}, err
			}
			return base.Envelope.Snapshot(ctx, principal, candidate)
		},
		Resolve: func(context.Context, commandledger.FullOwnership, commandexecution.EnvelopeCandidate, commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		},
		Authorize: func(ctx context.Context, got commandledger.FullOwnership, _ commandcontract.Envelope, definition commandcatalog.Definition) error {
			if !sameDesktopOwner(got, owner) || commandcli.UnavailableReason(definition) != "" {
				return commandexecution.ErrDenied
			}
			return authenticate(ctx)
		},
		AuthorizeLookup: func(ctx context.Context, got commandledger.FullOwnership, record commandledger.FullRecord) error {
			if !sameDesktopOwner(got, owner) || !commandRecordMatchesPrincipal(record, principal) || record.SourceType == nil || *record.SourceType != commandcontract.SourceCLI {
				return commandexecution.ErrDenied
			}
			if record.Envelope.SourceType != nil && *record.Envelope.SourceType != commandcontract.SourceCLI {
				return commandexecution.ErrDenied
			}
			return authenticate(ctx)
		},
	}
	config.Envelope = &envelope
	executor, err := commandexecution.NewComplete(config)
	if err != nil {
		return nil, err
	}
	service, err := commandcli.New(commandcli.Config{Registry: p.registry, Executor: executor, Authenticate: authenticate})
	if err != nil {
		closeUninstalledCommandService(executor, config.FinalizationTimeout)
		return nil, err
	}
	// O executor participa do core de epochs do App e é drenado no Shutdown.
	return service, nil
}

func waitCommandCLIReady(ctx context.Context, a *App, p *commandProductRuntime) error {
	if ctx == nil || a == nil || p == nil {
		return commandruntime.ErrNotReady
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: %w", commandruntime.ErrNotReady, err)
		}
		if a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
			return commandexecution.ErrDenied
		}
		v, err := p.host.Snapshot(ctx, p.principal)
		state, stateErr := CommandLifecycleSnapshot(a)
		if err == nil && stateErr == nil && v.Unlocked && state.State == commandruntime.StateReady && state.Published {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", commandruntime.ErrNotReady, ctx.Err())
		case <-ticker.C:
		}
	}
}
