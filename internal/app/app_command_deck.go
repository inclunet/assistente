package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"assistente/internal/commandadapter"
	"assistente/internal/commandbindings"
	"assistente/internal/commandbridge"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddeck"
	"assistente/internal/commandexecution"
	"assistente/internal/commandforeground"
	"assistente/internal/commandinput"
	"assistente/internal/commandruntime"
	"github.com/google/uuid"
)

type commandDeckBinding struct {
	commandID, title     string
	icon                 string
	imageRef             string
	imagePNG             []byte
	feedbackState        string
	feedbackInvocationID string
	persistentState      string
	variants             map[string]commandDeckVisual
	identity             string
	profileBound         bool
	conditions           []LocalCommandPaletteCondition
	origin               commandOriginContext
}
type commandDeckMap map[string]map[int]commandDeckBinding

// Compiled before opening the input epoch. Conditions are immutable and belong
// to this exact configuration/registry snapshot, not to the current UI context.
// Native facts (including foreground/profile) are still captured per press.
type commandDeckCompiledTrigger struct {
	identity      string
	configuration *commandbindings.Configuration
	registry      *commandcatalog.Registry
	versions      commandexecution.Versions
	required      []commandbindings.Field
	conditions    []LocalCommandPaletteCondition
}

type CommandDeckLocalUIEvent struct {
	CommandID   string                         `json:"commandId"`
	Generation  string                         `json:"generation"`
	UserID      string                         `json:"userId"`
	SessionID   string                         `json:"sessionId"`
	WorkspaceID string                         `json:"workspaceId"`
	Conditions  []LocalCommandPaletteCondition `json:"conditions,omitempty"`
}

// The native driver is the only producer. No public Wails method can forge
// a physical occurrence. Each map owns its epoch, handles and pressed state.
type commandDeckController struct {
	p                 *commandProductRuntime
	ctx               context.Context
	versions          commandexecution.Versions
	identities        map[string]map[int][]commandDeckCompiledTrigger
	mu                sync.Mutex
	pressed           map[string]bool
	instances         map[string]commandDeckInstance
	models            map[string]string
	generation        uint64
	feedbackAnnounced map[string]string
}

type commandDeckInstance struct {
	id     string
	ctx    context.Context
	cancel context.CancelFunc
}

func (c *commandDeckController) opened(serial string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.instances[serial]; ok {
		old.cancel()
	}
	for key := range c.pressed {
		if strings.HasPrefix(key, "streamdeck.key:"+serial+":") {
			delete(c.pressed, key)
		}
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.instances[serial] = commandDeckInstance{id: uuid.Must(uuid.NewV7()).String(), ctx: ctx, cancel: cancel}
}

func (c *commandDeckController) Input(ctx context.Context, event commandadapter.Event) (commandbridge.InvocationAck, error) {
	if ctx.Err() != nil || c.ctx.Err() != nil {
		return commandbridge.InvocationAck{}, commandexecution.ErrStale
	}
	serial := strings.TrimPrefix(event.SourceInstance, "streamdeck.key:")
	index, err := strconv.Atoi(strings.TrimPrefix(event.Key, "key:"))
	if err != nil || event.SourceInstance != "streamdeck.key:"+serial || event.Key != fmt.Sprintf("key:%d", index) {
		return commandbridge.InvocationAck{}, commandexecution.ErrDenied
	}
	c.mu.Lock()
	model := c.models[serial]
	if event.Kind == commandinput.KeyUp {
		delete(c.pressed, event.SourceInstance+":"+event.Key)
	}
	c.mu.Unlock()
	if c.p.captureDeckInput(ctx, event, serial, index, model) {
		return commandbridge.InvocationAck{}, nil
	}
	key := event.SourceInstance + ":" + event.Key
	c.mu.Lock()
	if event.Kind == commandinput.KeyUp {
		delete(c.pressed, key)
		c.mu.Unlock()
		return commandbridge.InvocationAck{}, nil
	}
	if event.Kind != commandinput.KeyDown || event.Repeat || c.pressed[key] {
		c.mu.Unlock()
		return commandbridge.InvocationAck{}, nil
	}
	c.pressed[key] = true
	instance, exists := c.instances[serial]
	c.mu.Unlock()
	if !exists || instance.ctx.Err() != nil {
		return commandbridge.InvocationAck{}, commandexecution.ErrStale
	}
	// A troca de aba pode envelhecer somente o guard da projeção de jobs.
	// Revalidar antes de consultar o snapshot evita depender de Alt+Tab para
	// reconstruí-lo. Mudanças efetivas ainda invalidam as versões capturadas.
	if err := c.p.refreshCommandJobProjection(ctx); err != nil {
		return commandbridge.InvocationAck{}, err
	}
	resolvedBinding, profileStamp, resolved := c.p.resolveDeckPress(ctx, c.identities, c.versions, serial, index)
	if !resolved {
		return commandbridge.InvocationAck{}, commandexecution.ErrStale
	}
	binding := resolvedBinding
	conditionalUI := binding.commandID == "" && len(binding.conditions) != 0
	if definition, ok := c.p.registry.Lookup(binding.commandID); ok && commandExecutionClassForDefinition(definition) == commandExecutionDurable && isCommandLayerAction(binding.commandID) {
		invocationID, durableErr := c.p.beginDeckDurableCommand(instance.ctx, serial, instance.id, index, c.versions, binding.commandID, c.generation, binding.origin)
		if durableErr != nil {
			return commandbridge.InvocationAck{}, durableErr
		}
		return commandbridge.InvocationAck{InvocationID: invocationID, Accepted: true}, nil
	}
	if conditionalUI || isLocalUICommand(binding.commandID) {
		if c.p.app.emitter == nil {
			return commandbridge.InvocationAck{}, commandexecution.ErrDenied
		}
		if binding.profileBound && !conditionalUI {
			return commandbridge.InvocationAck{}, commandexecution.ErrDenied
		}
		generation, keyboardVersions, keyboardReady := c.p.localKeyboardState()
		lifecycle, lifecycleErr := CommandLifecycleSnapshot(c.p.app)
		current, snapshotErr := c.p.host.Snapshot(ctx, c.p.principal)
		session, sessionErr := c.p.sessionSvc.RevalidateLocalSession(ctx, c.p.principal)
		if instance.ctx.Err() != nil || !keyboardReady || generation == "" || keyboardVersions != c.versions || c.p.currentDeckCapture() != nil ||
			snapshotErr != nil || current != c.versions || !current.Unlocked ||
			lifecycleErr != nil || lifecycle.State != commandruntime.StateReady || !lifecycle.Published ||
			sessionErr != nil || session != c.p.principal || !c.p.deckExecutionAllowed(c.generation) ||
			!c.p.dependenciesMatch(c.p.app) || c.p.app.commandProduct.Load() != c.p {
			return commandbridge.InvocationAck{}, commandexecution.ErrStale
		}
		if conditionalUI && deckConditionsHaveContextualCommand(binding.conditions) {
			offer, err := c.p.createDeckContextualOffer(instance.ctx, serial, instance.id, index, c.versions, c.generation, generation, binding.conditions, c)
			if err != nil {
				return commandbridge.InvocationAck{}, err
			}
			c.p.app.emitter.Emit("command:deck-contextual-ui", offer)
			return commandbridge.InvocationAck{Accepted: true}, nil
		}
		c.p.app.emitter.Emit("command:deck-local-ui", CommandDeckLocalUIEvent{
			CommandID: binding.commandID, Generation: generation, UserID: c.p.principal.UserID,
			SessionID: c.p.principal.SessionID, WorkspaceID: c.p.workspaceID,
			Conditions: cloneLocalCommandPaletteConditions(binding.conditions),
		})
		return commandbridge.InvocationAck{Accepted: true}, nil
	}
	reservation, err := c.p.beginDeckCommandWithOrigin(instance.ctx, serial, instance.id, index, c.versions, binding.commandID, c.generation, profileStamp, binding.origin)
	if err != nil {
		return commandbridge.InvocationAck{}, err
	}
	if c.p.app.emitter == nil {
		return commandbridge.InvocationAck{}, commandexecution.ErrDenied
	}
	c.p.app.emitter.Emit("command:deck-ui-reservation", reservation)
	return commandbridge.InvocationAck{InvocationID: reservation.InvocationID, Accepted: true}, nil
}
func (c *commandDeckController) Lock(context.Context) error   { c.reset(""); return nil }
func (c *commandDeckController) Logout(context.Context) error { c.reset(""); return nil }
func (c *commandDeckController) reset(serial string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, instance := range c.instances {
		if serial == "" || id == serial {
			instance.cancel()
			delete(c.instances, id)
		}
	}
	for key := range c.pressed {
		if serial == "" || strings.HasPrefix(key, "streamdeck.key:"+serial+":") {
			delete(c.pressed, key)
		}
	}
}

// A worker from a retired connection cannot erase the newer connection's
// occurrence authority or pressed state when reconnect races its cleanup.
func (c *commandDeckController) retire(serial, instanceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	instance, ok := c.instances[serial]
	if !ok || instance.id != instanceID {
		return
	}
	instance.cancel()
	delete(c.instances, serial)
	for key := range c.pressed {
		if strings.HasPrefix(key, "streamdeck.key:"+serial+":") {
			delete(c.pressed, key)
		}
	}
}

// clearDeckDownForDisconnect releases only the physical edge ledger for a
// confirmed hardware disconnect. Epoch/session rebuilds deliberately do not
// call this: a held key must remain suppressed until its native KeyUp.
func (p *commandProductRuntime) clearDeckDownForDisconnect(serial string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.deckDown {
		if strings.HasPrefix(key, "streamdeck.key:"+serial+":") {
			delete(p.deckDown, key)
		}
	}
}

type commandDeckDriverFilter struct {
	commanddeck.Driver
	bindings  commandDeckMap
	potential map[string]map[int][]commandDeckCompiledTrigger
	capture   bool
}

func (d commandDeckDriverFilter) Enumerate(ctx context.Context) ([]commanddeck.PhysicalDevice, error) {
	devices, err := d.Driver.Enumerate(ctx)
	if err != nil {
		return nil, err
	}
	var selected []commanddeck.PhysicalDevice
	for _, device := range devices {
		serial := string(device.ID)
		if d.capture || len(d.bindings[serial]) != 0 || len(d.potential[serial]) != 0 {
			selected = append(selected, device)
		}
	}
	return selected, nil
}

func (p *commandProductRuntime) deckMap(ctx context.Context) (commandDeckMap, commandexecution.Versions, error) {
	if !p.dependenciesMatch(p.app) || p.app.commandProduct.Load() != p {
		return nil, commandexecution.Versions{}, commandexecution.ErrDenied
	}
	configuration, activeIDs, versions, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil || !versions.Unlocked {
		return nil, versions, commandexecution.ErrDenied
	}
	bindings := commandDeckMap{}
	var frameForeground *commandforeground.Snapshot
	frameCaptureFailed := false
	locale := p.getDeckLocale()
	appPage := p.currentDeckPagePresentation()
	for _, identity := range configuration.TriggerIdentities() {
		if !strings.HasPrefix(identity, "streamdeck.key:") {
			continue
		}
		spec, err := commandconfig.ParseStreamDeckTriggerIdentity(identity)
		if err != nil {
			continue
		}
		required := configuration.RequiredFacts(identity)
		if conditions := contextualDeckUIConditions(configuration, p.registry, identity); len(conditions) != 0 {
			if bindings[spec.Device] == nil {
				bindings[spec.Device] = map[int]commandDeckBinding{}
			}
			binding := bindings[spec.Device][spec.Key]
			if binding.commandID != "" {
				continue
			}
			binding.identity = identity
			binding.conditions = append(binding.conditions, conditions...)
			binding.profileBound = true
			visual, variants := localDeckPresentationsForPage(configuration, p.registry, identity, binding.conditions, locale, appPage)
			binding.title, binding.icon, binding.imageRef, binding.variants = visual.title, visual.icon, visual.imageRef, variants
			bindings[spec.Device][spec.Key] = binding
			continue
		}
		if containsOriginField(required, commandbindings.Process) && frameForeground == nil && !frameCaptureFailed {
			captured, captureErr := p.capturePhysicalForeground(ctx)
			if captureErr != nil {
				frameCaptureFailed = true
			} else {
				frameForeground = &captured
			}
		}
		if frameCaptureFailed && containsOriginField(required, commandbindings.Process) {
			continue
		}
		var originContext commandOriginContext
		if frameForeground != nil {
			originContext, err = p.commandOriginFactsFromSnapshot(ctx, commandcatalog.StreamDeck, required, spec.Device, frameForeground)
		} else {
			originContext, err = p.commandOriginFactsFromSnapshot(ctx, commandcatalog.StreamDeck, required, spec.Device, nil)
		}
		if err != nil {
			continue
		}
		resolved, err := configuration.Resolve(identity, originContext.facts, nil)
		if err != nil || resolved.Status != commandbindings.Selected || resolved.ExecutionScopeKey != "global" {
			continue
		}
		definition, ok := p.registry.Lookup(resolved.CommandID)
		if !ok || !commandDeckDefinitionEligible(definition) {
			continue
		}
		title := commandDeckTitle(configuration, resolved.BindingIDs, definition, locale)
		if bindings[spec.Device] == nil {
			bindings[spec.Device] = map[int]commandDeckBinding{}
		}
		profileBound := len(configuration.RequiredFacts(identity)) != 0
		if profileBound && commandExecutionClassForDefinition(definition) == commandExecutionLocalUI {
			continue
		}
		bindings[spec.Device][spec.Key] = commandDeckBinding{commandID: definition.ID, title: title, icon: configuration.IconForBindings(resolved.BindingIDs), imageRef: configuration.ImageForBindings(resolved.BindingIDs), identity: identity, profileBound: profileBound,
			persistentState: p.commandDeckPersistentState(ctx, resolved, activeIDs, versions),
			variants:        commandDeckStateVisuals(configuration, resolved.BindingIDs, definition, locale)}
	}
	return bindings, versions, nil
}

func (p *commandProductRuntime) deckTriggerMap(ctx context.Context) (map[string]map[int][]commandDeckCompiledTrigger, commandexecution.Versions, error) {
	if !p.dependenciesMatch(p.app) || p.app.commandProduct.Load() != p {
		return nil, commandexecution.Versions{}, commandexecution.ErrDenied
	}
	configuration, _, versions, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil || !versions.Unlocked {
		return nil, versions, commandexecution.ErrDenied
	}
	return deckTriggerIdentities(configuration, p.registry, versions), versions, nil
}

func deckTriggerIdentities(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, versions commandexecution.Versions) map[string]map[int][]commandDeckCompiledTrigger {
	result := map[string]map[int][]commandDeckCompiledTrigger{}
	if configuration == nil || registry == nil {
		return result
	}
	for _, identity := range configuration.TriggerIdentities() {
		if !strings.HasPrefix(identity, "streamdeck.key:") {
			continue
		}
		spec, err := commandconfig.ParseStreamDeckTriggerIdentity(identity)
		if err != nil {
			continue
		}
		if result[spec.Device] == nil {
			result[spec.Device] = map[int][]commandDeckCompiledTrigger{}
		}
		result[spec.Device][spec.Key] = append(result[spec.Device][spec.Key], commandDeckCompiledTrigger{
			identity: identity, configuration: configuration, registry: registry, versions: versions,
			required:   configuration.RequiredFacts(identity),
			conditions: contextualDeckUIConditions(configuration, registry, identity),
		})
	}
	return result
}

func (p *commandProductRuntime) resolveDeckPress(ctx context.Context, identities map[string]map[int][]commandDeckCompiledTrigger, versions commandexecution.Versions, serial string, key int) (commandDeckBinding, string, bool) {
	if err := ctx.Err(); err != nil {
		return commandDeckBinding{}, "", false
	}
	configuration, _, current, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil || current != versions || !current.Unlocked {
		return commandDeckBinding{}, "", false
	}
	var pressForeground *commandforeground.Snapshot
	pressCaptureFailed := false
	for _, trigger := range identities[serial][key] {
		if err := ctx.Err(); err != nil {
			return commandDeckBinding{}, "", false
		}
		if trigger.registry != p.registry || trigger.versions != current ||
			(trigger.configuration != configuration && !trigger.configuration.EquivalentExceptValidityDeadline(configuration)) {
			return commandDeckBinding{}, "", false
		}
		identity := trigger.identity
		required := trigger.required
		if conditions := trigger.conditions; len(conditions) != 0 {
			return commandDeckBinding{identity: identity, profileBound: true, conditions: conditions}, "", true
		}
		if containsOriginField(required, commandbindings.Process) && pressForeground == nil && !pressCaptureFailed {
			captured, captureErr := p.capturePhysicalForeground(ctx)
			if captureErr != nil {
				pressCaptureFailed = true
			} else {
				pressForeground = &captured
			}
		}
		if pressCaptureFailed && containsOriginField(required, commandbindings.Process) {
			continue
		}
		var origin commandOriginContext
		var originErr error
		if pressForeground != nil {
			origin, originErr = p.commandOriginFactsFromSnapshot(ctx, commandcatalog.StreamDeck, required, serial, pressForeground)
		} else {
			origin, originErr = p.commandOriginFactsFromSnapshot(ctx, commandcatalog.StreamDeck, required, serial, nil)
		}
		if originErr != nil {
			continue
		}
		resolved, resolveErr := configuration.Resolve(identity, origin.facts, nil)
		if resolveErr != nil || resolved.Status != commandbindings.Selected || resolved.ExecutionScopeKey != "global" {
			continue
		}
		definition, ok := p.registry.Lookup(resolved.CommandID)
		class := commandExecutionLocalUI
		if ok {
			class = commandExecutionClassForDefinition(definition)
		}
		if !ok || !commandDeckDefinitionEligible(definition) {
			continue
		}
		if len(required) != 0 && class == commandExecutionLocalUI {
			continue
		}
		title := definition.ID
		if definition.Presentation != nil {
			if meta, titleOK := definition.Presentation.Locales[p.getDeckLocale()]; titleOK {
				title = meta.Name
			}
		}
		return commandDeckBinding{commandID: definition.ID, title: title, identity: identity, profileBound: len(required) != 0, origin: origin}, origin.version, true
	}
	return commandDeckBinding{}, "", false
}

func (p *commandProductRuntime) startDeck(ctx context.Context, driver commanddeck.Driver) {
	p.mu.Lock()
	if p.closed || p.deckCancel != nil {
		p.mu.Unlock()
		return
	}
	run, cancel := context.WithCancel(ctx)
	p.deckCancel = cancel
	p.deckDriver = driver
	p.workers.Add(1)
	p.mu.Unlock()
	go func() { defer p.workers.Done(); p.runDeck(run, driver) }()
}

func (p *commandProductRuntime) runDeck(ctx context.Context, driver commanddeck.Driver) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		p.runDeckEpoch(ctx, driver)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *commandProductRuntime) runDeckEpoch(ctx context.Context, driver commanddeck.Driver) {
	// O preview também usa o guard: se vier antes do refresh, uma projeção
	// obsoleta encerra todas as tentativas seguintes antes da recuperação.
	if err := p.refreshCommandJobProjection(ctx); err != nil {
		p.deckStatus("unavailable", nil)
		return
	}
	capture := p.currentDeckCapture()
	// O preview evita abrir HID sem bindings físicos. Apenas uma projeção
	// envelhecida acima exige reconstrução; snapshots atuais são só memória.
	_, _, previewErr := p.deckMap(ctx)
	if previewErr != nil {
		p.deckStatus("unavailable", nil)
		return
	}
	identities, identityVersions, identityErr := p.deckTriggerMap(ctx)
	if identityErr != nil || (len(identities) == 0 && capture == nil) {
		p.deckStatus("unconfigured", nil)
		return
	}
	current, err := p.app.authenticatedCommandProduct()
	if err != nil || current != p {
		p.deckStatus("unavailable", nil)
		return
	}
	epoch, err := p.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		principal, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || principal != p.principal || !p.dependenciesMatch(p.app) {
			return "", "", commandexecution.ErrDenied
		}
		return principal.UserID, principal.SessionID, nil
	})
	if err != nil {
		return
	}
	watch, release, err := p.epochs.WatchEpoch(ctx, epoch)
	if err != nil {
		return
	}
	defer release()
	locale := p.getDeckLocale()
	bindings, versions, err := p.deckMap(watch)
	if err != nil || versions != identityVersions {
		return
	}
	if len(bindings) == 0 && capture == nil {
		if len(identities) == 0 {
			p.deckStatus("unconfigured", nil)
			return
		}
		bindings = commandDeckMap{}
	}
	controller := &commandDeckController{p: p, ctx: watch, versions: versions, identities: identities, pressed: map[string]bool{}, instances: map[string]commandDeckInstance{}, models: map[string]string{}, generation: p.deckInputGeneration(), feedbackAnnounced: map[string]string{}}
	manager := commanddeck.NewManager(nil, commanddeck.BackoffPolicy{Initial: time.Second, Max: 8 * time.Second})
	adapter, err := commanddeck.NewDeviceAdapter(manager, controller)
	if err != nil {
		return
	}
	runtime, err := commanddeck.NewRuntime(commandDeckDriverFilter{Driver: driver, bindings: bindings, potential: identities, capture: capture != nil}, adapter)
	if err != nil {
		return
	}
	var polls sync.WaitGroup
	polling := map[commanddeck.DeviceID]bool{}
	var pollMu sync.Mutex
	defer func() {
		release()
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runtime.Shutdown(cleanup)
		polls.Wait()
		p.deckStatus("unavailable", nil)
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	imageRetries := map[commanddeck.DeviceID]time.Time{}
	for watch.Err() == nil {
		if p.getDeckLocale() != locale || p.currentDeckCapture() != capture || p.deckInputGeneration() != controller.generation {
			return
		}
		// O polling visual compartilha o epoch com a entrada física. Atualizar
		// o guard antes do preview evita cancelar uma tecla em processamento
		// apenas porque a navegação anterior trocou a aba ativa.
		if err := p.refreshCommandJobProjection(watch); err != nil {
			return
		}
		freshBindings, latestVersions, latestErr := p.deckMap(watch)
		if latestErr != nil {
			return
		}
		if latestVersions != versions {
			return
		}
		latestBindings := controller.overlayDeckFeedback(watch, freshBindings)
		mapDirty := !reflect.DeepEqual(bindings, latestBindings)
		if mapDirty {
			bindings = latestBindings
		}
		results := runtime.DiscoverDetailed(watch)
		status := "disconnected"
		hasDeviceIssue := false
		var devices []commandDeckDeviceStatus
		for _, result := range results {
			snapshot, err := manager.Snapshot(result.Device)
			device, connected := commandDeckDiscoveryStatus(result, snapshot, err)
			if result.Device != "" {
				devices = append(devices, device)
			}
			if !connected {
				hasDeviceIssue = true
				continue
			}
			status = "connected"
			retryAt := imageRetries[result.Device]
			if result.Opened || mapDirty || !retryAt.IsZero() && !time.Now().Before(retryAt) {
				controller.mu.Lock()
				controller.models[string(result.Device)] = snapshot.Model.Name
				controller.mu.Unlock()
				if result.Opened {
					controller.opened(string(result.Device))
				}
				renderBindings := bindings[string(result.Device)]
				if result.Opened {
					// A reconnect creates a new input instance. Rebuild the local
					// overlay after opening it so a frame can never inherit the
					// previous instance's invocation feedback.
					refreshed := controller.overlayDeckFeedback(watch, freshBindings)
					renderBindings = refreshed[string(result.Device)]
					if bindings[string(result.Device)] == nil {
						bindings[string(result.Device)] = map[int]commandDeckBinding{}
					}
					for index, binding := range renderBindings {
						bindings[string(result.Device)][index] = binding
					}
				}
				frame := commanddeck.Frame{Device: result.Device, Model: snapshot.Model, Keys: map[int]commanddeck.KeyView{}}
				retryImages := false
				for index, binding := range renderBindings {
					if index >= snapshot.Model.KeyCount() {
						continue
					}
					view, retry := p.commandDeckImageKeyView(watch, binding, locale, snapshot.Model)
					frame.Keys[index] = view
					retryImages = retryImages || retry
				}
				delete(imageRetries, result.Device)
				if retryImages {
					imageRetries[result.Device] = time.Now().Add(5 * time.Second)
				}
				if err := runtime.Render(watch, frame); err != nil {
					continue
				}
				controller.announceDeckFeedback(watch, string(result.Device), renderBindings)
			}
			pollMu.Lock()
			if !polling[result.Device] {
				polling[result.Device] = true
				polls.Add(1)
				controller.mu.Lock()
				instanceID := controller.instances[string(result.Device)].id
				controller.mu.Unlock()
				go func(device commanddeck.DeviceID, instanceID string) {
					defer polls.Done()
					defer func() {
						controller.retire(string(device), instanceID)
						pollMu.Lock()
						delete(polling, device)
						pollMu.Unlock()
					}()
					for watch.Err() == nil {
						if err := runtime.PollOne(watch, device); err != nil {
							state, stateErr := manager.Snapshot(device)
							if stateErr != nil || state.Status != commanddeck.DeviceConnected || watch.Err() != nil {
								if watch.Err() == nil && (stateErr != nil || state.Status != commanddeck.DeviceConnected) {
									p.clearDeckDownForDisconnect(string(device))
								}
								return
							}
							// A refused command does not disconnect healthy hardware.
							// Continue draining key-up so another press remains possible.
						}
					}
				}(result.Device, instanceID)
			}
			pollMu.Unlock()
		}
		if hasDeviceIssue {
			if status == "connected" {
				status = "degraded"
			} else {
				status = "unavailable"
			}
		}
		p.deckStatus(status, devices)
		if capture != nil {
			captureStatus := "no_device"
			switch status {
			case "connected", "degraded":
				captureStatus = "waiting"
			case "unavailable":
				captureStatus = "unavailable"
			}
			p.captureStatus(capture, captureStatus)
		}
		select {
		case <-watch.Done():
			return
		case <-ticker.C:
		}
	}
}

type commandDeckDeviceStatus struct {
	ID       string `json:"id"`
	Model    string `json:"model"`
	KeyCount int    `json:"keyCount"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

// Erros nativos podem conter paths/serial. Somente códigos fechados atravessam
// a ponte; AlreadyOpen com snapshot conectado é o handle deste runtime, não
// evidência de disputa com outro aplicativo.
func commandDeckDiscoveryStatus(result commanddeck.DiscoverResult, snapshot commanddeck.DeviceSnapshot, snapshotErr error) (commandDeckDeviceStatus, bool) {
	device := commandDeckDeviceStatus{ID: string(result.Device), Model: result.Model.Name, KeyCount: result.Model.KeyCount(), Status: "unavailable", Reason: "open_failed"}
	if snapshotErr == nil && snapshot.Status == commanddeck.DeviceConnected &&
		(result.Err == nil || errors.Is(result.Err, commanddeck.ErrDeviceAlreadyOpen)) {
		device.Model, device.KeyCount = snapshot.Model.Name, snapshot.Model.KeyCount()
		device.Status, device.Reason = "connected", ""
		return device, true
	}
	if errors.Is(result.Err, commanddeck.ErrReconnectBackoff) {
		device.Status, device.Reason = "reconnecting", "reconnect_backoff"
	}
	return device, false
}

func (p *commandProductRuntime) setDeckLocale(locale string) {
	if locale != "pt-BR" && locale != "es" {
		locale = "en"
	}
	p.mu.Lock()
	p.deckLocale = locale
	p.mu.Unlock()
}
func (p *commandProductRuntime) getDeckLocale() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deckLocale == "" {
		return "en"
	}
	return p.deckLocale
}
func (p *commandProductRuntime) deckStatus(status string, devices []commandDeckDeviceStatus) {
	if devices == nil {
		devices = []commandDeckDeviceStatus{}
	}
	if p.app.emitter != nil && p.app.commandProduct.Load() == p {
		p.app.emitter.Emit("command:deck-status", struct {
			Status  string                    `json:"status"`
			Devices []commandDeckDeviceStatus `json:"devices"`
		}{status, devices})
	}
}

func commandDeckKeyView(binding commandDeckBinding, locale string, model commanddeck.Model) commanddeck.KeyView {
	binding = commandDeckPresentedBinding(binding)
	icon := binding.icon
	if !commandDeckIconSupported(icon) {
		icon = ""
	}
	imageID := binding.commandID + ":" + binding.title + ":" + locale
	state := "ready"
	if len(binding.conditions) != 0 {
		state = "conditional"
		imageID = binding.identity + ":" + binding.title + ":" + locale
	}
	imageID += ":" + icon
	if len(binding.imagePNG) != 0 {
		imageID += ":" + binding.imageRef
	}
	presentationState := commandDeckPresentationState(binding)
	if presentationState != "" {
		state = presentationState
		imageID += ":feedback:" + binding.feedbackState + ":" + binding.feedbackInvocationID
	}
	imageID += ":state:" + presentationState
	statusLabel := commandDeckFeedbackStatusLabel(locale, presentationState)
	announce := binding.title
	if statusLabel != "" {
		announce += " — " + statusLabel
	}
	return commanddeck.KeyView{Title: binding.title, Announce: announce, State: state, ImageID: imageID, ImageRGBA: commandDeckPresentationImageWithStatus(binding.title, icon, binding.imagePNG, statusLabel, model)}
}

func commandDeckTitle(configuration *commandbindings.Configuration, bindingIDs []string, definition commandcatalog.Definition, locale string) string {
	if title, ok := configuration.TitleForBindings(bindingIDs, locale); ok {
		return title
	}
	if definition.Presentation != nil {
		if metadata, ok := definition.Presentation.Locales[locale]; ok && strings.TrimSpace(metadata.Name) != "" {
			return metadata.Name
		}
	}
	return definition.ID
}
