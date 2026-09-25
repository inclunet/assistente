package app

import (
	"context"
	"encoding/json"
	"time"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandui"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

// Event-only DTO. It carries no user-facing hardware identifier and grants no
// command authority by itself. Mixed local/durable choices are delivered once.
type CommandDeckContextualUIEvent struct {
	OfferID     string                         `json:"offerId"`
	Generation  string                         `json:"generation"`
	UserID      string                         `json:"userId"`
	SessionID   string                         `json:"sessionId"`
	WorkspaceID string                         `json:"workspaceId"`
	Conditions  []LocalCommandPaletteCondition `json:"conditions"`
}

type commandDeckContextualOffer struct {
	ctx                                    context.Context
	serial, instanceID, keyboardGeneration string
	key                                    int
	generation                             uint64
	versions                               commandexecution.Versions
	snapshot                               workspace.CommandSnapshot
	expiresAt                              time.Time
	controller                             *commandDeckController
}

func deckConditionsHaveContextualCommand(conditions []LocalCommandPaletteCondition) bool {
	for _, condition := range conditions {
		if isContextualDeckUICommand(condition.CommandID) {
			return true
		}
	}
	return false
}

// Only the physical controller calls this. A discarded local-only selection
// costs no IPC/ledger; unused offers expire lazily in the bounded collection.
func (p *commandProductRuntime) createDeckContextualOffer(ctx context.Context, serial, instanceID string, key int, versions commandexecution.Versions, deckGeneration uint64, keyboardGeneration string, conditions []LocalCommandPaletteCondition, controller *commandDeckController) (CommandDeckContextualUIEvent, error) {
	if p == nil || p.deckExecution == nil || ctx == nil || ctx.Err() != nil || !validCommandDeckInstanceID(instanceID) || serial == "" || key < 0 || !p.deckExecutionAllowed(deckGeneration) {
		return CommandDeckContextualUIEvent{}, commandexecution.ErrDenied
	}
	snapshot, err := p.workspaceMgr.CommandSnapshot()
	if err != nil || snapshot.WorkspaceID != p.workspaceID {
		return CommandDeckContextualUIEvent{}, commandexecution.ErrStale
	}
	id, err := uuid.NewV7()
	if err != nil {
		return CommandDeckContextualUIEvent{}, err
	}
	state := p.deckExecution
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	for key, offer := range state.offers {
		if !now.Before(offer.expiresAt) || offer.ctx.Err() != nil {
			delete(state.offers, key)
		}
	}
	if len(state.offers) >= 64 {
		return CommandDeckContextualUIEvent{}, commandexecution.ErrStale
	}
	if state.offers == nil {
		state.offers = make(map[string]commandDeckContextualOffer)
	}
	state.offers[id.String()] = commandDeckContextualOffer{ctx: ctx, serial: serial, instanceID: instanceID, key: key, generation: deckGeneration, keyboardGeneration: keyboardGeneration, versions: versions, snapshot: snapshot, expiresAt: now.Add(10 * time.Second), controller: controller}
	return CommandDeckContextualUIEvent{OfferID: id.String(), Generation: keyboardGeneration, UserID: p.principal.UserID, SessionID: p.principal.SessionID, WorkspaceID: p.workspaceID, Conditions: cloneLocalCommandPaletteConditions(conditions)}, nil
}

type contextualDeckAdmission struct {
	product                            *commandProductRuntime
	offer                              commandDeckContextualOffer
	proof                              *localCommandKeyboardContextProof
	raw                                json.RawMessage
	identity, commandID, originVersion string
	valid                              func() bool
	page                               bool
	keyboard                           *localCommandKeyboardState
}

// consumeContextualDeckOffer consumes a host-minted physical offer. The UI
// cannot choose command, key, device, arguments, owner or invocation ID.
// Focus/modal/IME stay UI guards; observed surface must match the original
// host snapshot, not merely the tab that happens to be active at this call.
func (a *App) consumeContextualDeckOffer(offerID, generation string, observed LocalCommandKeyboardContext) (result *contextualDeckAdmission, err error) {
	return a.consumeContextualDeckOfferForSurface(offerID, generation, observed, false)
}

func (a *App) consumeContextualDeckOfferForSurface(offerID, generation string, observed LocalCommandKeyboardContext, page bool) (result *contextualDeckAdmission, err error) {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return result, err
	}
	if p.deckExecution == nil || offerID == "" || generation == "" {
		return result, commandexecution.ErrDenied
	}
	state := p.deckExecution
	state.mu.Lock()
	offer, exists := state.offers[offerID]
	delete(state.offers, offerID)
	state.mu.Unlock()
	if !exists || !time.Now().Before(offer.expiresAt) || offer.ctx.Err() != nil || generation != offer.keyboardGeneration || !p.deckExecutionAllowed(offer.generation) {
		return result, commandexecution.ErrStale
	}
	keyboardGeneration, keyboardVersions, ready := p.localKeyboardState()
	if !ready || keyboardGeneration != generation || keyboardVersions != offer.versions {
		return result, commandexecution.ErrStale
	}
	p.keyboardMu.Lock()
	keyboard := p.keyboardMap
	mapCurrent := p.keyboardMap != nil && p.keyboardMap.view.Generation == generation &&
		(p.keyboardMap.view.ValidUntil == 0 || time.Now().UnixMilli() < p.keyboardMap.view.ValidUntil)
	p.keyboardMu.Unlock()
	if !mapCurrent {
		return result, commandexecution.ErrStale
	}
	var proof *localCommandKeyboardContextProof
	if page {
		proof, err = p.captureDeckPageContext(observed)
	} else {
		proof, err = a.captureLocalKeyboardContext(observed)
	}
	if err != nil || proof.snapshot != offer.snapshot {
		return result, commandexecution.ErrStale
	}
	raw, err := json.Marshal(commandDeckTrigger{Version: 1, Device: offer.serial, Key: offer.key})
	if err != nil {
		return result, err
	}
	identity, err := (commandconfig.StreamDeckTriggerPort{}).Normalize(offer.ctx, raw)
	if err != nil {
		return result, err
	}
	configuration, _, versions, err := p.host.ResolutionSnapshot(offer.ctx, p.principal)
	if err != nil || versions != offer.versions || !versions.Unlocked {
		return result, commandexecution.ErrStale
	}
	var origin commandOriginContext
	if page {
		origin, err = deckPageCommandFacts(proof, configuration.RequiredFacts(identity))
	} else {
		origin, err = workspaceVisualCommandFacts(proof, configuration.RequiredFacts(identity))
	}
	if err != nil {
		return result, err
	}
	origin.version = "deck-visual:" + proof.snapshot.Version
	selected, err := configuration.Resolve(identity, origin.facts, nil)
	if err != nil || selected.Status != commandbindings.Selected || selected.ExecutionScopeKey != "global" || !isContextualDeckUICommand(selected.CommandID) {
		return result, commandexecution.ErrDenied
	}
	if page != isContextualPagePaletteCommand(selected.CommandID) || (page && !deckPageCommandSurface(selected.CommandID, observed.SurfaceType)) {
		return result, commandexecution.ErrDenied
	}
	// Mermaid's modal is a UI-owned scope of its original editor, never a
	// standalone surface or a route into an unrelated workspace tab.
	if isMermaidMutation(selected.CommandID) && observed.SurfaceType != "editor" {
		return result, commandexecution.ErrDenied
	}
	definition, exists := p.registry.Lookup(selected.CommandID)
	if !exists || !commandDeckLedgerCommand(definition) {
		return result, commandexecution.ErrDenied
	}
	if _, err := definition.ValidateArguments([]byte(selected.ArgumentsKey)); err != nil {
		return result, err
	}
	valid := func() bool {
		if offer.ctx.Err() != nil || !p.deckExecutionAllowed(offer.generation) || a.commandProduct.Load() != p || !p.dependenciesMatch(a) || !p.localKeyboardContextCurrent(proof) {
			return false
		}
		if page || isMermaidMutation(selected.CommandID) {
			p.keyboardMu.Lock()
			current := p.keyboardMap != nil && p.keyboardMap == keyboard && p.keyboardMap.ctx.Err() == nil && p.keyboardMap.view.Generation == generation &&
				(p.keyboardMap.view.ValidUntil == 0 || time.Now().UnixMilli() < p.keyboardMap.view.ValidUntil)
			p.keyboardMu.Unlock()
			if !current {
				return false
			}
		}
		current, err := p.host.Snapshot(offer.ctx, p.principal)
		return err == nil && current == offer.versions && current.Unlocked
	}
	if !valid() {
		return result, commandexecution.ErrStale
	}
	return &contextualDeckAdmission{product: p, offer: offer, proof: proof, raw: raw, identity: identity,
		commandID: selected.CommandID, originVersion: origin.version, valid: valid, page: page, keyboard: keyboard}, nil
}

// BeginContextualDeckUICommand admits only commands with the existing UI
// preparation/handoff protocol. Layer actions use their direct backend ingress.
func (a *App) BeginContextualDeckUICommand(offerID, generation string, observed LocalCommandKeyboardContext) (result commandui.Reservation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	admission, err := a.consumeContextualDeckOffer(offerID, generation, observed)
	if err != nil {
		return result, err
	}
	if isCommandLayerAction(admission.commandID) {
		return result, commandexecution.ErrDenied
	}
	return a.beginAdmittedContextualDeckUICommand(admission)
}

func (a *App) beginAdmittedContextualDeckUICommand(admission *contextualDeckAdmission) (commandui.Reservation, error) {
	p, offer, proof := admission.product, admission.offer, admission.proof
	state := p.deckExecution
	var invocationID string
	reservation, err := a.beginCommandUIWithCleanup(p, admission.commandID, func(reservation commandui.Reservation) (commandexecution.EnvelopeCandidate, error) {
		invocationID = reservation.InvocationID
		state.mu.Lock()
		defer state.mu.Unlock()
		if offer.ctx.Err() != nil || len(state.occurrences) >= 64 {
			return commandexecution.EnvelopeCandidate{}, commandexecution.ErrStale
		}
		state.occurrences[invocationID] = commandDeckOccurrence{ctx: offer.ctx, versions: offer.versions, originVersion: admission.originVersion, identity: admission.identity, serial: offer.serial, instanceID: offer.instanceID, generation: offer.generation, visual: proof, page: admission.page, keyboard: admission.keyboard, controller: offer.controller}
		state.registerDeckFeedbackLocked(invocationID, state.occurrences[invocationID])
		return commandexecution.EnvelopeCandidate{InvocationID: invocationID, CorrelationID: invocationID, TriggerType: string(commandcatalog.StreamDeck), TriggerSpec: admission.raw, Arguments: json.RawMessage(`{}`)}, nil
	}, func(ctx context.Context, candidate commandexecution.EnvelopeCandidate) (commandledger.FullRecord, error) {
		return state.executeEnvelopeWithFeedback(ctx, "", candidate)
	}, admission.valid, nil, func() { state.remove(invocationID) })
	if err != nil {
		state.remove(invocationID)
	}
	return reservation, err
}

func (p *commandProductRuntime) deckOccurrenceOriginFacts(ctx context.Context, occurrence commandDeckOccurrence, required []commandbindings.Field) (commandOriginContext, error) {
	if occurrence.visual == nil {
		return p.commandOriginFactsFromSnapshot(ctx, commandcatalog.StreamDeck, required, occurrence.serial, occurrence.foreground)
	}
	if ctx == nil || ctx.Err() != nil || occurrence.ctx == nil || occurrence.ctx.Err() != nil || !p.deckExecutionAllowed(occurrence.generation) || !p.localKeyboardContextCurrent(occurrence.visual) {
		return commandOriginContext{}, commandexecution.ErrStale
	}
	var origin commandOriginContext
	var err error
	if occurrence.page {
		origin, err = deckPageCommandFacts(occurrence.visual, required)
	} else {
		origin, err = workspaceVisualCommandFacts(occurrence.visual, required)
	}
	if err != nil {
		return commandOriginContext{}, err
	}
	origin.version = "deck-visual:" + occurrence.visual.snapshot.Version
	return origin, nil
}
