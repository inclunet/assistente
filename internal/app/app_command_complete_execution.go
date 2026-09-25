package app

import (
	"context"
	"errors"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/credentials"
	"assistente/internal/database"
)

// newCommandCompleteExecutor monta o executor integral somente a partir de
// portas fornecidas pelo bootstrap confiável. A fábrica é interna e não é
// chamada pelo startup de produto: a montagem do catálogo e dos handlers
// continua sendo uma decisão do principal.
//
// Esta instância é local_session. Não há conversão de uma identidade
// externa, system ou job em sessão local; essas origens exigem o adapter
// EnvelopeIdentity apropriado no executor principal.
func (a *App) newCommandCompleteExecutor(config commandexecution.Config, state *commandexecution.HostState) (*commandexecution.Service, error) {
	prepared, err := a.prepareCommandExecutor(config, state)
	if err != nil {
		return nil, err
	}
	if config.Envelope == nil || config.Envelope.Identity != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	if config.Registry == nil || !config.Registry.Complete() || len(config.Handlers) == 0 || config.Now == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	if config.Envelope.Snapshot == nil || config.Envelope.Resolve == nil || config.Envelope.Authorize == nil || config.Envelope.AuthorizeLookup == nil || config.Envelope.Actor == nil {
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

	envelope := *config.Envelope
	envelope.Identity = nil
	envelope.Context = facts
	envelope.Snapshot = a.guardCommandSnapshot(prepared.sessions, prepared.manager, state, config.Envelope.Snapshot)
	envelope.Resolve = a.guardCommandResolve(prepared.sessions, prepared.manager, config.Envelope.Resolve)
	envelope.Authorize = a.guardCommandAuthorize(prepared.sessions, prepared.manager, config.Envelope.Authorize)
	envelope.AuthorizeLookup = a.guardCommandAuthorizeLookup(prepared.sessions, prepared.manager, config.Envelope.AuthorizeLookup)
	envelope.Actor = a.guardCommandActor(prepared.sessions, prepared.manager, config.Envelope.Actor)

	if err := a.bindCommandInvocationDecisions(config, &envelope); err != nil {
		return nil, err
	}

	config = prepared.config
	config.Envelope = &envelope
	service, err := commandexecution.NewComplete(config)
	if err != nil {
		return nil, err
	}
	if err := a.installCommandHost(state); err != nil {
		closeUninstalledCommandService(service, config.FinalizationTimeout)
		return nil, err
	}
	return service, nil
}

// As duas fábricas locais usam o mesmo presenter do App e o mesmo banco do
// ledger. Nenhuma aceita store externo como atalho para uma confirmação.
// A montagem acontece fora do DispatchGate; apenas o consumo do receipt é
// atômico com a fila no executor comum.
func (a *App) bindCommandInvocationDecisions(config commandexecution.Config, envelope *commandexecution.EnvelopeConfig) error {
	if a == nil || envelope == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	if !hasInteractiveCommand(config.Registry) {
		return nil
	}
	a.authMu.RLock()
	manager := a.questionnaireMgr
	a.authMu.RUnlock()
	if envelope.DecisionTTL <= 0 || envelope.DecisionBody == nil || manager == nil || envelope.Decisions != nil || config.Now == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	decisionDB := database.DB()
	if decisionDB == nil || !decisionDB.Migrator().HasTable("command_decision_receipts") || !decisionDB.Migrator().HasTable("command_decision_receipt_events") || config.Store == nil || !config.Store.UsesDatabase(decisionDB) {
		return commandexecution.ErrInvalidConfiguration
	}
	store, err := commanddecision.New(decisionDB, &commandDecisionPresenter{manager: manager}, config.Now)
	if err != nil {
		return commandexecution.ErrInvalidConfiguration
	}
	envelope.Decisions = store
	return nil
}

func (a *App) currentCommandPrincipal() (auth.LocalSessionPrincipal, error) {
	if a == nil {
		return auth.LocalSessionPrincipal{}, commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.currentAuthUser == nil || a.currentUserID == "" || a.currentAuthUser.UserID != a.currentUserID || a.currentAuthUser.SessionID == "" {
		return auth.LocalSessionPrincipal{}, commandexecution.ErrInvalidConfiguration
	}
	principal := auth.LocalSessionPrincipal{UserID: a.currentAuthUser.UserID, SessionID: a.currentAuthUser.SessionID}
	if err := validateCommandPrincipal(principal); err != nil {
		return auth.LocalSessionPrincipal{}, commandexecution.ErrInvalidConfiguration
	}
	return principal, nil
}

func hasInteractiveCommand(registry *commandcatalog.Registry) bool {
	if registry == nil {
		return false
	}
	for _, definition := range registry.List() {
		if definition.Decision == commandcatalog.Interactive {
			return true
		}
	}
	return false
}

func guardCommandContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("contexto nil")
	}
	return ctx.Err()
}

func (a *App) guardCommandSnapshot(sessions *auth.SessionService, manager *credentials.Manager, state *commandexecution.HostState, callback func(context.Context, auth.LocalSessionPrincipal, commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error)) func(context.Context, auth.LocalSessionPrincipal, commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
	return func(ctx context.Context, principal auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
		if err := guardCommandContext(ctx); err != nil {
			return commandcontract.Envelope{}, err
		}
		if !a.commandPrincipalMatches(sessions, manager, principal) {
			return commandcontract.Envelope{}, commandexecution.ErrDenied
		}
		before, err := state.Snapshot(ctx, principal)
		if err != nil {
			return commandcontract.Envelope{}, err
		}
		if !before.Unlocked {
			return commandcontract.Envelope{}, commandexecution.ErrDenied
		}
		envelope, err := callback(ctx, principal, candidate)
		if err != nil {
			return commandcontract.Envelope{}, err
		}
		after, err := state.Snapshot(ctx, principal)
		if err != nil || !after.Unlocked || before != after {
			if err != nil {
				return commandcontract.Envelope{}, err
			}
			return commandcontract.Envelope{}, commandexecution.ErrDenied
		}
		// Snapshot is a host projection, not an authorization callback. Still,
		// the generation fields selecting the executable state must come from the
		// same HostState that gates the local executor; a supplied callback cannot
		// forge an unlocked/old projection.
		envelope.RegistryVersion = after.Registry
		envelope.GlobalConfigGeneration = commandStringPointer(after.GlobalConfig)
		envelope.ActiveLayersGeneration = commandStringPointer(after.ActiveLayers)
		if !a.commandPrincipalMatches(sessions, manager, principal) {
			return commandcontract.Envelope{}, commandexecution.ErrDenied
		}
		return envelope, nil
	}
}

func commandStringPointer(value string) *string {
	copy := value
	return &copy
}

func (a *App) guardCommandResolve(sessions *auth.SessionService, manager *credentials.Manager, callback func(context.Context, auth.LocalSessionPrincipal, commandexecution.EnvelopeCandidate, commandcontract.Envelope) (commandexecution.EnvelopeResolution, error)) func(context.Context, auth.LocalSessionPrincipal, commandexecution.EnvelopeCandidate, commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
	return func(ctx context.Context, principal auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate, envelope commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
		if err := guardCommandContext(ctx); err != nil {
			return commandexecution.EnvelopeResolution{}, err
		}
		if !a.commandPrincipalMatches(sessions, manager, principal) {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		resolution, err := callback(ctx, principal, candidate, envelope)
		if err != nil {
			return commandexecution.EnvelopeResolution{}, err
		}
		if !a.commandPrincipalMatches(sessions, manager, principal) {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		}
		return resolution, nil
	}
}

func (a *App) guardCommandAuthorize(sessions *auth.SessionService, manager *credentials.Manager, callback func(context.Context, auth.LocalSessionPrincipal, commandcontract.Envelope, commandcatalog.Definition) error) func(context.Context, auth.LocalSessionPrincipal, commandcontract.Envelope, commandcatalog.Definition) error {
	return func(ctx context.Context, principal auth.LocalSessionPrincipal, envelope commandcontract.Envelope, definition commandcatalog.Definition) error {
		if err := guardCommandContext(ctx); err != nil {
			return err
		}
		if !a.commandPrincipalMatches(sessions, manager, principal) {
			return commandexecution.ErrDenied
		}
		if err := callback(ctx, principal, envelope, definition); err != nil {
			return err
		}
		if !a.commandPrincipalMatches(sessions, manager, principal) {
			return commandexecution.ErrDenied
		}
		return nil
	}
}

func (a *App) guardCommandAuthorizeLookup(sessions *auth.SessionService, manager *credentials.Manager, callback func(context.Context, auth.LocalSessionPrincipal, commandledger.FullRecord) error) func(context.Context, auth.LocalSessionPrincipal, commandledger.FullRecord) error {
	return func(ctx context.Context, principal auth.LocalSessionPrincipal, record commandledger.FullRecord) error {
		if err := guardCommandContext(ctx); err != nil {
			return err
		}
		if !a.commandPrincipalMatches(sessions, manager, principal) || !commandRecordMatchesPrincipal(record, principal) {
			return commandexecution.ErrDenied
		}
		if err := callback(ctx, principal, record); err != nil {
			return err
		}
		if !a.commandPrincipalMatches(sessions, manager, principal) {
			return commandexecution.ErrDenied
		}
		return nil
	}
}

func (a *App) guardCommandActor(sessions *auth.SessionService, manager *credentials.Manager, callback func(context.Context, auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error)) func(context.Context, auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error) {
	return func(ctx context.Context, principal auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error) {
		if err := guardCommandContext(ctx); err != nil {
			return "", "", err
		}
		if !a.commandPrincipalMatches(sessions, manager, principal) {
			return "", "", commandexecution.ErrDenied
		}
		actor, id, err := callback(ctx, principal)
		if err != nil {
			return "", "", err
		}
		// A local-session App does not turn a callback's arbitrary actor into
		// authority. Agent/automation actors need their own identity adapter.
		if actor != commandcontract.ActorUser || id != principal.UserID || !a.commandPrincipalMatches(sessions, manager, principal) {
			return "", "", commandexecution.ErrDenied
		}
		return actor, id, nil
	}
}

func commandRecordMatchesPrincipal(record commandledger.FullRecord, principal auth.LocalSessionPrincipal) bool {
	owner := record.Ownership
	return owner.UserID != nil && *owner.UserID == principal.UserID && owner.AuthContextType == commandcontract.AuthLocalSession && owner.AuthContextID == principal.SessionID && owner.ActorType == commandcontract.ActorUser && owner.ActorID == principal.UserID
}
