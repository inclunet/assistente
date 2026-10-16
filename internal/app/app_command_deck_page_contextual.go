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
func (a *App) BeginContextualDeckPageUICommand(offerID, generation string, observed LocalCommandKeyboardContext) (result commandui.Reservation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	admission, err := a.consumeContextualDeckOfferForSurface(offerID, generation, observed, true)
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

// Keep the page fact paired with its canonical surface. The legacy tasklist
// surface is only valid under the workspace page, never under the tasklists
// route.
func deckPageCommandPageSurface(commandID, appPage, surface string) bool {
	if !commandbindings.IsAppPage(appPage) || !deckPageCommandSurface(commandID, surface) {
		return false
	}
	switch appPage {
	case "profiles":
		return surface == "profiles"
	case "tasklists":
		return surface == "tasklists"
	case "workspace":
		return surface == "tasklist" && (commandID == "tasklists.duplicate" || commandID == "tasklists.clear")
	default:
		return false
	}
}

func (p *commandProductRuntime) captureDeckPageContext(observed LocalCommandKeyboardContext) (*localCommandKeyboardContextProof, error) {
	if observed.SurfaceID != "" || (observed.SurfaceType != "profiles" && observed.SurfaceType != "tasklists" && observed.SurfaceType != "tasklist") {
		return nil, commandexecution.ErrDenied
	}
	if observed.AppPage != "" && !commandbindings.IsAppPage(observed.AppPage) {
		return nil, commandexecution.ErrDenied
	}
	expectedPage := observed.SurfaceType
	if observed.SurfaceType == "tasklist" {
		expectedPage = "workspace"
	}
	if observed.AppPage != "" && observed.AppPage != expectedPage {
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
		case commandbindings.AppPage:
			if !commandbindings.IsAppPage(proof.observed.AppPage) || proof.observed.AppPage != proof.observed.SurfaceType &&
				(proof.observed.SurfaceType != "tasklist" || proof.observed.AppPage != "workspace") {
				return commandOriginContext{}, commandexecution.ErrDenied
			}
			facts[field] = proof.observed.AppPage
		default:
			return commandOriginContext{}, commandexecution.ErrDenied
		}
	}
	return commandOriginContext{facts: facts}, nil
}
