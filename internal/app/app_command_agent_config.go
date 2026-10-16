package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjson"
	"assistente/internal/database"
	commandtool "assistente/internal/tools/command"
	"gorm.io/gorm"
)

// Config é a porta de backend da command_config. Ela não é uma fachada Wails:
// a identidade vem do caller montado pelo executor de tools e o payload só
// contém a intenção de configuração, nunca owner, sessão ou credenciais.
func (b commandAgentTools) Config(ctx context.Context, req commandtool.Request) (result any, err error) {
	defer func() { err = commandAgentToolError(err) }()
	c, err := b.app.commandAgentAccess(ctx, commandtool.ConfigName)
	if err != nil {
		return nil, err
	}
	_, known := commandtool.ConfigActionMutates(req.Action)
	if !known {
		return nil, commandexecution.ErrInvalidRequest
	}
	if err := c.revalidate(ctx); err != nil {
		return nil, err
	}
	scope, err := commandAgentConfigScope(c, req.Scope)
	if err != nil {
		return nil, err
	}

	mutates, _ := commandtool.ConfigActionMutates(req.Action)
	if req.CommandID != "" || (req.Action != "config_export" && !mutates && (len(req.Arguments) != 0 || len(req.Payload) != 0)) {
		return nil, commandexecution.ErrInvalidRequest
	}
	if err := validateAgentConfigEnvelope(req, mutates); err != nil {
		return nil, err
	}
	if req.Action == "config_import" {
		return b.importAgentConfig(ctx, c, req, scope)
	}
	if req.Action == "config_export" {
		return b.exportAgentConfig(ctx, c, req, scope)
	}
	if !mutates {
		return b.readAgentConfig(ctx, c, req, scope)
	}
	return b.mutateAgentConfig(ctx, c, req, scope)
}

func validateAgentConfigEnvelope(req commandtool.Request, mutates bool) error {
	if req.Action == "config_export" || req.Action == "config_import" {
		if req.ID != "" || req.LayerID != "" {
			return commandexecution.ErrInvalidRequest
		}
		return nil
	}
	if mutates {
		if req.Action != "binding_create" && req.Action != "binding_update" && req.LayerID != "" {
			return commandexecution.ErrInvalidRequest
		}
		return nil
	}
	switch req.Action {
	case "layer_list":
		if req.ID != "" || req.LayerID != "" {
			return commandexecution.ErrInvalidRequest
		}
	case "layer_get":
		if req.ID == "" || req.LayerID != "" {
			return commandexecution.ErrInvalidRequest
		}
	case "binding_list":
		if req.ID != "" {
			return commandexecution.ErrInvalidRequest
		}
	case "binding_check_conflict":
		if req.ID != "" || req.LayerID != "" {
			return commandexecution.ErrInvalidRequest
		}
	}
	return nil
}

func commandAgentConfigScope(c *commandAgentCaller, name string) (commandconfig.Scope, error) {
	if c == nil || c.product == nil || strings.TrimSpace(c.principal.UserID) == "" {
		return commandconfig.Scope{}, commandexecution.ErrDenied
	}
	scope := commandconfig.Scope{UserID: c.principal.UserID}
	switch CommandSettingsScope(strings.TrimSpace(name)) {
	case CommandSettingsScopeGlobal:
		return scope, nil
	case CommandSettingsScopeWorkspace:
		workspace := strings.TrimSpace(c.product.workspaceID)
		if workspace == "" {
			return commandconfig.Scope{}, commandexecution.ErrDenied
		}
		scope.WorkspaceID = &workspace
		return scope, nil
	default:
		return commandconfig.Scope{}, commandexecution.ErrInvalidRequest
	}
}

func (b commandAgentTools) readAgentConfig(ctx context.Context, c *commandAgentCaller, req commandtool.Request, scope commandconfig.Scope) (any, error) {
	epoch, err := c.product.epochs.CaptureAuthenticated(ctx, func(check context.Context) (string, string, error) {
		if err := c.revalidate(check); err != nil {
			return "", "", err
		}
		return c.principal.UserID, c.principal.SessionID, nil
	})
	if err != nil {
		return nil, err
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	snapshot, projection, active, err := b.app.commandSettingsAuthority(ctx, c.product, scope, store)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.CheckCurrent(ctx, snapshot); err != nil {
		return nil, err
	}
	fingerprint, err := commandSettingsFingerprint(snapshot, projection, active)
	if err != nil {
		return nil, err
	}
	view := commandSettingsSnapshot(ctx, req.Locale, commandClaimsAtEpoch(snapshot, epoch), projection, active, c.principal, time.Now())
	view.Scope = string(CommandSettingsScope(strings.TrimSpace(req.Scope)))
	view.Revision = commandSettingsRevision(snapshot)
	view.Fingerprint = fingerprint
	view.Commands = nil

	var result any
	switch req.Action {
	case "layer_get":
		view.Layers = filterAgentLayers(view.Layers, req.ID)
		if len(view.Layers) != 1 {
			return nil, commandexecution.ErrInvalidRequest
		}
		view.Bindings = filterAgentBindings(view.Bindings, req.ID)
		view.Rules = filterAgentRules(view.Rules, req.ID)
		view.Diagnostics = nil
		view.Adjustments = nil
		result = view
	case "binding_check_conflict":
		configuration, err := commandconfig.ProjectComplete(ctx, snapshot, projection)
		if err != nil {
			return nil, err
		}
		witnesses, err := configuration.CheckConflicts(ctx)
		if err != nil {
			return nil, err
		}
		result = struct {
			Scope       string                            `json:"scope"`
			Revision    int64                             `json:"revision"`
			Fingerprint string                            `json:"fingerprint"`
			Witnesses   []commandbindings.ConflictWitness `json:"witnesses"`
		}{view.Scope, view.Revision, view.Fingerprint, witnesses}
	case "binding_list":
		view.Bindings = filterAgentBindings(view.Bindings, req.LayerID)
		view.Layers = nil
		view.Rules = nil
		view.Diagnostics = nil
		view.Adjustments = nil
		result = view
	case "layer_list":
		view.Bindings = nil
		view.Diagnostics = nil
		view.Adjustments = nil
		result = view
	default:
		return nil, commandexecution.ErrInvalidRequest
	}
	if err := c.product.epochs.Admit(ctx, epoch, func(check context.Context) error {
		return c.revalidate(check)
	}, func() error { return store.CheckCurrent(ctx, snapshot) }); err != nil {
		return nil, err
	}
	return result, nil
}

func filterAgentLayers(layers []CommandSettingsLayer, id string) []CommandSettingsLayer {
	filtered := make([]CommandSettingsLayer, 0, 1)
	for _, layer := range layers {
		if layer.ID == id {
			filtered = append(filtered, layer)
		}
	}
	return filtered
}

func filterAgentBindings(bindings []CommandSettingsBinding, layerID string) []CommandSettingsBinding {
	if layerID == "" {
		return bindings
	}
	filtered := make([]CommandSettingsBinding, 0, len(bindings))
	for _, binding := range bindings {
		if binding.LayerID == layerID {
			filtered = append(filtered, binding)
		}
	}
	return filtered
}

func filterAgentRules(rules []CommandSettingsRule, layerID string) []CommandSettingsRule {
	filtered := make([]CommandSettingsRule, 0, len(rules))
	for _, rule := range rules {
		if rule.LayerID == layerID {
			filtered = append(filtered, rule)
		}
	}
	return filtered
}

func (b commandAgentTools) mutateAgentConfig(ctx context.Context, c *commandAgentCaller, req commandtool.Request, scope commandconfig.Scope) (any, error) {
	settings, err := decodeAgentSettingsMutation(req)
	if err != nil {
		return nil, err
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	snapshot, projection, active, err := b.app.commandSettingsAuthority(ctx, c.product, scope, store)
	if err != nil {
		return nil, err
	}
	fingerprint, err := commandSettingsFingerprint(snapshot, projection, active)
	if err != nil {
		return nil, err
	}
	if settings.ExpectedRevision != commandSettingsRevision(snapshot) || settings.ExpectedFingerprint != fingerprint {
		return nil, commandexecution.ErrStale
	}
	intent, err := commandSettingsIntent(settings, commandconfig.Operation(req.Action), snapshot, projection, scope)
	if err != nil {
		return nil, err
	}

	// Reusa as portas de decisão/rebuild do applier comum. A camada de agente
	// apenas acrescenta o caller check; não abre transação nem apresenta UI.
	inputs, err := b.app.commandDesktopMutationInputs(c.product, ctx)
	if err != nil {
		return nil, err
	}
	locale := settings.Locale
	if locale == "" {
		locale = "pt-BR"
	}
	inputs.Render = func(diff commandconfig.MutationDiff) (string, error) {
		return renderCommandSettingsDiff(locale, diff)
	}
	baseAuthorize := inputs.Authorize
	inputs.Authorize = func(authCtx context.Context, principal auth.LocalSessionPrincipal, target commandconfig.Scope, operation commandconfig.Operation) error {
		if principal != c.principal || target.UserID != scope.UserID || !sameCommandWorkspace(target.WorkspaceID, scope.WorkspaceID) {
			return commandexecution.ErrDenied
		}
		if err := c.revalidate(authCtx); err != nil {
			return err
		}
		return baseAuthorize(authCtx, principal, target, operation)
	}
	baseHook := inputs.OnMutationTx
	inputs.OnMutationTx = func(hookCtx context.Context, tx *gorm.DB, diff commandconfig.MutationDiff) error {
		if err := c.revalidateTx(hookCtx, tx); err != nil {
			return err
		}
		if !commandSettingsDiffMatchesSnapshot(diff, snapshot) {
			return commandconfig.ErrStale
		}
		// baseHook usa o tx recebido; nenhuma transação adicional é criada.
		return baseHook(hookCtx, tx, diff)
	}
	applier, err := b.app.newCommandMutationApplierAuthenticated(inputs, c)
	if err != nil {
		return nil, err
	}
	applied, applyErr := applier.ApplyScoped(ctx, "", cloneCommandWorkspace(scope.WorkspaceID), intent)
	if applyErr != nil {
		if applied.Committed {
			return CommandSettingsMutation{Committed: true, Published: false, ID: commandSettingsResourceID(intent, applied.Diff)}, nil
		}
		return nil, applyErr
	}
	return CommandSettingsMutation{Committed: applied.Committed, Published: applied.Rebuilt, ID: commandSettingsResourceID(intent, applied.Diff)}, nil
}

func decodeAgentSettingsMutation(req commandtool.Request) (CommandSettingsMutationRequest, error) {
	if len(req.Payload) == 0 || bytes.Equal(bytes.TrimSpace(req.Payload), []byte("null")) || len(req.Arguments) != 0 || req.CommandID != "" {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	canonical, err := commandjson.Canonicalize(req.Payload)
	if err != nil || len(canonical) == 0 || canonical[0] != '{' {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &fields); err != nil || fields == nil {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	var result CommandSettingsMutationRequest
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	if strings.TrimSpace(req.Scope) == "" || result.ExpectedRevision < 1 || strings.TrimSpace(result.ExpectedFingerprint) == "" {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	if value, ok := fields["scope"]; ok && !sameAgentEnvelopeString(value, req.Scope) {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	if value, ok := fields["operation"]; ok && !sameAgentEnvelopeString(value, req.Action) {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	if value, ok := fields["id"]; ok && !sameAgentEnvelopeString(value, req.ID) {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	if req.Locale != "" && result.Locale != "" && result.Locale != req.Locale {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	if result.Rule != nil || result.Default != nil ||
		(req.Action != "layer_create" && req.Action != "layer_update" && result.Layer != nil) ||
		(req.Action != "binding_create" && req.Action != "binding_update" && result.Binding != nil) ||
		(req.Action != "layer_restore" && result.LayerRefKind != "") {
		return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
	}
	// Ação, escopo e ID são autoridade do envelope da tool, não do JSON do
	// modelo. Repetições só são aceitas quando idênticas.
	result.Scope = CommandSettingsScope(strings.TrimSpace(req.Scope))
	result.Operation = req.Action
	result.ID = req.ID
	if req.Locale != "" {
		result.Locale = req.Locale
	}
	if req.LayerID != "" && result.Binding != nil {
		if result.Binding.LayerID != "" && result.Binding.LayerID != req.LayerID {
			return CommandSettingsMutationRequest{}, commandexecution.ErrInvalidRequest
		}
		result.Binding.LayerID = req.LayerID
	}
	return result, nil
}

func sameAgentEnvelopeString(raw json.RawMessage, expected string) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && value == expected
}
