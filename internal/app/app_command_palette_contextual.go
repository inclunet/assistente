package app

import (
	"context"
	"encoding/json"
	"time"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/workspace"
)

// Workspace surfaces retain their existing preparation/decision/commit protocols.
// Page mutations, layer actions and purely local presentation are not admitted.
func isContextualPaletteWorkspaceCommand(id string) bool {
	_, editorMode := editorModeForCommand(id)
	return isWorkspaceTabMutationCommand(id) || id == commandWorkspaceCreateID || id == commandWorkspaceChatOpenID || editorMode ||
		id == commandConversationClearID || id == commandTerminalInterruptID || id == commandTerminalSessionCreateID || id == commandTerminalSessionCloseID ||
		isChatActionCommand(id) || isChatMessageCommand(id) || isEditorFileCommand(id) || isEditorFormatCommand(id)
}

// These operations have a captured target outside the create/update form.
// Form-only commands stay excluded while the palette cannot run in that modal.
func isContextualPagePaletteCommand(id string) bool {
	switch id {
	case "tasklists.duplicate", "tasklists.delete", "tasklists.clear", "profiles.duplicate", "profiles.delete", "profiles.activate":
		return true
	default:
		return false
	}
}

// BeginContextualPagePaletteUICommand admits the explicit UI selection, not a
// claim that the backend observed a page. The UI owns the page/focus guard;
// PreparePageMutationCommand separately validates the owned target/fingerprint.
// No UI surface ID, draft, target ID or caller-controlled authorization is used
// as a backend fact. The profile is always checked against the canonical source.
func (a *App) BeginContextualPagePaletteUICommand(generation, commandID, surfaceType, profile string) (result commandui.Reservation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	if generation == "" || !isContextualPagePaletteCommand(commandID) {
		return result, commandexecution.ErrDenied
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return result, err
	}
	definition, ok := p.registry.Lookup(commandID)
	if !ok || commandExecutionClassForDefinition(definition) != commandExecutionDurable || !definition.AllowsSource(commandcatalog.Palette) {
		return result, commandexecution.ErrDenied
	}
	snapshot, err := p.workspaceMgr.CommandSnapshot()
	if err != nil || snapshot.WorkspaceID != p.workspaceID {
		return result, commandexecution.ErrStale
	}
	if isProfileMutationCommand(commandID) {
		if surfaceType != "profiles" {
			return result, commandexecution.ErrDenied
		}
	} else if surfaceType != "tasklists" {
		if surfaceType != "tasklist" || (commandID != "tasklists.duplicate" && commandID != "tasklists.clear") || snapshot.Tab.Type != workspace.TabTypeTasklist {
			return result, commandexecution.ErrDenied
		}
	}
	if profile != "" && profile != localKeyboardEffectiveProfile(snapshot) {
		return result, commandexecution.ErrStale
	}
	proof := &localCommandKeyboardContextProof{snapshot: snapshot, observed: LocalCommandKeyboardContext{SurfaceType: surfaceType, Profile: profile}}
	return a.beginContextualPaletteCommand(p, generation, commandID, proof)
}

// BeginContextualPaletteUICommand binds an explicit UI selection to the
// canonical workspace snapshot. It does not accept focus, modal or IME facts:
// those remain UI guards at submission. No mutation occurs at Begin or Take.
func (a *App) BeginContextualPaletteUICommand(generation, commandID string, observed LocalCommandKeyboardContext) (result commandui.Reservation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	if !isContextualPaletteWorkspaceCommand(commandID) || generation == "" {
		return result, commandexecution.ErrDenied
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return result, err
	}
	definition, ok := p.registry.Lookup(commandID)
	class := commandExecutionClassForDefinition(definition)
	if !ok || (class != commandExecutionDurable && class != commandExecutionAuditedUI) || !definition.AllowsSource(commandcatalog.Palette) {
		return result, commandexecution.ErrDenied
	}
	proof, err := a.captureLocalKeyboardContext(observed)
	if err != nil {
		return result, err
	}
	return a.beginContextualPaletteCommand(p, generation, commandID, proof)
}

func (a *App) beginContextualPaletteCommand(p *commandProductRuntime, generation, commandID string, proof *localCommandKeyboardContextProof) (result commandui.Reservation, err error) {
	p.keyboardMu.Lock()
	state := p.keyboardMap
	valid := state != nil && state.ctx.Err() == nil && state.view.Generation == generation &&
		(state.view.ValidUntil == 0 || time.Now().UnixMilli() < state.view.ValidUntil)
	p.keyboardMu.Unlock()
	if !valid || state.configuration == nil {
		return result, commandexecution.ErrStale
	}
	identity := "palette:" + commandID
	required := state.configuration.RequiredFacts(identity)
	if len(required) == 0 || !localKeyboardSupportedVariableFacts(localKeyboardVariableFacts(required)) ||
		(localKeyboardProfileRequired(state.configuration, identity) && proof.observed.Profile == "") {
		return result, commandexecution.ErrDenied
	}
	if isContextualPagePaletteCommand(commandID) {
		for _, field := range required {
			if field != commandbindings.AppFocused && field != commandbindings.SurfaceType && field != commandbindings.Profile {
				return result, commandexecution.ErrDenied
			}
		}
	}
	sourceValid := func() bool {
		p.keyboardMu.Lock()
		current := p.keyboardMap == state && state.ctx.Err() == nil &&
			(state.view.ValidUntil == 0 || time.Now().UnixMilli() < state.view.ValidUntil)
		p.keyboardMu.Unlock()
		if !current || !p.localKeyboardContextCurrent(proof) {
			return false
		}
		versions, snapshotErr := p.host.Snapshot(a.commandBridgeContext(), p.principal)
		return snapshotErr == nil && versions == state.versions && versions.Unlocked
	}
	if !sourceValid() {
		return result, commandexecution.ErrStale
	}
	return a.beginCommandUIWithPaletteContext(p, commandID, func(reservation commandui.Reservation) (commandexecution.EnvelopeCandidate, error) {
		return commandPaletteCandidate(reservation.InvocationID, reservation.InvocationID, commandID, json.RawMessage(`{}`))
	}, p.execute, sourceValid, proof)
}

// The proof can only be recovered through the already reserved invocation.
// A direct palette request cannot supply it or convert UI observations into
// authorization. The snapshot describes the explicit backend target; focus
// is an admission predicate of the UI coordinator, not a native observation.
func (p *commandProductRuntime) paletteWorkspaceOccurrenceFacts(ctx context.Context, candidate commandexecution.EnvelopeCandidate, required []commandbindings.Field) (commandOriginContext, bool, error) {
	p.mu.Lock()
	var proof *localCommandKeyboardContextProof
	var sourceValid func() bool
	var commandID string
	for _, run := range p.uiRuns {
		if run.reservation.InvocationID == candidate.InvocationID && run.paletteContext != nil &&
			(isContextualPaletteWorkspaceCommand(run.reservation.CommandID) || isContextualPagePaletteCommand(run.reservation.CommandID)) {
			proof, sourceValid = run.paletteContext, run.sourceValid
			commandID = run.reservation.CommandID
			break
		}
	}
	p.mu.Unlock()
	if proof == nil {
		return commandOriginContext{}, false, nil
	}
	if ctx == nil || ctx.Err() != nil || sourceValid == nil || !sourceValid() || !localKeyboardSupportedVariableFacts(localKeyboardVariableFacts(required)) {
		return commandOriginContext{}, true, commandexecution.ErrStale
	}
	facts := commandbindings.Facts{commandbindings.AppFocused: true}
	if isContextualPagePaletteCommand(commandID) {
		// This is the admitted UI domain, not the background workspace tab or
		// the selected target ID. Target ownership/version remains in Prepare.
		facts[commandbindings.SurfaceType] = proof.observed.SurfaceType
		for _, field := range required {
			if field == commandbindings.SurfaceID {
				return commandOriginContext{}, true, commandexecution.ErrDenied
			}
		}
	} else {
		facts[commandbindings.SurfaceID] = proof.snapshot.Tab.ID
		facts[commandbindings.SurfaceType] = string(proof.snapshot.Tab.Type)
	}
	for _, field := range required {
		if field == commandbindings.Profile {
			profile := localKeyboardEffectiveProfile(proof.snapshot)
			if profile == "" || proof.observed.Profile != profile {
				return commandOriginContext{}, true, commandexecution.ErrDenied
			}
			facts[field] = profile
		}
	}
	return commandOriginContext{facts: facts, version: "palette-workspace:" + proof.snapshot.Version}, true, nil
}
