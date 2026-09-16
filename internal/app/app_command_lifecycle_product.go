package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandbridge"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"github.com/google/uuid"
)

const commandLifecycleSentinelID = "lifecycle.ready"
const commandLifecycleRegistryVersion = "lifecycle-v1"
const commandLifecycleBuiltinLayerID = "lifecycle.builtin"

type commandLifecycleSentinelAdapter struct{}

func (commandLifecycleSentinelAdapter) Dispatch(context.Context, commandbridge.Invocation) (commandbridge.InvocationAck, error) {
	return commandbridge.InvocationAck{}, commandbridge.ErrCapabilityDenied
}

func (commandLifecycleSentinelAdapter) Cancel(context.Context, commandbridge.CancelRequest) error {
	return commandbridge.ErrUnknownInvocation
}

// ensureCommandLifecycleMountedForCurrentUser monta a base produtiva mínima
// pós-auth. Ela publica somente um comando sentinel interno, suficiente para
// provar catálogo/store/bridge/providers/adapters sem migrar handlers legados.
func (a *App) ensureCommandLifecycleMountedForCurrentUser(ctx context.Context) error {
	if _, ok := loadCommandLifecycle(a); ok {
		return nil
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return err
	}
	epochs, err := a.commandSecurityService()
	if err != nil {
		return err
	}
	state, err := commandexecution.NewHostState(epochs, commandLifecycleRegistryVersion)
	if err != nil {
		return err
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		return err
	}
	facts, err := a.newCommandFactBus(principal)
	if err != nil {
		return err
	}
	store, err := commandledger.New(database.DB(), time.Now)
	if err != nil {
		return err
	}
	registry, handlers, err := commandLifecycleSentinelCatalog()
	if err != nil {
		return err
	}
	bridge, err := commandbridge.New(commandbridge.Config{
		Port: commandLifecycleSentinelAdapter{},
		Capabilities: []commandbridge.Capability{{
			ID: uuid.Must(uuid.NewV7()).String(), CommandID: commandLifecycleSentinelID, Generation: 1,
			Owner: commandbridge.Owner{UserID: principal.UserID, SessionID: principal.SessionID, WorkspaceID: "lifecycle"},
		}},
	})
	if err != nil {
		return err
	}
	inputs := CommandLifecycleMountInputs{
		Execution: commandexecution.Config{
			Envelope: &commandexecution.EnvelopeConfig{
				Context: facts,
				Snapshot: func(ctx context.Context, principal auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
					versions, err := state.Snapshot(ctx, principal)
					if err != nil {
						return commandcontract.Envelope{}, err
					}
					return commandcontract.Envelope{RegistryVersion: versions.Registry, GlobalConfigGeneration: &versions.GlobalConfig, ActiveLayersGeneration: &versions.ActiveLayers, CorrelationID: candidate.CorrelationID}, nil
				},
				Resolve: func(context.Context, auth.LocalSessionPrincipal, commandexecution.EnvelopeCandidate, commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
					return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
				},
				Authorize: func(context.Context, auth.LocalSessionPrincipal, commandcontract.Envelope, commandcatalog.Definition) error {
					return commandexecution.ErrDenied
				},
				AuthorizeLookup: func(context.Context, auth.LocalSessionPrincipal, commandledger.FullRecord) error {
					return commandexecution.ErrDenied
				},
				Actor: func(context.Context, auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error) {
					return commandcontract.ActorUser, principal.UserID, nil
				},
			},
			Epochs:          epochs,
			Store:           store,
			Registry:        registry,
			RegistryVersion: commandLifecycleRegistryVersion,
			Source:          commandcatalog.KeyboardLocal,
			Authorize: func(context.Context, auth.LocalSessionPrincipal, string, commandcatalog.Source) error {
				return commandexecution.ErrDenied
			},
			KeyVersion:          "v1",
			Now:                 time.Now,
			Retention:           time.Minute,
			ExecutionTimeout:    time.Second,
			FinalizationTimeout: time.Second,
			Handlers:            handlers,
		},
		Host:    state,
		Bridge:  bridge,
		Facts:   facts,
		Adapter: commandLifecycleSentinelAdapter{},
	}
	if err := ConfigureCommandLifecycleForApp(a, inputs); err != nil {
		return err
	}
	a.authMu.Lock()
	a.commandRegistry = registry
	a.startCommandOSSessionMonitorLocked()
	a.authMu.Unlock()
	a.wireCommandCatalog()
	return nil
}

func (a *App) rebuildCommandLifecycleSentinelConfiguration(ctx context.Context) error {
	if a == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	state := a.commandHost
	a.authMu.RUnlock()
	if state == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	configuration, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		return err
	}
	return state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return a.currentCommandPrincipal()
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	})
}

func (a *App) rebuildCommandLifecyclePersistedConfiguration(ctx context.Context) error {
	if a == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return err
	}
	registry, _, err := commandLifecycleSentinelCatalog()
	if err != nil {
		return err
	}
	options := commandconfig.LocalReadProjection{
		Registry:           registry,
		NoArgumentCommands: []string{commandLifecycleSentinelID},
		BuiltinLayers: []commandconfig.BuiltinLayer{{
			ID: commandLifecycleBuiltinLayerID, Active: true,
			Defaults: []commandbindings.Default{{
				Candidate: commandbindings.Candidate{
					ID:                "lifecycle.default.ready",
					Trigger:           "keyboard.local:Control+Shift+KeyL",
					CommandID:         commandLifecycleSentinelID,
					ArgumentsKey:      "{}",
					ExecutionScopeKey: "global",
					Scope:             commandbindings.Global,
					Enabled:           true,
					LayerActive:       true,
				},
				Version:     "1",
				Fingerprint: "lifecycle.default.ready.v1",
				Invariant:   false,
			}},
		}},
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return err
	}
	hasGeneration, err := commandLifecycleHasBaseGeneration(ctx, principal.UserID)
	if err != nil {
		return err
	}
	if !hasGeneration {
		return a.rebuildCommandLifecycleSentinelConfiguration(ctx)
	}
	if err := a.restoreCommandLifecyclePersistentClaims(ctx); err != nil && !errors.Is(err, commandactivation.ErrStale) {
		return err
	}
	loaded, hasSnapshot, err := a.loadCommandLifecyclePersistedConfiguration(ctx, store, options)
	if err != nil {
		return err
	}
	if !hasSnapshot {
		return a.rebuildCommandLifecycleSentinelConfiguration(ctx)
	}
	return loaded.publish(ctx)
}

type commandLifecycleLoadedConfiguration struct {
	app           *App
	store         *commandconfig.Store
	principal     auth.LocalSessionPrincipal
	snapshot      commandconfig.Snapshot
	configuration *commandbindings.Configuration
	activeLayers  []string
}

func (a *App) loadCommandLifecyclePersistedConfiguration(ctx context.Context, store *commandconfig.Store, options commandconfig.LocalReadProjection) (commandLifecycleLoadedConfiguration, bool, error) {
	if store == nil {
		return commandLifecycleLoadedConfiguration{}, false, commandexecution.ErrInvalidConfiguration
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return commandLifecycleLoadedConfiguration{}, false, err
	}
	snapshot, err := store.Load(ctx, commandconfig.Scope{UserID: principal.UserID})
	if err != nil {
		if errors.Is(err, commandconfig.ErrInvalid) {
			hasGeneration, generationErr := commandLifecycleHasBaseGeneration(ctx, principal.UserID)
			if generationErr != nil {
				return commandLifecycleLoadedConfiguration{}, false, generationErr
			}
			if !hasGeneration {
				return commandLifecycleLoadedConfiguration{}, false, nil
			}
		}
		return commandLifecycleLoadedConfiguration{}, false, err
	}
	activeLayers := commandLifecycleActiveUserLayerIDs(snapshot, principal, time.Now())
	options.ActiveUserLayerIDs = activeLayers
	configuration, err := commandconfig.ProjectLocalRead(ctx, snapshot, options)
	if err != nil {
		return commandLifecycleLoadedConfiguration{}, false, err
	}
	return commandLifecycleLoadedConfiguration{app: a, store: store, principal: principal, snapshot: snapshot, configuration: configuration, activeLayers: activeLayers}, true, nil
}

func (a *App) restoreCommandLifecyclePersistentClaims(ctx context.Context) error {
	if a == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	state := a.commandHost
	a.authMu.RUnlock()
	if state == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	return state.ChangeUserConfigurationWithEpoch(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return a.currentCommandPrincipal()
	}, func(ctx context.Context, principal auth.LocalSessionPrincipal, epoch commandsecurity.EpochSnapshot) (func(context.Context) error, error) {
		owner := commandactivation.Owner{
			Scope:           commandactivation.Scope{UserID: principal.UserID},
			AuthContextType: "local_session", AuthContextID: principal.SessionID,
			AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		}
		return func(commitCtx context.Context) error {
			service, err := newCommandLifecycleActivationService(owner)
			if err != nil {
				return err
			}
			_, err = service.RestorePersistent(commitCtx, owner, commandLifecycleRestoreOrigin(principal))
			return err
		}, nil
	})
}

func newCommandLifecycleActivationService(owner commandactivation.Owner) (*commandactivation.Service, error) {
	store, err := commandactivation.NewStore(database.DB())
	if err != nil {
		return nil, err
	}
	return commandactivation.New(database.DB(), &commandsecurity.DispatchGate{}, commandactivation.Ports{
		Owner: commandactivation.OwnerPortFunc(func(context.Context, commandactivation.Owner) (commandactivation.Owner, error) {
			return owner, nil
		}),
		Layer: commandLifecycleActivationLayerPort{},
		Origin: commandactivation.OriginPortFunc(func(_ context.Context, _ commandactivation.Owner, origin commandactivation.Origin) (commandactivation.Origin, error) {
			return origin, nil
		}),
		Rule:         store,
		GenerationTx: store,
	}, time.Now)
}

type commandLifecycleActivationLayerPort struct{}

func (commandLifecycleActivationLayerPort) ResolveLayer(ctx context.Context, owner commandactivation.Owner, ref commandactivation.Ref) (commandactivation.Layer, error) {
	if ref.Kind != commandactivation.UserRef {
		return commandactivation.Layer{}, commandactivation.ErrNotFound
	}
	var layer commandconfig.Layer
	query := database.DB().WithContext(ctx).Where("id = ? AND user_id = ?", ref.ID, owner.UserID)
	if owner.WorkspaceID == nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id = ?", *owner.WorkspaceID)
	}
	if err := query.First(&layer).Error; err != nil {
		return commandactivation.Layer{}, commandactivation.ErrNotFound
	}
	return commandactivation.Layer{Ref: commandactivation.Ref{Kind: commandactivation.UserRef, ID: layer.ID}, UserID: layer.UserID, WorkspaceID: layer.WorkspaceID, Enabled: layer.Enabled}, nil
}

func commandLifecycleRestoreOrigin(principal auth.LocalSessionPrincipal) commandactivation.Origin {
	return commandactivation.Origin{Type: "ui_action", SessionID: principal.SessionID, DeviceID: "command-lifecycle"}
}

func commandLifecycleActiveUserLayerIDs(snapshot commandconfig.Snapshot, principal auth.LocalSessionPrincipal, now time.Time) []string {
	rules := map[string]commandactivation.Rule{}
	for _, rule := range snapshot.ActivationRules {
		if rule.UserID != snapshot.Scope.UserID || rule.WorkspaceID != nil || !rule.Enabled || rule.ReviewStatus != "active" || rule.Lifecycle != commandactivation.LifecyclePersistent {
			continue
		}
		key := string(rule.LayerRefKind) + "\x00" + rule.LayerRef + "\x00" + string(rule.RuleRefKind) + "\x00" + rule.RuleRef
		rules[key] = rule
	}
	layers := map[string]commandconfig.Layer{}
	for _, layer := range snapshot.Layers {
		if layer.UserID == snapshot.Scope.UserID && layer.WorkspaceID == nil && layer.Enabled {
			layers[layer.ID] = layer
		}
	}
	active := map[string]bool{}
	var ids []string
	for _, claim := range snapshot.ActivationClaims {
		if claim.UserID != snapshot.Scope.UserID || claim.WorkspaceID != nil || claim.State != commandactivation.StateActive ||
			claim.SourceType != "manual" || claim.AuthContextID != principal.SessionID ||
			claim.LayerRefKind != commandactivation.UserRef || claim.ExpiresAt != nil && !claim.ExpiresAt.After(now) {
			continue
		}
		key := string(claim.LayerRefKind) + "\x00" + claim.LayerRef + "\x00" + string(claim.RuleRefKind) + "\x00" + claim.RuleRef
		if _, ok := rules[key]; !ok {
			continue
		}
		if _, ok := layers[claim.LayerRef]; !ok || active[claim.LayerRef] {
			continue
		}
		active[claim.LayerRef] = true
		ids = append(ids, claim.LayerRef)
	}
	return ids
}

func commandLifecycleHasBaseGeneration(ctx context.Context, userID string) (bool, error) {
	var count int64
	err := database.DB().WithContext(ctx).Model(&commandconfig.Generation{}).
		Where("user_id = ? AND workspace_id IS NULL", userID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (loaded commandLifecycleLoadedConfiguration) publish(ctx context.Context) error {
	if loaded.app == nil || loaded.store == nil || loaded.configuration == nil || loaded.principal.UserID == "" || loaded.principal.SessionID == "" {
		return commandexecution.ErrInvalidConfiguration
	}
	return loaded.app.rebuildCommandLifecycleConfigurationChecked(ctx, func(ctx context.Context, principal auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		if principal != loaded.principal || principal.UserID != loaded.snapshot.Scope.UserID {
			return nil, nil, commandexecution.ErrDenied
		}
		return loaded.configuration, loaded.activeLayers, nil
	}, func(ctx context.Context) error {
		return loaded.store.CheckCurrent(ctx, loaded.snapshot)
	})
}

func (a *App) rebuildCommandLifecycleConfigurationChecked(ctx context.Context,
	build func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error),
	check func(context.Context) error,
) error {
	if a == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	state := a.commandHost
	a.authMu.RUnlock()
	if state == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	return state.RebuildUserConfiguration(ctx, func(ctx context.Context) (auth.LocalSessionPrincipal, error) {
		principal, err := a.currentCommandPrincipal()
		if err != nil {
			return auth.LocalSessionPrincipal{}, err
		}
		if check != nil {
			if err := check(ctx); err != nil {
				return auth.LocalSessionPrincipal{}, err
			}
		}
		return principal, nil
	}, build)
}

func commandLifecycleSentinelCatalog() (*commandcatalog.Registry, map[string]commandexecution.Handler, error) {
	locales := map[string]commandcatalog.LocalizedMetadata{}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		locales[locale] = commandcatalog.LocalizedMetadata{Name: "Lifecycle ready", Description: "Internal lifecycle sentinel", Category: "Internal"}
	}
	definition := commandcatalog.Definition{
		ID:                    commandLifecycleSentinelID,
		Effect:                commandcatalog.Read,
		Decision:              commandcatalog.NoDecision,
		AllowedSources:        []commandcatalog.Source{commandcatalog.KeyboardLocal},
		Context:               commandcatalog.ContextPolicy{None: true},
		Presentation:          &commandcatalog.Presentation{Version: "1", Locales: locales},
		ArgumentsSchema:       &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		ResultSchema:          &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk:                  commandcatalog.RiskLow,
		Persistence:           commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceRedacted},
		Scopes:                []commandcatalog.Scope{commandcatalog.ScopeSession},
		Availability:          commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute:          "internal/lifecycle/ready",
		HandlerClassification: commandcatalog.HandlerInternal,
	}
	handler := commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerInternal}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: handler}})
	if err != nil {
		return nil, nil, err
	}
	handlers := map[string]commandexecution.Handler{
		commandLifecycleSentinelID: {
			Contract: handler,
			Start: func(context.Context, commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
				done := make(chan commandexecution.Outcome, 1)
				done <- commandexecution.Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
				return commandexecution.ExecutionHandle{ID: uuid.Must(uuid.NewV7()).String(), Done: done, Cancel: func() {}}, nil
			},
		},
	}
	return registry, handlers, nil
}
