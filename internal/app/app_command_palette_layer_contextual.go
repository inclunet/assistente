package app

import (
	"context"
	"encoding/json"
	"time"

	"assistente/internal/commandbindings"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"github.com/google/uuid"
)

type contextualPaletteLayerKey struct{}

// Private ingress proof: neither callers nor persisted bindings can mint it.
// The UI guards focus/modal/IME synchronously before submission. This proof
// binds that submission to the host's map, session and workspace snapshot.
type contextualPaletteLayerOccurrence struct {
	product                 *commandProductRuntime
	invocationID, commandID string
	proof                   *localCommandKeyboardContextProof
	valid                   func() bool
	versions                commandexecution.Versions
}

func (a *App) ExecuteContextualPaletteLayerCommand(generation, commandID string, observed LocalCommandKeyboardContext) (result CommandExecutionResult, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	if generation == "" || !isCommandLayerAction(commandID) {
		return result, commandexecution.ErrDenied
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return result, err
	}
	proof, err := a.captureLocalKeyboardContext(observed)
	if err != nil {
		return result, err
	}
	p.keyboardMu.Lock()
	state := p.keyboardMap
	current := state != nil && state.ctx.Err() == nil && state.view.Generation == generation &&
		(state.view.ValidUntil == 0 || time.Now().UnixMilli() < state.view.ValidUntil)
	p.keyboardMu.Unlock()
	if !current || state.configuration == nil {
		return result, commandexecution.ErrStale
	}
	required := state.configuration.RequiredFacts("palette:" + commandID)
	if len(required) == 0 {
		return result, commandexecution.ErrDenied
	}
	if _, err := workspaceVisualCommandFacts(proof, required); err != nil {
		return result, err
	}
	valid := func() bool {
		p.keyboardMu.Lock()
		current := p.keyboardMap == state && state.ctx.Err() == nil &&
			(state.view.ValidUntil == 0 || time.Now().UnixMilli() < state.view.ValidUntil)
		p.keyboardMu.Unlock()
		if !current || a.commandProduct.Load() != p || !p.dependenciesMatch(a) || !p.localKeyboardContextCurrent(proof) {
			return false
		}
		return true
	}
	if !valid() {
		return result, commandexecution.ErrStale
	}
	versions, err := p.host.Snapshot(a.commandBridgeContext(), p.principal)
	if err != nil || versions != state.versions || !versions.Unlocked {
		return result, commandexecution.ErrStale
	}
	id, err := uuid.NewV7()
	if err != nil {
		return result, err
	}
	candidate, err := commandPaletteCandidate(id.String(), id.String(), commandID, json.RawMessage(`{}`))
	if err != nil {
		return result, err
	}
	occurrence := &contextualPaletteLayerOccurrence{product: p, invocationID: id.String(), commandID: commandID, proof: proof, valid: valid, versions: versions}
	ctx := context.WithValue(state.ctx, contextualPaletteLayerKey{}, occurrence)
	record, err := p.execute(ctx, candidate)
	result = commandProductResult(record)
	if err != nil {
		return result, err
	}
	// Publishing our own mutation retires the map. CommitOwnership already
	// distinguishes a committed mutation from cancellation; do not reexecute it
	// or reject its success simply because the old map is now gone.
	if record.Status == commandledger.Succeeded {
		current, authErr := a.authenticatedCommandProduct()
		if authErr != nil || current != p || !commandRecordMatchesPrincipal(record, p.principal) {
			return CommandExecutionResult{}, commandexecution.ErrDenied
		}
	}
	return result, nil
}

func workspaceVisualCommandFacts(proof *localCommandKeyboardContextProof, required []commandbindings.Field) (commandOriginContext, error) {
	if proof == nil {
		return commandOriginContext{}, commandexecution.ErrDenied
	}
	facts := commandbindings.Facts{commandbindings.AppFocused: true, commandbindings.SurfaceType: string(proof.snapshot.Tab.Type), commandbindings.SurfaceID: proof.snapshot.Tab.ID}
	if proof.routePage {
		surface, known := localKeyboardRouteSurface(proof.observed.AppPage)
		if !known || proof.observed.SurfaceType != surface || proof.observed.SurfaceID != "command-toolbar" {
			return commandOriginContext{}, commandexecution.ErrDenied
		}
		facts[commandbindings.SurfaceType] = surface
		delete(facts, commandbindings.SurfaceID)
	}
	for _, field := range required {
		switch field {
		case commandbindings.AppFocused, commandbindings.SurfaceType:
		case commandbindings.SurfaceID:
			if proof.routePage {
				return commandOriginContext{}, commandexecution.ErrDenied
			}
		case commandbindings.AppPage:
			if !commandbindings.IsAppPage(proof.observed.AppPage) || !proof.routePage && proof.observed.AppPage != "workspace" {
				return commandOriginContext{}, commandexecution.ErrDenied
			}
			facts[field] = proof.observed.AppPage
		case commandbindings.Profile:
			profile := localKeyboardEffectiveProfile(proof.snapshot)
			if profile == "" || profile != proof.observed.Profile {
				return commandOriginContext{}, commandexecution.ErrDenied
			}
			facts[field] = profile
		default:
			return commandOriginContext{}, commandexecution.ErrDenied
		}
	}
	return commandOriginContext{facts: facts, version: "palette-layer:" + proof.snapshot.Version}, nil
}

func (p *commandProductRuntime) contextualPaletteLayerOccurrence(ctx context.Context, invocationID string) *contextualPaletteLayerOccurrence {
	if ctx == nil {
		return nil
	}
	occurrence, _ := ctx.Value(contextualPaletteLayerKey{}).(*contextualPaletteLayerOccurrence)
	if occurrence == nil || occurrence.product != p || occurrence.invocationID != invocationID {
		return nil
	}
	return occurrence
}
