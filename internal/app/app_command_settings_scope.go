package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjson"
	"assistente/internal/database"
	"gorm.io/gorm"
)

// CommandSettingsScope é deliberadamente pequeno: workspace significa apenas
// o workspace atualmente autenticado, nunca um ID escolhido pelo cliente.
type CommandSettingsScope string

const (
	CommandSettingsScopeGlobal    CommandSettingsScope = "global"
	CommandSettingsScopeWorkspace CommandSettingsScope = "workspace"
)

type CommandSettingsConditionClause struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type CommandSettingsCondition struct {
	Version int                              `json:"version"`
	Clauses []CommandSettingsConditionClause `json:"clauses"`
}

type CommandSettingsLayerInput struct {
	ID                 string `json:"id,omitempty"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Enabled            bool   `json:"enabled"`
	ResolutionPriority int    `json:"resolutionPriority"`
}

type CommandSettingsBindingInput struct {
	ID                         string                    `json:"id,omitempty"`
	LayerID                    string                    `json:"layerId"`
	CommandID                  string                    `json:"commandId,omitempty"`
	TriggerType                string                    `json:"triggerType"`
	TriggerSpec                string                    `json:"triggerSpec"`
	Arguments                  map[string]any            `json:"arguments,omitempty"`
	Condition                  *CommandSettingsCondition `json:"condition,omitempty"`
	Effect                     string                    `json:"effect"`
	Enabled                    bool                      `json:"enabled"`
	ResolutionPriority         int                       `json:"resolutionPriority"`
	ReplacesDefaultID          string                    `json:"replacesDefaultId,omitempty"`
	ReplacesDefaultVersion     string                    `json:"replacesDefaultVersion,omitempty"`
	ReplacesDefaultFingerprint string                    `json:"replacesDefaultFingerprint,omitempty"`
	Presentation               map[string]any            `json:"presentation,omitempty"`
}

type CommandSettingsRuleInput struct {
	ID                           string                    `json:"id,omitempty"`
	LayerID                      string                    `json:"layerId"`
	Mode                         string                    `json:"mode"`
	Condition                    *CommandSettingsCondition `json:"condition,omitempty"`
	Lifecycle                    string                    `json:"lifecycle"`
	EventName                    string                    `json:"eventName,omitempty"`
	AllowedInternalProducerTypes string                    `json:"allowedInternalProducerTypes,omitempty"`
	Enabled                      bool                      `json:"enabled"`
}

type CommandSettingsDefaultInput struct {
	BindingID string                    `json:"bindingId"`
	Default   CommandSettingsDefaultRef `json:"default"`
	Condition *CommandSettingsCondition `json:"condition,omitempty"`
}

type CommandSettingsDefaultRef struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
}

// CommandSettingsMutationRequest é o único contrato avançado de escrita.
// Campos de owner, sessão, workspace e grants não fazem parte dele.
type CommandSettingsMutationRequest struct {
	Locale              string                       `json:"locale,omitempty"`
	Scope               CommandSettingsScope         `json:"scope"`
	Operation           string                       `json:"operation"`
	ID                  string                       `json:"id,omitempty"`
	LayerRefKind        string                       `json:"layerRefKind,omitempty"`
	ExpectedRevision    int64                        `json:"expectedRevision"`
	ExpectedFingerprint string                       `json:"expectedFingerprint"`
	Layer               *CommandSettingsLayerInput   `json:"layer,omitempty"`
	Binding             *CommandSettingsBindingInput `json:"binding,omitempty"`
	Rule                *CommandSettingsRuleInput    `json:"rule,omitempty"`
	Default             *CommandSettingsDefaultInput `json:"default,omitempty"`
}

func (a *App) GetCommandSettingsForScope(locale, scopeName string) (result CommandSettingsSnapshot, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	ctx := a.commandBridgeContext()
	if ctx == nil {
		return CommandSettingsSnapshot{}, commandexecution.ErrDenied
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return CommandSettingsSnapshot{}, err
	}
	p.setDeckLocale(locale)
	scope, err := a.commandSettingsScope(p, scopeName)
	if err != nil {
		return CommandSettingsSnapshot{}, err
	}
	principal := p.principal
	epoch, err := p.epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, principal)
		if err != nil || current != principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, principal) {
			return "", "", commandexecution.ErrDenied
		}
		return principal.UserID, principal.SessionID, nil
	})
	if err != nil {
		return CommandSettingsSnapshot{}, err
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return CommandSettingsSnapshot{}, commandexecution.ErrInvalidConfiguration
	}
	snapshot, projection, active, err := a.commandSettingsAuthority(ctx, p, scope, store)
	if err != nil {
		return CommandSettingsSnapshot{}, err
	}
	result = commandSettingsSnapshot(ctx, locale, commandClaimsAtEpoch(snapshot, epoch), projection, active, principal, nowCommandSettings())
	result.Scope = string(commandSettingsScopeName(scope))
	result.Revision = commandSettingsRevision(snapshot)
	result.Fingerprint, err = commandSettingsFingerprint(snapshot, projection, active)
	if err != nil {
		return CommandSettingsSnapshot{}, err
	}
	result.KeyboardOperational = p.keyboardService != nil
	if err := store.CheckCurrent(ctx, snapshot); err != nil {
		return CommandSettingsSnapshot{}, err
	}
	if err := p.epochs.Admit(ctx, epoch, func(ctx context.Context) error {
		current, err := p.sessionSvc.RevalidateLocalSession(ctx, principal)
		if err != nil || current != principal || !a.commandPrincipalMatches(p.sessionSvc, p.credMgr, principal) {
			return commandexecution.ErrStale
		}
		return nil
	}, func() error { return nil }); err != nil {
		return CommandSettingsSnapshot{}, err
	}
	current, err := a.authenticatedCommandProduct()
	if err != nil || current != p {
		if err != nil {
			return CommandSettingsSnapshot{}, err
		}
		return CommandSettingsSnapshot{}, commandexecution.ErrStale
	}
	return result, nil
}

// PrepareManualCommandLayerForScope prepara a regra manual no escopo global ou
// no workspace atual. A concessão/claim continua separada desta preparação.
func (a *App) PrepareManualCommandLayerForScope(scopeName, layerID string) (CommandSettingsMutation, error) {
	if strings.TrimSpace(layerID) == "" {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
	}
	snapshot, err := a.GetCommandSettingsForScope("pt-BR", scopeName)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	scope, err := a.commandSettingsScope(p, scopeName)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	for _, layer := range snapshot.Layers {
		if layer.ID != layerID || layer.Builtin || !layer.Enabled || (scope.WorkspaceID == nil) != (layer.WorkspaceID == "") || scope.WorkspaceID != nil && layer.WorkspaceID != *scope.WorkspaceID {
			continue
		}
		for _, rule := range snapshot.Rules {
			if rule.ID != "" && rule.LayerID == layerID && rule.WorkspaceID == layer.WorkspaceID && rule.Mode == string(commandactivation.ModeManual) && rule.Lifecycle == string(commandactivation.LifecyclePersistent) && len(rule.Condition.Clauses) == 0 && rule.Enabled && rule.ReviewStatus == "active" {
				return CommandSettingsMutation{Committed: true, Published: true, ID: rule.ID}, nil
			}
		}
		return a.MutateCommandSettings(CommandSettingsMutationRequest{Locale: "pt-BR", Scope: CommandSettingsScope(scopeName), Operation: string(commandconfig.RuleCreate), ExpectedRevision: snapshot.Revision, ExpectedFingerprint: snapshot.Fingerprint, Rule: &CommandSettingsRuleInput{LayerID: layerID, Mode: string(commandactivation.ModeManual), Lifecycle: string(commandactivation.LifecyclePersistent), Enabled: true}})
	}
	return CommandSettingsMutation{}, commandexecution.ErrDenied
}

func (a *App) MutateCommandSettings(req CommandSettingsMutationRequest) (result CommandSettingsMutation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	ctx := a.commandBridgeContext()
	if ctx == nil || req.ExpectedRevision < 1 || strings.TrimSpace(req.ExpectedFingerprint) == "" {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
	}
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	scope, err := a.commandSettingsScope(p, string(req.Scope))
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidConfiguration
	}
	snapshot, projection, active, err := a.commandSettingsAuthority(ctx, p, scope, store)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	currentFingerprint, err := commandSettingsFingerprint(snapshot, projection, active)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	if req.ExpectedRevision != commandSettingsRevision(snapshot) || req.ExpectedFingerprint != currentFingerprint {
		return CommandSettingsMutation{}, commandexecution.ErrStale
	}
	operation := commandconfig.Operation(req.Operation)
	req, pendingImage, err := prepareCommandSettingsImage(req)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	intent, err := commandSettingsIntent(req, operation, snapshot, projection, scope)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	inputs, err := a.commandDesktopMutationInputs(p, ctx)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	locale := req.Locale
	if locale == "" {
		locale = "pt-BR"
	}
	inputs.Render = func(diff commandconfig.MutationDiff) (string, error) {
		return renderCommandSettingsDiff(locale, diff)
	}
	baseAuthorize := inputs.Authorize
	inputs.Authorize = func(authCtx context.Context, principal auth.LocalSessionPrincipal, target commandconfig.Scope, operation commandconfig.Operation) error {
		if scope.WorkspaceID != nil {
			current, currentErr := a.commandMutationCurrentScope(principal)
			if currentErr != nil || !sameCommandWorkspace(current.WorkspaceID, scope.WorkspaceID) {
				return commandexecution.ErrStale
			}
		}
		return baseAuthorize(authCtx, principal, target, operation)
	}
	baseHook := inputs.OnMutationTx
	inputs.OnMutationTx = func(hookCtx context.Context, tx *gorm.DB, diff commandconfig.MutationDiff) error {
		if !commandSettingsDiffMatchesSnapshot(diff, snapshot) {
			return commandconfig.ErrStale
		}
		if err := baseHook(hookCtx, tx, diff); err != nil {
			return err
		}
		return commitCommandSettingsImage(hookCtx, tx, scope.UserID, intent, pendingImage, diff.BeforeBindings)
	}
	applier, err := a.newCommandDesktopMutationApplier(inputs)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	var applied commandMutationResult
	switch operation {
	case commandconfig.DefaultUpgrade:
		applied, err = applier.UpgradeDefaults(ctx, "", cloneCommandWorkspace(scope.WorkspaceID))
	case commandconfig.DefaultRebase:
		if req.Default == nil {
			return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
		}
		condition, conditionErr := commandSettingsConditionFacts(req.Default.Condition)
		if conditionErr != nil {
			return CommandSettingsMutation{}, conditionErr
		}
		applied, err = applier.RebaseDefault(ctx, "", cloneCommandWorkspace(scope.WorkspaceID), commandconfig.DefaultRebaseRequest{BindingID: req.Default.BindingID, Default: commandconfig.DefaultReference{ID: req.Default.Default.ID, Version: req.Default.Default.Version, Fingerprint: req.Default.Default.Fingerprint}, Condition: condition})
	case commandconfig.RuleEnable:
		if rule, ok := commandSettingsRule(snapshot, req.ID, scope); ok && rule.Mode == commandactivation.ModeEvent {
			applied, err = applier.RegrantEventRule(ctx, "", cloneCommandWorkspace(scope.WorkspaceID), req.ID)
		} else {
			applied, err = applier.ApplyScoped(ctx, "", cloneCommandWorkspace(scope.WorkspaceID), intent)
		}
	default:
		applied, err = applier.ApplyScoped(ctx, "", cloneCommandWorkspace(scope.WorkspaceID), intent)
	}
	if err != nil {
		terminal := CommandSettingsMutation{Committed: applied.Committed, Published: false, ID: commandSettingsResourceID(intent, applied.Diff)}
		if applied.Committed {
			return terminal, nil
		}
		return CommandSettingsMutation{}, err
	}
	result = CommandSettingsMutation{Committed: applied.Committed, Published: applied.Rebuilt, ID: commandSettingsResourceID(intent, applied.Diff)}
	return result, nil
}

func commandSettingsDiffMatchesSnapshot(diff commandconfig.MutationDiff, snapshot commandconfig.Snapshot) bool {
	return diff.Scope.UserID == snapshot.Scope.UserID && sameCommandWorkspace(diff.Scope.WorkspaceID, snapshot.Scope.WorkspaceID) &&
		commandSettingsSlicesEqual(diff.BeforeLayers, snapshot.Layers) &&
		commandSettingsSlicesEqual(diff.BeforeBindings, snapshot.Bindings) &&
		commandSettingsSlicesEqual(diff.BeforeActivationRules, snapshot.ActivationRules) &&
		commandSettingsSlicesEqual(diff.BeforeAutomationGrants, snapshot.AutomationGrants) &&
		commandSettingsSlicesEqual(diff.BeforeActivationClaims, snapshot.ActivationClaims)
}

func commandSettingsSlicesEqual[T any](left, right []T) bool {
	return reflect.DeepEqual(append([]T{}, left...), append([]T{}, right...))
}

func (a *App) commandSettingsScope(p *commandProductRuntime, name string) (commandconfig.Scope, error) {
	if p == nil || p.principal.UserID == "" {
		return commandconfig.Scope{}, commandexecution.ErrDenied
	}
	switch CommandSettingsScope(strings.TrimSpace(name)) {
	case CommandSettingsScopeGlobal:
		return commandconfig.Scope{UserID: p.principal.UserID}, nil
	case CommandSettingsScopeWorkspace:
		scope, err := a.commandMutationCurrentScope(p.principal)
		if err != nil || scope.WorkspaceID == nil || strings.TrimSpace(*scope.WorkspaceID) == "" {
			if err != nil {
				return commandconfig.Scope{}, err
			}
			return commandconfig.Scope{}, commandexecution.ErrDenied
		}
		return scope, nil
	default:
		return commandconfig.Scope{}, commandexecution.ErrInvalidRequest
	}
}

func commandSettingsScopeName(scope commandconfig.Scope) CommandSettingsScope {
	if scope.WorkspaceID == nil {
		return CommandSettingsScopeGlobal
	}
	return CommandSettingsScopeWorkspace
}

// commandSettingsSelectOverride escolhe a camada efetiva sem depender da
// ordem do banco. Em workspace, o delta local vence o global; o global ainda
// é projetado separadamente como herdado quando ambos existem.
func commandSettingsSelectOverride(rows []commandconfig.Binding, workspace *string) (commandconfig.Binding, bool) {
	if workspace != nil {
		for _, row := range rows {
			if row.WorkspaceID != nil && *row.WorkspaceID == *workspace {
				return row, true
			}
		}
	}
	for _, row := range rows {
		if row.WorkspaceID == nil {
			return row, true
		}
	}
	return commandconfig.Binding{}, false
}

func (a *App) commandSettingsAuthority(ctx context.Context, p *commandProductRuntime, scope commandconfig.Scope, store *commandconfig.Store) (commandconfig.Snapshot, commandconfig.CompleteProjection, []string, error) {
	if ctx == nil || p == nil || store == nil {
		return commandconfig.Snapshot{}, commandconfig.CompleteProjection{}, nil, commandconfig.ErrInvalid
	}
	snapshot, err := store.Load(ctx, scope)
	if err != nil {
		return commandconfig.Snapshot{}, commandconfig.CompleteProjection{}, nil, err
	}
	epoch, _, err := p.commandManualClaimAuthority(ctx)
	if err != nil {
		return commandconfig.Snapshot{}, commandconfig.CompleteProjection{}, nil, err
	}
	active := commandCurrentManualLayerIDs(snapshot, p.principal, nowCommandSettings(), epoch)
	projection, err := a.commandProductGlobalProjection(ctx, p.registry, active)
	if err != nil {
		return commandconfig.Snapshot{}, commandconfig.CompleteProjection{}, nil, err
	}
	if _, err := commandconfig.ProjectComplete(ctx, snapshot, projection); err != nil {
		return commandconfig.Snapshot{}, commandconfig.CompleteProjection{}, nil, err
	}
	return snapshot, projection, active, nil
}

func commandSettingsRevision(snapshot commandconfig.Snapshot) int64 {
	var revision int64
	for _, generation := range snapshot.Generations {
		if generation.Generation > revision {
			revision = generation.Generation
		}
	}
	return revision
}

func commandSettingsFingerprint(snapshot commandconfig.Snapshot, projection commandconfig.CompleteProjection, active []string) (string, error) {
	value := struct {
		Scope       commandconfig.Scope
		Layers      []commandconfig.Layer
		Bindings    []commandconfig.Binding
		Rules       []commandactivation.Rule
		Grants      any
		Claims      []commandactivation.Claim
		Generations []commandconfig.Generation
		Builtin     []commandconfig.BuiltinLayer
		Active      []string
	}{snapshot.Scope, snapshot.Layers, snapshot.Bindings, snapshot.ActivationRules, snapshot.AutomationGrants, snapshot.ActivationClaims, snapshot.Generations, projection.BuiltinLayers, active}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", commandconfig.ErrInvalid
	}
	input := append([]byte("aep0103-command-settings-etag-v1\x00"), raw...)
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:]), nil
}

func commandSettingsRule(snapshot commandconfig.Snapshot, id string, scope commandconfig.Scope) (commandactivation.Rule, bool) {
	for _, rule := range snapshot.ActivationRules {
		if rule.ID == id && sameCommandWorkspace(rule.WorkspaceID, scope.WorkspaceID) {
			return rule, true
		}
	}
	return commandactivation.Rule{}, false
}

func commandSettingsLayerActivationModes(snapshot commandconfig.Snapshot, layerID string, workspace *string) ([]string, bool) {
	seen := make(map[string]bool)
	known := true
	for _, rule := range snapshot.ActivationRules {
		if rule.LayerRef == layerID && sameCommandWorkspace(rule.WorkspaceID, workspace) && rule.Enabled && rule.ReviewStatus == "active" {
			mode := string(rule.Mode)
			if !seen[mode] {
				seen[mode] = true
			}
			if rule.Mode != commandactivation.ModeManual && rule.Mode != commandactivation.ModeToggle {
				known = false
			}
		}
	}
	modes := make([]string, 0, len(seen))
	for _, mode := range []string{"manual", "always", "context", "condition", "toggle", "event"} {
		if seen[mode] {
			modes = append(modes, mode)
		}
	}
	return modes, known
}

func commandSettingsIntent(req CommandSettingsMutationRequest, operation commandconfig.Operation, snapshot commandconfig.Snapshot, projection commandconfig.CompleteProjection, scope commandconfig.Scope) (commandconfig.MutationIntent, error) {
	intent := commandconfig.MutationIntent{Operation: operation, ID: req.ID, LayerRefKind: req.LayerRefKind}
	switch operation {
	case commandconfig.LayerCreate:
		if req.Layer == nil || req.ID != "" || strings.TrimSpace(req.Layer.Name) == "" {
			return intent, commandexecution.ErrInvalidRequest
		}
		intent.Layer = &commandconfig.Layer{Name: req.Layer.Name, Description: req.Layer.Description, Enabled: req.Layer.Enabled, ResolutionPriority: req.Layer.ResolutionPriority}
	case commandconfig.LayerUpdate:
		row, ok := commandSettingsLayer(snapshot, req.ID, scope)
		if !ok || req.Layer == nil || req.Layer.ID != req.ID || req.Layer.Enabled != row.Enabled {
			return intent, commandexecution.ErrInvalidRequest
		}
		intent.Layer = &commandconfig.Layer{ID: row.ID, UserID: row.UserID, WorkspaceID: cloneCommandWorkspace(row.WorkspaceID), Name: req.Layer.Name, Description: req.Layer.Description, Enabled: row.Enabled, Source: row.Source, ResolutionPriority: req.Layer.ResolutionPriority, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	case commandconfig.LayerDelete, commandconfig.LayerEnable, commandconfig.LayerDisable:
		if _, ok := commandSettingsLayer(snapshot, req.ID, scope); !ok {
			return intent, commandexecution.ErrInvalidRequest
		}
	case commandconfig.LayerRestore:
		if req.ID == "" || (req.LayerRefKind != "builtin" && req.LayerRefKind != "user") {
			return intent, commandexecution.ErrInvalidRequest
		}
	case commandconfig.BindingCreate:
		binding, err := commandSettingsBindingInput(req.Binding, "")
		if err != nil || req.ID != "" {
			return intent, commandexecution.ErrInvalidRequest
		}
		if !commandSettingsUserLayer(snapshot, binding.LayerRef, scope) {
			if binding.ReplacesDefaultID == nil || !commandSettingsBuiltinLayer(projection, binding.LayerRef) {
				return intent, commandexecution.ErrInvalidRequest
			}
			binding.LayerRefKind = "builtin"
		}
		if err := commandSettingsValidateDefaultOverride(binding, projection); err != nil {
			return intent, err
		}
		intent.Binding = &binding
	case commandconfig.BindingUpdate:
		row, ok := commandSettingsBinding(snapshot, req.ID, scope)
		if !ok || req.Binding == nil || req.Binding.ID != req.ID {
			return intent, commandexecution.ErrInvalidRequest
		}
		binding, err := commandSettingsBindingInput(req.Binding, req.ID)
		if err != nil || binding.LayerRef != row.LayerRef || (row.LayerRefKind == "user" && !commandSettingsUserLayer(snapshot, binding.LayerRef, scope)) || (row.LayerRefKind == "builtin" && binding.ReplacesDefaultID == nil) {
			return intent, commandexecution.ErrInvalidRequest
		}
		binding.LayerRefKind = row.LayerRefKind
		binding.UserID, binding.WorkspaceID, binding.Source = row.UserID, cloneCommandWorkspace(row.WorkspaceID), row.Source
		if err := commandSettingsValidateDefaultOverride(binding, projection); err != nil {
			return intent, err
		}
		intent.Binding = &binding
	case commandconfig.BindingDelete, commandconfig.BindingEnable, commandconfig.BindingDisable, commandconfig.BindingRestore:
		row, ok := commandSettingsBinding(snapshot, req.ID, scope)
		if !ok {
			return intent, commandexecution.ErrInvalidRequest
		}
		if operation == commandconfig.BindingRestore && row.ReplacesDefaultID == nil {
			return intent, commandexecution.ErrInvalidRequest
		}
	case commandconfig.ConfigRestore:
		if req.ID != "" {
			return intent, commandexecution.ErrInvalidRequest
		}
	case commandconfig.RuleCreate:
		rule, err := commandSettingsRuleInput(req.Rule, "")
		if err != nil || req.ID != "" || !commandSettingsUserLayer(snapshot, rule.LayerRef, scope) {
			return intent, commandexecution.ErrInvalidRequest
		}
		intent.Rule = &rule
	case commandconfig.RuleUpdate:
		old, ok := commandSettingsRule(snapshot, req.ID, scope)
		if !ok || req.Rule == nil || req.Rule.ID != req.ID {
			return intent, commandexecution.ErrInvalidRequest
		}
		if old.Mode == commandactivation.ModeEvent && req.Rule.Enabled {
			// Alterar uma regra event-driven exige revogar e obter um novo grant;
			// não desabilitamos/reautorizamos silenciosamente durante update.
			return intent, commandactivation.ErrGrantUnavailable
		}
		rule, err := commandSettingsRuleInput(req.Rule, req.ID)
		if err != nil || rule.LayerRef != old.LayerRef || rule.Enabled != old.Enabled || rule.RuleRef != old.RuleRef {
			return intent, commandexecution.ErrInvalidRequest
		}
		rule.UserID, rule.WorkspaceID, rule.Source = old.UserID, cloneCommandWorkspace(old.WorkspaceID), old.Source
		intent.Rule = &rule
	case commandconfig.RuleDelete, commandconfig.RuleEnable, commandconfig.RuleDisable, commandconfig.RuleRestore:
		row, ok := commandSettingsRule(snapshot, req.ID, scope)
		if !ok {
			return intent, commandexecution.ErrInvalidRequest
		}
		if operation == commandconfig.RuleRestore && row.ReplacesDefaultID == nil {
			return intent, commandexecution.ErrInvalidRequest
		}
	case commandconfig.DefaultUpgrade:
		if req.ID != "" || req.Default != nil {
			return intent, commandexecution.ErrInvalidRequest
		}
	case commandconfig.DefaultRebase:
		if req.Default == nil {
			return intent, commandexecution.ErrInvalidRequest
		}
	default:
		return intent, commandexecution.ErrInvalidRequest
	}
	return intent, nil
}

func commandSettingsLayer(snapshot commandconfig.Snapshot, id string, scope commandconfig.Scope) (commandconfig.Layer, bool) {
	for _, row := range snapshot.Layers {
		if row.ID == id && sameCommandWorkspace(row.WorkspaceID, scope.WorkspaceID) && row.Source == "user" {
			return row, true
		}
	}
	return commandconfig.Layer{}, false
}

func commandSettingsBinding(snapshot commandconfig.Snapshot, id string, scope commandconfig.Scope) (commandconfig.Binding, bool) {
	for _, row := range snapshot.Bindings {
		if row.ID == id && sameCommandWorkspace(row.WorkspaceID, scope.WorkspaceID) {
			return row, true
		}
	}
	return commandconfig.Binding{}, false
}

func commandSettingsUserLayer(snapshot commandconfig.Snapshot, id string, scope commandconfig.Scope) bool {
	_, ok := commandSettingsLayer(snapshot, id, scope)
	return ok
}

func commandSettingsBuiltinLayer(projection commandconfig.CompleteProjection, id string) bool {
	for _, layer := range projection.BuiltinLayers {
		if layer.ID == id {
			return true
		}
	}
	return false
}

func commandSettingsConditionFacts(condition *CommandSettingsCondition) (commandbindings.Facts, error) {
	facts := commandbindings.Facts{}
	if condition == nil {
		return facts, nil
	}
	if condition.Version != 1 {
		return nil, commandexecution.ErrInvalidRequest
	}
	for _, clause := range condition.Clauses {
		if strings.TrimSpace(clause.Field) == "" || clause.Op != "" && clause.Op != "eq" {
			return nil, commandexecution.ErrInvalidRequest
		}
		field := commandbindings.Field(clause.Field)
		if _, exists := facts[field]; exists {
			return nil, commandexecution.ErrInvalidRequest
		}
		switch field {
		case commandbindings.AppFocused:
			value, ok := clause.Value.(bool)
			if !ok {
				return nil, commandexecution.ErrInvalidRequest
			}
			facts[field] = value
		case commandbindings.Process:
			value, ok := clause.Value.(string)
			if !ok || strings.TrimSpace(value) == "" || strings.ContainsAny(value, "/\\:\r\n\t\x00") {
				return nil, commandexecution.ErrInvalidRequest
			}
			facts[field] = strings.ToLower(strings.TrimSpace(value))
		case commandbindings.SurfaceType, commandbindings.SurfaceID, commandbindings.Profile, commandbindings.Device:
			value, ok := clause.Value.(string)
			if !ok || strings.TrimSpace(value) == "" {
				return nil, commandexecution.ErrInvalidRequest
			}
			facts[field] = value
		default:
			return nil, commandexecution.ErrInvalidRequest
		}
	}
	return facts, nil
}

func commandSettingsConditionDocument(condition *CommandSettingsCondition) (string, error) {
	facts, err := commandSettingsConditionFacts(condition)
	if err != nil {
		return "", err
	}
	clauses := make([]map[string]any, 0, len(facts))
	for field, value := range facts {
		clauses = append(clauses, map[string]any{"field": string(field), "op": "eq", "value": value})
	}
	// Canonicalization sorts object keys but not clauses; sort by field to make
	// equivalent typed requests produce one persisted document.
	for i := range clauses {
		for j := i + 1; j < len(clauses); j++ {
			if clauses[j]["field"].(string) < clauses[i]["field"].(string) {
				clauses[i], clauses[j] = clauses[j], clauses[i]
			}
		}
	}
	raw, err := json.Marshal(map[string]any{"version": 1, "clauses": clauses})
	if err != nil {
		return "", commandexecution.ErrInvalidRequest
	}
	canonical, err := commandjson.Canonicalize(raw)
	if err != nil {
		return "", commandexecution.ErrInvalidRequest
	}
	return string(canonical), nil
}

func commandSettingsBindingDiagnostics(row commandconfig.Binding) []CommandSettingsDiagnostic {
	fields, err := commandSettingsConditionFields(row.Condition)
	if err != nil {
		return []CommandSettingsDiagnostic{{Code: "invalid_condition", Severity: "error", ResourceID: row.ID, Message: "A condição persistida não pôde ser validada; o binding está indisponível."}}
	}
	class := commandSettingsBindingClass(row)
	if commandSettingsDeckLayerMixedFacts(row, fields) {
		return []CommandSettingsDiagnostic{commandSettingsDeckLayerMixedDiagnostic(row)}
	}
	if row.TriggerType == "streamdeck.key" && row.CommandID == nil && row.Effect == "suppress" && len(fields) > 0 {
		return []CommandSettingsDiagnostic{{Code: "unsupported_origin_condition", Severity: "error", ResourceID: row.ID, Message: "A supressão do Deck não possui alvo confiável para publicar condições."}}
	}
	var result []CommandSettingsDiagnostic
	for _, field := range fields {
		if commandSettingsBindingSupportsField(row, class, commandbindings.Field(field)) {
			continue
		}
		code := "unsupported_origin_condition"
		if row.TriggerType == "keyboard.local" {
			code = "unsupported_local_condition"
		}
		result = append(result, CommandSettingsDiagnostic{Code: code, Severity: "error", ResourceID: row.ID, Message: "Este acionador depende de um fato sem fonte operacional nesta origem."})
		break
	}
	return result
}

// commandSettingsBindingClass usa o commandID persistido quando disponível.
// Suppression não o possui, então somente a seleção normalizada do documento
// palette pode fornecer a classe; nenhum texto arbitrário do trigger é aceito.
func commandSettingsBindingClass(row commandconfig.Binding) commandExecutionClass {
	return commandExecutionClassForID(commandSettingsBindingTargetID(row))
}

func commandSettingsBindingTargetID(row commandconfig.Binding) string {
	if row.CommandID != nil {
		return *row.CommandID
	}
	if row.TriggerType != "palette" {
		return ""
	}
	identity, err := (commandconfig.PaletteTriggerPort{}).Normalize(context.Background(), []byte(row.TriggerSpec))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(identity, "palette:")
}

func commandSettingsBindingSupportsField(row commandconfig.Binding, class commandExecutionClass, field commandbindings.Field) bool {
	if row.TriggerType == "streamdeck.key" && isContextualPagePaletteCommand(commandSettingsBindingTargetID(row)) {
		return field == commandbindings.AppFocused || field == commandbindings.SurfaceType || field == commandbindings.Profile
	}
	if row.TriggerType == "streamdeck.key" && isCommandLayerAction(commandSettingsBindingTargetID(row)) && (field == commandbindings.Process || field == commandbindings.Device) {
		return true
	}
	if row.TriggerType == "streamdeck.key" && isContextualDeckUICommand(commandSettingsBindingTargetID(row)) {
		return field == commandbindings.AppFocused || field == commandbindings.SurfaceType || field == commandbindings.SurfaceID || field == commandbindings.Profile
	}
	if row.TriggerType == "palette" && isContextualPagePaletteCommand(commandSettingsBindingTargetID(row)) {
		return field == commandbindings.AppFocused || field == commandbindings.SurfaceType || field == commandbindings.Profile
	}
	if row.TriggerType == "palette" && (isContextualPaletteWorkspaceCommand(commandSettingsBindingTargetID(row)) || isCommandLayerAction(commandSettingsBindingTargetID(row))) {
		return field == commandbindings.AppFocused || field == commandbindings.SurfaceType || field == commandbindings.SurfaceID || field == commandbindings.Profile
	}
	return commandSettingsOriginSupportsFieldForClass(row.TriggerType, class, field)
}

func commandSettingsOriginSupportsField(origin string, field commandbindings.Field) bool {
	return commandSettingsOriginSupportsFieldForClass(origin, commandExecutionUnknown, field)
}

// Native layer facts remain supported, but cannot share a visual selection.
func commandSettingsDeckLayerMixedFacts(row commandconfig.Binding, fields []string) bool {
	if row.TriggerType != "streamdeck.key" || !isCommandLayerAction(commandSettingsBindingTargetID(row)) {
		return false
	}
	visual, physical := false, false
	for _, field := range fields {
		switch commandbindings.Field(field) {
		case commandbindings.AppFocused, commandbindings.SurfaceType, commandbindings.SurfaceID:
			visual = true
		case commandbindings.Process, commandbindings.Device:
			physical = true
		}
	}
	return visual && physical
}

func commandSettingsDeckLayerMixedDiagnostic(row commandconfig.Binding) CommandSettingsDiagnostic {
	return CommandSettingsDiagnostic{Code: "unsupported_origin_condition", Severity: "error", ResourceID: row.ID, Message: "Condições visuais de camada no Deck não podem ser combinadas com processo ou dispositivo."}
}

func commandSettingsOriginSupportsFieldForClass(origin string, class commandExecutionClass, field commandbindings.Field) bool {
	switch origin {
	case "keyboard.local":
		return field == commandbindings.AppFocused || field == commandbindings.SurfaceType || field == commandbindings.SurfaceID || field == commandbindings.Profile
	case "palette":
		if class == commandExecutionLocalUI {
			return field == commandbindings.AppFocused || field == commandbindings.SurfaceType || field == commandbindings.SurfaceID || field == commandbindings.Profile
		}
		return field == commandbindings.Profile
	case "keyboard.global":
		return field == commandbindings.Profile || field == commandbindings.Process
	case "streamdeck.key":
		if class == commandExecutionLocalUI {
			return field == commandbindings.AppFocused || field == commandbindings.SurfaceType || field == commandbindings.SurfaceID || field == commandbindings.Profile
		}
		return field == commandbindings.Profile || field == commandbindings.Process || field == commandbindings.Device
	default:
		return false
	}
}

func commandSettingsLocalAdapterDiagnostics(snapshot commandconfig.Snapshot, registry *commandcatalog.Registry, configuration *commandbindings.Configuration) []CommandSettingsDiagnostic {
	var result []CommandSettingsDiagnostic
	for _, row := range snapshot.Bindings {
		if !row.Enabled {
			continue
		}
		if row.TriggerType != "keyboard.local" {
			var identity string
			var err error
			switch row.TriggerType {
			case "palette":
				identity, err = (commandconfig.PaletteTriggerPort{}).Normalize(context.Background(), []byte(row.TriggerSpec))
			case "streamdeck.key":
				identity, err = (commandconfig.StreamDeckTriggerPort{}).Normalize(context.Background(), []byte(row.TriggerSpec))
			case "keyboard.global":
				identity, err = (commandconfig.KeyboardGlobalTriggerPort{}).Normalize(context.Background(), []byte(row.TriggerSpec))
			default:
				continue
			}
			if err != nil {
				continue
			}
			if row.TriggerType == "streamdeck.key" && row.CommandID == nil && row.Effect == "suppress" {
				fields, conditionErr := commandSettingsConditionFields(row.Condition)
				if conditionErr != nil || len(fields) > 0 || len(configuration.RequiredFacts(identity)) > 0 {
					result = append(result, CommandSettingsDiagnostic{Code: "unsupported_origin_condition", Severity: "error", ResourceID: row.ID, Message: "A supressão do Deck não possui alvo confiável para publicar condições."})
					continue
				}
			}
			class := commandExecutionUnknown
			if row.CommandID != nil {
				if definition, exists := registry.Lookup(*row.CommandID); exists {
					class = commandExecutionClassForDefinition(definition)
				}
			} else if row.TriggerType == "palette" {
				identity, identityErr := (commandconfig.PaletteTriggerPort{}).Normalize(context.Background(), []byte(row.TriggerSpec))
				if identityErr == nil {
					commandID := strings.TrimPrefix(identity, "palette:")
					if definition, exists := registry.Lookup(commandID); exists {
						class = commandExecutionClassForDefinition(definition)
					}
				}
			}
			if row.TriggerType == "streamdeck.key" && class != commandExecutionLocalUI && !isContextualDeckUICommand(commandSettingsBindingTargetID(row)) && len(contextualDeckUIConditions(configuration, registry, identity)) > 0 {
				result = append(result, CommandSettingsDiagnostic{Code: "unsupported_contextual_binding", Severity: "error", ResourceID: row.ID, Message: "Este binding durável compartilha o acionador com uma seleção visual do Deck e não pode ser publicado com segurança."})
				continue
			}
			combinedFields, _ := commandSettingsConditionFields(row.Condition)
			for _, field := range configuration.RequiredFacts(identity) {
				combinedFields = append(combinedFields, string(field))
			}
			if commandSettingsDeckLayerMixedFacts(row, combinedFields) {
				result = append(result, commandSettingsDeckLayerMixedDiagnostic(row))
				continue
			}
			for _, field := range configuration.RequiredFacts(identity) {
				if !commandSettingsBindingSupportsField(row, class, field) {
					result = append(result, CommandSettingsDiagnostic{Code: "unsupported_origin_condition", Severity: "error", ResourceID: row.ID, Message: "As condições deste acionador ou de suas camadas dependem de um fato não disponível nesta origem."})
					break
				}
			}
			continue
		}
		identity, err := (commandconfig.KeyboardLocalTriggerPort{}).Normalize(context.Background(), []byte(row.TriggerSpec))
		if err != nil {
			continue // The projection validates trigger grammar before this point.
		}
		fields := localKeyboardVariableFacts(configuration.RequiredFacts(identity))
		if len(fields) == 0 {
			continue
		}
		code := ""
		if !localKeyboardSupportedVariableFacts(fields) {
			code = "unsupported_local_condition"
		} else if localKeyboardProfileRequired(configuration, identity) && localKeyboardHasNonWorkspaceSurface(configuration, identity) {
			// Perfil no teclado local só é publicável junto das superfícies
			// canônicas do workspace. Campos isolados são aceitos, mas a
			// combinação com uma página administrativa não é executável.
			code = "unsupported_contextual_binding"
		} else {
			var shortcut LocalCommandShortcut
			if json.Unmarshal([]byte(row.TriggerSpec), &shortcut) != nil || !localCommandShortcutAllowed(shortcut) {
				code = "unsupported_contextual_binding"
			} else if row.CommandID != nil {
				definition, exists := registry.Lookup(*row.CommandID)
				if !exists || !localKeyboardCommandAllowed(definition.ID) {
					code = "unsupported_contextual_binding"
				} else if commandExecutionClassForDefinition(definition) != commandExecutionLocalUI {
					for _, surface := range configuration.SurfaceValues(identity) {
						if !localKeyboardWorkspaceSurface(surface) {
							code = "unsupported_contextual_binding"
						}
					}
				}
			}
		}
		if code != "" {
			result = append(result, CommandSettingsDiagnostic{Code: code, Severity: "error", ResourceID: row.ID, Message: "Este binding depende de uma combinação de contexto e handler ainda não publicável pelo teclado local."})
		}
	}
	return result
}

func localKeyboardHasNonWorkspaceSurface(configuration *commandbindings.Configuration, identity string) bool {
	for _, surface := range configuration.SurfaceValues(identity) {
		if !localKeyboardWorkspaceSurface(surface) {
			return true
		}
	}
	return false
}

func commandSettingsConditionFields(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "{}" || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var document struct {
		Version int `json:"version"`
		Clauses []struct {
			Field string `json:"field"`
			Op    string `json:"op"`
		} `json:"clauses"`
	}
	if err := json.Unmarshal([]byte(raw), &document); err != nil || document.Version != 1 {
		return nil, commandexecution.ErrInvalidRequest
	}
	fields := make([]string, 0, len(document.Clauses))
	seen := make(map[string]bool, len(document.Clauses))
	for _, clause := range document.Clauses {
		if clause.Op != "eq" || clause.Field == "" || seen[clause.Field] {
			return nil, commandexecution.ErrInvalidRequest
		}
		seen[clause.Field] = true
		fields = append(fields, clause.Field)
	}
	return fields, nil
}

func commandSettingsRuleDiagnostics(rule commandactivation.Rule) []CommandSettingsDiagnostic {
	if rule.Mode != commandactivation.ModeCondition && rule.Mode != commandactivation.ModeContext {
		return nil
	}
	fields, err := commandSettingsConditionFields(rule.Condition)
	if err != nil {
		return []CommandSettingsDiagnostic{{Code: "invalid_activation_condition", Severity: "error", ResourceID: rule.ID, Message: "A condição da camada não pôde ser validada; a camada não será ativada por ela."}}
	}
	for _, field := range fields {
		if field != string(commandbindings.SurfaceType) && field != string(commandbindings.SurfaceID) && field != string(commandbindings.AppFocused) && field != string(commandbindings.Profile) && field != string(commandbindings.Process) && field != string(commandbindings.Device) {
			return []CommandSettingsDiagnostic{{Code: "unsupported_activation_condition", Severity: "error", ResourceID: rule.ID, Message: "Esta condição de ativação não é suportada por todas as origens; confira o diagnóstico de cada acionador."}}
		}
	}
	return nil
}

func commandSettingsPublicArguments(raw string) map[string]any {
	var value map[string]any
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "{}" {
		return map[string]any{}
	}
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
		return nil
	}
	return value
}

func commandSettingsPublicCondition(raw string) CommandSettingsCondition {
	if strings.TrimSpace(raw) == "{}" || strings.TrimSpace(raw) == "" {
		return CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{}}
	}
	var value CommandSettingsCondition
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value.Version != 1 || value.Clauses == nil {
		return CommandSettingsCondition{}
	}
	return value
}

func commandSettingsPublicConditionFacts(facts commandbindings.Facts) CommandSettingsCondition {
	clauses := make([]CommandSettingsConditionClause, 0, len(facts))
	for field, value := range facts {
		clauses = append(clauses, CommandSettingsConditionClause{Field: string(field), Op: "eq", Value: value})
	}
	for i := range clauses {
		for j := i + 1; j < len(clauses); j++ {
			if clauses[j].Field < clauses[i].Field {
				clauses[i], clauses[j] = clauses[j], clauses[i]
			}
		}
	}
	return CommandSettingsCondition{Version: 1, Clauses: clauses}
}

func commandSettingsValidateBindingCapability(binding commandconfig.Binding) error {
	if binding.TriggerType != "keyboard.local" {
		return nil
	}
	fields, err := commandSettingsConditionFields(binding.Condition)
	if err != nil {
		return commandexecution.ErrInvalidRequest
	}
	for _, field := range fields {
		if !commandSettingsOriginSupportsField(binding.TriggerType, commandbindings.Field(field)) {
			return commandexecution.ErrInvalidRequest
		}
	}
	return nil
}

func commandSettingsBindingInput(input *CommandSettingsBindingInput, id string) (commandconfig.Binding, error) {
	if input == nil || input.LayerID == "" || input.TriggerType == "" || strings.TrimSpace(input.TriggerSpec) != input.TriggerSpec {
		return commandconfig.Binding{}, commandexecution.ErrInvalidRequest
	}
	if input.Effect == "" {
		input.Effect = "execute"
	}
	if input.Effect != "execute" && input.Effect != "suppress" || input.Effect == "suppress" && input.CommandID != "" {
		return commandconfig.Binding{}, commandexecution.ErrInvalidRequest
	}
	condition, err := commandSettingsConditionDocument(input.Condition)
	if err != nil {
		return commandconfig.Binding{}, err
	}
	arguments := []byte(commandSettingsDefaultArguments)
	if input.Arguments != nil {
		arguments, err = json.Marshal(input.Arguments)
		if err != nil {
			return commandconfig.Binding{}, commandexecution.ErrInvalidRequest
		}
	}
	presentation := []byte(commandSettingsDefaultPresentation)
	if input.Presentation != nil && len(input.Presentation) > 0 {
		presentation, err = json.Marshal(input.Presentation)
		if err != nil {
			return commandconfig.Binding{}, commandexecution.ErrInvalidRequest
		}
	}
	arguments, err = commandjson.Canonicalize(arguments)
	if err != nil {
		return commandconfig.Binding{}, commandexecution.ErrInvalidRequest
	}
	presentation, err = commandjson.Canonicalize(presentation)
	if err != nil {
		return commandconfig.Binding{}, commandexecution.ErrInvalidRequest
	}
	b := commandconfig.Binding{ID: id, LayerRefKind: "user", LayerRef: input.LayerID, TriggerType: input.TriggerType, TriggerSpec: input.TriggerSpec, Arguments: string(arguments), Condition: condition, Effect: input.Effect, Enabled: input.Enabled, Source: "user", ResolutionPriority: input.ResolutionPriority, Presentation: string(presentation), ReviewStatus: "active"}
	if input.CommandID != "" {
		commandID := input.CommandID
		b.CommandID = &commandID
	}
	if input.ReplacesDefaultID != "" || input.ReplacesDefaultVersion != "" || input.ReplacesDefaultFingerprint != "" {
		idRef, version, fingerprint := input.ReplacesDefaultID, input.ReplacesDefaultVersion, input.ReplacesDefaultFingerprint
		b.ReplacesDefaultID, b.ReplacesDefaultVersion, b.ReplacesDefaultFingerprint = &idRef, &version, &fingerprint
	}
	if err := commandSettingsValidateBindingCapability(b); err != nil {
		return commandconfig.Binding{}, err
	}
	return b, nil
}

func commandSettingsValidateDefaultOverride(binding commandconfig.Binding, projection commandconfig.CompleteProjection) error {
	if binding.ReplacesDefaultID == nil {
		return nil
	}
	if binding.ReplacesDefaultVersion == nil || binding.ReplacesDefaultFingerprint == nil || *binding.ReplacesDefaultID == "" || *binding.ReplacesDefaultVersion == "" || *binding.ReplacesDefaultFingerprint == "" {
		return commandexecution.ErrInvalidRequest
	}
	for _, layer := range projection.BuiltinLayers {
		for _, item := range layer.Defaults {
			if item.Candidate.ID == *binding.ReplacesDefaultID && item.Version == *binding.ReplacesDefaultVersion && item.Fingerprint == *binding.ReplacesDefaultFingerprint {
				return nil
			}
		}
	}
	return commandexecution.ErrStale
}

func commandSettingsRuleInput(input *CommandSettingsRuleInput, id string) (commandactivation.Rule, error) {
	if input == nil || input.LayerID == "" || input.Mode == "" || input.Lifecycle == "" {
		return commandactivation.Rule{}, commandexecution.ErrInvalidRequest
	}
	condition, err := commandSettingsConditionDocument(input.Condition)
	if err != nil {
		return commandactivation.Rule{}, err
	}
	mode := commandactivation.Mode(input.Mode)
	switch mode {
	case commandactivation.ModeManual, commandactivation.ModeAlways, commandactivation.ModeContext, commandactivation.ModeCondition, commandactivation.ModeToggle, commandactivation.ModeEvent:
	default:
		return commandactivation.Rule{}, commandexecution.ErrInvalidRequest
	}
	if mode == commandactivation.ModeManual || mode == commandactivation.ModeAlways || mode == commandactivation.ModeToggle {
		// These modes do not evaluate a condition. Never save a restriction
		// that would be ignored, and use the manual service's canonical form.
		if input.Condition != nil && len(input.Condition.Clauses) != 0 {
			return commandactivation.Rule{}, commandexecution.ErrInvalidRequest
		}
		condition = "{}"
	}
	rule := commandactivation.Rule{ID: id, LayerRefKind: commandactivation.UserRef, LayerRef: input.LayerID, RuleRefKind: commandactivation.UserRef, RuleRef: id, Mode: mode, Condition: condition, Lifecycle: commandactivation.Lifecycle(input.Lifecycle), Enabled: input.Enabled, Source: "user", ReviewStatus: "active"}
	if mode == commandactivation.ModeEvent {
		eventName, producers := "command-context.job-run-state.v1", `["jobs.runtime"]`
		if input.Enabled || input.EventName != "" && input.EventName != eventName {
			return commandactivation.Rule{}, commandexecution.ErrInvalidRequest
		}
		if input.AllowedInternalProducerTypes != "" {
			var supplied []string
			if json.Unmarshal([]byte(input.AllowedInternalProducerTypes), &supplied) != nil || len(supplied) != 1 || supplied[0] != "jobs.runtime" {
				return commandactivation.Rule{}, commandexecution.ErrInvalidRequest
			}
		}
		rule.EventName, rule.AllowedInternalProducerTypes = &eventName, &producers
	} else if input.EventName != "" || input.AllowedInternalProducerTypes != "" {
		return commandactivation.Rule{}, commandexecution.ErrInvalidRequest
	}
	return rule, nil
}
