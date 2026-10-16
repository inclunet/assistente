package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
)

func TestCommandPaletteDefaultFingerprintIncludesSemanticsNotPresentation(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	options, err := commandProductProjection(p.registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	var base commandbindings.Default
	for _, binding := range options.BuiltinLayers[0].Defaults {
		if binding.Candidate.CommandID == commandProductWorkspaceListID {
			base = binding
			break
		}
	}
	if base.Candidate.ID == "" {
		t.Fatal("default de workspace.list ausente")
	}
	definition, _ := p.registry.Lookup(base.Candidate.CommandID)
	definition.Presentation = &commandcatalog.Presentation{Version: "new-labels"}
	cosmetic, err := commandPaletteDefaultFingerprint(base.Candidate, definition, commandPaletteLayerID)
	if err != nil || cosmetic != base.Fingerprint {
		t.Fatalf("apresentação alterou fingerprint: %q %v", cosmetic, err)
	}
	for _, change := range []struct {
		name   string
		mutate func(*commandbindings.Candidate, *commandcatalog.Definition)
	}{
		{"risk", func(_ *commandbindings.Candidate, d *commandcatalog.Definition) { d.Risk = commandcatalog.RiskHigh }},
		{"decision", func(_ *commandbindings.Candidate, d *commandcatalog.Definition) {
			d.Decision = commandcatalog.Interactive
		}},
		{"capability", func(_ *commandbindings.Candidate, d *commandcatalog.Definition) { d.MutatesEffectiveCapability = true }},
		{"scope", func(c *commandbindings.Candidate, _ *commandcatalog.Definition) { c.Scope = commandbindings.Global }},
		{"condition", func(c *commandbindings.Candidate, _ *commandcatalog.Definition) {
			c.Condition = commandbindings.Facts{commandbindings.Profile: "other"}
		}},
		{"priority", func(c *commandbindings.Candidate, _ *commandcatalog.Definition) { c.BindingPriority++ }},
		{"trigger", func(c *commandbindings.Candidate, _ *commandcatalog.Definition) {
			c.Trigger = "palette:other.selection"
		}},
		{"arguments", func(c *commandbindings.Candidate, _ *commandcatalog.Definition) { c.ArgumentsKey = `{"x":1}` }},
		{"source", func(_ *commandbindings.Candidate, d *commandcatalog.Definition) {
			d.AllowedSources = []commandcatalog.Source{commandcatalog.Palette}
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			candidate := base.Candidate
			modified := definition
			change.mutate(&candidate, &modified)
			fingerprint, err := commandPaletteDefaultFingerprint(candidate, modified, commandPaletteLayerID)
			if err != nil || fingerprint == base.Fingerprint {
				t.Fatalf("mudança semântica não invalidou default: %v", err)
			}
		})
	}
}

func TestCommandProductResolverRejectsForgedSourceArgumentsAndStalePublication(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	ctx := context.Background()
	_, _, versions, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	source := commandcontract.SourcePalette
	envelope := commandcontract.Envelope{SourceType: &source, RegistryVersion: versions.Registry,
		GlobalConfigGeneration: &versions.GlobalConfig, ActiveLayersGeneration: &versions.ActiveLayers}
	candidate := commandexecution.EnvelopeCandidate{TriggerType: "palette", TriggerSpec: json.RawMessage(`{"version":1,"selection":"workspace.list"}`), Arguments: json.RawMessage(`{}`)}
	resolved, err := p.resolvePersistedTrigger(ctx, p.principal, candidate, envelope)
	if err != nil || resolved.Mode != commandcontract.ResolutionExecute || resolved.CommandID != commandProductWorkspaceListID {
		t.Fatalf("valid resolution: %+v %v", resolved, err)
	}
	for _, kind := range []string{"keyboard.local", "keyboard.global", "system"} {
		forged := candidate
		forged.TriggerType = kind
		if _, err := p.resolvePersistedTrigger(ctx, p.principal, forged, envelope); !errors.Is(err, commandexecution.ErrDenied) {
			t.Fatalf("source %q: %v", kind, err)
		}
	}
	forged := candidate
	forged.Arguments = json.RawMessage(`{"command_id":"help.shortcuts.show"}`)
	if _, err := p.resolvePersistedTrigger(ctx, p.principal, forged, envelope); !errors.Is(err, commandexecution.ErrInvalidRequest) {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.resolvePersistedTrigger(ctx, p.principal, candidate, envelope); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("old publication: %v", err)
	}
}
