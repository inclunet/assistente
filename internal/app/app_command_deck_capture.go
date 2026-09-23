package app

import (
	"context"
	"encoding/json"
	"time"

	"assistente/internal/commandadapter"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandinput"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

type commandDeckCapture struct {
	id       string
	ctx      context.Context
	cancel   context.CancelFunc
	epoch    commandsecurity.EpochSnapshot
	finished bool
	status   string
	pending  *commandDeckCaptureEvent
}

type commandDeckCaptureEvent struct {
	RequestID   string `json:"requestId"`
	Status      string `json:"status"`
	Model       string `json:"model,omitempty"`
	Key         int    `json:"key"`
	TriggerSpec string `json:"triggerSpec,omitempty"`
}

// BeginCommandDeckCapture grants no command authority. Only native input can
// fill the draft, and capture is bound to the current authenticated epoch.
func (a *App) BeginCommandDeckCapture(requestID string) (err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	if _, err := uuid.Parse(requestID); err != nil {
		return commandexecution.ErrInvalidRequest
	}
	ctx := a.commandBridgeContext()
	if ctx == nil {
		return commandexecution.ErrDenied
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return err
	}
	epoch, err := p.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		principal, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || principal != p.principal || !p.dependenciesMatch(a) {
			return "", "", commandexecution.ErrDenied
		}
		return principal.UserID, principal.SessionID, nil
	})
	if err != nil {
		return err
	}
	watch, release, err := p.epochs.WatchEpoch(ctx, epoch)
	if err != nil {
		return err
	}
	run, cancel := context.WithTimeout(watch, 30*time.Second)
	capture := &commandDeckCapture{id: requestID, ctx: run, cancel: cancel, epoch: epoch}
	err = p.epochs.Admit(ctx, epoch, func(context.Context) error {
		if a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
			return commandexecution.ErrStale
		}
		return nil
	}, func() error {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.closed || p.deckDriver == nil {
			return commandexecution.ErrDenied
		}
		if p.deckCapture != nil {
			p.deckCapture.cancel()
		}
		p.deckCapture = capture
		p.deckCaptureGeneration++
		p.workers.Add(1)
		return nil
	})
	if err != nil {
		cancel()
		release()
		return err
	}
	go func() {
		defer p.workers.Done()
		defer release()
		defer cancel()
		<-run.Done()
		p.mu.Lock()
		current := p.deckCapture == capture
		if current {
			p.deckCapture = nil
			p.deckCaptureGeneration++
		}
		p.mu.Unlock()
		if current && a.commandProduct.Load() == p {
			status := "cancelled"
			if run.Err() == context.DeadlineExceeded {
				status = "timeout"
			}
			p.emitDeckCapture(commandDeckCaptureEvent{RequestID: requestID, Status: status})
		}
	}()
	p.captureStatus(capture, "starting")
	discovery := p.discoverDeck(run)
	if discovery.Status == "unavailable" {
		p.captureStatus(capture, "unavailable")
	} else if len(discovery.Devices) == 0 {
		p.captureStatus(capture, "no_device")
	}
	return nil
}

func (a *App) CancelCommandDeckCapture(requestID string) error {
	p := a.commandProduct.Load()
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deckCapture != nil && p.deckCapture.id == requestID {
		p.deckCapture.cancel()
		p.deckCapture = nil
		p.deckCaptureGeneration++
	}
	return nil
}

func (p *commandProductRuntime) currentDeckCapture() *commandDeckCapture {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deckCapture == nil || p.deckCapture.ctx.Err() != nil {
		return nil
	}
	return p.deckCapture
}

func (p *commandProductRuntime) deckInputGeneration() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.deckCaptureGeneration
}
func (p *commandProductRuntime) deckExecutionAllowed(generation uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.deckCapture == nil && p.deckCaptureGeneration == generation
}

func (p *commandProductRuntime) emitDeckCapture(event commandDeckCaptureEvent) {
	if p.app.emitter != nil && p.app.commandProduct.Load() == p {
		p.app.emitter.Emit("command:deck-capture", event)
	}
}

func (p *commandProductRuntime) captureStatus(capture *commandDeckCapture, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deckCapture != capture || capture == nil || capture.ctx.Err() != nil || capture.finished || capture.status == status {
		return
	}
	capture.status = status
	p.emitDeckCapture(commandDeckCaptureEvent{RequestID: capture.id, Status: status})
}

// All pressed keys observed during capture remain suppressed through release,
// even if the dialog cancels or the runtime reopens a handle in the meantime.
func (p *commandProductRuntime) captureDeckInput(ctx context.Context, event commandadapter.Event, serial string, index int, model string) bool {
	key := event.SourceInstance + ":" + event.Key
	p.mu.Lock()
	if event.Kind == commandinput.KeyUp {
		held := p.deckHeld[key]
		delete(p.deckHeld, key)
		delete(p.deckDown, key)
		capture := p.deckCapture
		active := capture != nil
		var pending *commandDeckCaptureEvent
		if capture != nil && capture.pending != nil && capture.pending.Key == index {
			var spec commandDeckTrigger
			if json.Unmarshal([]byte(capture.pending.TriggerSpec), &spec) == nil && spec.Device == serial {
				pending = capture.pending
			}
		}
		p.mu.Unlock()
		if pending != nil && ctx.Err() == nil {
			p.completeDeckCapture(capture, pending)
		}
		return held || active
	}
	alreadyDown := p.deckDown[key]
	if event.Kind == commandinput.KeyDown {
		if p.deckDown == nil {
			p.deckDown = map[string]bool{}
		}
		p.deckDown[key] = true
	}
	if p.deckHeld[key] {
		p.mu.Unlock()
		return true
	}
	capture := p.deckCapture
	if capture == nil {
		p.mu.Unlock()
		// deckDown is the physical edge ledger, not capture-local state. Keep
		// the down edge suppressed across a Deck session reopen so a driver
		// re-emitting Down for a held key cannot toggle/activate twice. KeyUp
		// above clears it before the next real press is admitted.
		return alreadyDown
	}
	if event.Kind == commandinput.KeyDown {
		if p.deckHeld == nil {
			p.deckHeld = map[string]bool{}
		}
		p.deckHeld[key] = true
	}
	p.mu.Unlock()
	if event.Kind != commandinput.KeyDown || alreadyDown || event.Repeat || capture.ctx.Err() != nil || ctx.Err() != nil {
		return true
	}
	if _, err := commandconfig.ParseStreamDeckTriggerIdentity(key); err != nil {
		return true
	}
	_ = p.epochs.Admit(capture.ctx, capture.epoch, func(context.Context) error {
		if p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
			return commandexecution.ErrStale
		}
		return nil
	}, func() error {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.deckCapture != capture || capture.finished || capture.pending != nil || capture.ctx.Err() != nil {
			return nil
		}
		spec, _ := json.Marshal(struct {
			Version int    `json:"version"`
			Device  string `json:"device"`
			Key     int    `json:"key"`
		}{1, serial, index})
		capture.pending = &commandDeckCaptureEvent{RequestID: capture.id, Status: "captured", Model: model, Key: index, TriggerSpec: string(spec)}
		return nil
	})
	return true
}

// Publish only on release so the UI can stop capture without losing the
// release edge and suppressing the first real command after saving.
func (p *commandProductRuntime) completeDeckCapture(capture *commandDeckCapture, pending *commandDeckCaptureEvent) {
	_ = p.epochs.Admit(capture.ctx, capture.epoch, func(context.Context) error {
		if p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
			return commandexecution.ErrStale
		}
		return nil
	}, func() error {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.deckCapture != capture || capture.finished || capture.pending != pending || capture.ctx.Err() != nil {
			return nil
		}
		capture.finished = true
		capture.pending = nil
		p.emitDeckCapture(*pending)
		return nil
	})
}
