package commandportability

import (
	"context"
	"slices"
	"strings"

	"assistente/internal/commandconfig"
	"gorm.io/gorm"
)

// ExportFromStore lê command_layers através do Store existente. A consulta de
// seleção de IDs só compara owner; nunca retorna o conteúdo de outro usuário.
// ActivationRuleExport recebe apenas colunas portáveis e não claims/grants.
// Catálogo e Trigger são obrigatórios: sem a autoridade de schema/sensibilidade
// e sem adapter registrado, não há exportação segura.
func ExportFromStore(ctx context.Context, db *gorm.DB, userID string, refs ReferencePort, layerIDs []string, includeWorkspace bool) ([]LayerExport, error) {
	if ctx == nil || db == nil || strings.TrimSpace(userID) == "" || refs.Catalog == nil || !refs.Catalog.Complete() || refs.Trigger == nil {
		return nil, ErrInvalid
	}
	store, err := commandconfig.New(db)
	if err != nil {
		return nil, err
	}
	if err := rejectBuiltinDeltas(ctx, db, userID); err != nil {
		return nil, err
	}
	var rows []commandconfig.Layer
	query := db.WithContext(ctx).Where("user_id = ?", userID)
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
		for _, binding := range snapshot.Bindings {
			if binding.LayerRefKind == "builtin" {
				// Este formato ainda não possui um contêiner seguro para deltas
				// soltos sobre uma camada builtin. Recusar evita export parcial.
				return nil, ErrUnsupported
			}
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
	slices.SortFunc(result, func(a, b LayerExport) int { return strings.Compare(a.ID, b.ID) })
	return result, nil
}

func rejectBuiltinDeltas(ctx context.Context, db *gorm.DB, userID string) error {
	for _, table := range []string{"command_bindings", "command_layer_activation_rules"} {
		var count int64
		if err := db.WithContext(ctx).Table(table).Where("user_id = ? AND layer_ref_kind = ?", userID, "builtin").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrUnsupported
		}
	}
	return nil
}

func validateExportLayer(ctx context.Context, layer *LayerExport, refs ReferencePort) error {
	if err := layer.validate(); err != nil {
		return err
	}
	for i := range layer.Bindings {
		if err := validateCatalogBinding(ctx, &layer.Bindings[i], refs); err != nil {
			return err
		}
	}
	return nil
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
