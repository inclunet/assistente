package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"gorm.io/gorm"
)

var errCommandSettingsManualRuleAlreadyPrepared = errors.New("regra manual já preparada")

const commandLayerMutationHandoffTimeout = 35 * time.Second

type commandLayerMutationHandoff struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	parent  context.Context
	claimed bool
	stop    func() bool
}

func newCommandLayerMutationHandoff(parent context.Context) *commandLayerMutationHandoff {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), commandLayerMutationHandoffTimeout)
	h := &commandLayerMutationHandoff{ctx: ctx, cancel: cancel, parent: parent}
	h.stop = context.AfterFunc(parent, func() {
		h.mu.Lock()
		if !h.claimed {
			h.cancel()
		}
		h.mu.Unlock()
	})
	return h
}

func (h *commandLayerMutationHandoff) Context() context.Context { return h.ctx }

func (h *commandLayerMutationHandoff) Claim() error {
	return h.ClaimOwnership(nil)
}

func (h *commandLayerMutationHandoff) ClaimOwnership(ownership *commandexecution.CommitOwnership) error {
	if h == nil || h.parent == nil {
		return commandexecution.ErrInvalidRequest
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.parent.Err(); err != nil {
		h.cancel()
		return err
	}
	if err := h.ctx.Err(); err != nil {
		return err
	}
	if ownership != nil {
		if err := ownership.Claim(h.parent); err != nil {
			return err
		}
	}
	h.claimed = true
	if h.stop != nil {
		h.stop()
	}
	return nil
}

func (h *commandLayerMutationHandoff) Cancel() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if !h.claimed {
		h.cancel()
	}
	if h.stop != nil {
		h.stop()
	}
	h.mu.Unlock()
}

func (h *commandLayerMutationHandoff) Dispose() {
	if h == nil {
		return
	}
	if h.stop != nil {
		h.stop()
	}
	h.cancel()
}

// PrepareManualCommandLayer cria, mediante a decisão comum de mutação, a
// regra global persistente que autoriza a ação manual. Preparar não cria claim
// e portanto não ativa a camada.
func (a *App) PrepareManualCommandLayer(layerID string) (result CommandSettingsMutation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	if layerID == "" {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
	}
	layer, err := a.commandSettingsLayer(layerID)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	if layer.WorkspaceID != nil || layer.UserID == "" {
		return CommandSettingsMutation{}, commandexecution.ErrDenied
	}
	snapshot, _, err := a.commandSettingsSnapshotForActivation()
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	if rule, ok := commandSettingsCompatibleManualRule(snapshot, layerID); ok {
		return CommandSettingsMutation{Committed: true, Published: true, ID: rule.ID}, nil
	}
	for _, existing := range snapshot.ActivationRules {
		if commandSettingsManualRuleMatches(existing, snapshot.Scope.UserID, layerID) {
			// Mais de uma regra compatível exige resolução explícita; preparar
			// novamente não deve criar uma terceira nem escolher arbitrariamente.
			return CommandSettingsMutation{}, commandexecution.ErrInvalidConfiguration
		}
	}
	rule := commandactivation.Rule{LayerRefKind: commandactivation.UserRef, LayerRef: layerID,
		RuleRefKind: commandactivation.UserRef, Mode: commandactivation.ModeManual,
		Condition: `{}`, Lifecycle: commandactivation.LifecyclePersistent, Enabled: true,
		Source: "user", ReviewStatus: "active"}
	result, err = a.applyCommandSettingsMutation(commandconfig.MutationIntent{Operation: commandconfig.RuleCreate, Rule: &rule})
	if errors.Is(err, errCommandSettingsManualRuleAlreadyPrepared) {
		final, _, finalErr := a.commandSettingsSnapshotForActivation()
		if finalErr != nil {
			return CommandSettingsMutation{}, finalErr
		}
		if existing, ok := commandSettingsCompatibleManualRule(final, layerID); ok {
			return CommandSettingsMutation{Committed: true, Published: true, ID: existing.ID}, nil
		}
	}
	return result, err
}

// SetCommandLayerActive altera somente a claim manual mais recente da UI
// atual. A autoridade de owner, sessão, dispositivo e epochs é derivada aqui.
func (a *App) SetCommandLayerActive(layerID string, active bool) (result CommandSettingsMutation, err error) {
	return a.setCommandLayerActiveForScope(string(CommandSettingsScopeGlobal), layerID, active)
}

// SetCommandLayerActiveForScope é a variante autoritativa para o workspace
// atual. O ID do workspace não é aceito no payload; ele é derivado do host.
func (a *App) SetCommandLayerActiveForScope(scopeName, layerID string, active bool) (result CommandSettingsMutation, err error) {
	return a.setCommandLayerActiveForScope(scopeName, layerID, active)
}

// ApplyCommandLayerAction aplica uma ação manual sobre uma regra já
// confirmada na configuração do escopo atualmente autenticado. A identidade
// da camada é sempre derivada da regra; o chamador não escolhe owner,
// workspace, origem ou manual_stack_key.
func (a *App) ApplyCommandLayerAction(scopeName, ruleID, action string, durationSeconds int) (result CommandSettingsMutation, err error) {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	return a.applyCommandLayerActionWithOrigin(a.commandBridgeContext(), p.principal, scopeName, ruleID, action, durationSeconds, commandSettingsManualOrigin(p.principal), nil, nil)
}

func (a *App) applyCommandLayerActionWithOrigin(ctx context.Context, expected auth.LocalSessionPrincipal, scopeName, ruleID, action string, durationSeconds int, origin commandactivation.Origin, ownership *commandexecution.CommitOwnership, handoff *commandLayerMutationHandoff) (result CommandSettingsMutation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	if ctx == nil {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return CommandSettingsMutation{}, err
	}
	action = strings.TrimSpace(action)
	if action != "back" && action != "pin" && action != "toggle" && action != "deactivate" {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
	}
	if action == "back" {
		if ruleID != "" || durationSeconds != 0 {
			return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
		}
	} else if ruleID == "" || durationSeconds < 0 || durationSeconds > 86400 || action == "deactivate" && durationSeconds != 0 {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
	}

	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	if p.principal != expected {
		return CommandSettingsMutation{}, commandexecution.ErrStale
	}
	if p.principal.UserID == "" || p.principal.SessionID == "" {
		return CommandSettingsMutation{}, commandexecution.ErrDenied
	}
	scope, err := a.commandSettingsScope(p, scopeName)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	epoch, err := p.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || current != p.principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, current) {
			return "", "", commandexecution.ErrDenied
		}
		return current.UserID, current.SessionID, nil
	})
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	settingsStore, err := commandconfig.New(database.DB())
	if err != nil {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidConfiguration
	}
	snapshot, err := settingsStore.Load(ctx, scope)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	if err := settingsStore.CheckCurrent(ctx, snapshot); err != nil {
		return CommandSettingsMutation{}, err
	}

	var rule commandactivation.Rule
	var layer commandconfig.Layer
	if action != "back" {
		var targetErr error
		rule, layer, targetErr = commandSettingsManualActionTarget(snapshot, scope, p.principal, ruleID, action, durationSeconds)
		if targetErr != nil {
			return CommandSettingsMutation{}, targetErr
		}
	}

	owner := commandactivation.Owner{Scope: commandactivation.Scope{UserID: p.principal.UserID, WorkspaceID: cloneCommandWorkspace(scope.WorkspaceID)}, AuthContextType: "local_session", AuthContextID: p.principal.SessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}
	service, err := a.newCommandSettingsActivationServiceForOrigin(p, owner, snapshot, settingsStore, origin, ownership, handoff)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	if a.commandHost == nil {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidConfiguration
	}
	watched, release, err := p.epochs.WatchEpoch(ctx, epoch)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	defer release()
	var mutation commandactivation.Mutation
	switch action {
	case "back":
		// BackLatest é deliberadamente resolvido pelo serviço para não aceitar
		// uma regra/camada escolhida pelo cliente.
		mutation, err = service.BackLatest(watched, owner, origin)
	case "pin", "toggle":
		var expiresAt *time.Time
		if durationSeconds > 0 {
			expires := time.Now().UTC().Add(time.Duration(durationSeconds) * time.Second)
			expiresAt = &expires
		}
		layerRef := commandactivation.Ref{Kind: commandactivation.UserRef, ID: layer.ID}
		ruleRef := commandactivation.Ref{Kind: commandactivation.UserRef, ID: rule.ID}
		if action == "pin" {
			mutation, err = service.Pin(watched, owner, layerRef, ruleRef, origin, expiresAt)
		} else {
			mutation, err = service.Toggle(watched, owner, layerRef, ruleRef, origin, expiresAt)
		}
	case "deactivate":
		mutation, err = service.Back(watched, owner, commandactivation.Ref{Kind: commandactivation.UserRef, ID: layer.ID}, commandactivation.Ref{Kind: commandactivation.UserRef, ID: rule.ID}, origin)
	}
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	result = CommandSettingsMutation{Committed: true, ID: ruleID}
	if action == "back" && mutation.Claim.RuleRef != "" {
		result.ID = mutation.Claim.RuleRef
	}
	if err := a.rebuildCommandLifecycleProjection(ctx, false); err != nil {
		return result, nil
	}
	result.Published = true
	return result, nil
}

func (a *App) setCommandLayerActiveForScope(scopeName, layerID string, active bool) (result CommandSettingsMutation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	if layerID == "" {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	if p.principal.UserID == "" || p.principal.SessionID == "" {
		return CommandSettingsMutation{}, commandexecution.ErrDenied
	}
	scope, err := a.commandSettingsScope(p, scopeName)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	epoch, err := p.epochs.CaptureAuthenticated(a.commandBridgeContext(), func(ctx context.Context) (string, string, error) {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || current != p.principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, current) {
			return "", "", commandexecution.ErrDenied
		}
		return current.UserID, current.SessionID, nil
	})
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	settingsStore, err := commandconfig.New(database.DB())
	if err != nil {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidConfiguration
	}
	snapshot, err := settingsStore.Load(a.commandBridgeContext(), scope)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	if err := settingsStore.CheckCurrent(a.commandBridgeContext(), snapshot); err != nil {
		return CommandSettingsMutation{}, err
	}
	rule, ok := commandSettingsCompatibleManualRuleScoped(snapshot, layerID, scope.WorkspaceID)
	if !ok {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
	}
	owner := commandactivation.Owner{Scope: commandactivation.Scope{UserID: p.principal.UserID, WorkspaceID: cloneCommandWorkspace(scope.WorkspaceID)}, AuthContextType: "local_session", AuthContextID: p.principal.SessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}
	service, err := a.newCommandSettingsActivationService(p, owner, snapshot, settingsStore)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	origin := commandSettingsManualOrigin(p.principal)
	state := a.commandHost
	if state == nil {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidConfiguration
	}
	watched, release, err := p.epochs.WatchEpoch(a.commandBridgeContext(), epoch)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	defer release()
	if active {
		_, err = service.Pin(watched, owner, commandactivation.Ref{Kind: commandactivation.UserRef, ID: layerID}, commandactivation.Ref{Kind: commandactivation.UserRef, ID: rule.ID}, origin, nil)
	} else {
		_, err = service.Back(watched, owner, commandactivation.Ref{Kind: commandactivation.UserRef, ID: layerID}, commandactivation.Ref{Kind: commandactivation.UserRef, ID: rule.ID}, origin)
		if errors.Is(err, commandactivation.ErrNotFound) {
			current, snapshotErr := settingsStore.Load(a.commandBridgeContext(), scope)
			if snapshotErr != nil {
				return CommandSettingsMutation{}, snapshotErr
			}
			currentRule, compatible := commandSettingsCompatibleManualRuleScoped(current, layerID, scope.WorkspaceID)
			layerExists := false
			for _, layer := range current.Layers {
				layerExists = layerExists || (layer.ID == layerID && layer.UserID == p.principal.UserID && sameCommandWorkspace(layer.WorkspaceID, scope.WorkspaceID))
			}
			if current.Scope.UserID != p.principal.UserID || !sameCommandWorkspace(current.Scope.WorkspaceID, scope.WorkspaceID) || !layerExists || !compatible || currentRule.ID != rule.ID {
				return CommandSettingsMutation{}, commandexecution.ErrStale
			}
			if commandSettingsManualClaimPresentScoped(current, layerID, rule.ID, p.principal, scope.WorkspaceID, nowCommandSettings()) {
				return CommandSettingsMutation{}, err
			}
			publishedErr := a.rebuildCommandLifecycleProjection(a.commandBridgeContext(), false)
			return CommandSettingsMutation{Committed: true, Published: publishedErr == nil, ID: rule.ID}, nil
		}
	}
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	result = CommandSettingsMutation{Committed: true, ID: rule.ID}
	// A claim committed but not published is terminal for this request; callers
	// must not retry the writer implicitly. Rebuild reads the persisted state.
	if err := a.rebuildCommandLifecycleProjection(a.commandBridgeContext(), false); err != nil {
		return result, nil
	}
	result.Published = true
	return result, nil
}

func (a *App) commandSettingsSnapshotForActivation() (commandconfig.Snapshot, *commandconfig.Store, error) {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return commandconfig.Snapshot{}, nil, err
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return commandconfig.Snapshot{}, nil, commandexecution.ErrInvalidConfiguration
	}
	ctx := a.commandBridgeContext()
	epoch, err := p.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || current != p.principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, current) {
			return "", "", commandexecution.ErrDenied
		}
		return current.UserID, current.SessionID, nil
	})
	if err != nil {
		return commandconfig.Snapshot{}, nil, err
	}
	snapshot, err := store.Load(ctx, commandconfig.Scope{UserID: p.principal.UserID})
	if err != nil {
		return commandconfig.Snapshot{}, store, err
	}
	if err := store.CheckCurrent(ctx, snapshot); err != nil {
		return commandconfig.Snapshot{}, store, err
	}
	if err := p.epochs.Admit(ctx, epoch, func(ctx context.Context) error {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
		if err != nil || current != p.principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, current) {
			return commandexecution.ErrStale
		}
		return nil
	}, func() error { return nil }); err != nil {
		return commandconfig.Snapshot{}, store, err
	}
	if current, err := a.authenticatedCommandProduct(); err != nil || current != p {
		if err != nil {
			return commandconfig.Snapshot{}, store, err
		}
		return commandconfig.Snapshot{}, store, commandexecution.ErrStale
	}
	return snapshot, store, nil
}

func commandSettingsCompatibleManualRule(snapshot commandconfig.Snapshot, layerID string) (commandactivation.Rule, bool) {
	return commandSettingsCompatibleManualRuleScoped(snapshot, layerID, nil)
}

func commandSettingsCompatibleManualRuleScoped(snapshot commandconfig.Snapshot, layerID string, workspace *string) (commandactivation.Rule, bool) {
	var found commandactivation.Rule
	for _, rule := range snapshot.ActivationRules {
		if !commandSettingsManualRuleMatchesScoped(rule, snapshot.Scope.UserID, layerID, workspace) {
			continue
		}
		if found.ID != "" {
			return commandactivation.Rule{}, false
		}
		found = rule
	}
	return found, found.ID != ""
}

func commandSettingsManualRuleMatches(rule commandactivation.Rule, userID, layerID string) bool {
	return commandSettingsManualRuleMatchesScoped(rule, userID, layerID, nil)
}

func commandSettingsManualRuleMatchesScoped(rule commandactivation.Rule, userID, layerID string, workspace *string) bool {
	return rule.UserID == userID && sameCommandWorkspace(rule.WorkspaceID, workspace) && rule.LayerRefKind == commandactivation.UserRef && rule.LayerRef == layerID &&
		rule.RuleRefKind == commandactivation.UserRef && rule.Mode == commandactivation.ModeManual && rule.Condition == `{}` && rule.Lifecycle == commandactivation.LifecyclePersistent &&
		rule.Enabled && rule.Source == "user" && rule.ReviewStatus == "active" && rule.EventName == nil && rule.AllowedInternalProducerTypes == nil
}

// commandSettingsManualActionTarget é o predicado autoritativo compartilhado
// pelo executor durável e pelo readiness. Ele só aceita uma regra manual ou
// toggle efetivamente utilizável no escopo autenticado.
func commandSettingsManualActionTarget(snapshot commandconfig.Snapshot, scope commandconfig.Scope, principal auth.LocalSessionPrincipal, ruleID, action string, durationSeconds int) (commandactivation.Rule, commandconfig.Layer, error) {
	if ruleID == "" || (action != "pin" && action != "toggle" && action != "deactivate") || durationSeconds < 0 || durationSeconds > 86400 {
		return commandactivation.Rule{}, commandconfig.Layer{}, commandexecution.ErrInvalidRequest
	}
	if action == "deactivate" && durationSeconds != 0 {
		return commandactivation.Rule{}, commandconfig.Layer{}, commandexecution.ErrInvalidRequest
	}
	var found commandactivation.Rule
	for _, candidate := range snapshot.ActivationRules {
		if candidate.ID != ruleID || candidate.UserID != principal.UserID ||
			!sameCommandWorkspace(candidate.WorkspaceID, scope.WorkspaceID) ||
			candidate.LayerRefKind != commandactivation.UserRef || candidate.RuleRefKind != commandactivation.UserRef ||
			candidate.Source != "user" || !candidate.Enabled || candidate.ReviewStatus != "active" ||
			(candidate.Mode != commandactivation.ModeManual && candidate.Mode != commandactivation.ModeToggle) ||
			candidate.Condition != `{}` || candidate.EventName != nil || candidate.AllowedInternalProducerTypes != nil {
			continue
		}
		if found.ID != "" {
			return commandactivation.Rule{}, commandconfig.Layer{}, commandexecution.ErrInvalidConfiguration
		}
		found = candidate
	}
	if found.ID == "" {
		return commandactivation.Rule{}, commandconfig.Layer{}, commandexecution.ErrInvalidRequest
	}
	layer, ok := commandSettingsLayer(snapshot, found.LayerRef, scope)
	// Retiring an existing manual claim must remain possible after its layer
	// is disabled. Activation/toggle still require an enabled target.
	if !ok || layer.UserID != principal.UserID || (!layer.Enabled && action != "deactivate") {
		return commandactivation.Rule{}, commandconfig.Layer{}, commandexecution.ErrInvalidRequest
	}
	if durationSeconds > 0 && found.Lifecycle != commandactivation.LifecycleTemporary ||
		durationSeconds == 0 && found.Lifecycle != commandactivation.LifecyclePersistent && found.Lifecycle != commandactivation.LifecycleSession {
		return commandactivation.Rule{}, commandconfig.Layer{}, commandexecution.ErrInvalidRequest
	}
	return found, layer, nil
}

func commandSettingsManualClaimPresent(snapshot commandconfig.Snapshot, layerID, ruleID string, principal auth.LocalSessionPrincipal, now time.Time) bool {
	return commandSettingsManualClaimPresentScoped(snapshot, layerID, ruleID, principal, nil, now)
}

func commandSettingsManualClaimPresentScoped(snapshot commandconfig.Snapshot, layerID, ruleID string, principal auth.LocalSessionPrincipal, workspace *string, now time.Time) bool {
	stackKey, err := commandactivation.ManualStackKey(commandSettingsManualOrigin(principal))
	if err != nil {
		return false
	}
	for _, claim := range snapshot.ActivationClaims {
		if claim.UserID == snapshot.Scope.UserID && sameCommandWorkspace(claim.WorkspaceID, workspace) && claim.LayerRefKind == commandactivation.UserRef && claim.LayerRef == layerID &&
			claim.RuleRefKind == commandactivation.UserRef && claim.RuleRef == ruleID && claim.SourceType == "manual" && claim.AuthContextID == principal.SessionID &&
			claim.ManualStackKey != nil && *claim.ManualStackKey == stackKey && claim.State == commandactivation.StateActive && (claim.ExpiresAt == nil || claim.ExpiresAt.After(now)) {
			return true
		}
	}
	return false
}

func (a *App) newCommandSettingsActivationService(p *commandProductRuntime, owner commandactivation.Owner, snapshot commandconfig.Snapshot, configStore *commandconfig.Store) (*commandactivation.Service, error) {
	if p == nil {
		return nil, commandactivation.ErrInvalid
	}
	return a.newCommandSettingsActivationServiceForOrigin(p, owner, snapshot, configStore, commandSettingsManualOrigin(p.principal), nil, nil)
}

func (a *App) newCommandSettingsActivationServiceForOrigin(p *commandProductRuntime, owner commandactivation.Owner, snapshot commandconfig.Snapshot, configStore *commandconfig.Store, expectedOrigin commandactivation.Origin, ownership *commandexecution.CommitOwnership, handoff *commandLayerMutationHandoff) (*commandactivation.Service, error) {
	if a == nil || p == nil || a.commandGate == nil {
		return nil, commandactivation.ErrInvalid
	}
	store, err := commandactivation.NewStore(database.DB())
	if err != nil {
		return nil, err
	}
	if configStore == nil {
		return nil, commandactivation.ErrInvalid
	}
	return commandactivation.New(database.DB(), a.commandGate, commandactivation.Ports{
		Owner: commandactivation.OwnerPortFunc(func(ctx context.Context, asserted commandactivation.Owner) (commandactivation.Owner, error) {
			if err := ctx.Err(); err != nil {
				return commandactivation.Owner{}, err
			}
			if err := configStore.CheckCurrent(ctx, snapshot); err != nil {
				return commandactivation.Owner{}, err
			}
			if asserted.UserID != owner.UserID || asserted.AuthContextID != owner.AuthContextID || !sameCommandWorkspace(asserted.WorkspaceID, owner.WorkspaceID) || a.commandProduct.Load() != p {
				return commandactivation.Owner{}, commandexecution.ErrDenied
			}
			current, err := p.sessionSvc.RevalidateLocalSession(ctx, p.principal)
			if err != nil || current != p.principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, current) {
				return commandactivation.Owner{}, commandexecution.ErrDenied
			}
			// Este callback roda dentro de service.gate.WithMutation. A
			// autoridade das gerações já foi vinculada por WatchEpoch; consultar
			// EpochService aqui readquiriria o mesmo gate e causaria deadlock.
			if err := ctx.Err(); err != nil {
				return commandactivation.Owner{}, err
			}
			versions, err := a.commandHost.Snapshot(ctx, p.principal)
			if err != nil || !versions.Unlocked {
				return commandactivation.Owner{}, commandexecution.ErrDenied
			}
			currentScope, scopeErr := a.commandMutationCurrentScope(current)
			if owner.WorkspaceID != nil && (scopeErr != nil || !sameCommandWorkspace(currentScope.WorkspaceID, owner.WorkspaceID)) {
				return commandactivation.Owner{}, commandexecution.ErrStale
			}
			return owner, nil
		}),
		Layer: commandLifecycleActivationLayerPort{},
		Origin: commandactivation.OriginPortFunc(func(_ context.Context, _ commandactivation.Owner, asserted commandactivation.Origin) (commandactivation.Origin, error) {
			if asserted != expectedOrigin {
				return commandactivation.Origin{}, errors.New("origem manual divergente")
			}
			return expectedOrigin, nil
		}),
		Rule: store,
		GenerationTx: commandactivation.GenerationTxPortFunc(func(ctx context.Context, tx *gorm.DB, current commandactivation.Owner) (commandactivation.GenerationSnapshot, error) {
			if err := ctx.Err(); err != nil {
				return commandactivation.GenerationSnapshot{}, err
			}
			validated, err := p.sessionSvc.RevalidateLocalSessionTx(ctx, tx, p.principal)
			if err != nil || validated != p.principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, validated) {
				return commandactivation.GenerationSnapshot{}, commandexecution.ErrDenied
			}
			currentScope, scopeErr := a.commandMutationCurrentScope(validated)
			if current.WorkspaceID != nil && (scopeErr != nil || !sameCommandWorkspace(currentScope.WorkspaceID, current.WorkspaceID)) {
				return commandactivation.GenerationSnapshot{}, commandexecution.ErrStale
			}
			if ready, err := a.commandHost.SourceSecurityReady(ctx); err != nil || !ready {
				return commandactivation.GenerationSnapshot{}, commandexecution.ErrDenied
			}
			// A contextual visual submission can become obsolete while the
			// handler waits for the mutation gate. Revalidate its private proof
			// at the authoritative boundary, before claiming commit ownership or
			// suspending the configuration that this mutation itself replaces.
			claim := func() error {
				if handoff != nil {
					return handoff.ClaimOwnership(ownership)
				}
				if ownership != nil {
					return ownership.Claim(ctx)
				}
				return nil
			}
			if err := p.claimContextualLayerOwnership(ctx, claim); err != nil {
				return commandactivation.GenerationSnapshot{}, err
			}
			if err := a.commandHost.SuspendUserConfiguration(ctx, current.UserID); err != nil && !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
				return commandactivation.GenerationSnapshot{}, err
			}
			return store.BumpActiveLayersTx(ctx, tx, current)
		}),
	}, time.Now)
}

func commandSettingsManualOrigin(principal auth.LocalSessionPrincipal) commandactivation.Origin {
	// A UI usa a origem hostside da restauração para manter a claim removível
	// após rebuild/reabertura, sem aceitar uma stack informada pelo cliente.
	return commandLifecycleRestoreOrigin(principal)
}
