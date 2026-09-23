package commandconfig

import (
	"context"
	"slices"
	"strings"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

// CommitConfirmedImportBatch exige o gate exclusivo e reautenticação do host.
// Cada escopo tem seu diff/receipt; nenhum é aplicado até todos serem aceitos.
// Consumo, CAS, alterações e auditorias compartilham uma única transação.
func (s *Store) CommitConfirmedImportBatch(ctx context.Context, confirmed []*ConfirmedMutation, epoch commandsecurity.EpochSnapshot, hook MutationTxHook) error {
	if s == nil || ctx == nil || len(confirmed) == 0 || len(confirmed) > 64 || hook == nil {
		return ErrInvalid
	}
	ordered := slices.Clone(confirmed)
	seen := make(map[string]bool, len(ordered))
	var receipts *commanddecision.Store
	for _, c := range ordered {
		if c == nil || c.prepared == nil || c.prepared.store != s || c.receipts == nil || c.epoch != epoch || c.prepared.diff.Operation != ConfigImport || c.applyBeforeHook != nil {
			return ErrInvalid
		}
		scope := c.prepared.before.Scope
		if !validScope(scope) || scope.UserID != epoch.UserID {
			return ErrInvalid
		}
		key := importScopeKey(scope.WorkspaceID)
		if seen[key] || (receipts != nil && receipts != c.receipts) {
			return ErrInvalid
		}
		seen[key], receipts = true, c.receipts
	}
	// Workspaces herdam o global: não modificar essa baseline até seus CAS.
	slices.SortFunc(ordered, func(a, b *ConfirmedMutation) int {
		left, right := a.prepared.before.Scope.WorkspaceID, b.prepared.before.Scope.WorkspaceID
		if left == nil {
			if right == nil {
				return 0
			}
			return 1
		}
		if right == nil {
			return -1
		}
		return strings.Compare(*left, *right)
	})
	requests := make([]commanddecision.Request, len(ordered))
	for i, c := range ordered {
		requests[i] = c.request
	}
	return receipts.ConsumeBatchForDatabase(ctx, s.db, requests, func(tx *gorm.DB) error {
		for _, c := range ordered {
			if err := s.commitConfirmedMutationTx(ctx, tx, c, hook); err != nil {
				return err
			}
		}
		// Um hook posterior não pode reescrever silenciosamente um escopo
		// já auditado. Conferir cada parte própria, sem globais herdados.
		for _, c := range ordered {
			p := c.prepared
			generations, err := readGenerations(tx, p.after.Scope)
			if err != nil {
				return err
			}
			for _, expected := range p.before.Generations {
				if !sameWorkspace(expected.WorkspaceID, p.after.Scope.WorkspaceID) {
					continue
				}
				found := false
				for _, actual := range generations {
					if actual.ID == expected.ID && actual.Generation == expected.Generation+1 {
						found = true
					}
				}
				if !found {
					return ErrStale
				}
			}
			var layers []Layer
			var bindings []Binding
			if err := scoped(tx, p.after.Scope).Order("id").Find(&layers).Error; err != nil {
				return err
			}
			if err := scoped(tx, p.after.Scope).Order("id").Find(&bindings).Error; err != nil {
				return err
			}
			actual, err := readAggregateSnapshot(ctx, tx, p.after.Scope)
			if err != nil {
				return err
			}
			actual.Layers, actual.Bindings = layers, bindings
			if !sameExactImportState(actual, p.after) {
				return ErrStale
			}
		}
		return nil
	})
}

func importScopeKey(workspace *string) string {
	if workspace == nil {
		return "global"
	}
	return "workspace:" + *workspace
}

func exactImportRows[T any](rows []T, workspace *string, scopeOf func(T) *string) []T {
	result := make([]T, 0, len(rows))
	for _, row := range rows {
		if sameWorkspace(scopeOf(row), workspace) {
			result = append(result, row)
		}
	}
	return result
}

func sameExactImportState(actual, expected Snapshot) bool {
	w := expected.Scope.WorkspaceID
	layerScope := func(row Layer) *string { return row.WorkspaceID }
	bindingScope := func(row Binding) *string { return row.WorkspaceID }
	ruleScope := func(row commandactivation.Rule) *string { return row.WorkspaceID }
	grantScope := func(row commandautomation.Grant) *string { return row.Owner.WorkspaceID }
	claimScope := func(row commandactivation.Claim) *string { return row.WorkspaceID }
	return equalRows(exactImportRows(actual.Layers, w, layerScope), exactImportRows(expected.Layers, w, layerScope)) &&
		equalRows(exactImportRows(actual.Bindings, w, bindingScope), exactImportRows(expected.Bindings, w, bindingScope)) &&
		equalRows(exactImportRows(actual.ActivationRules, w, ruleScope), exactImportRows(expected.ActivationRules, w, ruleScope)) &&
		equalRows(exactImportRows(actual.AutomationGrants, w, grantScope), exactImportRows(expected.AutomationGrants, w, grantScope)) &&
		equalRows(exactImportRows(actual.ActivationClaims, w, claimScope), exactImportRows(expected.ActivationClaims, w, claimScope))
}
