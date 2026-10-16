package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"github.com/google/uuid"
)

func bindingCacheFixture(t *testing.T) (*commandProductRuntime, *commandbindings.Configuration, commandcontract.Envelope) {
	t.Helper()
	p := &commandProductRuntime{principal: auth.LocalSessionPrincipal{UserID: uuid.Must(uuid.NewV7()).String(), SessionID: uuid.Must(uuid.NewV7()).String()}, workspaceID: "ws-0192f3a4b5c6d7e8", resolutionCache: commandcontext.MustNewResolutionCache(8)}
	c, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{{ID: "binding", Trigger: "palette:workspace.list", CommandID: "workspace.list", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: commandbindings.Application, Enabled: true, LayerActive: true, Condition: commandbindings.Facts{commandbindings.Profile: "dev"}, LayerRef: "user:layer"}})
	if err != nil {
		t.Fatal(err)
	}
	source := commandcontract.SourcePalette
	e := commandcontract.Envelope{SourceType: &source, WorkspaceID: &p.workspaceID, RegistryVersion: "registry:1", GlobalConfigGeneration: commandStringPointer("global:1"), WorkspaceConfigGeneration: commandStringPointer("workspace:1"), ActiveLayersGeneration: commandStringPointer("layers:1"), ContextVersion: commandStringPointer("context:1")}
	return p, c, e
}

func TestCommandBindingCacheKeysEveryDimensionAndTypedContext(t *testing.T) {
	p, _, e := bindingCacheFixture(t)
	facts := commandbindings.Facts{commandbindings.Profile: "dev"}
	base, ok := p.bindingResolutionCacheKey("palette:workspace.list", facts, "origin:1", "", e)
	if !ok {
		t.Fatal("complete key rejected")
	}
	if err := p.resolutionCache.Put(base, commandbindings.Result{Status: commandbindings.Selected}); err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*commandcontract.Envelope){
		"registry":             func(e *commandcontract.Envelope) { e.RegistryVersion = "registry:2" },
		"global":               func(e *commandcontract.Envelope) { e.GlobalConfigGeneration = commandStringPointer("global:2") },
		"workspace-generation": func(e *commandcontract.Envelope) { e.WorkspaceConfigGeneration = commandStringPointer("workspace:2") },
		"layers":               func(e *commandcontract.Envelope) { e.ActiveLayersGeneration = commandStringPointer("layers:2") },
		"context":              func(e *commandcontract.Envelope) { e.ContextVersion = commandStringPointer("context:2") },
		"context-nil":          func(e *commandcontract.Envelope) { e.ContextVersion = nil },
		"source":               func(e *commandcontract.Envelope) { s := commandcontract.SourceStreamDeck; e.SourceType = &s },
		"global-scope":         func(e *commandcontract.Envelope) { e.WorkspaceID = nil; e.WorkspaceConfigGeneration = nil },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			changed := e
			change(&changed)
			key, valid := p.bindingResolutionCacheKey("palette:workspace.list", facts, "origin:1", "", changed)
			if !valid {
				t.Fatal("valid changed dimension rejected")
			}
			if _, hit := p.resolutionCache.Get(key); hit {
				t.Fatal("different dimension reused cached selection")
			}
		})
	}
	for _, sample := range []struct {
		trigger, origin string
		facts           commandbindings.Facts
	}{
		{"palette:other", "origin:1", facts}, {"palette:workspace.list", "origin:2", facts},
		{"palette:workspace.list", "origin:1", nil}, {"palette:workspace.list", "origin:1", commandbindings.Facts{commandbindings.Profile: "other"}},
	} {
		key, valid := p.bindingResolutionCacheKey(sample.trigger, sample.facts, sample.origin, "", e)
		if !valid {
			t.Fatal("changed context rejected")
		}
		if _, hit := p.resolutionCache.Get(key); hit {
			t.Fatal("trigger/facts/ABA origin reused cache")
		}
	}
	other := &commandProductRuntime{principal: auth.LocalSessionPrincipal{UserID: uuid.Must(uuid.NewV7()).String(), SessionID: p.principal.SessionID}, workspaceID: p.workspaceID}
	foregroundKey, valid := p.bindingResolutionCacheKey("palette:workspace.list", facts, "origin:1", "foreground:2", e)
	if _, hit := p.resolutionCache.Get(foregroundKey); !valid || hit {
		t.Fatal("foreground version isolation failed")
	}
	key, valid := other.bindingResolutionCacheKey("palette:workspace.list", facts, "origin:1", "", e)
	if _, hit := p.resolutionCache.Get(key); !valid || hit {
		t.Fatal("user isolation failed")
	}
	other.principal = p.principal
	other.workspaceID = "another-workspace"
	otherEnvelope := e
	otherEnvelope.WorkspaceID = &other.workspaceID
	key, valid = other.bindingResolutionCacheKey("palette:workspace.list", facts, "origin:1", "", otherEnvelope)
	if _, hit := p.resolutionCache.Get(key); !valid || hit {
		t.Fatal("workspace isolation failed")
	}
	if _, valid := p.bindingResolutionCacheKey("palette:workspace.list", facts, "origin:1", "", otherEnvelope); valid {
		t.Fatal("foreign workspace cached by the wrong product runtime")
	}
	e.WorkspaceConfigGeneration = nil
	if _, valid := p.bindingResolutionCacheKey("palette:workspace.list", facts, "origin:1", "", e); valid {
		t.Fatal("missing workspace generation cached")
	}
}

func TestCommandBindingCacheHitNegativeInvalidationAndDetachedResults(t *testing.T) {
	p, configuration, envelope := bindingCacheFixture(t)
	ctx := context.Background()
	facts := commandbindings.Facts{commandbindings.Profile: "dev"}
	first, err := p.resolveCachedBinding(ctx, configuration, "palette:workspace.list", facts, "origin:1", "", envelope)
	if err != nil || first.Status != commandbindings.Selected || p.resolutionCache.Len() != 1 {
		t.Fatalf("cold selection: %+v %v", first, err)
	}
	first.BindingIDs[0] = "mutated"
	first.LayerRefs[0] = "mutated"
	again, err := p.resolveCachedBinding(ctx, configuration, "palette:workspace.list", facts, "origin:1", "", envelope)
	if err != nil || again.BindingIDs[0] != "binding" || again.LayerRefs[0] != "user:layer" {
		t.Fatalf("cached provenance mutated: %+v %v", again, err)
	}
	// Seed an explicit negative entry to prove that the adapter consumes hits,
	// rather than merely filling the LRU while always calling the resolver.
	key, _ := p.bindingResolutionCacheKey("palette:workspace.list", facts, "origin:1", "", envelope)
	if err := p.resolutionCache.Put(key, commandbindings.Result{Status: commandbindings.NoMatch}); err != nil {
		t.Fatal(err)
	}
	negative, err := p.resolveCachedBinding(ctx, configuration, "palette:workspace.list", facts, "origin:1", "", envelope)
	if err != nil || negative.Status != commandbindings.NoMatch {
		t.Fatalf("negative cache hit lost: %+v %v", negative, err)
	}
	_, replacement, _ := bindingCacheFixture(t)
	selected, err := p.resolveCachedBinding(ctx, replacement, "palette:workspace.list", facts, "origin:1", "", envelope)
	if err != nil || selected.Status != commandbindings.Selected {
		t.Fatalf("retired negative survived new configuration: %+v %v", selected, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.resolveCachedBinding(cancelled, replacement, "palette:workspace.list", facts, "origin:1", "", envelope); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cache hit: %v", err)
	}
}

func TestCommandProductResolutionCacheWiredRebuildLockAndShutdown(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	for i := 0; i < 2; i++ {
		result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
		if err != nil || result.Status != string(commandledger.Succeeded) {
			t.Fatalf("real execution: %+v %v", result, err)
		}
	}
	if p.resolutionCache == nil || p.resolutionCache.Len() != 1 {
		t.Fatal("production selection did not populate the bounded cache")
	}
	delta := installPaletteDelta(t, a, "active", `{"version":1,"clauses":[]}`)
	result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err != nil || result.Status != string(commandledger.Suppressed) {
		t.Fatalf("warm positive survived suppression: %+v %v", result, err)
	}
	removePaletteDeltaAndRebuild(t, a, delta)
	result, err = a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err != nil || result.Status != string(commandledger.Succeeded) {
		t.Fatalf("warm negative survived rebuild: %+v %v", result, err)
	}
	if err := p.host.SetVaultUnlocked(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`)); err == nil {
		t.Fatal("cache authorized locked vault")
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.resolutionCache.Len() != 0 {
		t.Fatal("shutdown retained selections")
	}
	_, configuration, envelope := bindingCacheFixture(t)
	if _, err := p.resolveCachedBinding(context.Background(), configuration, "palette:workspace.list", nil, "", "", envelope); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("shutdown accepted cache refill: %v", err)
	}
}
