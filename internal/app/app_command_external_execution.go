package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbridge"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandidentity"
	"assistente/internal/commandledger"
	"assistente/internal/commandui"
	"assistente/internal/config"
	"assistente/internal/database"
)

const externalCommandExecuteScope = "commands:execute"

const (
	externalCommandHTTPExecutionTimeout = 5 * time.Minute
	// 30s cobrem até 10s de validação/JWKS, 5s de finalização e 15s de
	// serialização/agendamento; a margem continua limitada e separada do
	// timeout de execução do comando.
	externalCommandHTTPWriteMargin = 30 * time.Second
)

func externalCommandAuthorizationRules(definitions []commandcatalog.Definition) []commandidentity.AuthorizationRule {
	rules := make([]commandidentity.AuthorizationRule, 0, len(definitions))
	for _, definition := range definitions {
		if !definition.AllowsSource(commandcatalog.UI) {
			continue
		}
		switch definition.HandlerClassification {
		case commandcatalog.HandlerUI:
			if !commandExternalUICommandSupported(definition.ID) {
				continue
			}
		case commandcatalog.HandlerBackend:
			// A superfície externa backend é intencionalmente uma allowlist:
			// não herda novas operações só porque o catálogo as publica para UI.
			if definition.ID != commandProductWorkspaceListID {
				continue
			}
		default:
			continue
		}
		rules = append(rules, commandidentity.AuthorizationRule{
			CommandID:      definition.ID,
			Actors:         []commandcontract.ActorType{commandcontract.ActorUser},
			RequiredRoles:  []string{database.UserRoleUser, database.UserRoleAdmin},
			RequiredScopes: []string{externalCommandExecuteScope},
		})
	}
	return rules
}

func hasConfiguredExternalCommandScope(cfg *config.AuthConfig) bool {
	if cfg == nil || cfg.Mode != "external" {
		return false
	}
	for _, scope := range cfg.External.RequiredScopes {
		if scope == externalCommandExecuteScope {
			return true
		}
	}
	return false
}

// newExternalUICommandExecutor reaproveita catálogo, handlers, ledger, épocas e
// a projeção já publicada para o mesmo usuário. O principal externo continua
// dono do envelope; a sessão local só prova que essa projeção está viva.
func (a *App) newExternalUICommandExecutor(p *commandProductRuntime, authenticator *auth.ExternalCommandAuthenticator, admin *auth.ExternalIdentityAdminService, cfg *config.AuthConfig) (*commandexecution.ExternalService, error) {
	if a == nil || p == nil || authenticator == nil || admin == nil || !hasConfiguredExternalCommandScope(cfg) ||
		a.commandProduct.Load() != p || !p.dependenciesMatch(a) || p.registry == nil || p.agentConfig.Envelope == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	prepared, err := a.prepareCommandExecutor(p.agentConfig, p.host)
	if err != nil {
		return nil, err
	}
	config := prepared.config
	config.Source = commandcatalog.UI
	// A rota HTTP tem deadline individual limitado a este timeout mais uma
	// margem de serialização; o cancelamento do request continua cancelando o
	// executor (sem goroutine detached). Decisões mantêm o TTL comum de 5 min.
	config.ExecutionTimeout = externalCommandHTTPExecutionTimeout
	manager := a.ensureExternalUIConnections()
	manager.SetReadyNotifier(func(event commandui.ExternalUIReadyEvent) {
		if a.emitter != nil {
			a.emitter.Emit("external:command:ready", event)
		}
	})
	ownerFor := func(ctx context.Context, userID string) (commandbridge.Owner, error) {
		if userID == "" || userID != p.principal.UserID || a.commandProduct.Load() != p || !p.dependenciesMatch(a) ||
			!a.commandPrincipalMatches(prepared.sessions, prepared.manager, p.principal) {
			return commandbridge.Owner{}, commandexecution.ErrDenied
		}
		if ctx == nil || ctx.Err() != nil {
			if ctx != nil {
				return commandbridge.Owner{}, ctx.Err()
			}
			return commandbridge.Owner{}, commandexecution.ErrDenied
		}
		principal, err := prepared.sessions.RevalidateLocalSession(ctx, p.principal)
		if err != nil {
			return commandbridge.Owner{}, err
		}
		if principal != p.principal {
			return commandbridge.Owner{}, commandexecution.ErrDenied
		}
		versions, err := p.host.Snapshot(ctx, p.principal)
		if err != nil {
			return commandbridge.Owner{}, err
		}
		if !versions.Unlocked || versions.Registry != p.agentConfig.RegistryVersion {
			return commandbridge.Owner{}, commandexecution.ErrStale
		}
		return p.owner(), nil
	}
	externalPrincipal := func(principal commandexecution.ExternalUIPrincipal) commandui.ExternalUIPrincipal {
		return commandui.ExternalUIPrincipal{Issuer: principal.Issuer, Subject: principal.Subject, UserID: principal.UserID, AuthContextID: principal.AuthContextID}
	}
	validateBinding := func(ctx context.Context, principal commandexecution.ExternalUIPrincipal, binding commandexecution.ExternalUIBinding) error {
		if ctx == nil || ctx.Err() != nil {
			return commandexecution.ErrDenied
		}
		owner, err := ownerFor(ctx, principal.UserID)
		if err != nil {
			return err
		}
		identity := externalPrincipal(principal)
		if err := manager.Validate(binding.ConnectionID, binding.Generation, identity, binding.TargetSnapshotID, binding.ContextVersion); err != nil {
			return commandexecution.ErrStale
		}
		status, err := manager.ReadForPrincipal(identity, binding.ConnectionID, binding.Generation)
		if err != nil || status.Owner != owner || status.TargetSnapshotID != binding.TargetSnapshotID || status.ContextVersion != binding.ContextVersion || status.Target.WorkspaceID != owner.WorkspaceID {
			return commandexecution.ErrStale
		}
		return nil
	}
	facts, err := commandcontext.NewFactBus(map[string]commandcontext.ScopedProvider{
		"workspace": externalUIContextProvider{manager: manager, principal: externalPrincipal, validate: validateBinding},
	})
	if err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	config.ExternalUI = &commandexecution.ExternalUIHooks{
		Validate: validateBinding,
		Start: func(ctx context.Context, principal commandexecution.ExternalUIPrincipal, binding commandexecution.ExternalUIBinding, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
			if a.emitter == nil {
				return commandexecution.ExecutionHandle{}, commandexecution.ErrDenied
			}
			if err := validateBinding(ctx, principal, binding); err != nil {
				return commandexecution.ExecutionHandle{}, err
			}
			if invocation.Envelope == nil || invocation.Envelope.CommandID == nil || invocation.Envelope.Arguments == nil {
				return commandexecution.ExecutionHandle{}, commandexecution.ErrDenied
			}
			definition, ok := p.registry.Lookup(invocation.CommandID)
			if !ok || definition.HandlerClassification != commandcatalog.HandlerUI || !definition.AllowsSource(commandcatalog.UI) || !commandExternalUICommandSupported(definition.ID) {
				return commandexecution.ExecutionHandle{}, commandexecution.ErrDenied
			}
			owner, err := ownerFor(ctx, principal.UserID)
			if err != nil {
				return commandexecution.ExecutionHandle{}, err
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.closed || a.commandProduct.Load() != p {
				return commandexecution.ExecutionHandle{}, commandexecution.ErrStale
			}
			return manager.StartExternal(owner, externalPrincipal(principal), binding.ConnectionID, binding.Generation,
				binding.TargetSnapshotID, binding.ContextVersion, invocation.ID, invocation.CommandID, *invocation.Envelope.Arguments)
		},
	}

	identity := &commandexecution.EnvelopeIdentityPorts{}
	identity.Snapshot = func(ctx context.Context, owner commandledger.FullOwnership, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
		if owner.UserID == nil || candidate.CommandID == "" || candidate.TriggerType != "" || candidate.WorkspaceID != nil {
			return commandcontract.Envelope{}, commandexecution.ErrDenied
		}
		uiPrincipal, binding, linked := commandexecution.ExternalUIBindingFromContext(ctx)
		if linked {
			if err := validateBinding(ctx, uiPrincipal, binding); err != nil {
				return commandcontract.Envelope{}, err
			}
		}
		if _, err := ownerFor(ctx, *owner.UserID); err != nil {
			return commandcontract.Envelope{}, err
		}
		versions, err := p.host.Snapshot(ctx, p.principal)
		if err != nil || !versions.Unlocked {
			return commandcontract.Envelope{}, commandexecution.ErrStale
		}
		envelope := commandcontract.Envelope{
			RegistryVersion:        versions.Registry,
			GlobalConfigGeneration: externalStringPointer(versions.GlobalConfig),
			ActiveLayersGeneration: externalStringPointer(versions.ActiveLayers),
		}
		if linked {
			status, err := manager.ReadForPrincipal(externalPrincipal(uiPrincipal), binding.ConnectionID, binding.Generation)
			if err != nil || status.Target.WorkspaceID != p.workspaceID || status.Owner != p.owner() {
				return commandcontract.Envelope{}, commandexecution.ErrStale
			}
			envelope.WorkspaceID = externalStringPointer(status.Target.WorkspaceID)
			envelope.WorkspaceConfigGeneration = externalStringPointer(versions.GlobalConfig)
			envelope.SurfaceType = externalStringPointer(status.Target.Surface.SurfaceType)
			envelope.SurfaceID = externalStringPointer(status.Target.Surface.SurfaceID)
			envelope.SurfaceSnapshotVersion = externalStringPointer(status.Target.Surface.SnapshotVersion)
			provenance, err := json.Marshal(map[string]any{
				"version": 1,
				"external_ui": map[string]string{
					"connection_id": binding.ConnectionID, "generation": binding.Generation,
					"target_snapshot_id": binding.TargetSnapshotID, "context_version": binding.ContextVersion,
				},
			})
			if err != nil {
				return commandcontract.Envelope{}, commandexecution.ErrExecution
			}
			hostProvenance := json.RawMessage(provenance)
			envelope.Provenance = &hostProvenance
		}
		return envelope, nil
	}
	identity.Resolve = func(context.Context, commandledger.FullOwnership, commandexecution.EnvelopeCandidate, commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
		return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
	}
	identity.Authorize = func(ctx context.Context, owner commandledger.FullOwnership, envelope commandcontract.Envelope, definition commandcatalog.Definition) error {
		if owner.UserID == nil || definition.ID == "" || !definition.AllowsSource(commandcatalog.UI) {
			return commandexecution.ErrDenied
		}
		if definition.HandlerClassification == commandcatalog.HandlerUI && !commandExternalUICommandSupported(definition.ID) {
			return commandexecution.ErrDenied
		}
		if definition.HandlerClassification != commandcatalog.HandlerUI && definition.HandlerClassification != commandcatalog.HandlerBackend {
			return commandexecution.ErrDenied
		}
		if _, err := ownerFor(ctx, *owner.UserID); err != nil {
			return err
		}
		if !definition.Context.None {
			_, _, linked := commandexecution.ExternalUIBindingFromContext(ctx)
			if !linked {
				return commandexecution.ErrDenied
			}
		}
		return nil
	}
	identity.AuthorizeLookup = func(ctx context.Context, owner commandledger.FullOwnership, record commandledger.FullRecord) error {
		if owner.UserID == nil || record.Envelope.CommandID == nil || record.SourceType == nil || *record.SourceType != commandcontract.SourceUI {
			return commandexecution.ErrDenied
		}
		if _, err := ownerFor(ctx, *owner.UserID); err != nil {
			return err
		}
		definition, ok := p.registry.Lookup(*record.Envelope.CommandID)
		if !ok || !definition.AllowsSource(commandcatalog.UI) ||
			(definition.HandlerClassification == commandcatalog.HandlerUI && !commandExternalUICommandSupported(definition.ID)) ||
			(definition.HandlerClassification != commandcatalog.HandlerUI && definition.HandlerClassification != commandcatalog.HandlerBackend) {
			return commandexecution.ErrDenied
		}
		return nil
	}
	envelope := &commandexecution.EnvelopeConfig{
		Identity: identity, Context: facts, DecisionTTL: 5 * time.Minute,
		DecisionBody: config.Envelope.DecisionBody,
	}
	if hasInteractiveCommand(config.Registry) {
		if err := a.bindCommandInvocationDecisions(config, envelope); err != nil {
			return nil, err
		}
	}
	config.Envelope = envelope
	rules := externalCommandAuthorizationRules(p.registry.List())
	service, err := commandexecution.NewExternal(config, authenticator, admin, rules)
	if err != nil {
		return nil, err
	}
	return service, nil
}

// externalUIContextProvider nunca consulta a aba local. Cada captura e
// revalidação relê a conexão e exige os quatro stamps do request autenticado.
type externalUIContextProvider struct {
	manager   *commandui.ExternalUIConnections
	principal func(commandexecution.ExternalUIPrincipal) commandui.ExternalUIPrincipal
	validate  func(context.Context, commandexecution.ExternalUIPrincipal, commandexecution.ExternalUIBinding) error
}

func (p externalUIContextProvider) Snapshot(ctx context.Context, scope commandcontext.Scope, fact string) (commandcontext.OwnedSnapshot, error) {
	if ctx == nil || p.manager == nil || p.principal == nil || p.validate == nil || fact != "active_tab" || scope.WorkspaceID == nil {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	principal, binding, ok := commandexecution.ExternalUIBindingFromContext(ctx)
	if !ok || scope.UserID != principal.UserID || scope.AuthContextID != principal.AuthContextID {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}
	if err := p.validate(ctx, principal, binding); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	status, err := p.manager.ReadForPrincipal(p.principal(principal), binding.ConnectionID, binding.Generation)
	if err != nil || status.State != "connected" || status.Target.WorkspaceID != *scope.WorkspaceID ||
		status.Target.TabID == "" || status.Target.Surface.SurfaceType == "" || status.Target.Surface.SurfaceID == "" ||
		status.Target.Surface.SnapshotVersion == "" || status.TargetSnapshotID != binding.TargetSnapshotID || status.ContextVersion != binding.ContextVersion {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}
	// ContextFact/ExactVersion contracts only a version witness, not a payload
	// value. Bind it to the live connection's actual active-tab identity and
	// revision; the full surface remains in the authorized envelope projection.
	versionInput, err := json.Marshal([]string{
		status.Target.WorkspaceID, status.Target.TabID, status.Target.Surface.SurfaceType,
		status.Target.Surface.SurfaceID, status.Target.Surface.SnapshotVersion,
		binding.TargetSnapshotID, binding.ContextVersion,
	})
	if err != nil {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	versionDigest := sha256.Sum256(versionInput)
	version := hex.EncodeToString(versionDigest[:])
	return commandcontext.NewOwnedSnapshot(scope, commandcontext.Snapshot{Version: version})
}

func externalStringPointer(value string) *string {
	copy := value
	return &copy
}
