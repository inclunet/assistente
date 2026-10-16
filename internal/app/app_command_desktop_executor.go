package app

import (
	"context"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
)

// newCommandDesktopExecutor monta o executor completo para a sessão desktop
// local. A identidade é capturada pelo App e revalidada pelo SessionService;
// nenhum token ou identidade recebida pelo bridge entra nesta porta.
//
// A fábrica não instala HostState. A publicação atômica de host, bridge e
// controller pertence ao bootstrap do lifecycle.
func (a *App) newCommandDesktopExecutor(config commandexecution.Config, state *commandexecution.HostState) (*commandexecution.Service, error) {
	prepared, err := a.prepareCommandExecutor(config, state)
	if err != nil {
		return nil, err
	}
	if config.Envelope == nil || config.Envelope.Identity != nil || config.Registry == nil ||
		!config.Registry.Complete() || len(config.Handlers) == 0 || config.Now == nil ||
		config.Envelope.Snapshot == nil || config.Envelope.Resolve == nil ||
		config.Envelope.Authorize == nil || config.Envelope.AuthorizeLookup == nil ||
		config.Envelope.Actor == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}

	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return nil, err
	}
	facts, err := a.newCommandFactBus(principal)
	if err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}

	localOwner := desktopOwner(principal)
	ownerPrincipal := func(owner commandledger.FullOwnership) (auth.LocalSessionPrincipal, error) {
		if !sameDesktopOwner(owner, localOwner) {
			return auth.LocalSessionPrincipal{}, commandexecution.ErrDenied
		}
		return principal, nil
	}
	identity := &commandexecution.EnvelopeIdentityPorts{
		Authenticate: func(ctx context.Context, token string) (commandexecution.EnvelopeAuthenticatedIdentity, error) {
			if token != "" || !a.commandPrincipalMatches(prepared.sessions, prepared.manager, principal) {
				return commandexecution.EnvelopeAuthenticatedIdentity{}, commandexecution.ErrDenied
			}
			revalidated, revalidateErr := prepared.sessions.RevalidateLocalSession(ctx, principal)
			if revalidateErr != nil || revalidated != principal || !a.commandPrincipalMatches(prepared.sessions, prepared.manager, principal) {
				return commandexecution.EnvelopeAuthenticatedIdentity{}, commandexecution.ErrDenied
			}
			return commandexecution.EnvelopeAuthenticatedIdentity{
				Ownership:        localOwner,
				ContextPrincipal: commandsecurity.ContextPrincipal{UserID: principal.UserID, Type: string(commandcontract.AuthLocalSession), ID: principal.SessionID},
				WireSessionID:    stringPointer(principal.SessionID),
			}, nil
		},
		Snapshot: func(ctx context.Context, owner commandledger.FullOwnership, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
			p, err := ownerPrincipal(owner)
			if err != nil {
				return commandcontract.Envelope{}, err
			}
			return a.guardCommandSnapshot(prepared.sessions, prepared.manager, state, config.Envelope.Snapshot)(ctx, p, candidate)
		},
		Resolve: func(ctx context.Context, owner commandledger.FullOwnership, candidate commandexecution.EnvelopeCandidate, envelope commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
			p, err := ownerPrincipal(owner)
			if err != nil {
				return commandexecution.EnvelopeResolution{}, err
			}
			return a.guardCommandResolve(prepared.sessions, prepared.manager, config.Envelope.Resolve)(ctx, p, candidate, envelope)
		},
		Authorize: func(ctx context.Context, owner commandledger.FullOwnership, envelope commandcontract.Envelope, definition commandcatalog.Definition) error {
			p, err := ownerPrincipal(owner)
			if err != nil {
				return err
			}
			return a.guardCommandAuthorize(prepared.sessions, prepared.manager, config.Envelope.Authorize)(ctx, p, envelope, definition)
		},
		AuthorizeLookup: func(ctx context.Context, owner commandledger.FullOwnership, record commandledger.FullRecord) error {
			p, err := ownerPrincipal(owner)
			if err != nil {
				return err
			}
			return a.guardCommandAuthorizeLookup(prepared.sessions, prepared.manager, config.Envelope.AuthorizeLookup)(ctx, p, record)
		},
	}

	envelope := *config.Envelope
	envelope.Identity = identity
	envelope.Context = facts
	if err := a.bindCommandInvocationDecisions(config, &envelope); err != nil {
		return nil, err
	}
	config = prepared.config
	config.Envelope = &envelope
	service, err := commandexecution.NewComplete(config)
	if err != nil {
		return nil, err
	}
	return service, nil
}

func desktopOwner(principal auth.LocalSessionPrincipal) commandledger.FullOwnership {
	userID := principal.UserID
	return commandledger.FullOwnership{
		UserID: &userID, AuthContextType: commandcontract.AuthLocalSession,
		AuthContextID: principal.SessionID, ActorType: commandcontract.ActorUser, ActorID: principal.UserID,
	}
}

func sameDesktopOwner(got, want commandledger.FullOwnership) bool {
	return got.UserID != nil && want.UserID != nil && *got.UserID == *want.UserID &&
		got.AuthContextType == want.AuthContextType && got.AuthContextID == want.AuthContextID &&
		got.ActorType == want.ActorType && got.ActorID == want.ActorID
}

func stringPointer(value string) *string {
	copy := value
	return &copy
}
