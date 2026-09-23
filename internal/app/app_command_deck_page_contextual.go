package app

import (
	"context"
	"time"

	"assistente/internal/commandbindings"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
)

// Called only at the backend admission/claim frontier. The callback must be
// memory-only and must not reacquire these locks. Map reset, capture and native
// disconnect cannot interleave between checking the source and admitting it.
// Lock order: published host -> keyboard -> physical controller -> product -> workspace.
func (p *commandProductRuntime) withContextualDeckPageSource(ctx context.Context, run *commandUIRun, claim func() error) error {
	if !run.deckPage {
		return claim()
	}
	if p.deckExecution == nil {
		return commandexecution.ErrStale
	}
	p.deckExecution.mu.Lock()
	occurrence, exists := p.deckExecution.occurrences[run.reservation.InvocationID]
	p.deckExecution.mu.Unlock()
	if !exists || !occurrence.page {
		return commandexecution.ErrStale
	}
	keyboard, controller := occurrence.keyboard, occurrence.controller
	if keyboard == nil || controller == nil || controller.p != p {
		return commandexecution.ErrStale
	}
	p.keyboardMu.Lock()
	defer p.keyboardMu.Unlock()
	controller.mu.Lock()
	defer controller.mu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	instance, exists := controller.instances[occurrence.serial]
	if ctx.Err() != nil || p.closed || p.app.commandProduct.Load() != p || p.keyboardMap != keyboard || keyboard.ctx.Err() != nil ||
		(keyboard.view.ValidUntil != 0 && time.Now().UnixMilli() >= keyboard.view.ValidUntil) || keyboard.versions != occurrence.versions ||
		p.deckCapture != nil || p.deckCaptureGeneration != occurrence.generation || !exists || instance.id != occurrence.instanceID ||
		instance.ctx.Err() != nil || occurrence.ctx.Err() != nil {
		return commandexecution.ErrStale
	}
	return claim()
}

// BeginContextualDeckPageUICommand consumes the original physical occurrence.
// The UI supplies its admitted page domain, never a command or hardware target.
// Selected object ownership and revision are verified by PreparePageMutationCommand.
func (a *App) BeginContextualDeckPageUICommand(offerID, generation, surfaceType, profile string) (result commandui.Reservation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	admission, err := a.consumeContextualDeckOfferForSurface(offerID, generation, LocalCommandKeyboardContext{SurfaceType: surfaceType, Profile: profile}, true)
	if err != nil {
		return result, err
	}
	return a.beginAdmittedContextualDeckUICommand(admission)
}

func deckPageCommandSurface(commandID, surface string) bool {
	switch commandID {
	case "profiles.duplicate", "profiles.delete", "profiles.activate":
		return surface == "profiles"
	case "tasklists.delete":
		return surface == "tasklists"
	case "tasklists.duplicate", "tasklists.clear":
		return surface == "tasklists" || surface == "tasklist"
	default:
		return false
	}
}

func (p *commandProductRuntime) captureDeckPageContext(observed LocalCommandKeyboardContext) (*localCommandKeyboardContextProof, error) {
	if observed.SurfaceID != "" || (observed.SurfaceType != "profiles" && observed.SurfaceType != "tasklists" && observed.SurfaceType != "tasklist") {
		return nil, commandexecution.ErrDenied
	}
	snapshot, err := p.workspaceMgr.CommandSnapshot()
	if err != nil || snapshot.WorkspaceID != p.workspaceID || (observed.SurfaceType == "tasklist" && string(snapshot.Tab.Type) != "tasklist") ||
		(observed.Profile != "" && observed.Profile != localKeyboardEffectiveProfile(snapshot)) {
		return nil, commandexecution.ErrStale
	}
	return &localCommandKeyboardContextProof{snapshot: snapshot, observed: observed}, nil
}

func deckPageCommandFacts(proof *localCommandKeyboardContextProof, required []commandbindings.Field) (commandOriginContext, error) {
	if proof == nil || proof.observed.SurfaceID != "" {
		return commandOriginContext{}, commandexecution.ErrDenied
	}
	facts := commandbindings.Facts{commandbindings.AppFocused: true, commandbindings.SurfaceType: proof.observed.SurfaceType}
	for _, field := range required {
		switch field {
		case commandbindings.AppFocused, commandbindings.SurfaceType:
		case commandbindings.Profile:
			profile := localKeyboardEffectiveProfile(proof.snapshot)
			if profile == "" || proof.observed.Profile != profile {
				return commandOriginContext{}, commandexecution.ErrDenied
			}
			facts[field] = profile
		default:
			return commandOriginContext{}, commandexecution.ErrDenied
		}
	}
	return commandOriginContext{facts: facts}, nil
}
