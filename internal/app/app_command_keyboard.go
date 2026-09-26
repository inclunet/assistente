package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandui"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

type LocalCommandShortcut struct {
	Version   int                        `json:"version"`
	Code      string                     `json:"code"`
	Modifiers []string                   `json:"modifiers"`
	Steps     []LocalCommandShortcutStep `json:"steps,omitempty"`
}

// LocalCommandShortcutStep é um passo da gramática v2. O primeiro passo
// carrega os modificadores de prefixo; o segundo é uma tecla sem
// modificadores. A forma v1 continua sendo serializada exatamente como
// antes, inclusive com "modifiers" obrigatório.
type LocalCommandShortcutStep struct {
	Code      string   `json:"code"`
	Modifiers []string `json:"modifiers"`
}

func (s LocalCommandShortcut) MarshalJSON() ([]byte, error) {
	if s.Version == 2 {
		if s.Code != "" || s.Modifiers != nil || len(s.Steps) != 2 {
			return nil, commandexecution.ErrInvalidRequest
		}
		return json.Marshal(struct {
			Version int                        `json:"version"`
			Steps   []LocalCommandShortcutStep `json:"steps"`
		}{Version: s.Version, Steps: s.Steps})
	}
	if s.Version != 1 || s.Steps != nil {
		return nil, commandexecution.ErrInvalidRequest
	}
	return json.Marshal(struct {
		Version   int      `json:"version"`
		Code      string   `json:"code"`
		Modifiers []string `json:"modifiers"`
	}{Version: s.Version, Code: s.Code, Modifiers: s.Modifiers})
}

func (s *LocalCommandShortcut) UnmarshalJSON(raw []byte) error {
	var wire struct {
		Version   json.RawMessage `json:"version"`
		Code      json.RawMessage `json:"code"`
		Modifiers json.RawMessage `json:"modifiers"`
		Steps     json.RawMessage `json:"steps"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return commandexecution.ErrInvalidRequest
	}
	if len(wire.Version) == 0 || string(wire.Version) == "null" || json.Unmarshal(wire.Version, &s.Version) != nil {
		return commandexecution.ErrInvalidRequest
	}
	s.Code, s.Modifiers, s.Steps = "", nil, nil
	switch s.Version {
	case 1:
		if len(wire.Code) == 0 || string(wire.Code) == "null" || len(wire.Modifiers) == 0 || string(wire.Modifiers) == "null" || len(wire.Steps) != 0 {
			return commandexecution.ErrInvalidRequest
		}
		if json.Unmarshal(wire.Code, &s.Code) != nil || json.Unmarshal(wire.Modifiers, &s.Modifiers) != nil {
			return commandexecution.ErrInvalidRequest
		}
	case 2:
		if len(wire.Steps) == 0 || string(wire.Steps) == "null" || len(wire.Code) != 0 || len(wire.Modifiers) != 0 {
			return commandexecution.ErrInvalidRequest
		}
		var rawSteps []json.RawMessage
		if json.Unmarshal(wire.Steps, &rawSteps) != nil || len(rawSteps) != 2 {
			return commandexecution.ErrInvalidRequest
		}
		s.Steps = make([]LocalCommandShortcutStep, 2)
		for i, rawStep := range rawSteps {
			step, err := decodeLocalCommandShortcutStep(rawStep)
			if err != nil {
				return err
			}
			s.Steps[i] = step
		}
	default:
		return commandexecution.ErrInvalidRequest
	}
	return nil
}

func decodeLocalCommandShortcutStep(raw json.RawMessage) (LocalCommandShortcutStep, error) {
	var wire struct {
		Code      json.RawMessage `json:"code"`
		Modifiers json.RawMessage `json:"modifiers"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil || len(wire.Code) == 0 || string(wire.Code) == "null" || len(wire.Modifiers) == 0 || string(wire.Modifiers) == "null" {
		return LocalCommandShortcutStep{}, commandexecution.ErrInvalidRequest
	}
	var step LocalCommandShortcutStep
	if json.Unmarshal(wire.Code, &step.Code) != nil || json.Unmarshal(wire.Modifiers, &step.Modifiers) != nil {
		return LocalCommandShortcutStep{}, commandexecution.ErrInvalidRequest
	}
	return step, nil
}

type LocalCommandKeyboardMap struct {
	ValidUntil                  int64                                   `json:"validUntil,omitempty"`
	Generation                  string                                  `json:"generation"`
	OwnerID                     string                                  `json:"ownerId"`
	SessionID                   string                                  `json:"sessionId"`
	WorkspaceID                 string                                  `json:"workspaceId"`
	Bindings                    []LocalCommandKeyboardBinding           `json:"bindings"`
	ContextualBindings          []LocalCommandKeyboardContextualBinding `json:"contextualBindings,omitempty"`
	LocalPaletteCommands        []string                                `json:"localPaletteCommands"`
	LocalPaletteConditions      []LocalCommandPaletteCondition          `json:"localPaletteConditions,omitempty"`
	ContextualPaletteConditions []LocalCommandPaletteCondition          `json:"contextualPaletteConditions,omitempty"`
}

type LocalCommandKeyboardBinding struct {
	Shortcut  LocalCommandShortcut `json:"shortcut"`
	CommandID string               `json:"commandId"`
	Handler   string               `json:"handler"`
}

// Observação da surface real, não uma identidade ou autorização do chamador.
// O host exige correspondência com a aba ativa ou o par canônico da rota na
// toolbar; a UI mantém sua própria lease de foco/surface até o handoff. Nunca
// deriva SurfaceContext de uma aba.
type LocalCommandKeyboardContext struct {
	SurfaceID   string `json:"surfaceId"`
	SurfaceType string `json:"surfaceType"`
	Profile     string `json:"profile,omitempty"`
	AppPage     string `json:"appPage,omitempty"`
}

type LocalCommandKeyboardContextualBinding struct {
	FallbackToSequences bool                                               `json:"fallbackToSequences,omitempty"`
	Shortcut            LocalCommandShortcut                               `json:"shortcut"`
	BySurface           map[string]*LocalCommandKeyboardBinding            `json:"bySurface"`
	BySurfaceID         map[string]map[string]*LocalCommandKeyboardBinding `json:"bySurfaceId,omitempty"`
	ByProfile           map[string]*LocalCommandKeyboardContextualBinding  `json:"byProfile,omitempty"`
	ByPage              map[string]*LocalCommandKeyboardContextualBinding  `json:"byPage,omitempty"`
	// Empty ID denotes the type fallback. Only NoMatch permits sequences;
	// suppression, review, conflicts and unavailable handlers remain barriers.
	SequenceFallbacks map[string]map[string]bool   `json:"sequenceFallbacks,omitempty"`
	Fallback          *LocalCommandKeyboardBinding `json:"fallback"`
}

type localCommandKeyboardState struct {
	view          LocalCommandKeyboardMap
	configuration *commandbindings.Configuration
	versions      commandexecution.Versions
	ctx           context.Context
	release       func()
	identities    map[string]LocalCommandKeyboardBinding
	pressed       map[string]string
}

type localCommandKeyboardOccurrence struct {
	state    *localCommandKeyboardState
	identity string
	mermaid  bool
	binding  LocalCommandKeyboardBinding
	context  *localCommandKeyboardContextProof
}

type localCommandKeyboardContextProof struct {
	observed  LocalCommandKeyboardContext
	snapshot  workspace.CommandSnapshot
	routePage bool
}

// GetLocalCommandKeyboardMap publica somente combinações resolvidas para o
// catálogo operacional de teclado. Não infere permissão de catálogo:
// ativação, conflitos e condições desconhecidas são avaliados no mapa real.
func (a *App) GetLocalCommandKeyboardMap() (result LocalCommandKeyboardMap, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return result, err
	}
	ctx := a.commandBridgeContext()
	if err := p.refreshCommandJobProjection(ctx); err != nil {
		return result, err
	}
	epoch, err := p.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || current != p.principal || !p.dependenciesMatch(a) {
			return "", "", commandexecution.ErrDenied
		}
		return current.UserID, current.SessionID, nil
	})
	if err != nil {
		return result, err
	}
	watched, release, err := p.epochs.WatchEpoch(ctx, epoch)
	if err != nil {
		return result, err
	}
	retained := false
	defer func() {
		if !retained {
			release()
		}
	}()
	configuration, _, versions, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil || !versions.Unlocked {
		return result, commandexecution.ErrDenied
	}
	view := LocalCommandKeyboardMap{Generation: uuid.Must(uuid.NewV7()).String(), OwnerID: p.principal.UserID, SessionID: p.principal.SessionID, WorkspaceID: p.workspaceID, Bindings: []LocalCommandKeyboardBinding{}, ContextualBindings: []LocalCommandKeyboardContextualBinding{}, LocalPaletteCommands: localPaletteUICommands(configuration, p.registry), LocalPaletteConditions: localPaletteUIConditions(configuration, p.registry)}
	view.ContextualPaletteConditions = contextualPaletteUIConditions(configuration, p.registry)
	if deadline := configuration.ValidUntil(); !deadline.IsZero() {
		view.ValidUntil = deadline.UnixMilli()
	}
	identities := make(map[string]LocalCommandKeyboardBinding)
	for _, identity := range configuration.TriggerIdentities() {
		if !strings.HasPrefix(identity, "keyboard.local:") {
			continue
		}
		raw, decodeErr := commandSettingsTriggerSpec(ctx, identity)
		var shortcut LocalCommandShortcut
		if decodeErr != nil || json.Unmarshal([]byte(raw), &shortcut) != nil || !localCommandShortcutAllowed(shortcut) {
			continue
		}
		fields := localKeyboardVariableFacts(configuration.RequiredFacts(identity))
		if len(fields) != 0 {
			if !localKeyboardSupportedVariableFacts(fields) {
				continue
			}
			contextual, ok := contextualKeyboardBinding(ctx, configuration, p.registry, identity, shortcut)
			if ok {
				view.ContextualBindings = append(view.ContextualBindings, contextual)
			}
			continue
		}
		binding, ok := resolvedLocalKeyboardBinding(configuration, p.registry, identity, shortcut, commandbindings.Facts{commandbindings.AppFocused: true})
		if !ok {
			continue
		}
		view.Bindings = append(view.Bindings, binding)
		identities[identity] = binding
	}
	err = p.epochs.Admit(ctx, epoch, func(ctx context.Context) error {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || current != p.principal || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
			return commandexecution.ErrDenied
		}
		return nil
	}, func() error {
		p.mu.Lock()
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return commandexecution.ErrDenied
		}
		current, _, v, err := p.host.ResolutionSnapshot(ctx, p.principal)
		if err != nil || current != configuration || v != versions || watched.Err() != nil {
			return commandexecution.ErrStale
		}
		p.keyboardMu.Lock()
		defer p.keyboardMu.Unlock()
		if old := p.keyboardMap; old != nil && old.ctx.Err() == nil && old.configuration == configuration && old.versions == versions {
			result = cloneLocalCommandKeyboardMap(old.view)
			return nil
		}
		if p.keyboardMap != nil {
			p.keyboardMap.release()
		}
		p.keyboardMap = &localCommandKeyboardState{view: view, configuration: configuration, versions: versions, ctx: watched, release: release, identities: identities, pressed: map[string]string{}}
		p.clearDeckPagePresentation("")
		retained = true
		result = cloneLocalCommandKeyboardMap(view)
		return nil
	})
	return result, err
}

func cloneLocalCommandKeyboardMap(in LocalCommandKeyboardMap) LocalCommandKeyboardMap {
	out := LocalCommandKeyboardMap{Generation: in.Generation, OwnerID: in.OwnerID, SessionID: in.SessionID, WorkspaceID: in.WorkspaceID, Bindings: make([]LocalCommandKeyboardBinding, len(in.Bindings)), ContextualBindings: cloneContextualKeyboardBindings(in.ContextualBindings), LocalPaletteCommands: append([]string{}, in.LocalPaletteCommands...), LocalPaletteConditions: cloneLocalCommandPaletteConditions(in.LocalPaletteConditions)}
	out.ContextualPaletteConditions = cloneLocalCommandPaletteConditions(in.ContextualPaletteConditions)
	out.ValidUntil = in.ValidUntil
	for i, b := range in.Bindings {
		out.Bindings[i] = b
		out.Bindings[i].Shortcut.Modifiers = slices.Clone(b.Shortcut.Modifiers)
		out.Bindings[i].Shortcut.Steps = cloneLocalCommandShortcutSteps(b.Shortcut.Steps)
		for j := range out.Bindings[i].Shortcut.Steps {
			out.Bindings[i].Shortcut.Steps[j].Modifiers = append([]string{}, b.Shortcut.Steps[j].Modifiers...)
		}
	}
	return out
}

func localKeyboardSupportedVariableFacts(fields []commandbindings.Field) bool {
	seenType, seenID := false, false
	for _, field := range fields {
		switch field {
		case commandbindings.SurfaceType:
			seenType = true
		case commandbindings.SurfaceID:
			seenID = true
		case commandbindings.Profile:
			// Perfil pode ser a única condição variável; sua prova é
			// validada no ingresso e a identidade vem do snapshot canônico.
		case commandbindings.AppPage:
			// A página vem da rota UI confiável; ausência deixa a resolução sem fato.
		default:
			return false
		}
	}
	return !seenID || seenType
}

// DOM keyboard input belongs to the focused application. This fixed fact is
// distinct from surface, which is selected synchronously by the local dispatcher.
func localKeyboardVariableFacts(fields []commandbindings.Field) []commandbindings.Field {
	result := make([]commandbindings.Field, 0, len(fields))
	for _, field := range fields {
		if field != commandbindings.AppFocused {
			result = append(result, field)
		}
	}
	return result
}

func localKeyboardWorkspaceSurface(surface string) bool {
	return surface == "chat" || surface == "editor" || surface == "terminal" || surface == "tasklist"
}

// Route-level keyboard input uses the toolbar surface lease, while a few
// standalone pages retain their established surface.type for contextual
// bindings. Keep this mapping closed and in parity with keyboardSurfaceForPage.
func localKeyboardRouteSurface(page string) (string, bool) {
	if !commandbindings.IsAppPage(page) {
		return "", false
	}
	switch page {
	case "profiles", "tasklists", "history":
		return page, true
	default:
		return "toolbar", true
	}
}

func (a *App) captureLocalKeyboardContext(observed LocalCommandKeyboardContext) (*localCommandKeyboardContextProof, error) {
	if observed.AppPage != "" && !commandbindings.IsAppPage(observed.AppPage) {
		return nil, commandexecution.ErrDenied
	}
	routeSurface, knownPage := localKeyboardRouteSurface(observed.AppPage)
	routePageContext := observed.SurfaceID == "command-toolbar" && knownPage && observed.SurfaceType == routeSurface
	workspaceTabContext := observed.SurfaceID != "" && observed.SurfaceID != "command-toolbar" && localKeyboardWorkspaceSurface(observed.SurfaceType)
	if !routePageContext && !workspaceTabContext {
		return nil, commandexecution.ErrDenied
	}
	if workspaceTabContext && observed.AppPage != "" && observed.AppPage != "workspace" {
		return nil, commandexecution.ErrDenied
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return nil, err
	}
	snapshot, err := p.workspaceMgr.CommandSnapshot()
	if err != nil || snapshot.WorkspaceID != p.workspaceID || !routePageContext &&
		(snapshot.ActiveTabID != observed.SurfaceID || string(snapshot.Tab.Type) != observed.SurfaceType) {
		return nil, commandexecution.ErrStale
	}
	if observed.Profile != "" && observed.Profile != localKeyboardEffectiveProfile(snapshot) {
		return nil, commandexecution.ErrStale
	}
	return &localCommandKeyboardContextProof{observed: observed, snapshot: snapshot, routePage: routePageContext}, nil
}

func localKeyboardEffectiveProfile(snapshot workspace.CommandSnapshot) string {
	if snapshot.Tab.ProfileOverrideSlug != "" {
		return snapshot.Tab.ProfileOverrideSlug
	}
	return snapshot.WorkspaceProfile
}

func localKeyboardProfileRequired(configuration *commandbindings.Configuration, identity string) bool {
	for _, field := range configuration.RequiredFacts(identity) {
		if field == commandbindings.Profile {
			return true
		}
	}
	return false
}

func (p *commandProductRuntime) localKeyboardContextCurrent(proof *localCommandKeyboardContextProof) bool {
	if proof == nil {
		return true
	}
	if p == nil || !p.dependenciesMatch(p.app) {
		return false
	}
	current, err := p.workspaceMgr.CommandSnapshot()
	return err == nil && current == proof.snapshot && current.WorkspaceID == p.workspaceID
}

func localKeyboardOccurrenceBinding(s *localCommandKeyboardState, registry *commandcatalog.Registry, identity string, shortcut LocalCommandShortcut, proof *localCommandKeyboardContextProof) (LocalCommandKeyboardBinding, bool) {
	if proof == nil {
		binding, ok := s.identities[identity]
		return binding, ok
	}
	profile := localKeyboardEffectiveProfile(proof.snapshot)
	fields := localKeyboardVariableFacts(s.configuration.RequiredFacts(identity))
	if len(fields) != 0 && !localKeyboardSupportedVariableFacts(fields) {
		return LocalCommandKeyboardBinding{}, false
	}
	facts := commandbindings.Facts{commandbindings.AppFocused: true}
	if proof.routePage {
		facts[commandbindings.SurfaceType] = proof.observed.SurfaceType
	} else {
		facts[commandbindings.SurfaceType] = string(proof.snapshot.Tab.Type)
		facts[commandbindings.SurfaceID] = proof.snapshot.Tab.ID
	}
	if containsOriginField(s.configuration.RequiredFacts(identity), commandbindings.AppPage) {
		if !commandbindings.IsAppPage(proof.observed.AppPage) {
			return LocalCommandKeyboardBinding{}, false
		}
		facts[commandbindings.AppPage] = proof.observed.AppPage
	}
	if localKeyboardProfileRequired(s.configuration, identity) {
		if proof.observed.Profile == "" || profile == "" || proof.observed.Profile != profile {
			return LocalCommandKeyboardBinding{}, false
		}
		facts[commandbindings.Profile] = profile
	}
	return resolvedLocalKeyboardBinding(s.configuration, registry, identity, shortcut, facts)
}

func resolvedLocalKeyboardBinding(configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, shortcut LocalCommandShortcut, facts commandbindings.Facts) (LocalCommandKeyboardBinding, bool) {
	resolved, err := configuration.Resolve(identity, facts, nil)
	if err != nil || resolved.Status != commandbindings.Selected || resolved.ExecutionScopeKey != "global" {
		return LocalCommandKeyboardBinding{}, false
	}
	definition, exists := registry.Lookup(resolved.CommandID)
	if !exists || !definition.AllowsSource(commandcatalog.KeyboardLocal) || !localKeyboardCommandAllowed(definition.ID) {
		return LocalCommandKeyboardBinding{}, false
	}
	handler := "backend"
	switch commandExecutionClassForDefinition(definition) {
	case commandExecutionLocalUI:
		handler = "local_ui"
	case commandExecutionAuditedUI:
		handler = "ui"
	case commandExecutionDurable:
		if isWorkspaceMutationCommand(definition.ID) {
			handler = "contextual"
		} else if (definition.ID != commandProductWorkspaceListID && !isCommandLayerAction(definition.ID)) || definition.HandlerClassification != commandcatalog.HandlerBackend {
			return LocalCommandKeyboardBinding{}, false
		}
	default:
		return LocalCommandKeyboardBinding{}, false
	}
	return LocalCommandKeyboardBinding{Shortcut: shortcut, CommandID: definition.ID, Handler: handler}, true
}

func contextualKeyboardBinding(ctx context.Context, configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, shortcut LocalCommandShortcut) (LocalCommandKeyboardContextualBinding, bool) {
	profiles := configuration.FieldValues(identity, commandbindings.Profile)
	values := configuration.FieldValues(identity, commandbindings.SurfaceType)
	pages := configuration.FieldValues(identity, commandbindings.AppPage)
	if len(values) == 0 && len(profiles) == 0 && len(pages) == 0 {
		return LocalCommandKeyboardContextualBinding{}, false
	}
	if len(profiles) != 0 && len(pages) == 0 {
		workspaceValues := make([]string, 0, len(values))
		for _, value := range values {
			if localKeyboardWorkspaceSurface(value) {
				workspaceValues = append(workspaceValues, value)
			}
		}
		values = workspaceValues
	}
	if len(values) == 0 && len(profiles) != 0 && len(pages) == 0 {
		values = []string{"chat", "editor", "terminal", "tasklist"}
	}
	if len(values) == 0 {
		if len(pages) == 0 {
			return LocalCommandKeyboardContextualBinding{}, false
		}
		// A route page has its own canonical route surface; workspace pages are
		// expanded to actual tab surfaces inside the page branch below.
		values = []string{"toolbar"}
	}
	fallbackProfile := contextualFallbackProfile(profiles)
	build := func(appPage string) (LocalCommandKeyboardContextualBinding, bool) {
		branchValues := values
		if len(pages) != 0 {
			if appPage == "workspace" {
				if len(configuration.FieldValues(identity, commandbindings.SurfaceType)) == 0 {
					branchValues = []string{"chat", "editor", "terminal", "tasklist"}
				} else {
					branchValues = make([]string, 0, len(values))
					for _, value := range values {
						if localKeyboardWorkspaceSurface(value) {
							branchValues = append(branchValues, value)
						}
					}
				}
			} else if routeSurface, ok := localKeyboardRouteSurface(appPage); ok {
				branchValues = []string{routeSurface}
			}
		}
		entry, ok := contextualKeyboardBindingForProfile(ctx, configuration, registry, identity, shortcut, branchValues, fallbackProfile, len(profiles) != 0, appPage)
		if !ok {
			return LocalCommandKeyboardContextualBinding{}, false
		}
		if len(profiles) != 0 {
			slices.Sort(profiles)
			entry.ByProfile = make(map[string]*LocalCommandKeyboardContextualBinding, len(profiles))
			for _, profile := range profiles {
				leaf, leafOK := contextualKeyboardBindingForProfile(ctx, configuration, registry, identity, shortcut, branchValues, profile, true, appPage)
				if !leafOK {
					return LocalCommandKeyboardContextualBinding{}, false
				}
				entry.ByProfile[profile] = &leaf
			}
		}
		return entry, true
	}
	if len(pages) != 0 {
		entry := LocalCommandKeyboardContextualBinding{Shortcut: shortcut, BySurface: map[string]*LocalCommandKeyboardBinding{},
			ByPage: make(map[string]*LocalCommandKeyboardContextualBinding)}
		for _, appPage := range commandbindings.AppPages() {
			branch, ok := build(appPage)
			if !ok {
				return LocalCommandKeyboardContextualBinding{}, false
			}
			entry.ByPage[appPage] = &branch
		}
		return entry, true
	}
	return build("")
}

func contextualFallbackProfile(values []string) string {
	for i := 0; ; i++ {
		candidate := "__keyboard_local_profile_fallback__"
		if i > 0 {
			candidate += fmt.Sprintf("_%d", i)
		}
		if !slices.Contains(values, candidate) {
			return candidate
		}
	}
}

func contextualKeyboardBindingForProfile(ctx context.Context, configuration *commandbindings.Configuration, registry *commandcatalog.Registry, identity string, shortcut LocalCommandShortcut, values []string, profile string, withProfile bool, appPage string) (LocalCommandKeyboardContextualBinding, bool) {
	entry := LocalCommandKeyboardContextualBinding{Shortcut: shortcut, BySurface: map[string]*LocalCommandKeyboardBinding{}}
	requiresSurfaceType := len(configuration.FieldValues(identity, commandbindings.SurfaceType)) != 0
	markNoMatch := func(surface, id string, result commandbindings.Result) {
		if shortcut.Version != 1 || result.Status != commandbindings.NoMatch {
			return
		}
		if entry.SequenceFallbacks == nil {
			entry.SequenceFallbacks = make(map[string]map[string]bool)
		}
		if entry.SequenceFallbacks[surface] == nil {
			entry.SequenceFallbacks[surface] = make(map[string]bool)
		}
		entry.SequenceFallbacks[surface][id] = true
	}
	idValues := configuration.FieldValues(identity, commandbindings.SurfaceID)
	if len(idValues) != 0 {
		entry.BySurfaceID = make(map[string]map[string]*LocalCommandKeyboardBinding, len(values))
	}
	for _, value := range values {
		if err := ctx.Err(); err != nil {
			return LocalCommandKeyboardContextualBinding{}, false
		}
		facts := commandbindings.Facts{commandbindings.AppFocused: true}
		if requiresSurfaceType {
			facts[commandbindings.SurfaceType] = value
		}
		if withProfile {
			facts[commandbindings.Profile] = profile
		}
		if appPage != "" {
			facts[commandbindings.AppPage] = appPage
		}
		resolved, err := configuration.Resolve(identity, facts, nil)
		if err != nil {
			return LocalCommandKeyboardContextualBinding{}, false
		}
		binding, ok := resolvedLocalKeyboardBinding(configuration, registry, identity, shortcut, facts)
		if resolved.Status != commandbindings.Selected || !ok || (binding.Handler != "local_ui" && !localKeyboardWorkspaceSurface(value)) {
			entry.BySurface[value] = nil
			markNoMatch(value, "", resolved)
		} else {
			entry.BySurface[value] = &binding
		}
		if len(idValues) != 0 {
			entry.BySurfaceID[value] = make(map[string]*LocalCommandKeyboardBinding, len(idValues))
			for _, id := range idValues {
				if err := ctx.Err(); err != nil {
					return LocalCommandKeyboardContextualBinding{}, false
				}
				facts := commandbindings.Facts{commandbindings.AppFocused: true}
				if requiresSurfaceType {
					facts[commandbindings.SurfaceType] = value
				}
				if len(idValues) != 0 {
					facts[commandbindings.SurfaceID] = id
				}
				if withProfile {
					facts[commandbindings.Profile] = profile
				}
				if appPage != "" {
					facts[commandbindings.AppPage] = appPage
				}
				binding, ok := resolvedLocalKeyboardBinding(configuration, registry, identity, shortcut, facts)
				if !ok || (binding.Handler != "local_ui" && !localKeyboardWorkspaceSurface(value)) {
					entry.BySurfaceID[value][id] = nil
					resolved, err := configuration.Resolve(identity, facts, nil)
					if err != nil {
						return LocalCommandKeyboardContextualBinding{}, false
					}
					markNoMatch(value, id, resolved)
					continue
				}
				entry.BySurfaceID[value][id] = &binding
			}
		}
	}
	fallbackFacts := commandbindings.Facts{commandbindings.AppFocused: true}
	if requiresSurfaceType {
		fallbackFacts[commandbindings.SurfaceType] = contextualFallbackSurface(values)
	}
	if withProfile {
		fallbackFacts[commandbindings.Profile] = profile
	}
	if appPage != "" {
		fallbackFacts[commandbindings.AppPage] = appPage
	}
	fallback, ok := resolvedLocalKeyboardBinding(configuration, registry, identity, shortcut, fallbackFacts)
	if ok {
		entry.Fallback = &fallback
	} else {
		resolved, err := configuration.Resolve(identity, fallbackFacts, nil)
		// Only absence of a standalone binding permits the independently resolved
		// sequence prefix. Suppression, conflict and unavailable branches remain barriers.
		entry.FallbackToSequences = shortcut.Version == 1 && err == nil && resolved.Status == commandbindings.NoMatch
	}
	return entry, true
}

func contextualFallbackSurface(values []string) string {
	for i := 0; ; i++ {
		candidate := "__contextual_other__"
		if i > 0 {
			candidate += fmt.Sprintf("_%d", i)
		}
		if !slices.Contains(values, candidate) {
			return candidate
		}
	}
}

func cloneContextualKeyboardBindings(in []LocalCommandKeyboardContextualBinding) []LocalCommandKeyboardContextualBinding {
	if in == nil {
		return nil
	}
	out := make([]LocalCommandKeyboardContextualBinding, len(in))
	for i, entry := range in {
		out[i].FallbackToSequences = entry.FallbackToSequences
		if entry.SequenceFallbacks != nil {
			out[i].SequenceFallbacks = make(map[string]map[string]bool, len(entry.SequenceFallbacks))
			for surface, ids := range entry.SequenceFallbacks {
				out[i].SequenceFallbacks[surface] = make(map[string]bool, len(ids))
				for id, fallback := range ids {
					out[i].SequenceFallbacks[surface][id] = fallback
				}
			}
		}
		out[i].Shortcut = entry.Shortcut
		out[i].Shortcut.Modifiers = slices.Clone(entry.Shortcut.Modifiers)
		out[i].Shortcut.Steps = cloneLocalCommandShortcutSteps(entry.Shortcut.Steps)
		out[i].BySurface = make(map[string]*LocalCommandKeyboardBinding, len(entry.BySurface))
		for surface, binding := range entry.BySurface {
			if binding == nil {
				out[i].BySurface[surface] = nil
				continue
			}
			copy := *binding
			copy.Shortcut.Modifiers = slices.Clone(binding.Shortcut.Modifiers)
			copy.Shortcut.Steps = cloneLocalCommandShortcutSteps(binding.Shortcut.Steps)
			out[i].BySurface[surface] = &copy
		}
		if entry.BySurfaceID != nil {
			out[i].BySurfaceID = make(map[string]map[string]*LocalCommandKeyboardBinding, len(entry.BySurfaceID))
			for surface, ids := range entry.BySurfaceID {
				out[i].BySurfaceID[surface] = make(map[string]*LocalCommandKeyboardBinding, len(ids))
				for id, binding := range ids {
					if binding == nil {
						out[i].BySurfaceID[surface][id] = nil
						continue
					}
					copy := *binding
					copy.Shortcut.Modifiers = slices.Clone(binding.Shortcut.Modifiers)
					copy.Shortcut.Steps = cloneLocalCommandShortcutSteps(binding.Shortcut.Steps)
					out[i].BySurfaceID[surface][id] = &copy
				}
			}
		}
		if entry.ByProfile != nil {
			out[i].ByProfile = make(map[string]*LocalCommandKeyboardContextualBinding, len(entry.ByProfile))
			for profile, leaf := range entry.ByProfile {
				if leaf == nil {
					out[i].ByProfile[profile] = nil
					continue
				}
				cloned := cloneContextualKeyboardBindings([]LocalCommandKeyboardContextualBinding{*leaf})
				out[i].ByProfile[profile] = &cloned[0]
			}
		}
		if entry.ByPage != nil {
			out[i].ByPage = make(map[string]*LocalCommandKeyboardContextualBinding, len(entry.ByPage))
			for page, branch := range entry.ByPage {
				if branch == nil {
					out[i].ByPage[page] = nil
					continue
				}
				cloned := cloneContextualKeyboardBindings([]LocalCommandKeyboardContextualBinding{*branch})
				out[i].ByPage[page] = &cloned[0]
			}
		}
		if entry.Fallback != nil {
			copy := *entry.Fallback
			copy.Shortcut.Modifiers = slices.Clone(entry.Fallback.Shortcut.Modifiers)
			copy.Shortcut.Steps = cloneLocalCommandShortcutSteps(entry.Fallback.Shortcut.Steps)
			out[i].Fallback = &copy
		}
	}
	return out
}

func cloneLocalCommandShortcutSteps(in []LocalCommandShortcutStep) []LocalCommandShortcutStep {
	if in == nil {
		return nil
	}
	out := make([]LocalCommandShortcutStep, len(in))
	for i, step := range in {
		out[i] = step
		out[i].Modifiers = slices.Clone(step.Modifiers)
	}
	return out
}

func (p *commandProductRuntime) localKeyboardState() (string, commandexecution.Versions, bool) {
	if p == nil {
		return "", commandexecution.Versions{}, false
	}
	p.keyboardMu.Lock()
	defer p.keyboardMu.Unlock()
	if p.keyboardMap == nil || p.keyboardMap.ctx.Err() != nil {
		return "", commandexecution.Versions{}, false
	}
	return p.keyboardMap.view.Generation, p.keyboardMap.versions, true
}

// Só dispensa a notificação quando a projeção efetivamente publicada continua
// sendo exatamente a do mapa vivo. Deadline, configuração, sessão ou versões
// diferentes exigem a atualização normal do frontend.
func (p *commandProductRuntime) localKeyboardProjectionCurrent(ctx context.Context) bool {
	if p == nil || p.app == nil || p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
		return false
	}
	configuration, _, versions, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil {
		return false
	}
	p.keyboardMu.Lock()
	defer p.keyboardMu.Unlock()
	state := p.keyboardMap
	return state != nil && state.ctx.Err() == nil &&
		(state.view.ValidUntil == 0 || time.Now().UnixMilli() < state.view.ValidUntil) &&
		state.configuration == configuration && state.versions == versions
}

func localPaletteUICommands(configuration *commandbindings.Configuration, registry *commandcatalog.Registry) []string {
	if configuration == nil || registry == nil {
		return []string{}
	}
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, identity := range configuration.TriggerIdentities() {
		if !strings.HasPrefix(identity, "palette:") || len(configuration.RequiredFacts(identity)) != 0 {
			continue
		}
		definitionID := strings.TrimPrefix(identity, "palette:")
		if !localPaletteUISelection(configuration, registry, identity, definitionID, commandbindings.Facts{commandbindings.AppFocused: true}) {
			continue
		}
		if _, ok := seen[definitionID]; ok {
			continue
		}
		seen[definitionID] = struct{}{}
		result = append(result, definitionID)
	}
	slices.Sort(result)
	return result
}

func localCommandShortcutAllowed(shortcut LocalCommandShortcut) bool {
	if shortcut.Version == 2 {
		if len(shortcut.Steps) != 2 || shortcut.Steps[0].Code == "" || shortcut.Steps[1].Code == "" || shortcut.Steps[1].Code == "Escape" || len(shortcut.Steps[1].Modifiers) != 0 {
			return false
		}
		return localCommandShortcutModifiersAllowed(shortcut.Steps[0].Modifiers)
	}
	if shortcut.Version != 1 {
		return false
	}
	if len(shortcut.Modifiers) == 0 {
		return shortcut.Code == "F1" || shortcut.Code == "F5" || shortcut.Code == "F6"
	}
	if shortcut.Code == "F6" && len(shortcut.Modifiers) == 1 && shortcut.Modifiers[0] == "Shift" {
		return true
	}
	return localCommandShortcutModifiersAllowed(shortcut.Modifiers)
}

func localCommandShortcutModifiersAllowed(modifiers []string) bool {
	seen := make(map[string]bool, len(modifiers))
	primary := false
	for _, modifier := range modifiers {
		if modifier != "Control" && modifier != "Alt" && modifier != "Meta" && modifier != "Shift" || seen[modifier] {
			return false
		}
		seen[modifier] = true
		if modifier == "Control" || modifier == "Alt" || modifier == "Meta" {
			primary = true
		}
	}
	return primary
}

// DispatchLocalCommandKey não recebe command ID, owner, occurrence ID ou
// source. Uma borda down inédita ganha IDs hostside, antes do executor comum.
func (a *App) DispatchLocalCommandKey(generation string, shortcut LocalCommandShortcut, kind string, repeat bool) (result *CommandExecutionResult, err error) {
	return a.dispatchLocalCommandKey(generation, shortcut, kind, repeat, nil)
}

func (a *App) DispatchContextualLocalCommandKey(generation string, shortcut LocalCommandShortcut, kind string, repeat bool, observed LocalCommandKeyboardContext) (*CommandExecutionResult, error) {
	return a.dispatchLocalCommandKey(generation, shortcut, kind, repeat, &observed)
}

func (a *App) dispatchLocalCommandKey(generation string, shortcut LocalCommandShortcut, kind string, repeat bool, observed *LocalCommandKeyboardContext) (result *CommandExecutionResult, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	var proof *localCommandKeyboardContextProof
	if observed != nil && kind == "down" {
		proof, err = a.captureLocalKeyboardContext(*observed)
		if err != nil {
			return nil, err
		}
	}
	p, s, identity, raw, err := a.claimLocalCommandKeyScoped(generation, shortcut, kind, repeat, "backend", false, proof)
	if err != nil || s == nil {
		return nil, err
	}
	id := uuid.Must(uuid.NewV7()).String()
	if err := p.registerLocalCommandOccurrence(s, identity, id, proof); err != nil {
		return nil, err
	}
	defer p.removeLocalCommandOccurrence(id)
	candidate := localKeyboardCandidate(id, raw)
	record, output, err := p.keyboardService.ExecuteEnvelopeWithResult(s.ctx, "", candidate)
	if err == nil && record.Status == commandledger.Succeeded && record.Envelope.CommandID != nil && isCommandLayerAction(*record.Envelope.CommandID) {
		// A publicação da própria ativação aposenta o mapa de entrada. Isso
		// não desfaz um efeito já confirmado; devolver apenas o status para
		// a mesma sessão, nunca dados vivos ou uma segunda execução.
		current, currentErr := a.authenticatedCommandProduct()
		if currentErr != nil || current != p || !commandRecordMatchesPrincipal(record, p.principal) {
			return nil, commandexecution.ErrDenied
		}
		value := commandProductResult(record)
		return &value, nil
	}
	if s.ctx.Err() != nil {
		return nil, commandexecution.ErrStale
	}
	value := commandProductResult(record)
	if err == nil && record.Status == commandledger.Succeeded {
		binding, ok := localKeyboardOccurrenceBinding(s, p.registry, identity, shortcut, proof)
		if !ok {
			return nil, commandexecution.ErrStale
		}
		value, err = a.commandProductResultWithOutput(p, binding.CommandID, record, output)
	}
	return &value, err
}

// A UI só escolhe a combinação. O destino é derivado do mapa hostside e é
// novamente resolvido pelo executor antes do handoff compartilhado.
func (a *App) BeginLocalCommandUIKey(generation string, shortcut LocalCommandShortcut, repeat bool) (result *commandui.Reservation, err error) {
	return a.beginLocalCommandUIKey(generation, shortcut, repeat, false, nil)
}

func (a *App) BeginContextualLocalCommandUIKey(generation string, shortcut LocalCommandShortcut, repeat bool, observed LocalCommandKeyboardContext) (*commandui.Reservation, error) {
	return a.beginLocalCommandUIKey(generation, shortcut, repeat, false, &observed)
}

// BeginEditorMermaidUIKey routes a frontend-observed scope, not a trusted
// topmost proof. The UI must capture its lease and close/revalidate its own
// modal before applying the effect. No arbitrary command ID is accepted.
func (a *App) BeginEditorMermaidUIKey(generation string, shortcut LocalCommandShortcut, repeat bool) (*commandui.Reservation, error) {
	if !mermaidSubmitShortcut(shortcut) {
		return nil, commandexecution.ErrDenied
	}
	return a.beginLocalCommandUIKey(generation, shortcut, repeat, true, nil)
}

func (a *App) beginLocalCommandUIKey(generation string, shortcut LocalCommandShortcut, repeat, mermaid bool, observed *LocalCommandKeyboardContext) (result *commandui.Reservation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	var proof *localCommandKeyboardContextProof
	if observed != nil {
		proof, err = a.captureLocalKeyboardContext(*observed)
		if err != nil {
			return nil, err
		}
	}
	p, s, identity, raw, err := a.claimLocalCommandKeyScoped(generation, shortcut, "down", repeat, "ui", mermaid, proof)
	if err != nil || s == nil {
		return nil, err
	}
	var invocationID string
	defer func() {
		if err != nil {
			p.removeLocalCommandOccurrence(invocationID)
		}
	}()
	binding, _ := localKeyboardOccurrenceBinding(s, p.registry, identity, shortcut, proof)
	var mermaidSnapshot workspace.CommandSnapshot
	if mermaid {
		binding = LocalCommandKeyboardBinding{Shortcut: shortcut, CommandID: commandEditorMermaidApplyID, Handler: "ui"}
		mermaidSnapshot, err = p.workspaceMgr.CommandSnapshot()
		if err != nil || mermaidSnapshot.WorkspaceID != p.workspaceID || mermaidSnapshot.Tab.Type != workspace.TabTypeEditor {
			return nil, commandexecution.ErrDenied
		}
	}
	reservation, err := a.beginCommandUI(p, binding.CommandID,
		func(reservation commandui.Reservation) (commandexecution.EnvelopeCandidate, error) {
			invocationID = reservation.InvocationID
			if err := p.registerLocalCommandOccurrence(s, identity, invocationID, proof); err != nil {
				return commandexecution.EnvelopeCandidate{}, err
			}
			p.keyboardMu.Lock()
			occurrence := p.keyboardEvents[invocationID]
			occurrence.mermaid = mermaid
			p.keyboardEvents[invocationID] = occurrence
			p.keyboardMu.Unlock()
			return localKeyboardCandidate(invocationID, raw), nil
		}, func(ctx context.Context, candidate commandexecution.EnvelopeCandidate) (commandledger.FullRecord, error) {
			defer p.removeLocalCommandOccurrence(candidate.InvocationID)
			return p.keyboardService.ExecuteEnvelope(ctx, "", candidate)
		}, func() bool {
			if s.ctx.Err() != nil {
				return false
			}
			if mermaid {
				current, err := p.workspaceMgr.CommandSnapshot()
				return err == nil && current == mermaidSnapshot
			}
			return p.localKeyboardContextCurrent(proof)
		})
	if err != nil {
		return nil, err
	}
	return &reservation, nil
}

func (a *App) claimLocalCommandKeyScoped(generation string, shortcut LocalCommandShortcut, kind string, repeat bool, handler string, mermaid bool, proof *localCommandKeyboardContextProof) (p *commandProductRuntime, s *localCommandKeyboardState, identity string, raw json.RawMessage, err error) {
	if a == nil || generation == "" || (kind != "down" && kind != "up") {
		return nil, nil, "", nil, commandexecution.ErrInvalidRequest
	}
	p = a.commandProduct.Load()
	if p == nil || p.keyboardService == nil || !p.dependenciesMatch(a) {
		return nil, nil, "", nil, commandexecution.ErrDenied
	}
	if !p.localKeyboardContextCurrent(proof) {
		return nil, nil, "", nil, commandexecution.ErrStale
	}
	raw, err = json.Marshal(shortcut)
	if err != nil {
		return nil, nil, "", nil, commandexecution.ErrInvalidRequest
	}
	identity, err = (commandconfig.KeyboardLocalTriggerPort{}).Normalize(a.commandBridgeContext(), raw)
	if err != nil {
		return nil, nil, "", nil, err
	}
	p.keyboardMu.Lock()
	defer p.keyboardMu.Unlock()
	s = p.keyboardMap
	if s == nil || s.view.Generation != generation || s.ctx.Err() != nil {
		return nil, nil, "", nil, commandexecution.ErrStale
	}
	if proof != nil && localKeyboardProfileRequired(s.configuration, identity) && (proof.observed.Profile == "" || proof.observed.Profile != localKeyboardEffectiveProfile(proof.snapshot)) {
		return nil, nil, "", nil, commandexecution.ErrDenied
	}
	pressKey := localCommandShortcutPressKey(shortcut)
	if kind == "up" {
		delete(s.pressed, pressKey)
		return p, nil, "", nil, nil
	}
	binding, exists := localKeyboardOccurrenceBinding(s, p.registry, identity, shortcut, proof)
	if mermaid {
		if !mermaidSubmitShortcut(shortcut) {
			return nil, nil, "", nil, commandexecution.ErrDenied
		}
		binding, exists = LocalCommandKeyboardBinding{Shortcut: shortcut, CommandID: commandEditorMermaidApplyID, Handler: "ui"}, true
	}
	if !exists || (binding.Handler != handler && (handler != "ui" || binding.Handler != "contextual")) {
		return nil, nil, "", nil, commandexecution.ErrDenied
	}
	pressedIdentity, alreadyPressed := s.pressed[pressKey]
	if repeat {
		// Repetição é uma responsabilidade da projeção local. Nenhum ingresso
		// backend deve transformar repeat em ocorrência auditada.
		_ = pressedIdentity
		_ = alreadyPressed
		return nil, nil, "", nil, commandexecution.ErrDenied
	}
	if alreadyPressed {
		return p, nil, "", nil, nil
	}
	if len(p.keyboardEvents) >= 64 {
		return nil, nil, "", nil, commandexecution.ErrDenied
	}
	s.pressed[pressKey] = identity
	return p, s, identity, raw, nil
}

func localCommandShortcutPressKey(shortcut LocalCommandShortcut) string {
	if shortcut.Version == 2 && len(shortcut.Steps) == 2 {
		return shortcut.Steps[1].Code
	}
	return shortcut.Code
}

func (p *commandProductRuntime) registerLocalCommandOccurrence(s *localCommandKeyboardState, identity, id string, proof *localCommandKeyboardContextProof) error {
	if !p.localKeyboardContextCurrent(proof) {
		return commandexecution.ErrStale
	}
	p.keyboardMu.Lock()
	defer p.keyboardMu.Unlock()
	if s != p.keyboardMap || s.ctx.Err() != nil || len(p.keyboardEvents) >= 64 {
		return commandexecution.ErrStale
	}
	if p.keyboardEvents == nil {
		p.keyboardEvents = make(map[string]localCommandKeyboardOccurrence)
	}
	raw, err := commandSettingsTriggerSpec(s.ctx, identity)
	var shortcut LocalCommandShortcut
	if err != nil || json.Unmarshal([]byte(raw), &shortcut) != nil {
		return commandexecution.ErrDenied
	}
	binding, ok := localKeyboardOccurrenceBinding(s, p.registry, identity, shortcut, proof)
	// Dedicated Mermaid ingress has an invariant independent of the user map.
	if !ok && !mermaidSubmitShortcut(shortcut) {
		return commandexecution.ErrDenied
	}
	p.keyboardEvents[id] = localCommandKeyboardOccurrence{state: s, identity: identity, binding: binding, context: proof}
	return nil
}

func (p *commandProductRuntime) removeLocalCommandOccurrence(id string) {
	p.keyboardMu.Lock()
	defer p.keyboardMu.Unlock()
	delete(p.keyboardEvents, id)
}

func localKeyboardCandidate(id string, raw json.RawMessage) commandexecution.EnvelopeCandidate {
	return commandexecution.EnvelopeCandidate{InvocationID: id, CorrelationID: id, TriggerType: string(commandcatalog.KeyboardLocal), TriggerSpec: raw, Arguments: json.RawMessage(`{}`)}
}

func localKeyboardCommandAllowed(id string) bool {
	if id == commandProductWorkspaceListID || isCommandLayerAction(id) || isLocalUICommand(id) || isWorkspaceMutationCommand(id) || isAuditedUIContextualCommand(id) {
		return true
	}
	for _, navigation := range commandProductUINavigation {
		if id == navigation.id {
			return true
		}
	}
	return false
}

func (a *App) ResetLocalCommandKeyboard(generation string) {
	if a == nil || generation == "" {
		return
	}
	if p := a.commandProduct.Load(); p != nil {
		p.clearLocalCommandKeyboard(generation)
	}
}

func (p *commandProductRuntime) clearLocalCommandKeyboard(generation string) {
	p.keyboardMu.Lock()
	defer p.keyboardMu.Unlock()
	if s := p.keyboardMap; s != nil && (generation == "" || generation == s.view.Generation) {
		s.release()
		p.keyboardMap = nil
		p.clearDeckPagePresentation(s.view.Generation)
	}
}

func (a *App) newLocalCommandKeyboardExecutor(p *commandProductRuntime, base commandexecution.Config, host *commandexecution.HostState, policy func(context.Context, auth.LocalSessionPrincipal, string, commandcatalog.Source) error) (*commandexecution.Service, error) {
	config := base
	config.Source = commandcatalog.KeyboardLocal
	envelope := *base.Envelope
	envelope.Snapshot = func(ctx context.Context, owner auth.LocalSessionPrincipal, candidate commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
		value, err := base.Envelope.Snapshot(ctx, owner, candidate)
		if err != nil {
			return value, err
		}
		identity, err := (commandconfig.KeyboardLocalTriggerPort{}).Normalize(ctx, candidate.TriggerSpec)
		if err != nil {
			return commandcontract.Envelope{}, err
		}
		p.keyboardMu.Lock()
		defer p.keyboardMu.Unlock()
		occurrence, exists := p.keyboardEvents[candidate.InvocationID]
		s := occurrence.state
		if !exists || s == nil || s != p.keyboardMap || s.ctx.Err() != nil || identity != occurrence.identity || candidate.TriggerType != string(commandcatalog.KeyboardLocal) ||
			!p.localKeyboardContextCurrent(occurrence.context) ||
			value.GlobalConfigGeneration == nil || *value.GlobalConfigGeneration != s.versions.GlobalConfig || value.ActiveLayersGeneration == nil || *value.ActiveLayersGeneration != s.versions.ActiveLayers {
			return commandcontract.Envelope{}, commandexecution.ErrStale
		}
		value.SourceInstanceID = commandStringPointer(s.view.Generation)
		value.SourceEventID = commandStringPointer(candidate.InvocationID)
		value.ObserverType = commandStringPointer(string(commandcatalog.KeyboardLocal))
		value.ObservedTriggerType = commandStringPointer(string(commandcatalog.KeyboardLocal))
		return value, nil
	}
	envelope.Authorize = func(ctx context.Context, owner auth.LocalSessionPrincipal, e commandcontract.Envelope, d commandcatalog.Definition) error {
		if !localKeyboardCommandAllowed(d.ID) || !d.AllowsSource(commandcatalog.KeyboardLocal) || e.SourceType == nil || *e.SourceType != commandcontract.SourceKeyboardLocal || !p.dependenciesMatch(a) {
			return commandexecution.ErrDenied
		}
		return policy(ctx, owner, d.ID, commandcatalog.KeyboardLocal)
	}
	envelope.AuthorizeLookup = func(ctx context.Context, owner auth.LocalSessionPrincipal, r commandledger.FullRecord) error {
		if !commandRecordMatchesPrincipal(r, owner) || r.Envelope.SourceType == nil || *r.Envelope.SourceType != commandcontract.SourceKeyboardLocal || !p.dependenciesMatch(a) {
			return commandexecution.ErrDenied
		}
		if r.Envelope.CommandID == nil || !localKeyboardCommandAllowed(*r.Envelope.CommandID) {
			return commandexecution.ErrDenied
		}
		return policy(ctx, owner, *r.Envelope.CommandID, commandcatalog.KeyboardLocal)
	}
	config.Envelope = &envelope
	return a.newCommandDesktopExecutor(config, host)
}
