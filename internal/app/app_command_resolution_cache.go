package app

import (
	"context"
	"encoding/json"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
)

// Only the pure binding selection is memoized. Occurrence ownership, native
// origin freshness, UI leases, authorization and commit guards remain outside
// the cache and are checked for every invocation, including cache hits.
func (p *commandProductRuntime) resolveCachedBinding(ctx context.Context, configuration *commandbindings.Configuration, identity string, facts commandbindings.Facts, originVersion, foregroundVersion string, envelope commandcontract.Envelope) (commandbindings.Result, error) {
	if err := ctx.Err(); err != nil {
		return commandbindings.Result{}, err
	}
	key, cacheable := p.bindingResolutionCacheKey(identity, facts, originVersion, foregroundVersion, envelope)
	p.resolutionMu.Lock()
	defer p.resolutionMu.Unlock()
	if err := ctx.Err(); err != nil {
		return commandbindings.Result{}, err
	}
	if p.resolutionStopped {
		return commandbindings.Result{}, commandexecution.ErrStale
	}
	if p.resolutionCache == nil || !cacheable {
		return configuration.Resolve(identity, facts, nil)
	}
	// A product runtime belongs to one user/session/workspace. Publication of
	// another immutable configuration retires both positive and negative entries.
	if p.resolutionConfiguration != configuration {
		p.resolutionCache.Clear()
		p.resolutionConfiguration = configuration
	}
	if result, ok := p.resolutionCache.Get(key); ok {
		return result, nil
	}
	result, err := configuration.Resolve(identity, facts, nil)
	if err != nil {
		return commandbindings.Result{}, err
	}
	// Put validates the same complete key as Get. Errors are never cached.
	if err := p.resolutionCache.Put(key, result); err != nil {
		return commandbindings.Result{}, err
	}
	return result, nil
}

func (p *commandProductRuntime) bindingResolutionCacheKey(identity string, facts commandbindings.Facts, originVersion, foregroundVersion string, envelope commandcontract.Envelope) (commandcontext.CacheKey, bool) {
	if envelope.SourceType == nil || envelope.GlobalConfigGeneration == nil || envelope.ActiveLayersGeneration == nil {
		return commandcontext.CacheKey{}, false
	}
	// Do not invent workspace ownership from context_version, nor equate an
	// absent workspace with an empty ID. Incomplete snapshots bypass caching.
	if envelope.WorkspaceID != nil && *envelope.WorkspaceID != p.workspaceID {
		return commandcontext.CacheKey{}, false
	}
	// Selection consumes these exact facts. Their canonical JSON distinguishes
	// absent facts and typed values; versions retain ABA changes even when facts
	// are equal again. No invocation ID/authorization decision is cached.
	contextKey, err := json.Marshal(struct {
		Version    *string               `json:"version"`
		Origin     string                `json:"origin"`
		Foreground string                `json:"foreground"`
		Facts      commandbindings.Facts `json:"facts"`
	}{envelope.ContextVersion, originVersion, foregroundVersion, facts})
	if err != nil {
		return commandcontext.CacheKey{}, false
	}
	key, err := commandcontext.NewCacheKey(
		commandcontext.Scope{UserID: p.principal.UserID, AuthContextID: p.principal.SessionID, WorkspaceID: envelope.WorkspaceID},
		identity, commandcatalog.Source(*envelope.SourceType), string(contextKey), envelope.RegistryVersion,
		commandcontext.GenerationSnapshot{GlobalConfigGeneration: *envelope.GlobalConfigGeneration,
			WorkspaceConfigGeneration: envelope.WorkspaceConfigGeneration, ActiveLayersGeneration: *envelope.ActiveLayersGeneration},
	)
	return key, err == nil
}
