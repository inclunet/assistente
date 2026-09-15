package app

import (
	"context"
	"encoding/json"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandbridge"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"github.com/google/uuid"
)

const commandLifecycleSentinelID = "lifecycle.ready"
const commandLifecycleRegistryVersion = "lifecycle-v1"

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
