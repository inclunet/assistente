package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjson"
	"assistente/internal/database"
	"gorm.io/gorm"
)

// CommandSettingsSnapshot é a projeção pública, sem identidade de sessão,
// owner, workspace, SQL ou documentos de argumentos/condições.
type CommandSettingsSnapshot struct {
	Scope               string                      `json:"scope,omitempty"`
	Revision            int64                       `json:"revision,omitempty"`
	Fingerprint         string                      `json:"fingerprint,omitempty"`
	Layers              []CommandSettingsLayer      `json:"layers"`
	Bindings            []CommandSettingsBinding    `json:"bindings"`
	Rules               []CommandSettingsRule       `json:"rules,omitempty"`
	Diagnostics         []CommandSettingsDiagnostic `json:"diagnostics,omitempty"`
	Adjustments         []CommandSettingsAdjustment `json:"adjustments,omitempty"`
	Commands            []CommandSettingsCommand    `json:"commands"`
	KeyboardOperational bool                        `json:"keyboardOperational"`
}

type CommandSettingsDiagnostic struct {
	Code        string   `json:"code"`
	Severity    string   `json:"severity"`
	ResourceID  string   `json:"resourceId,omitempty"`
	ResourceIDs []string `json:"resourceIds,omitempty"`
	Trigger     string   `json:"trigger,omitempty"`
	Message     string   `json:"message"`
}

type CommandSettingsAdjustment struct {
	DeltaID               string `json:"deltaId"`
	Status                string `json:"status"`
	CurrentDefaultVersion string `json:"currentDefaultVersion"`
	Reason                string `json:"reason"`
}

type CommandSettingsLayer struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Builtin            bool     `json:"builtin"`
	Enabled            bool     `json:"enabled"`
	Active             bool     `json:"active"`
	ManualReady        bool     `json:"manualReady"`
	ManualActive       bool     `json:"manualActive"`
	ResolutionPriority int      `json:"resolutionPriority"`
	WorkspaceID        string   `json:"workspaceId,omitempty"`
	ActivationModes    []string `json:"activationModes,omitempty"`
	ActiveKnown        bool     `json:"activeKnown"`
}

type CommandSettingsBinding struct {
	ID          string `json:"id"`
	LayerID     string `json:"layerId"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	CommandID   string `json:"commandId"`
	TriggerType string `json:"triggerType"`
	TriggerSpec string `json:"triggerSpec"`
	Enabled     bool   `json:"enabled"`
	// PersistedEnabled is the stored value. Enabled remains the effective
	// presentation for older clients; suppress bindings must use Suppressed and
	// PersistedEnabled instead of interpreting Enabled as the inverse.
	PersistedEnabled           bool                     `json:"persistedEnabled"`
	Suppressed                 bool                     `json:"suppressed"`
	Customized                 bool                     `json:"customized"`
	ReadOnly                   bool                     `json:"readOnly"`
	Inherited                  bool                     `json:"inherited,omitempty"`
	DefaultID                  string                   `json:"defaultId"`
	ReviewStatus               string                   `json:"reviewStatus"`
	Arguments                  map[string]any           `json:"arguments,omitempty"`
	Presentation               map[string]any           `json:"presentation,omitempty"`
	Condition                  CommandSettingsCondition `json:"condition"`
	Effect                     string                   `json:"effect,omitempty"`
	ResolutionPriority         int                      `json:"resolutionPriority"`
	ReplacesDefaultVersion     string                   `json:"replacesDefaultVersion,omitempty"`
	ReplacesDefaultFingerprint string                   `json:"replacesDefaultFingerprint,omitempty"`
	CurrentDefaultVersion      string                   `json:"currentDefaultVersion,omitempty"`
	CurrentDefaultFingerprint  string                   `json:"currentDefaultFingerprint,omitempty"`
}

type CommandSettingsRule struct {
	ManualActive                 bool                     `json:"manualActive"`
	ManualExpiresAt              int64                    `json:"manualExpiresAt,omitempty"`
	ID                           string                   `json:"id"`
	LayerID                      string                   `json:"layerId"`
	WorkspaceID                  string                   `json:"workspaceId,omitempty"`
	Mode                         string                   `json:"mode"`
	Condition                    CommandSettingsCondition `json:"condition"`
	Lifecycle                    string                   `json:"lifecycle"`
	EventName                    string                   `json:"eventName,omitempty"`
	AllowedInternalProducerTypes string                   `json:"allowedInternalProducerTypes,omitempty"`
	Enabled                      bool                     `json:"enabled"`
	Source                       string                   `json:"source"`
	ReviewStatus                 string                   `json:"reviewStatus"`
	GrantGeneration              int64                    `json:"grantGeneration,omitempty"`
	GrantFingerprint             string                   `json:"grantFingerprint,omitempty"`
}

type CommandSettingsCommand struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	AllowedSources []string `json:"allowedSources"`
}

type CommandLayerEdit struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type CommandBindingEdit struct {
	ID          string `json:"id"`
	LayerID     string `json:"layerId"`
	CommandID   string `json:"commandId"`
	TriggerType string `json:"triggerType"`
	TriggerSpec string `json:"triggerSpec"`
	Enabled     bool   `json:"enabled"`
}

type CommandSettingsMutation struct {
	Committed bool   `json:"committed"`
	Published bool   `json:"published"`
	ID        string `json:"id"`
}

const commandSettingsDefaultCondition = `{"version":1,"clauses":[]}`
const commandSettingsDefaultArguments = `{}`
const commandSettingsDefaultPresentation = `{"version":1}`

// GetCommandSettings retorna somente a configuração global persistida e os
// defaults reais do catálogo. A atividade de user layers vem exclusivamente
// das claims persistidas e autenticadas; não é inferida do pedido.
func (a *App) GetCommandSettings(locale string) (result CommandSettingsSnapshot, err error) {
	return a.GetCommandSettingsForScope(locale, string(CommandSettingsScopeGlobal))
}

// SaveCommandLayer persiste apenas camadas globais do usuário autenticado.
func (a *App) SaveCommandLayer(req CommandLayerEdit) (result CommandSettingsMutation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	if strings.TrimSpace(req.ID) == "" {
		return a.applyCommandSettingsMutation(commandconfig.MutationIntent{Operation: commandconfig.LayerCreate, Layer: &commandconfig.Layer{Name: req.Name, Description: req.Description, Enabled: req.Enabled, Source: "user"}})
	}
	row, err := a.commandSettingsLayer(req.ID)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	if row.Enabled != req.Enabled {
		if row.Name != req.Name || row.Description != req.Description {
			return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
		}
		operation := commandconfig.LayerDisable
		if req.Enabled {
			operation = commandconfig.LayerEnable
		}
		return a.applyCommandSettingsMutation(commandconfig.MutationIntent{Operation: operation, ID: req.ID})
	}
	row.Name, row.Description = req.Name, req.Description
	return a.applyCommandSettingsMutation(commandconfig.MutationIntent{Operation: commandconfig.LayerUpdate, ID: req.ID, Layer: &row})
}

// SaveCommandBinding cria/edita somente bindings de user layer. Overrides de
// defaults e linhas com condições/argumentos são deliberadamente read-only.
func (a *App) SaveCommandBinding(req CommandBindingEdit) (result CommandSettingsMutation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	if err := validateCommandBindingEdit(req); err != nil {
		return CommandSettingsMutation{}, err
	}
	if req.ID != "" {
		row, err := a.commandSettingsBinding(req.ID)
		if err != nil {
			return CommandSettingsMutation{}, err
		}
		if commandSettingsBindingReadOnly(row) || row.LayerRefKind != "user" {
			return CommandSettingsMutation{}, commandexecution.ErrDenied
		}
	}
	commandID := req.CommandID
	binding := &commandconfig.Binding{ID: req.ID, LayerRefKind: "user", LayerRef: req.LayerID, TriggerType: req.TriggerType, TriggerSpec: req.TriggerSpec, CommandID: &commandID, Arguments: commandSettingsDefaultArguments, Condition: commandSettingsDefaultCondition, Effect: "execute", Enabled: req.Enabled, Source: "user", ReviewStatus: "active", Presentation: commandSettingsDefaultPresentation}
	operation := commandconfig.BindingCreate
	if req.ID != "" {
		row, err := a.commandSettingsBinding(req.ID)
		if err != nil {
			return CommandSettingsMutation{}, err
		}
		if row.LayerRefKind != "user" {
			return CommandSettingsMutation{}, commandexecution.ErrDenied
		}
		row.LayerRef, row.TriggerType, row.TriggerSpec, row.Enabled, row.CommandID = req.LayerID, req.TriggerType, req.TriggerSpec, req.Enabled, &commandID
		binding = &row
		operation = commandconfig.BindingUpdate
	}
	return a.applyCommandSettingsMutation(commandconfig.MutationIntent{Operation: operation, ID: req.ID, Binding: binding})
}

// DeleteCommandBinding não apaga overrides nem registros que carreguem dados
// contextuais potencialmente sensíveis.
func (a *App) DeleteCommandBinding(id string) (result CommandSettingsMutation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	row, err := a.commandSettingsBinding(id)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	if commandSettingsBindingReadOnly(row) || row.LayerRefKind != "user" {
		return CommandSettingsMutation{}, commandexecution.ErrDenied
	}
	return a.applyCommandSettingsMutation(commandconfig.MutationIntent{Operation: commandconfig.BindingDelete, ID: id})
}

// SetDefaultCommandSuppressed cria ou restaura o override de supressão do
// default. A referência completa (id, versão e fingerprint) é obtida do
// catálogo, nunca do cliente.
func (a *App) SetDefaultCommandSuppressed(defaultID string, suppressed bool) (result CommandSettingsMutation, err error) {
	defer func() { err = safeCommandSettingsError(err) }()
	ctx := a.commandBridgeContext()
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	projection, err := a.commandProductGlobalProjection(ctx, p.registry, nil)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	var found *commandconfig.BuiltinLayer
	var itemID, version, fingerprint, trigger string
	for i := range projection.BuiltinLayers {
		for _, item := range projection.BuiltinLayers[i].Defaults {
			if item.Candidate.ID == defaultID {
				layer := projection.BuiltinLayers[i]
				found = &layer
				itemID, version, fingerprint, trigger = item.Candidate.ID, item.Version, item.Fingerprint, item.Candidate.Trigger
			}
		}
	}
	if found == nil {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
	}
	if !suppressed {
		row, err := a.commandSettingsSuppression(defaultID)
		if err != nil {
			return CommandSettingsMutation{}, err
		}
		if row == nil {
			return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
		}
		return a.applyCommandSettingsMutation(commandconfig.MutationIntent{Operation: commandconfig.BindingRestore, ID: row.ID})
	}
	if existing, err := a.commandSettingsSuppression(defaultID); err != nil {
		return CommandSettingsMutation{}, err
	} else if existing != nil {
		return CommandSettingsMutation{}, commandexecution.ErrInvalidRequest
	}
	spec, err := commandSettingsTriggerSpec(ctx, trigger)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	return a.applyCommandSettingsMutation(commandconfig.MutationIntent{Operation: commandconfig.BindingCreate, Binding: &commandconfig.Binding{LayerRefKind: "builtin", LayerRef: found.ID, TriggerType: commandSettingsTriggerType(trigger), TriggerSpec: spec, Arguments: commandSettingsDefaultArguments, Condition: commandSettingsDefaultCondition, Effect: "suppress", Enabled: true, Source: "user", ReplacesDefaultID: &itemID, ReplacesDefaultVersion: &version, ReplacesDefaultFingerprint: &fingerprint, ReviewStatus: "active", Presentation: commandSettingsDefaultPresentation}})
}

func (a *App) applyCommandSettingsMutation(intent commandconfig.MutationIntent) (CommandSettingsMutation, error) {
	ctx := a.commandBridgeContext()
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	inputs, err := a.commandDesktopMutationInputs(p, ctx)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	baseHook := inputs.OnMutationTx
	inputs.OnMutationTx = func(ctx context.Context, tx *gorm.DB, diff commandconfig.MutationDiff) error {
		if err := validateCommandSettingsMutationDiff(intent, diff); err != nil {
			return err
		}
		return baseHook(ctx, tx, diff)
	}
	applier, err := a.newCommandDesktopMutationApplier(inputs)
	if err != nil {
		return CommandSettingsMutation{}, err
	}
	result, applyErr := applier.Apply(ctx, "", intent)
	out := CommandSettingsMutation{Committed: result.Committed, Published: result.Rebuilt, ID: commandSettingsResourceID(intent, result.Diff)}
	if result.Committed {
		// Commit confirmado não é repetível, mesmo se a publicação falhar.
		return out, nil
	}
	if applyErr != nil {
		return CommandSettingsMutation{}, applyErr
	}
	return out, nil
}

func validateCommandSettingsMutationDiff(intent commandconfig.MutationIntent, diff commandconfig.MutationDiff) error {
	if intent.Operation == commandconfig.RuleCreate && intent.Rule != nil {
		for _, rule := range diff.BeforeActivationRules {
			if commandSettingsManualRuleMatches(rule, diff.Scope.UserID, intent.Rule.LayerRef) {
				return errCommandSettingsManualRuleAlreadyPrepared
			}
		}
	}
	if intent.Operation == commandconfig.BindingUpdate || intent.Operation == commandconfig.BindingDelete {
		for _, row := range diff.BeforeBindings {
			if row.ID == intent.ID && commandSettingsBindingReadOnly(row) {
				return commandexecution.ErrDenied
			}
		}
	}
	if intent.Operation == commandconfig.LayerUpdate {
		for _, before := range diff.BeforeLayers {
			for _, after := range diff.AfterLayers {
				if before.ID == after.ID && (before.Enabled != after.Enabled || before.Source != after.Source || before.ResolutionPriority != after.ResolutionPriority || before.UserID != after.UserID || !sameCommandWorkspace(before.WorkspaceID, after.WorkspaceID)) {
					return commandexecution.ErrStale
				}
			}
		}
	}
	if intent.Operation == commandconfig.BindingUpdate {
		for _, before := range diff.BeforeBindings {
			for _, after := range diff.AfterBindings {
				if before.ID == after.ID && (before.Source != after.Source || before.ResolutionPriority != after.ResolutionPriority || before.ReviewStatus != after.ReviewStatus || before.Arguments != after.Arguments || before.Condition != after.Condition || before.Presentation != after.Presentation || before.Effect != after.Effect) {
					return commandexecution.ErrStale
				}
			}
		}
	}
	return nil
}

func commandSettingsResourceID(intent commandconfig.MutationIntent, diff commandconfig.MutationDiff) string {
	if intent.ID != "" {
		return intent.ID
	}
	if intent.Operation == commandconfig.LayerCreate {
		for _, after := range diff.AfterLayers {
			found := false
			for _, before := range diff.BeforeLayers {
				if before.ID == after.ID {
					found = true
					break
				}
			}
			if !found {
				return after.ID
			}
		}
	}
	if intent.Operation == commandconfig.BindingCreate {
		for _, after := range diff.AfterBindings {
			found := false
			for _, before := range diff.BeforeBindings {
				if before.ID == after.ID {
					found = true
					break
				}
			}
			if !found {
				return after.ID
			}
		}
	}
	if intent.Operation == commandconfig.RuleCreate {
		for _, after := range diff.AfterActivationRules {
			found := false
			for _, before := range diff.BeforeActivationRules {
				if before.ID == after.ID {
					found = true
					break
				}
			}
			if !found {
				return after.ID
			}
		}
	}
	return diff.MutationID
}

func validateCommandBindingEdit(req CommandBindingEdit) error {
	if req.LayerID == "" || req.CommandID == "" || (req.TriggerType != string(commandcatalog.KeyboardLocal) && req.TriggerType != string(commandcatalog.StreamDeck) && req.TriggerType != string(commandcatalog.Palette)) || strings.TrimSpace(req.TriggerSpec) != req.TriggerSpec || !json.Valid([]byte(req.TriggerSpec)) {
		return commandexecution.ErrInvalidRequest
	}
	return nil
}

func (a *App) commandSettingsBinding(id string) (commandconfig.Binding, error) {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return commandconfig.Binding{}, err
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return commandconfig.Binding{}, commandexecution.ErrInvalidConfiguration
	}
	snapshot, err := store.Load(a.commandBridgeContext(), commandconfig.Scope{UserID: p.principal.UserID})
	if err != nil {
		return commandconfig.Binding{}, err
	}
	for _, row := range snapshot.Bindings {
		if row.ID == id {
			return row, nil
		}
	}
	return commandconfig.Binding{}, commandexecution.ErrInvalidRequest
}

func (a *App) commandSettingsLayer(id string) (commandconfig.Layer, error) {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return commandconfig.Layer{}, err
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return commandconfig.Layer{}, commandexecution.ErrInvalidConfiguration
	}
	snapshot, err := store.Load(a.commandBridgeContext(), commandconfig.Scope{UserID: p.principal.UserID})
	if err != nil {
		return commandconfig.Layer{}, err
	}
	for _, row := range snapshot.Layers {
		if row.ID == id {
			return row, nil
		}
	}
	return commandconfig.Layer{}, commandexecution.ErrInvalidRequest
}

func (a *App) commandSettingsSuppression(defaultID string) (*commandconfig.Binding, error) {
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		return nil, err
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	snapshot, err := store.Load(a.commandBridgeContext(), commandconfig.Scope{UserID: p.principal.UserID})
	if err != nil {
		return nil, err
	}
	var found *commandconfig.Binding
	for i := range snapshot.Bindings {
		row := snapshot.Bindings[i]
		if row.ReplacesDefaultID != nil && *row.ReplacesDefaultID == defaultID && row.LayerRefKind == "builtin" {
			if found != nil {
				return nil, commandexecution.ErrStale
			}
			found = &row
		}
	}
	return found, nil
}

func commandSettingsBindingReadOnly(row commandconfig.Binding) bool {
	return row.ReplacesDefaultID != nil || row.ReviewStatus != "active" || row.Effect != "execute" ||
		strings.TrimSpace(row.Condition) != commandSettingsDefaultCondition || strings.TrimSpace(row.Arguments) != commandSettingsDefaultArguments ||
		strings.TrimSpace(row.Presentation) != commandSettingsDefaultPresentation
}

func commandSettingsTriggerType(identity string) string {
	if strings.HasPrefix(identity, "palette:") {
		return string(commandcatalog.Palette)
	}
	if strings.HasPrefix(identity, "keyboard.global:") {
		return string(commandcatalog.KeyboardGlobal)
	}
	return string(commandcatalog.KeyboardLocal)
}

func commandSettingsTriggerSpec(ctx context.Context, identity string) (string, error) {
	if strings.HasPrefix(identity, "palette:") {
		raw, err := commandjson.Marshal(map[string]any{"version": 1, "selection": strings.TrimPrefix(identity, "palette:")})
		return string(raw), err
	}
	if strings.HasPrefix(identity, "keyboard.global:") {
		if err := (commandconfig.KeyboardGlobalTriggerPort{}).ValidateIdentity(ctx, identity); err != nil {
			return "", err
		}
		// Apenas serialização da gramática compartilhada de acorde, após validar
		// a origem global. Isso não registra nem converte a autoridade da hotkey.
		identity = "keyboard.local:" + strings.TrimPrefix(identity, "keyboard.global:")
	}
	if !strings.HasPrefix(identity, "keyboard.local:") {
		return "", commandexecution.ErrInvalidRequest
	}
	raw, err := commandconfig.EncodeKeyboardLocalIdentity(identity)
	if err != nil {
		return "", err
	}
	canonical, err := commandjson.Canonicalize(raw)
	return string(canonical), err
}

func commandSettingsSnapshot(ctx context.Context, locale string, snapshot commandconfig.Snapshot, projection commandconfig.CompleteProjection, active []string, principal auth.LocalSessionPrincipal, now time.Time) CommandSettingsSnapshot {
	activeSet := make(map[string]bool, len(active))
	for _, id := range active {
		activeSet[id] = true
	}
	result := CommandSettingsSnapshot{KeyboardOperational: false}
	if configuration, err := commandconfig.ProjectComplete(ctx, snapshot, projection); err == nil {
		result.Diagnostics = append(result.Diagnostics, commandSettingsLocalAdapterDiagnostics(snapshot, projection.Registry, configuration)...)
		for _, adjustment := range configuration.Adjustments() {
			result.Adjustments = append(result.Adjustments, CommandSettingsAdjustment{DeltaID: adjustment.DeltaID, Status: string(adjustment.ReviewStatus), CurrentDefaultVersion: adjustment.DefaultVersion, Reason: adjustment.Reason})
		}
		if witnesses, err := configuration.CheckConflicts(ctx); err != nil {
			result.Diagnostics = append(result.Diagnostics, CommandSettingsDiagnostic{Code: "conflict_diagnostic_unavailable", Severity: "error", Message: "Não foi possível concluir a análise determinística de conflitos; a configuração não deve ser anunciada como livre de conflitos."})
		} else {
			for _, witness := range witnesses {
				resourceIDs := append([]string(nil), witness.BindingIDs...)
				resourceID := ""
				if len(resourceIDs) > 0 {
					resourceID = resourceIDs[0]
				}
				result.Diagnostics = append(result.Diagnostics, CommandSettingsDiagnostic{Code: "binding_conflict", Severity: "error", ResourceID: resourceID, ResourceIDs: resourceIDs, Trigger: witness.Trigger, Message: "Este acionador possui bindings concorrentes na mesma partição de contexto."})
			}
		}
	}
	overrides := make(map[string][]commandconfig.Binding)
	adjustments := make(map[string]CommandSettingsAdjustment, len(result.Adjustments))
	for _, adjustment := range result.Adjustments {
		adjustments[adjustment.DeltaID] = adjustment
	}
	currentDefaults := make(map[string]struct{ version, fingerprint string })
	representedDefaults := make(map[string]bool)
	for _, row := range snapshot.Bindings {
		if row.ReplacesDefaultID != nil {
			overrides[*row.ReplacesDefaultID] = append(overrides[*row.ReplacesDefaultID], row)
		}
	}
	for _, layer := range projection.BuiltinLayers {
		for _, item := range layer.Defaults {
			representedDefaults[item.Candidate.ID] = true
			currentDefaults[item.Candidate.ID] = struct{ version, fingerprint string }{item.Version, item.Fingerprint}
		}
	}
	for _, layer := range projection.BuiltinLayers {
		name, description := commandSettingsBuiltinText(locale, layer.ID)
		result.Layers = append(result.Layers, CommandSettingsLayer{ID: layer.ID, Name: name, Description: description, Builtin: true, Enabled: true, Active: layer.Active, ActiveKnown: true, ResolutionPriority: 0})
		for _, item := range layer.Defaults {
			spec, _ := commandSettingsTriggerSpec(ctx, item.Candidate.Trigger)
			binding := CommandSettingsBinding{ID: item.Candidate.ID, LayerID: layer.ID, CommandID: item.Candidate.CommandID, TriggerType: commandSettingsTriggerType(item.Candidate.Trigger), TriggerSpec: spec, Enabled: item.Candidate.Enabled, PersistedEnabled: item.Candidate.Enabled, ReadOnly: true, DefaultID: item.Candidate.ID, ReviewStatus: "active", Arguments: commandSettingsPublicArguments(item.Candidate.ArgumentsKey), Presentation: commandSettingsPublicArguments(commandSettingsDefaultPresentation), Condition: commandSettingsPublicConditionFacts(item.Candidate.Condition), Effect: "execute", ResolutionPriority: item.Candidate.BindingPriority}
			binding.CurrentDefaultVersion, binding.CurrentDefaultFingerprint = item.Version, item.Fingerprint
			if override, ok := commandSettingsSelectOverride(overrides[item.Candidate.ID], snapshot.Scope.WorkspaceID); ok {
				result.Diagnostics = append(result.Diagnostics, commandSettingsBindingDiagnostics(override)...)
				binding.Customized = true
				binding.ID = override.ID
				binding.LayerID = override.LayerRef
				if override.WorkspaceID != nil {
					binding.WorkspaceID = *override.WorkspaceID
				} else {
					binding.WorkspaceID = ""
				}
				binding.Arguments = commandSettingsPublicArguments(override.Arguments)
				binding.Presentation = commandSettingsPublicArguments(override.Presentation)
				binding.Condition = commandSettingsPublicCondition(override.Condition)
				binding.Effect = override.Effect
				binding.PersistedEnabled = override.Enabled
				binding.Suppressed = override.Effect == "suppress"
				binding.ResolutionPriority = override.ResolutionPriority
				binding.ReplacesDefaultVersion = commandSettingsOptionalString(override.ReplacesDefaultVersion)
				binding.ReplacesDefaultFingerprint = commandSettingsOptionalString(override.ReplacesDefaultFingerprint)
				binding.ReadOnly = snapshot.Scope.WorkspaceID != nil && override.WorkspaceID == nil
				binding.ReviewStatus = override.ReviewStatus
				if adjustment, adjusted := adjustments[override.ID]; adjusted {
					binding.ReviewStatus = adjustment.Status
				}
				if override.ReplacesDefaultFingerprint == nil || *override.ReplacesDefaultFingerprint != item.Fingerprint {
					binding.ReviewStatus = "needs_review"
				}
				if override.Effect == "suppress" {
					binding.Enabled = !override.Enabled
					binding.CommandID = ""
				} else {
					binding.Enabled = override.Enabled
					if override.CommandID != nil {
						binding.CommandID = *override.CommandID
					}
					binding.TriggerType, binding.TriggerSpec = override.TriggerType, override.TriggerSpec
				}
			}
			result.Bindings = append(result.Bindings, binding)
		}
	}
	for defaultID, rows := range overrides {
		if representedDefaults[defaultID] {
			if snapshot.Scope.WorkspaceID != nil {
				if selected, ok := commandSettingsSelectOverride(rows, snapshot.Scope.WorkspaceID); ok && selected.WorkspaceID != nil {
					for _, row := range rows {
						if row.WorkspaceID != nil || row.ID == selected.ID {
							continue
						}
						commandID := ""
						if row.CommandID != nil {
							commandID = *row.CommandID
						}
						result.Bindings = append(result.Bindings, CommandSettingsBinding{ID: row.ID, LayerID: row.LayerRef, CommandID: commandID, TriggerType: row.TriggerType, TriggerSpec: row.TriggerSpec, Enabled: row.Enabled, PersistedEnabled: row.Enabled, Suppressed: row.Effect == "suppress", Customized: true, ReadOnly: true, Inherited: true, DefaultID: defaultID, ReviewStatus: row.ReviewStatus, Arguments: commandSettingsPublicArguments(row.Arguments), Presentation: commandSettingsPublicArguments(row.Presentation), Condition: commandSettingsPublicCondition(row.Condition), Effect: row.Effect, ResolutionPriority: row.ResolutionPriority, ReplacesDefaultVersion: commandSettingsOptionalString(row.ReplacesDefaultVersion), ReplacesDefaultFingerprint: commandSettingsOptionalString(row.ReplacesDefaultFingerprint)})
					}
				}
			}
			continue
		}
		for _, row := range rows {
			commandID := ""
			if row.CommandID != nil {
				commandID = *row.CommandID
			}
			workspaceID := ""
			if row.WorkspaceID != nil {
				workspaceID = *row.WorkspaceID
			}
			inherited := snapshot.Scope.WorkspaceID != nil && row.WorkspaceID == nil
			result.Bindings = append(result.Bindings, CommandSettingsBinding{ID: row.ID, LayerID: row.LayerRef, WorkspaceID: workspaceID, CommandID: commandID, TriggerType: row.TriggerType, TriggerSpec: row.TriggerSpec, Enabled: row.Enabled, PersistedEnabled: row.Enabled, Suppressed: row.Effect == "suppress", Customized: true, ReadOnly: inherited, Inherited: inherited, DefaultID: defaultID, ReviewStatus: "needs_review", Arguments: commandSettingsPublicArguments(row.Arguments), Presentation: commandSettingsPublicArguments(row.Presentation), Condition: commandSettingsPublicCondition(row.Condition), Effect: row.Effect, ResolutionPriority: row.ResolutionPriority, ReplacesDefaultVersion: commandSettingsOptionalString(row.ReplacesDefaultVersion), ReplacesDefaultFingerprint: commandSettingsOptionalString(row.ReplacesDefaultFingerprint)})
			result.Diagnostics = append(result.Diagnostics, CommandSettingsDiagnostic{Code: "default_needs_review", Severity: "error", ResourceID: row.ID, Message: "O default referenciado não está disponível na versão atual; restaure ou faça rebase confirmado."})
		}
	}
	for _, layer := range snapshot.Layers {
		manualReady, manualActive := commandSettingsManualState(snapshot, layer.ID, layer.WorkspaceID, principal, now)
		workspaceID := ""
		if layer.WorkspaceID != nil {
			workspaceID = *layer.WorkspaceID
		}
		modes, activeKnown := commandSettingsLayerActivationModes(snapshot, layer.ID, layer.WorkspaceID)
		result.Layers = append(result.Layers, CommandSettingsLayer{ID: layer.ID, Name: layer.Name, Description: layer.Description, Enabled: layer.Enabled, Active: activeSet[layer.ID], ManualReady: manualReady, ManualActive: manualActive, ResolutionPriority: layer.ResolutionPriority, WorkspaceID: workspaceID, ActivationModes: modes, ActiveKnown: activeKnown})
	}
	for _, rule := range snapshot.ActivationRules {
		result.Diagnostics = append(result.Diagnostics, commandSettingsRuleDiagnostics(rule)...)
		workspaceID := ""
		if rule.WorkspaceID != nil {
			workspaceID = *rule.WorkspaceID
		}
		grantGeneration := int64(0)
		grantFingerprint := ""
		if rule.AutomationGrantGeneration != nil {
			grantGeneration = *rule.AutomationGrantGeneration
		}
		if rule.AutomationGrantFingerprint != nil {
			grantFingerprint = *rule.AutomationGrantFingerprint
		}
		manualActive := commandSettingsManualClaimPresentScoped(snapshot, rule.LayerRef, rule.ID, principal, rule.WorkspaceID, now)
		var manualExpiresAt int64
		if manualActive {
			stack, _ := commandactivation.ManualStackKey(commandSettingsManualOrigin(principal))
			for _, claim := range snapshot.ActivationClaims {
				if claim.RuleRef == rule.ID && claim.SourceType == "manual" && claim.State == commandactivation.StateActive && claim.AuthContextID == principal.SessionID && claim.ManualStackKey != nil && *claim.ManualStackKey == stack && claim.ExpiresAt != nil && claim.ExpiresAt.After(now) && sameCommandWorkspace(claim.WorkspaceID, rule.WorkspaceID) {
					if manualExpiresAt == 0 || claim.ExpiresAt.UnixMilli() < manualExpiresAt {
						manualExpiresAt = claim.ExpiresAt.UnixMilli()
					}
				}
			}
		}
		result.Rules = append(result.Rules, CommandSettingsRule{ManualActive: manualActive, ManualExpiresAt: manualExpiresAt, ID: rule.ID, LayerID: rule.LayerRef, WorkspaceID: workspaceID, Mode: string(rule.Mode), Condition: commandSettingsPublicCondition(rule.Condition), Lifecycle: string(rule.Lifecycle), EventName: commandSettingsOptionalString(rule.EventName), AllowedInternalProducerTypes: commandSettingsOptionalString(rule.AllowedInternalProducerTypes), Enabled: rule.Enabled, Source: rule.Source, ReviewStatus: rule.ReviewStatus, GrantGeneration: grantGeneration, GrantFingerprint: grantFingerprint})
	}
	for _, row := range snapshot.Bindings {
		if row.ReplacesDefaultID != nil {
			continue
		}
		result.Diagnostics = append(result.Diagnostics, commandSettingsBindingDiagnostics(row)...)
		commandID := ""
		if row.CommandID != nil {
			commandID = *row.CommandID
		}
		workspaceID := ""
		if row.WorkspaceID != nil {
			workspaceID = *row.WorkspaceID
		}
		defaultID := commandSettingsDefaultID(row)
		currentDefault := currentDefaults[defaultID]
		inherited := snapshot.Scope.WorkspaceID != nil && row.WorkspaceID == nil
		result.Bindings = append(result.Bindings, CommandSettingsBinding{ID: row.ID, LayerID: row.LayerRef, WorkspaceID: workspaceID, CommandID: commandID, TriggerType: row.TriggerType, TriggerSpec: row.TriggerSpec, Enabled: row.Enabled, PersistedEnabled: row.Enabled, Suppressed: row.Effect == "suppress", Customized: true, ReadOnly: inherited, Inherited: inherited, DefaultID: defaultID, ReviewStatus: row.ReviewStatus, Arguments: commandSettingsPublicArguments(row.Arguments), Presentation: commandSettingsPublicArguments(row.Presentation), Condition: commandSettingsPublicCondition(row.Condition), Effect: row.Effect, ResolutionPriority: row.ResolutionPriority, ReplacesDefaultVersion: commandSettingsOptionalString(row.ReplacesDefaultVersion), ReplacesDefaultFingerprint: commandSettingsOptionalString(row.ReplacesDefaultFingerprint), CurrentDefaultVersion: currentDefault.version, CurrentDefaultFingerprint: currentDefault.fingerprint})
	}
	for _, definition := range projection.Registry.List() {
		metadata := definition.Presentation
		name, description := definition.ID, ""
		if metadata != nil {
			localized, ok := metadata.Locales[locale]
			if !ok {
				localized = metadata.Locales["pt-BR"]
			}
			name, description = localized.Name, localized.Description
		}
		sources := make([]string, 0, len(definition.AllowedSources))
		for _, source := range definition.AllowedSources {
			sources = append(sources, string(source))
		}
		result.Commands = append(result.Commands, CommandSettingsCommand{ID: definition.ID, Name: name, Description: description, AllowedSources: sources})
	}
	return result
}

func commandSettingsOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func commandSettingsManualState(snapshot commandconfig.Snapshot, layerID string, workspace *string, principal auth.LocalSessionPrincipal, now time.Time) (ready, active bool) {
	rule, ready := commandSettingsCompatibleManualRuleScoped(snapshot, layerID, workspace)
	if !ready {
		return ready, false
	}
	return true, commandSettingsManualClaimPresentScoped(snapshot, layerID, rule.ID, principal, workspace, now)
}

func commandSettingsDefaultID(row commandconfig.Binding) string {
	if row.ReplacesDefaultID == nil {
		return ""
	}
	return *row.ReplacesDefaultID
}
func commandSettingsBuiltinText(locale, id string) (string, string) {
	if id == commandGlobalLayerID {
		switch locale {
		case "en":
			return "Global voice and job shortcuts", "Shortcuts registered in voice profiles and jobs, including when the app is not focused."
		case "es":
			return "Atajos globales de voz y tareas", "Atajos registrados en perfiles de voz y tareas, incluso cuando la aplicación no tiene el foco."
		}
		return "Atalhos globais de voz e jobs", "Atalhos registrados nos perfis de voz e jobs, inclusive quando o aplicativo está sem foco."
	}
	if id == commandPaletteLayerID {
		switch locale {
		case "en":
			return "Default commands", "Makes commands available in the palette without defining shortcuts."
		case "es":
			return "Comandos predeterminados", "Pone comandos a disposición en la paleta sin definir atajos."
		}
		return "Comandos padrão", "Disponibiliza comandos na paleta sem definir atalhos."
	}
	if id == commandKeyboardLayerID {
		switch locale {
		case "en":
			return "Default keyboard map", "Associates default shortcuts with commands."
		case "es":
			return "Mapa de teclado predeterminado", "Asocia atajos predeterminados a comandos."
		}
		return "Mapa de teclado padrão", "Associa atalhos padrão a comandos."
	}
	return id, ""
}

func nowCommandSettings() time.Time { return time.Now() }

func safeCommandSettingsError(err error) error {
	if err == nil {
		return nil
	}
	known := []error{errChatConversationUnavailable, context.Canceled, context.DeadlineExceeded, commandexecution.ErrDenied, commandexecution.ErrStale, commandexecution.ErrInvalidRequest, commandexecution.ErrInvalidConfiguration, commandconfig.ErrInvalid, commandconfig.ErrStale}
	for _, sentinel := range known {
		if errors.Is(err, sentinel) {
			return sentinel
		}
	}
	return commandexecution.ErrInvalidConfiguration
}
