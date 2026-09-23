package app

import (
	"context"
	"encoding/json"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	commandtool "assistente/internal/tools/command"
)

func (a *App) executeAgentCommand(ctx context.Context, c *commandAgentCaller, req commandtool.Request) (any, error) {
	p := c.product
	d, ok := p.registry.Lookup(req.CommandID)
	if !ok || !d.AllowsSource(commandcatalog.Chat) || (d.ID != commandProductWorkspaceListID && !isCommandLayerAction(d.ID)) {
		return nil, commandexecution.ErrDenied
	}
	base := p.agentConfig
	if base.Envelope == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	prepared, err := a.prepareCommandExecutor(base, p.host)
	if err != nil {
		return nil, err
	}
	config := prepared.config
	config.Source = commandcatalog.Chat
	config.Authorize = func(check context.Context, owner auth.LocalSessionPrincipal, id string, source commandcatalog.Source) error {
		if owner != c.principal || source != commandcatalog.Chat || id != d.ID {
			return commandexecution.ErrDenied
		}
		return c.revalidate(check)
	}
	envelope := *base.Envelope
	envelope.Identity = nil
	envelope.Context = p.facts
	envelope.Actor = func(check context.Context, owner auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error) {
		if owner != c.principal {
			return "", "", commandexecution.ErrDenied
		}
		return commandcontract.ActorAgent, c.invocationID, c.revalidate(check)
	}
	envelope.Snapshot = func(check context.Context, owner auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
		if candidate.CommandID != d.ID || candidate.TriggerType != "" {
			return commandcontract.Envelope{}, commandexecution.ErrDenied
		}
		if err := c.revalidate(check); err != nil {
			return commandcontract.Envelope{}, err
		}
		e, err := base.Envelope.Snapshot(check, owner, candidate)
		if err != nil {
			return e, err
		}
		e.ConversationID = stringPointer(c.conversationID)
		e.TurnID = stringPointer(c.turnID)
		e.SourceProfileSlug = stringPointer(c.profile)
		e.TargetProfileSlug = stringPointer(c.profile)
		e.WorkspaceID = stringPointer(p.workspaceID)
		e.WorkspaceConfigGeneration = e.GlobalConfigGeneration
		return e, nil
	}
	envelope.Authorize = func(check context.Context, owner auth.LocalSessionPrincipal, e commandcontract.Envelope, definition commandcatalog.Definition) error {
		if owner != c.principal || definition.ID != d.ID || e.ActorType != commandcontract.ActorAgent || e.ActorID != c.invocationID {
			return commandexecution.ErrDenied
		}
		return c.revalidate(check)
	}
	envelope.AuthorizeLookup = func(check context.Context, owner auth.LocalSessionPrincipal, r commandledger.FullRecord) error {
		if owner != c.principal || r.Envelope.ActorID != c.invocationID {
			return commandexecution.ErrDenied
		}
		return c.revalidate(check)
	}
	envelope.DecisionBody = func(definition commandcatalog.Definition, e commandcontract.Envelope) (string, error) {
		if definition.ID != d.ID || !isCommandLayerAction(d.ID) || e.Arguments == nil {
			return "", commandexecution.ErrDenied
		}
		metadata := definition.Presentation.Locales[req.Locale]
		if metadata.Name == "" {
			metadata = definition.Presentation.Locales[p.getDeckLocale()]
		}
		if metadata.Name == "" {
			metadata = definition.Presentation.Locales["en"]
		}
		return metadata.Name + "\n" + metadata.Description + "\n" + definition.ID + "\n" + string(*e.Arguments), nil
	}
	envelope.Decisions = nil
	if err := a.bindCommandInvocationDecisions(config, &envelope); err != nil {
		return nil, err
	}
	// O adapter autentica a chamada local do agente, não um token sintético
	// nem a identidade ActorUser usada pelos adapters físicos/da paleta.
	validOwner := func(owner commandledger.FullOwnership) bool {
		return owner.UserID != nil && *owner.UserID == c.principal.UserID && owner.AuthContextType == commandcontract.AuthLocalSession && owner.AuthContextID == c.principal.SessionID && owner.ActorType == commandcontract.ActorAgent && owner.ActorID == c.invocationID
	}
	envelope.Identity = &commandexecution.EnvelopeIdentityPorts{
		Authenticate: func(check context.Context, token string) (commandexecution.EnvelopeAuthenticatedIdentity, error) {
			principal, err := c.AuthenticateLocalAccess(check, token)
			if err != nil {
				return commandexecution.EnvelopeAuthenticatedIdentity{}, err
			}
			return commandexecution.EnvelopeAuthenticatedIdentity{
				Ownership:        commandledger.FullOwnership{UserID: stringPointer(principal.UserID), AuthContextType: commandcontract.AuthLocalSession, AuthContextID: principal.SessionID, ActorType: commandcontract.ActorAgent, ActorID: c.invocationID},
				ContextPrincipal: commandsecurity.ContextPrincipal{UserID: principal.UserID, Type: string(commandcontract.AuthLocalSession), ID: principal.SessionID}, WireSessionID: stringPointer(principal.SessionID),
			}, nil
		},
		Snapshot: func(check context.Context, owner commandledger.FullOwnership, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
			if !validOwner(owner) {
				return commandcontract.Envelope{}, commandexecution.ErrDenied
			}
			return envelope.Snapshot(check, c.principal, candidate)
		},
		Resolve: func(context.Context, commandledger.FullOwnership, commandexecution.EnvelopeCandidate, commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
			return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
		},
		Authorize: func(check context.Context, owner commandledger.FullOwnership, e commandcontract.Envelope, definition commandcatalog.Definition) error {
			if !validOwner(owner) {
				return commandexecution.ErrDenied
			}
			return envelope.Authorize(check, c.principal, e, definition)
		},
		AuthorizeLookup: func(check context.Context, owner commandledger.FullOwnership, r commandledger.FullRecord) error {
			if !validOwner(owner) {
				return commandexecution.ErrDenied
			}
			return envelope.AuthorizeLookup(check, c.principal, r)
		},
	}
	config.Envelope = &envelope
	service, err := commandexecution.NewComplete(config)
	if err != nil {
		return nil, err
	}
	defer closeUninstalledCommandService(service, 5*time.Second)
	args := req.Arguments
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	// Uma execução por tool invocation. Uma repetição do transporte consulta
	// o mesmo ledger em vez de produzir um segundo efeito ou outra confirmação.
	record, output, err := service.ExecuteEnvelopeWithResult(ctx, "", commandexecution.EnvelopeCandidate{InvocationID: c.invocationID, CorrelationID: c.invocationID, CommandID: d.ID, Arguments: args})
	if err != nil {
		return nil, err
	}
	return struct {
		CommandExecutionResult
		CommandID string          `json:"command_id"`
		Result    json.RawMessage `json:"result,omitempty"`
	}{commandProductResult(record), d.ID, output}, nil
}
