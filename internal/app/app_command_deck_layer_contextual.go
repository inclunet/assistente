package app

import (
	"context"
	"encoding/json"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

type contextualDeckLayerKey struct{}

// Private proof carried through the asynchronous layer handler to its claim.
// It supplements, never replaces, the physical occurrence in the Deck executor.
type contextualDeckLayerOccurrence struct {
	product                 *commandProductRuntime
	invocationID, commandID string
	proof                   *localCommandKeyboardContextProof
	valid                   func() bool
	versions                commandexecution.Versions
	keyboard                *localCommandKeyboardState
}

func (p *commandProductRuntime) contextualDeckLayerOccurrence(ctx context.Context, invocationID string) *contextualDeckLayerOccurrence {
	if ctx == nil {
		return nil
	}
	occurrence, _ := ctx.Value(contextualDeckLayerKey{}).(*contextualDeckLayerOccurrence)
	if occurrence == nil || occurrence.product != p || occurrence.invocationID != invocationID {
		return nil
	}
	return occurrence
}

// ExecuteContextualDeckLayerCommand cannot accept a command, rule, device or
// arguments from the UI. All are resolved from the consumed physical offer.
// Layer mutations are backend effects; no artificial UI reservation is created.
func (a *App) ExecuteContextualDeckLayerCommand(offerID, generation string, observed LocalCommandKeyboardContext) (result CommandExecutionResult, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	admission, err := a.consumeContextualDeckOffer(offerID, generation, observed)
	if err != nil {
		return result, err
	}
	if !isCommandLayerAction(admission.commandID) {
		return result, commandexecution.ErrDenied
	}
	p, offer := admission.product, admission.offer
	id, err := uuid.NewV7()
	if err != nil {
		return result, err
	}
	invocationID := id.String()
	ctx, cancel := context.WithTimeout(offer.ctx, commandLayerMutationHandoffTimeout)
	defer cancel()
	p.keyboardMu.Lock()
	keyboard := p.keyboardMap
	p.keyboardMu.Unlock()
	if keyboard == nil || keyboard.view.Generation != generation {
		return result, commandexecution.ErrStale
	}
	// Unlike native-file UI commands, a layer action has no authorized blur
	// continuation. Retiring its visual map cancels pending commit ownership.
	stopKeyboardWatch := context.AfterFunc(keyboard.ctx, cancel)
	defer stopKeyboardWatch()
	// This predicate must stay memory-only: it is also called inside GenerationTx.
	valid := func() bool {
		p.keyboardMu.Lock()
		current := p.keyboardMap == keyboard && keyboard.ctx.Err() == nil &&
			p.keyboardMap.view.Generation == generation && p.keyboardMap.versions == offer.versions &&
			(p.keyboardMap.view.ValidUntil == 0 || time.Now().UnixMilli() < p.keyboardMap.view.ValidUntil)
		p.keyboardMu.Unlock()
		return current && ctx.Err() == nil && p.deckExecutionAllowed(offer.generation) &&
			a.commandProduct.Load() == p && p.dependenciesMatch(a) && p.localKeyboardContextCurrent(admission.proof)
	}
	if !valid() {
		return result, commandexecution.ErrStale
	}
	state := p.deckExecution
	state.mu.Lock()
	if len(state.occurrences) >= 64 {
		state.mu.Unlock()
		return result, commandexecution.ErrStale
	}
	state.occurrences[invocationID] = commandDeckOccurrence{ctx: ctx, versions: offer.versions,
		originVersion: admission.originVersion, identity: admission.identity, serial: offer.serial,
		instanceID: offer.instanceID, generation: offer.generation, visual: admission.proof}
	state.mu.Unlock()
	defer state.remove(invocationID)
	p.mu.Lock()
	if p.closed || p.deckCapture != nil || p.deckCaptureGeneration != offer.generation {
		p.mu.Unlock()
		return result, commandexecution.ErrStale
	}
	p.workers.Add(1)
	p.mu.Unlock()
	defer p.workers.Done()
	occurrence := &contextualDeckLayerOccurrence{product: p, invocationID: invocationID,
		commandID: admission.commandID, proof: admission.proof, valid: valid, versions: offer.versions, keyboard: keyboard}
	ctx = context.WithValue(ctx, contextualDeckLayerKey{}, occurrence)
	candidate := commandexecution.EnvelopeCandidate{InvocationID: invocationID, CorrelationID: invocationID,
		TriggerType: string(commandcatalog.StreamDeck), TriggerSpec: admission.raw, Arguments: json.RawMessage(`{}`)}
	record, err := state.service.ExecuteEnvelope(ctx, "", candidate)
	result = commandProductResult(record)
	if err != nil {
		return result, err
	}
	// Our own publication retires the old map/physical generation. The handler's
	// commit ownership, not the retired lease, determines the committed outcome.
	if record.Status == commandledger.Succeeded {
		current, authErr := a.authenticatedCommandProduct()
		if authErr != nil || current != p || !commandRecordMatchesPrincipal(record, p.principal) {
			return CommandExecutionResult{}, commandexecution.ErrDenied
		}
	}
	return result, nil
}

// The same authoritative frontier protects palette and physical contextual
// layer actions. No projection-guard I/O under the transaction/mutation gate.
func (p *commandProductRuntime) claimContextualLayerOwnership(ctx context.Context, claim func() error) error {
	var proof *localCommandKeyboardContextProof
	var valid func() bool
	var versions commandexecution.Versions
	var keyboard *localCommandKeyboardState
	if occurrence, ok := ctx.Value(contextualPaletteLayerKey{}).(*contextualPaletteLayerOccurrence); ok {
		if occurrence == nil || occurrence.product != p {
			return commandexecution.ErrStale
		}
		proof, valid, versions = occurrence.proof, occurrence.valid, occurrence.versions
	} else if occurrence, ok := ctx.Value(contextualDeckLayerKey{}).(*contextualDeckLayerOccurrence); ok {
		if occurrence == nil || occurrence.product != p || occurrence.keyboard == nil {
			return commandexecution.ErrStale
		}
		proof, valid, versions = occurrence.proof, occurrence.valid, occurrence.versions
		keyboard = occurrence.keyboard
	} else {
		return claim()
	}
	if proof == nil || valid == nil || !valid() {
		return commandexecution.ErrStale
	}
	return p.host.WithPublishedVersions(ctx, p.principal, versions, func() error {
		if keyboard != nil {
			// Lock order matches keyboard envelope capture: keyboard before
			// workspace. Reset cannot retire the map between this check and claim;
			// correctness does not depend on scheduling the cancellation watcher.
			p.keyboardMu.Lock()
			defer p.keyboardMu.Unlock()
			if p.keyboardMap != keyboard || keyboard.ctx.Err() != nil ||
				(keyboard.view.ValidUntil != 0 && time.Now().UnixMilli() >= keyboard.view.ValidUntil) {
				return commandexecution.ErrStale
			}
		}
		return p.workspaceMgr.WithCommandSnapshot(ctx, func(snapshot workspace.CommandSnapshot) error {
			if snapshot != proof.snapshot {
				return commandexecution.ErrStale
			}
			return claim()
		})
	})
}
