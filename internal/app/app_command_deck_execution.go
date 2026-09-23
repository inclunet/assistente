package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandforeground"
	"assistente/internal/commandledger"
	"assistente/internal/commandui"
	"github.com/google/uuid"
)

// commandDeckOccurrence é a prova transitória do evento físico que autorizou
// uma reserva UI. O candidato não pode carregar essa autoridade por conta
// própria: ela só é recuperada pelo invocation ID gerado pelo broker.
type commandDeckOccurrence struct {
	ctx           context.Context
	versions      commandexecution.Versions
	originVersion string
	identity      string
	serial        string
	instanceID    string
	generation    uint64
	foreground    *commandforeground.Snapshot
	visual        *localCommandKeyboardContextProof
	page          bool
	keyboard      *localCommandKeyboardState
	controller    *commandDeckController
}

type commandDeckExecution struct {
	mu          sync.Mutex
	service     *commandexecution.Service
	occurrences map[string]commandDeckOccurrence
	offers      map[string]commandDeckContextualOffer
}

type commandDeckTrigger struct {
	Version int    `json:"version"`
	Device  string `json:"device"`
	Key     int    `json:"key"`
}

func (a *App) newCommandDeckExecutor(p *commandProductRuntime, base commandexecution.Config, host *commandexecution.HostState, policy func(context.Context, auth.LocalSessionPrincipal, string, commandcatalog.Source) error) (*commandDeckExecution, error) {
	if p == nil || host == nil || policy == nil || base.Envelope == nil || base.Registry == nil ||
		!base.Registry.Complete() || len(base.Handlers) == 0 || base.Now == nil ||
		base.Envelope.Snapshot == nil || base.Envelope.Actor == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}

	config := base
	config.Source = commandcatalog.StreamDeck
	envelope := *base.Envelope
	envelope.Snapshot = func(ctx context.Context, owner auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
		value, err := base.Envelope.Snapshot(ctx, owner, candidate)
		if err != nil {
			return commandcontract.Envelope{}, err
		}
		occurrence, ok := commandDeckOccurrenceFor(p, candidate.InvocationID)
		if !ok || !p.deckExecutionAllowed(occurrence.generation) || candidate.TriggerType != string(commandcatalog.StreamDeck) || occurrence.ctx == nil || occurrence.ctx.Err() != nil ||
			owner != p.principal || candidate.InvocationID == "" || !validCommandDeckInstanceID(occurrence.instanceID) {
			return commandcontract.Envelope{}, commandexecution.ErrStale
		}
		identity, triggerErr := (commandconfig.StreamDeckTriggerPort{}).Normalize(ctx, candidate.TriggerSpec)
		trigger, parseErr := commandconfig.ParseStreamDeckTriggerIdentity(identity)
		err = triggerErr
		if err == nil {
			err = parseErr
		}
		if err != nil || identity != occurrence.identity || trigger.Device != occurrence.serial {
			return commandcontract.Envelope{}, commandexecution.ErrStale
		}
		current, err := host.Snapshot(ctx, owner)
		if err != nil || current != occurrence.versions || !current.Unlocked || !p.dependenciesMatch(p.app) {
			return commandcontract.Envelope{}, commandexecution.ErrStale
		}
		if occurrence.originVersion != "" {
			configuration, _, _, snapshotErr := p.host.ResolutionSnapshot(ctx, p.principal)
			if snapshotErr != nil {
				return commandcontract.Envelope{}, snapshotErr
			}
			required := configuration.RequiredFacts(occurrence.identity)
			// Este callback pode executar sob o gate. Snapshot ausente deve
			// recusar o fato físico, nunca provocar uma nova captura nativa.
			origin, originErr := p.deckOccurrenceOriginFacts(ctx, occurrence, required)
			if originErr != nil || origin.version != occurrence.originVersion || validateCommandOriginContext(required, origin, occurrence.serial) != nil {
				return commandcontract.Envelope{}, commandexecution.ErrStale
			}
		}
		value.SourceInstanceID = commandStringPointer(occurrence.instanceID)
		value.SourceEventID = commandStringPointer(candidate.InvocationID)
		value.ObserverType = commandStringPointer(string(commandcatalog.StreamDeck))
		value.ObservedTriggerType = commandStringPointer(string(commandcatalog.StreamDeck))
		value.ForegroundSnapshot, err = foregroundSummaryRaw(occurrence.foreground)
		if err != nil {
			return commandcontract.Envelope{}, err
		}
		return value, nil
	}
	if base.Envelope.Resolve == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	envelope.Resolve = base.Envelope.Resolve
	envelope.Authorize = func(ctx context.Context, owner auth.LocalSessionPrincipal, e commandcontract.Envelope, definition commandcatalog.Definition) error {
		if p.currentDeckCapture() != nil {
			return commandexecution.ErrDenied
		}
		if !commandDeckDefinitionEligible(definition) ||
			!definition.AllowsSource(commandcatalog.StreamDeck) || e.SourceType == nil || *e.SourceType != commandcontract.SourceStreamDeck ||
			!p.dependenciesMatch(p.app) {
			return commandexecution.ErrDenied
		}
		return policy(ctx, owner, definition.ID, commandcatalog.StreamDeck)
	}
	envelope.AuthorizeLookup = func(ctx context.Context, owner auth.LocalSessionPrincipal, record commandledger.FullRecord) error {
		definition, definitionOK := commandcatalog.Definition{}, false
		if record.Envelope.CommandID != nil && p.registry != nil {
			definition, definitionOK = p.registry.Lookup(*record.Envelope.CommandID)
		}
		if !commandRecordMatchesPrincipal(record, owner) || record.Envelope.SourceType == nil ||
			*record.Envelope.SourceType != commandcontract.SourceStreamDeck || record.Envelope.CommandID == nil ||
			!definitionOK || !commandDeckLedgerCommand(definition) || !p.dependenciesMatch(p.app) {
			return commandexecution.ErrDenied
		}
		return policy(ctx, owner, *record.Envelope.CommandID, commandcatalog.StreamDeck)
	}
	config.Envelope = &envelope
	service, err := a.newCommandDesktopExecutor(config, host)
	if err != nil {
		return nil, err
	}
	state := &commandDeckExecution{service: service, occurrences: make(map[string]commandDeckOccurrence)}
	return state, nil
}

// beginDeckCommand é chamado somente pelo adapter nativo confiável. Ele
// reserva uma ação UI; a entrega à UI continua passando por TakeUICommand.
func (p *commandProductRuntime) beginDeckCommand(ctx context.Context, serial, instanceID string, key int, versions commandexecution.Versions, commandID string, generation uint64, profileStamp string) (commandui.Reservation, error) {
	return p.beginDeckCommandWithOrigin(ctx, serial, instanceID, key, versions, commandID, generation, profileStamp, commandOriginContext{})
}

// beginDeckDurableCommand executes durable Deck actions directly through the
// common executor. They never create a UI reservation.
func (p *commandProductRuntime) beginDeckDurableCommand(ctx context.Context, serial, instanceID string, key int, versions commandexecution.Versions, commandID string, generation uint64, captured commandOriginContext) (string, error) {
	if p == nil || ctx == nil || ctx.Err() != nil || !validCommandDeckInstanceID(instanceID) || serial == "" || key < 0 || !versions.Unlocked || !isCommandLayerAction(commandID) || !p.deckExecutionAllowed(generation) || p.deckExecution == nil || p.deckExecution.service == nil {
		return "", commandexecution.ErrDenied
	}
	trigger := commandDeckTrigger{Version: 1, Device: serial, Key: key}
	raw, err := json.Marshal(trigger)
	if err != nil {
		return "", err
	}
	identity, err := (commandconfig.StreamDeckTriggerPort{}).Normalize(ctx, raw)
	if err != nil {
		return "", err
	}
	configuration, _, current, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil || current != versions || !current.Unlocked {
		return "", commandexecution.ErrStale
	}
	required := configuration.RequiredFacts(identity)
	if captured.facts == nil {
		captured, err = p.commandOriginFactsWithDevice(ctx, commandcatalog.StreamDeck, required, serial)
	} else {
		err = validateCommandOriginContext(required, captured, serial)
	}
	if err != nil {
		return "", err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	invocationID := id.String()
	state := p.deckExecution
	state.mu.Lock()
	if len(state.occurrences) >= 64 {
		state.mu.Unlock()
		return "", commandexecution.ErrStale
	}
	state.occurrences[invocationID] = commandDeckOccurrence{ctx: ctx, versions: versions, originVersion: captured.version, identity: identity, serial: serial, instanceID: instanceID, generation: generation, foreground: cloneForegroundSnapshot(captured.foreground)}
	state.mu.Unlock()
	p.mu.Lock()
	if p.closed || p.deckCapture != nil || p.deckCaptureGeneration != generation {
		p.mu.Unlock()
		state.remove(invocationID)
		return "", commandexecution.ErrStale
	}
	p.workers.Add(1)
	p.mu.Unlock()
	go func() {
		defer p.workers.Done()
		defer state.remove(invocationID)
		record, executeErr := state.service.ExecuteEnvelope(ctx, "", commandexecution.EnvelopeCandidate{InvocationID: invocationID, CorrelationID: invocationID, TriggerType: string(commandcatalog.StreamDeck), TriggerSpec: raw, Arguments: json.RawMessage(`{}`)})
		if executeErr != nil || record.Status != commandledger.Succeeded {
			// O executor já persiste o estado terminal; não há handoff visual
			// para relatar. Manter a leitura explícita evita perder falhas.
			return
		}
	}()
	return invocationID, nil
}

func (p *commandProductRuntime) beginDeckCommandWithOrigin(ctx context.Context, serial, instanceID string, key int, versions commandexecution.Versions, commandID string, generation uint64, profileStamp string, captured commandOriginContext) (commandui.Reservation, error) {
	if !p.deckExecutionAllowed(generation) {
		return commandui.Reservation{}, commandexecution.ErrStale
	}
	definition, definitionOK := commandcatalog.Definition{}, false
	if p != nil && p.registry != nil {
		definition, definitionOK = p.registry.Lookup(commandID)
	}
	if p == nil || ctx == nil || ctx.Err() != nil || strings.TrimSpace(serial) != serial || serial == "" || !validCommandDeckInstanceID(instanceID) || key < 0 ||
		!versions.Unlocked || commandID == "" || !definitionOK || !commandDeckLedgerCommand(definition) || p.app == nil || p.app.commandProduct.Load() != p ||
		!p.dependenciesMatch(p.app) {
		return commandui.Reservation{}, commandexecution.ErrDenied
	}
	state := p.deckExecution
	if state == nil || state.service == nil {
		return commandui.Reservation{}, commandexecution.ErrDenied
	}
	trigger := commandDeckTrigger{Version: 1, Device: serial, Key: key}
	raw, err := json.Marshal(trigger)
	if err != nil {
		return commandui.Reservation{}, commandexecution.ErrInvalidRequest
	}
	identity, err := (commandconfig.StreamDeckTriggerPort{}).Normalize(ctx, raw)
	if err != nil {
		return commandui.Reservation{}, err
	}
	configuration, _, current, snapshotErr := p.host.ResolutionSnapshot(ctx, p.principal)
	if snapshotErr != nil || current != versions || !current.Unlocked {
		return commandui.Reservation{}, commandexecution.ErrStale
	}
	required := configuration.RequiredFacts(identity)
	if (len(required) == 0) != (profileStamp == "") {
		return commandui.Reservation{}, commandexecution.ErrStale
	}
	origin := captured
	var originErr error
	if origin.facts == nil {
		origin, originErr = p.commandOriginFactsWithDevice(ctx, commandcatalog.StreamDeck, required, serial)
	} else {
		originErr = validateCommandOriginContext(required, origin, serial)
	}
	if originErr != nil || (profileStamp != "" && origin.version != profileStamp) {
		return commandui.Reservation{}, commandexecution.ErrStale
	}
	var invocationID string
	reservation, err := p.app.beginCommandUIWithCleanup(p, commandID, func(reservation commandui.Reservation) (commandexecution.EnvelopeCandidate, error) {
		invocationID = reservation.InvocationID
		state.mu.Lock()
		defer state.mu.Unlock()
		if ctx.Err() != nil || len(state.occurrences) >= 64 {
			return commandexecution.EnvelopeCandidate{}, commandexecution.ErrStale
		}
		state.occurrences[invocationID] = commandDeckOccurrence{ctx: ctx, versions: versions, originVersion: profileStamp, identity: identity, serial: serial, instanceID: instanceID, generation: generation, foreground: cloneForegroundSnapshot(origin.foreground)}
		return commandexecution.EnvelopeCandidate{InvocationID: invocationID, CorrelationID: invocationID,
			TriggerType: string(commandcatalog.StreamDeck), TriggerSpec: raw, Arguments: json.RawMessage(`{}`)}, nil
	}, func(runCtx context.Context, candidate commandexecution.EnvelopeCandidate) (commandledger.FullRecord, error) {
		return state.service.ExecuteEnvelope(runCtx, "", candidate)
	}, func() bool {
		if ctx.Err() != nil || p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) || !p.deckExecutionAllowed(generation) {
			return false
		}
		current, err := p.host.Snapshot(ctx, p.principal)
		if err != nil || current != versions || !current.Unlocked {
			return false
		}
		if profileStamp == "" {
			return true
		}
		return originVersionCurrent(ctx, origin, profileStamp)
	}, nil, func() { state.remove(invocationID) })
	if err != nil {
		state.remove(invocationID)
		return commandui.Reservation{}, err
	}
	return reservation, nil
}

func validateCommandOriginContext(required []commandbindings.Field, origin commandOriginContext, serial string) error {
	for _, field := range required {
		value, ok := origin.facts[field]
		if !ok {
			return commandexecution.ErrDenied
		}
		if field == commandbindings.Device && value != serial {
			return commandexecution.ErrDenied
		}
	}
	if containsOriginField(required, commandbindings.Process) && origin.foreground == nil {
		return commandexecution.ErrDenied
	}
	return nil
}

func originVersionCurrent(ctx context.Context, origin commandOriginContext, want string) bool {
	if err := ctx.Err(); err != nil || origin.version != want {
		return false
	}
	if origin.foreground != nil && (origin.foreground.Identity.IsZero() || origin.foreground.CapturedAt.IsZero() || time.Until(origin.foreground.CapturedAt) > 0 || time.Since(origin.foreground.CapturedAt) > commandPhysicalOriginMaxAge) {
		return false
	}
	return true
}

func cloneForegroundSnapshot(snapshot *commandforeground.Snapshot) *commandforeground.Snapshot {
	if snapshot == nil {
		return nil
	}
	copy := *snapshot
	return &copy
}

func commandDeckOccurrenceFor(p *commandProductRuntime, invocationID string) (commandDeckOccurrence, bool) {
	if p == nil || p.deckExecution == nil || invocationID == "" {
		return commandDeckOccurrence{}, false
	}
	state := p.deckExecution
	state.mu.Lock()
	defer state.mu.Unlock()
	occurrence, exists := state.occurrences[invocationID]
	return occurrence, exists
}

func (s *commandDeckExecution) remove(invocationID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.occurrences, invocationID)
	s.mu.Unlock()
}

func commandDeckLedgerCommand(definition commandcatalog.Definition) bool {
	return commandDeckDefinitionEligible(definition) &&
		(commandExecutionClassForDefinition(definition) == commandExecutionAuditedUI || isWorkspaceMutationCommand(definition.ID) || (commandExecutionClassForDefinition(definition) == commandExecutionDurable && isCommandLayerAction(definition.ID)))
}

func commandDeckDefinitionEligible(definition commandcatalog.Definition) bool {
	class := commandExecutionClassForDefinition(definition)
	return commandDeckUICommand(definition.ID) && definition.AllowsSource(commandcatalog.StreamDeck) &&
		(class == commandExecutionLocalUI || class == commandExecutionAuditedUI || isWorkspaceMutationCommand(definition.ID) || (class == commandExecutionDurable && isCommandLayerAction(definition.ID)))
}

func commandDeckUICommand(id string) bool {
	if _, _, navigation := workspaceTabNavigationForCommand(id); navigation {
		return true
	}
	if id == commandProductShortcutsShowID || id == commandWorkspacePanelFocusID || isWorkspaceMutationCommand(id) || isCommandLayerAction(id) {
		return true
	}
	for _, navigation := range commandProductUINavigation {
		if navigation.id == id {
			return true
		}
	}
	for _, picker := range commandProductChatPickers {
		if picker.id == id {
			return true
		}
	}
	for _, item := range commandProductPagePresentation {
		if item.id == id {
			return true
		}
	}
	for _, editorCommand := range commandProductEditorMenus {
		if editorCommand.id == id {
			return true
		}
	}
	if isAuditedUIContextualCommand(id) {
		return true
	}
	return false
}

func validCommandDeckInstanceID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.String() == value && id.Version() == 7 && id.Variant() == uuid.RFC4122
}
