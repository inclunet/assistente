package commandportability

import (
	"context"
	"slices"
	"strings"

	"assistente/internal/commandactivation"
	"assistente/internal/commandconfig"
	"gorm.io/gorm"
)

// ExportFromStore lê command_layers através do Store existente. A consulta de
// seleção de IDs só compara owner; nunca retorna o conteúdo de outro usuário.
// ActivationRuleExport recebe apenas colunas portáveis e não claims/grants.
// Catálogo e Trigger são obrigatórios: sem a autoridade de schema/sensibilidade
// e sem adapter registrado, não há exportação segura.
func ExportFromStore(ctx context.Context, db *gorm.DB, userID string, refs ReferencePort, layerIDs []string, includeWorkspace bool) ([]LayerExport, error) {
	return exportFromStore(ctx, db, userID, refs, layerIDs, includeWorkspace, nil)
}

// ExportScopeFromStore lê somente o escopo exato autorizado pelo host. Ao
// exportar workspace, globals herdados e outros workspaces não são recursos
// selecionados. A função compartilha as validações e DTO do export completo.
func ExportScopeFromStore(ctx context.Context, db *gorm.DB, scope commandconfig.Scope, refs ReferencePort) ([]LayerExport, error) {
	return exportFromStore(ctx, db, scope.UserID, refs, nil, scope.WorkspaceID != nil, scope.WorkspaceID)
}

func exportFromStore(ctx context.Context, db *gorm.DB, userID string, refs ReferencePort, layerIDs []string, includeWorkspace bool, exactWorkspace *string) ([]LayerExport, error) {
	if ctx == nil || db == nil || strings.TrimSpace(userID) == "" || refs.Catalog == nil || !refs.Catalog.Complete() || refs.Trigger == nil {
		return nil, ErrInvalid
	}
	store, err := commandconfig.New(db)
	if err != nil {
		return nil, err
	}
	var rows []commandconfig.Layer
	query := db.WithContext(ctx).Where("user_id = ?", userID)
	if exactWorkspace != nil {
		query = query.Where("workspace_id = ?", *exactWorkspace)
	}
	if len(layerIDs) > 0 {
		for _, id := range layerIDs {
			if !validUUID7(id) {
				return nil, ErrInvalid
			}
		}
		var owners []struct{ UserID string }
		if err := db.WithContext(ctx).Table("command_layers").Select("user_id").Where("id IN ?", layerIDs).Find(&owners).Error; err != nil {
			return nil, err
		}
		for _, owner := range owners {
			if owner.UserID != userID {
				return nil, ErrForeignOwner
			}
		}
		query = query.Where("id IN ?", layerIDs)
	} else if !includeWorkspace {
		query = query.Where("workspace_id IS NULL")
	}
	if err := query.Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(layerIDs) > 0 && len(rows) != len(uniqueStrings(layerIDs)) {
		return nil, ErrInvalid
	}
	result := make([]LayerExport, 0, len(rows))
	for _, row := range rows {
		var workspace *string
		if row.WorkspaceID != nil {
			value := *row.WorkspaceID
			workspace = &value
		}
		scope := commandconfig.Scope{UserID: userID, WorkspaceID: workspace}
		snapshot, err := store.Load(ctx, scope)
		if err != nil {
			return nil, err
		}
		selected := commandconfig.Snapshot{Scope: scope, Layers: []commandconfig.Layer{row}}
		for _, binding := range snapshot.Bindings {
			if binding.LayerRef == row.ID && binding.LayerRefKind == "user" {
				selected.Bindings = append(selected.Bindings, binding)
			}
		}
		layers, err := FromSnapshot(selected)
		if err != nil {
			return nil, err
		}
		if len(layers) != 1 {
			return nil, ErrInvalid
		}
		layers[0].ActivationRules, err = exportActivationRules(ctx, db, userID, row.ID, workspace)
		if err != nil {
			return nil, err
		}
		if err := validateExportLayer(ctx, &layers[0], refs); err != nil {
			return nil, err
		}
		result = append(result, layers[0])
	}
	builtin, err := exportBuiltinDeltas(ctx, db, userID, layerIDs, includeWorkspace, exactWorkspace, refs)
	if err != nil {
		return nil, err
	}
	result = append(result, builtin...)
	slices.SortFunc(result, compareLayerExport)
	return result, nil
}

func validateExportLayer(ctx context.Context, layer *LayerExport, refs ReferencePort) error {
	if err := layer.validate(); err != nil {
		return err
	}
	bindings := append(slices.Clone(layer.Bindings), layer.BuiltinDeltas...)
	for i := range bindings {
		if err := validateCatalogBinding(ctx, &bindings[i], refs); err != nil {
			return err
		}
		if bindings[i].LayerRefKind == string(commandactivation.BuiltinRef) {
			if refs.BuiltinLayer == nil || refs.BuiltinLayer(ctx, bindings[i].LayerRef) != nil {
				return ErrMissingReference
			}
		}
		if bindings[i].ReplacesDefaultID != nil {
			if refs.BuiltinDefault == nil || refs.BuiltinDefault(ctx, *bindings[i].ReplacesDefaultID) != nil {
				return ErrMissingReference
			}
		}
		if err := validateCredentialReferences(ctx, bindings[i].Arguments, refs.CredentialPattern, &Plan{}); err != nil {
			return err
		}
	}
	for _, rule := range append(slices.Clone(layer.ActivationRules), layer.BuiltinRuleDeltas...) {
		if rule.LayerRefKind == string(commandactivation.BuiltinRef) {
			if refs.BuiltinLayer == nil || refs.BuiltinLayer(ctx, rule.LayerRef) != nil {
				return ErrMissingReference
			}
		}
		if rule.RuleRefKind == string(commandactivation.BuiltinRef) {
			if refs.BuiltinRuleReference == nil || refs.BuiltinRuleReference(ctx, rule.RuleRef) != nil {
				return ErrMissingReference
			}
		}
		if rule.ReplacesDefaultID != nil {
			if refs.BuiltinDefault == nil || refs.BuiltinDefault(ctx, *rule.ReplacesDefaultID) != nil {
				return ErrMissingReference
			}
		}
	}
	return nil
}

func exportBuiltinDeltas(ctx context.Context, db *gorm.DB, userID string, layerIDs []string, includeWorkspace bool, exactWorkspace *string, refs ReferencePort) ([]LayerExport, error) {
	// Uma seleção explícita identifica somente camadas user. Deltas builtin não
	// são dependências implícitas: só entram no export completo, quando não há
	// filtro de layerIDs.
	if len(layerIDs) > 0 {
		return nil, nil
	}
	query := db.WithContext(ctx).Where("user_id = ? AND layer_ref_kind = ?", userID, "builtin")
	if exactWorkspace != nil {
		query = query.Where("workspace_id = ?", *exactWorkspace)
	}
	if !includeWorkspace {
		query = query.Where("workspace_id IS NULL")
	}
	var bindings []commandconfig.Binding
	if err := query.Order("workspace_id, id").Find(&bindings).Error; err != nil {
		return nil, err
	}
	groups := make(map[string]*LayerExport)
	for _, binding := range bindings {
		group := deltaGroup(groups, portableScopeFromWorkspace(binding.WorkspaceID))
		group.BuiltinDeltas = append(group.BuiltinDeltas, BindingExport{ID: binding.ID, LayerRefKind: binding.LayerRefKind, LayerRef: binding.LayerRef, TriggerType: binding.TriggerType, TriggerSpec: binding.TriggerSpec, CommandID: cloneString(binding.CommandID), Arguments: binding.Arguments, Condition: binding.Condition, Effect: binding.Effect, Enabled: binding.Enabled, ResolutionPriority: binding.ResolutionPriority, ReplacesDefaultID: cloneString(binding.ReplacesDefaultID), ReplacesDefaultVersion: cloneString(binding.ReplacesDefaultVersion), ReplacesDefaultFingerprint: cloneString(binding.ReplacesDefaultFingerprint), ReviewStatus: binding.ReviewStatus, Presentation: binding.Presentation})
	}
	if db.Migrator().HasTable((commandactivation.Rule{}).TableName()) {
		ruleQuery := db.WithContext(ctx).Table((commandactivation.Rule{}).TableName()).Where("user_id = ? AND layer_ref_kind = ?", userID, string(commandactivation.BuiltinRef))
		if exactWorkspace != nil {
			ruleQuery = ruleQuery.Where("workspace_id = ?", *exactWorkspace)
		}
		if !includeWorkspace {
			ruleQuery = ruleQuery.Where("workspace_id IS NULL")
		}
		var rules []activationRuleRow
		if err := ruleQuery.Order("workspace_id, id").Find(&rules).Error; err != nil {
			return nil, err
		}
		for _, row := range rules {
			group := deltaGroup(groups, portableScopeFromWorkspace(row.WorkspaceID))
			group.BuiltinRuleDeltas = append(group.BuiltinRuleDeltas, ActivationRuleExport{ID: row.ID, LayerRefKind: row.LayerRefKind, LayerRef: row.LayerRef, RuleRefKind: row.RuleRefKind, RuleRef: row.RuleRef, Mode: row.Mode, Condition: row.Condition, Lifecycle: row.Lifecycle, EventName: cloneString(row.EventName), AllowedInternalProducerTypes: cloneString(row.AllowedInternalProducerTypes), Enabled: row.Enabled, ReplacesDefaultID: cloneString(row.ReplacesDefaultID), ReplacesDefaultVersion: cloneString(row.ReplacesDefaultVersion), ReplacesDefaultFingerprint: cloneString(row.ReplacesDefaultFingerprint), ReviewStatus: row.ReviewStatus})
		}
	}
	result := make([]LayerExport, 0, len(groups))
	for _, group := range groups {
		if err := validateExportLayer(ctx, group, refs); err != nil {
			return nil, err
		}
		result = append(result, *group)
	}
	slices.SortFunc(result, compareLayerExport)
	return result, nil
}

type activationRuleRow struct {
	ID                           string  `gorm:"column:id"`
	WorkspaceID                  *string `gorm:"column:workspace_id"`
	LayerRefKind                 string  `gorm:"column:layer_ref_kind"`
	LayerRef                     string  `gorm:"column:layer_ref"`
	RuleRefKind                  string  `gorm:"column:rule_ref_kind"`
	RuleRef                      string  `gorm:"column:rule_ref"`
	Mode                         string  `gorm:"column:mode"`
	Condition                    string  `gorm:"column:condition"`
	Lifecycle                    string  `gorm:"column:lifecycle"`
	EventName                    *string `gorm:"column:event_name"`
	AllowedInternalProducerTypes *string `gorm:"column:allowed_internal_producer_types"`
	Enabled                      bool    `gorm:"column:enabled"`
	ReplacesDefaultID            *string `gorm:"column:replaces_default_id"`
	ReplacesDefaultVersion       *string `gorm:"column:replaces_default_version"`
	ReplacesDefaultFingerprint   *string `gorm:"column:replaces_default_fingerprint"`
	ReviewStatus                 string  `gorm:"column:review_status"`
}

func exportActivationRules(ctx context.Context, db *gorm.DB, userID, layerID string, workspace *string) ([]ActivationRuleExport, error) {
	if !db.Migrator().HasTable((commandactivation.Rule{}).TableName()) {
		return nil, nil
	}
	query := db.WithContext(ctx).Table("command_layer_activation_rules").Where("user_id = ? AND layer_ref_kind = ? AND layer_ref = ?", userID, "user", layerID)
	if workspace == nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id = ?", *workspace)
	}
	var rows []activationRuleRow
	if err := query.Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]ActivationRuleExport, 0, len(rows))
	for _, row := range rows {
		rule := ActivationRuleExport{ID: row.ID, LayerRefKind: row.LayerRefKind, LayerRef: row.LayerRef, RuleRefKind: row.RuleRefKind, RuleRef: row.RuleRef, Mode: row.Mode, Condition: row.Condition, Lifecycle: row.Lifecycle, EventName: cloneString(row.EventName), AllowedInternalProducerTypes: cloneString(row.AllowedInternalProducerTypes), Enabled: row.Enabled, ReplacesDefaultID: cloneString(row.ReplacesDefaultID), ReplacesDefaultVersion: cloneString(row.ReplacesDefaultVersion), ReplacesDefaultFingerprint: cloneString(row.ReplacesDefaultFingerprint), ReviewStatus: row.ReviewStatus}
		if err := rule.validate(); err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
