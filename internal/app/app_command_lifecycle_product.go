package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandbridge"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
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
	a.startCommandOSSessionMonitorLocked()
	a.authMu.Unlock()
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
	snapshot      commandconfig.Snapshot
	configuration *commandbindings.Configuration
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
	configuration, err := commandconfig.ProjectLocalRead(ctx, snapshot, options)
	if err != nil {
		return commandLifecycleLoadedConfiguration{}, false, err
	}
	return commandLifecycleLoadedConfiguration{app: a, store: store, snapshot: snapshot, configuration: configuration}, true, nil
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
	if loaded.app == nil || loaded.store == nil || loaded.configuration == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	loadedPrincipal := loaded.snapshot.Scope.UserID
	return loaded.app.rebuildCommandLifecycleConfigurationChecked(ctx, func(ctx context.Context, principal auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		if principal.UserID != loadedPrincipal {
			return nil, nil, commandexecution.ErrDenied
		}
		return loaded.configuration, nil, nil
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
