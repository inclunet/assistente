package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
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
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type commandDeckBinding struct {
	commandID, title string
	identity         string
	profileBound     bool
	conditions       []LocalCommandPaletteCondition
	origin           commandOriginContext
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
	p          *commandProductRuntime
	ctx        context.Context
	versions   commandexecution.Versions
	identities map[string]map[int][]commandDeckCompiledTrigger
	mu         sync.Mutex
	pressed    map[string]bool
	instances  map[string]commandDeckInstance
	models     map[string]string
	generation uint64
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
	configuration, _, versions, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil || !versions.Unlocked {
		return nil, versions, commandexecution.ErrDenied
	}
	bindings := commandDeckMap{}
	var frameForeground *commandforeground.Snapshot
	frameCaptureFailed := false
	locale := p.getDeckLocale()
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
			binding.title = localDeckConditionTitle(binding.conditions, p.registry, locale)
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
		title := definition.ID
		if definition.Presentation != nil {
			if meta, ok := definition.Presentation.Locales[locale]; ok {
				title = meta.Name
			}
		}
		if bindings[spec.Device] == nil {
			bindings[spec.Device] = map[int]commandDeckBinding{}
		}
		profileBound := len(configuration.RequiredFacts(identity)) != 0
		if profileBound && commandExecutionClassForDefinition(definition) == commandExecutionLocalUI {
			continue
		}
		bindings[spec.Device][spec.Key] = commandDeckBinding{commandID: definition.ID, title: title, identity: identity, profileBound: profileBound}
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
		if trigger.configuration != configuration || trigger.registry != p.registry || trigger.versions != current {
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
	capture := p.currentDeckCapture()
	// Idle users with no physical bindings do not query SQLite or open HID.
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
	controller := &commandDeckController{p: p, ctx: watch, versions: versions, identities: identities, pressed: map[string]bool{}, instances: map[string]commandDeckInstance{}, models: map[string]string{}, generation: p.deckInputGeneration()}
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
	for watch.Err() == nil {
		if p.getDeckLocale() != locale || p.currentDeckCapture() != capture || p.deckInputGeneration() != controller.generation {
			return
		}
		latestBindings, latestVersions, latestErr := p.deckMap(watch)
		if latestErr != nil {
			return
		}
		if latestVersions != versions {
			return
		}
		mapDirty := !reflect.DeepEqual(bindings, latestBindings)
		if mapDirty {
			bindings = latestBindings
		}
		results := runtime.DiscoverDetailed(watch)
		status := "disconnected"
		var devices []commandDeckDeviceStatus
		for _, result := range results {
			if result.Err != nil && !errors.Is(result.Err, commanddeck.ErrDeviceAlreadyOpen) {
				status = "unavailable"
				continue
			}
			snapshot, err := manager.Snapshot(result.Device)
			if err != nil {
				continue
			}
			if snapshot.Status != commanddeck.DeviceConnected {
				continue
			}
			status = "connected"
			devices = append(devices, commandDeckDeviceStatus{ID: string(result.Device), Model: snapshot.Model.Name, KeyCount: snapshot.Model.KeyCount(), Status: "connected"})
			if result.Opened || mapDirty {
				controller.mu.Lock()
				controller.models[string(result.Device)] = snapshot.Model.Name
				controller.mu.Unlock()
				if result.Opened {
					controller.opened(string(result.Device))
				}
				frame := commanddeck.Frame{Device: result.Device, Model: snapshot.Model, Keys: map[int]commanddeck.KeyView{}}
				for index, binding := range bindings[string(result.Device)] {
					if index >= snapshot.Model.KeyCount() {
						continue
					}
					imageID := binding.commandID + ":" + locale
					state := "ready"
					if len(binding.conditions) != 0 {
						state = "conditional"
						imageID = binding.identity + ":" + binding.title + ":" + locale
					}
					frame.Keys[index] = commanddeck.KeyView{Title: binding.title, Announce: binding.title, State: state, ImageID: imageID, ImageRGBA: commandDeckTitleImage(binding.title, snapshot.Model)}
				}
				if err := runtime.Render(watch, frame); err != nil {
					continue
				}
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
		p.deckStatus(status, devices)
		if capture != nil {
			captureStatus := "no_device"
			if status == "connected" {
				captureStatus = "waiting"
			} else if status == "unavailable" {
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

func commandDeckTitleImage(title string, model commanddeck.Model) []byte {
	img := image.NewRGBA(image.Rect(0, 0, model.KeyImageW, model.KeyImageH))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{24, 24, 24, 255}), image.Point{}, draw.Src)
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return img.Pix
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 11, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return img.Pix
	}
	defer face.Close()
	drawer := font.Drawer{Dst: img, Src: image.White, Face: face}
	line := ""
	y := 14
	for _, word := range strings.Fields(title) {
		candidate := strings.TrimSpace(line + " " + word)
		if drawer.MeasureString(candidate).Ceil() > model.KeyImageW-4 && line != "" {
			drawer.Dot = fixed.P(2, y)
			drawer.DrawString(line)
			y += 14
			line = word
		} else {
			line = candidate
		}
		if y > model.KeyImageH-3 {
			break
		}
	}
	if y <= model.KeyImageH-3 {
		drawer.Dot = fixed.P(2, y)
		drawer.DrawString(line)
	}
	return img.Pix
}
