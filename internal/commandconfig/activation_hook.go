package commandconfig

import (
	"context"
	"errors"
	"reflect"
	"sort"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"gorm.io/gorm"
)

// ActivationOwner é uma porta do host autenticado. O callback deve devolver
// a autoridade canônica para o Scope recebido e não deve adquirir
// DispatchGate: o hook é executado dentro do gate exclusivo do commit.
type ActivationOwner func(context.Context, Scope) (commandactivation.Owner, error)

var (
	// ErrActivationRestoreRequiresExpandedContract impede que um diff sem as
	// regras/grants assinados seja interpretado como restore agregado.
	ErrActivationRestoreRequiresExpandedContract = errors.New("restore de ativação requer contrato expandido")
	ErrActivationOwnerScopeMismatch              = errors.New("owner de ativação fora do escopo da mutação")
)

// NewActivationMutationHook compõe o commit de commandconfig com a
// revogação de grants e a reconciliação de claims. O TX recebido é sempre o
// TX do CommitConfirmedMutation; esta função nunca abre uma transação nem
// adquire gate.
//
// A ordem é deliberada: grants de camadas que deixaram de estar habilitadas
// são revogados antes da reconciliação, e ambas as operações ficam sujeitas ao
// rollback do mesmo TX.
func NewActivationMutationHook(activation *commandactivation.Service, grants *commandautomation.Store, owner ActivationOwner) (MutationTxHook, error) {
	if activation == nil || activation.Store() == nil || grants == nil || owner == nil {
		return nil, ErrInvalid
	}
	return func(ctx context.Context, tx *gorm.DB, diff MutationDiff) error {
		return runActivationMutationHook(ctx, tx, diff, activation, grants, owner)
	}, nil
}

// NewMutationTxHook é o nome curto mantido como API de composição para o
// bootstrap. Ele é um alias sem comportamento adicional.
func NewMutationTxHook(activation *commandactivation.Service, grants *commandautomation.Store, owner ActivationOwner) (MutationTxHook, error) {
	return NewActivationMutationHook(activation, grants, owner)
}

func runActivationMutationHook(ctx context.Context, tx *gorm.DB, diff MutationDiff, activation *commandactivation.Service, grants *commandautomation.Store, owner ActivationOwner) error {
	if ctx == nil || tx == nil || activation == nil || activation.Store() == nil || grants == nil || owner == nil {
		return ErrInvalid
	}
	if diff.Operation == LayerRestore || diff.Operation == ConfigRestore {
		return ErrActivationRestoreRequiresExpandedContract
	}

	layers, anyLayerDelta, err := activationLayerChanges(diff)
	if err != nil {
		return err
	}
	if diff.Operation == BindingRestore && anyLayerDelta {
		return ErrActivationRestoreRequiresExpandedContract
	}
	// Binding-only mutations, including BindingRestore with an unchanged layer
	// snapshot, have no activation side effect. In particular, do not ask the
	// owner port to turn a no-op into authority.
	if len(layers) == 0 {
		return nil
	}

	canonical, err := owner(ctx, diff.Scope)
	if err != nil {
		return err
	}
	if !sameActivationScope(canonical.Scope, diff.Scope) {
		return ErrActivationOwnerScopeMismatch
	}

	grantOwner := commandautomation.Owner{UserID: canonical.UserID, WorkspaceID: cloneWorkspace(diff.Scope.WorkspaceID)}
	for _, change := range layers {
		if !change.AfterPresent || !change.AfterEnabled {
			reason := "layer_disabled"
			if !change.AfterPresent {
				reason = "layer_deleted"
			}
			if err := grants.RevokeLayerTx(ctx, tx, grantOwner, commandautomation.RuleRef{Kind: "user", Ref: change.Ref.ID}, canonical.UserID, reason); err != nil {
				return err
			}
		}
	}
	_, err = activation.ReconcileLayersTx(ctx, tx, canonical, layers)
	return err
}

// activationLayerChanges intentionally projects only presence/enabled state.
// Names, descriptions and priorities do not alter activation state. A local
// commandconfig snapshot may contain inherited global rows; those rows are
// ignored for a local mutation, while any state delta outside the exact scope
// is rejected rather than being applied to another scope.
func activationLayerChanges(diff MutationDiff) ([]commandactivation.LayerChange, bool, error) {
	beforeOutside, err := outOfScopeLayerStates(diff.BeforeLayers, diff.Scope)
	if err != nil {
		return nil, false, err
	}
	afterOutside, err := outOfScopeLayerStates(diff.AfterLayers, diff.Scope)
	if err != nil {
		return nil, false, err
	}
	if !reflect.DeepEqual(beforeOutside, afterOutside) {
		return nil, false, ErrActivationOwnerScopeMismatch
	}
	before, err := indexMutationLayers(diff.BeforeLayers, diff.Scope)
	if err != nil {
		return nil, false, err
	}
	after, err := indexMutationLayers(diff.AfterLayers, diff.Scope)
	if err != nil {
		return nil, false, err
	}

	allIDs := make(map[string]struct{}, len(before)+len(after))
	for id := range before {
		allIDs[id] = struct{}{}
	}
	for id := range after {
		allIDs[id] = struct{}{}
	}
	ids := make([]string, 0, len(allIDs))
	for id := range allIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	changes := make([]commandactivation.LayerChange, 0, len(ids))
	anyDelta := false
	for _, id := range ids {
		old, oldOK := before[id]
		current, currentOK := after[id]
		if !oldOK || !currentOK || old.Enabled != current.Enabled {
			anyDelta = true
			change := commandactivation.LayerChange{Ref: commandactivation.Ref{Kind: commandactivation.UserRef, ID: id}, WorkspaceID: cloneWorkspace(diff.Scope.WorkspaceID), BeforePresent: oldOK, AfterPresent: currentOK}
			if oldOK {
				change.BeforeEnabled = old.Enabled
			}
			if currentOK {
				change.AfterEnabled = current.Enabled
			}
			changes = append(changes, change)
		}
	}

	// A restore is only safe when its layer snapshot is byte-for-byte the same
	// set of rows. For ordinary binding mutations, non-layer metadata is not an
	// activation concern, so the projected delta above is sufficient.
	if diff.Operation == BindingRestore && !sameLayerRows(diff.BeforeLayers, diff.AfterLayers) {
		return nil, false, ErrActivationRestoreRequiresExpandedContract
	}
	return changes, anyDelta, nil
}

type outOfScopeLayerState struct {
	WorkspaceID *string
	Enabled     bool
}

func outOfScopeLayerStates(rows []Layer, scope Scope) (map[string]outOfScopeLayerState, error) {
	result := make(map[string]outOfScopeLayerState, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.UserID != scope.UserID {
			return nil, ErrActivationOwnerScopeMismatch
		}
		if _, exists := seen[row.ID]; exists || row.ID == "" {
			return nil, ErrInvalid
		}
		seen[row.ID] = struct{}{}
		if !sameWorkspace(row.WorkspaceID, scope.WorkspaceID) {
			result[row.ID] = outOfScopeLayerState{WorkspaceID: cloneWorkspace(row.WorkspaceID), Enabled: row.Enabled}
		}
	}
	return result, nil
}

func indexMutationLayers(rows []Layer, scope Scope) (map[string]Layer, error) {
	result := make(map[string]Layer, len(rows))
	for _, row := range rows {
		if row.UserID != scope.UserID {
			return nil, ErrActivationOwnerScopeMismatch
		}
		// Only the exact owner/workspace layer set can become an activation
		// change. Inherited global rows are valid input for local snapshots.
		if !sameWorkspace(row.WorkspaceID, scope.WorkspaceID) {
			continue
		}
		if row.ID == "" {
			return nil, ErrInvalid
		}
		if _, exists := result[row.ID]; exists {
			return nil, ErrInvalid
		}
		result[row.ID] = row
	}
	return result, nil
}

func sameLayerRows(before, after []Layer) bool {
	left := make(map[string]Layer, len(before))
	right := make(map[string]Layer, len(after))
	for _, row := range before {
		if _, exists := left[row.ID]; exists {
			return false
		}
		left[row.ID] = row
	}
	for _, row := range after {
		if _, exists := right[row.ID]; exists {
			return false
		}
		right[row.ID] = row
	}
	return reflect.DeepEqual(left, right)
}

func sameActivationScope(left commandactivation.Scope, right Scope) bool {
	return left.UserID == right.UserID && sameWorkspace(left.WorkspaceID, right.WorkspaceID)
}

func cloneWorkspace(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
