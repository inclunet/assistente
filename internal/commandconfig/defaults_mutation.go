package commandconfig

import (
	"context"
	"reflect"

	"assistente/internal/commandbindings"
	"github.com/google/uuid"
)

// DefaultUpgrade é uma alteração automática do metadado de deltas que ainda
// apontam para o mesmo default semântico. O host deve acrescentar esta
// operação ao seu switch de auditoria/aplicação antes de habilitar o fluxo.
const (
	DefaultUpgrade Operation = "default_upgrade"
	DefaultRebase  Operation = "default_rebase"
)

// DefaultReference identifica a versão do default que um rebase confirmado
// afirma ter visto. Os três campos são uma única prova: nenhum deles pode ser
// omitido ou comparado isoladamente.
type DefaultReference struct {
	ID          string
	Version     string
	Fingerprint string
}

// DefaultRebaseRequest descreve a confirmação estreita de uma pendência. A
// condição precisa ser exatamente a conjunção persistida no delta; o rebase
// nunca amplia o contexto de um override.
type DefaultRebaseRequest struct {
	BindingID string
	Default   DefaultReference
	Condition commandbindings.Facts
}

// PrepareDefaultUpgrade prepara, sem escrever, o avanço de versão de todos os
// deltas do escopo. Fingerprint igual permite somente trocar a versão. Quando
// o fingerprint mudou (ou o default desapareceu), a única transição permitida
// é review_status=needs_review; a versão antiga fica intacta. O resultado usa
// o mesmo Store, stamp privado e diff privado de PreparedMutation, portanto a
// confirmação/CAS posterior permanece ligada ao snapshot carregado.
func (s *Store) PrepareDefaultUpgrade(ctx context.Context, scope Scope, options CompleteProjection) (*PreparedMutation, error) {
	if s == nil || ctx == nil || !validScope(scope) {
		return nil, ErrInvalid
	}
	before, err := s.Load(ctx, scope)
	if err != nil {
		return nil, err
	}
	if _, err := ProjectComplete(ctx, before, options); err != nil {
		return nil, err
	}
	defaults, err := completeDefaultReferences(ctx, options)
	if err != nil {
		return nil, err
	}
	after := cloneConfigSnapshot(before)
	changed := false
	for i := range after.Bindings {
		row := &after.Bindings[i]
		if !sameWorkspace(row.WorkspaceID, scope.WorkspaceID) {
			continue
		}
		if row.ReplacesDefaultID == nil || row.ReplacesDefaultVersion == nil || row.ReplacesDefaultFingerprint == nil {
			continue
		}
		current, exists := defaults[*row.ReplacesDefaultID]
		if exists && *row.ReplacesDefaultFingerprint == current.Fingerprint {
			if *row.ReplacesDefaultVersion != current.Version {
				version := current.Version
				row.ReplacesDefaultVersion = &version
				changed = true
			}
			continue
		}
		if row.ReviewStatus != string(commandbindings.NeedsReview) {
			row.ReviewStatus = string(commandbindings.NeedsReview)
			changed = true
		}
	}
	if !changed {
		return nil, ErrInvalid
	}
	if _, err := ProjectComplete(ctx, after, options); err != nil {
		return nil, err
	}
	return newDefaultPreparedMutation(s, before, after, DefaultUpgrade)
}

// PrepareDefaultRebase prepara a confirmação de uma nova versão sem apagar o
// override: atualiza o trio persistido e volta review_status para active. Antes
// de montar a mutação, verifica sob o mesmo snapshot o trio atual do default e
// a condição exata persistida. O chamador deve reexecutar esta preparação
// durante a confirmação/gate; assim, uma troca do catálogo ou da condição entre
// preview e confirmação falha fechado por CAS/revalidação, sem fallback
// permissivo.
func (s *Store) PrepareDefaultRebase(ctx context.Context, scope Scope, request DefaultRebaseRequest, options CompleteProjection) (*PreparedMutation, error) {
	if s == nil || ctx == nil || !validScope(scope) || !validID(request.BindingID) || !validDefaultReference(request.Default) {
		return nil, ErrInvalid
	}
	before, err := s.Load(ctx, scope)
	if err != nil {
		return nil, err
	}
	if _, err := ProjectComplete(ctx, before, options); err != nil {
		return nil, err
	}
	var row *Binding
	for i := range before.Bindings {
		if before.Bindings[i].ID == request.BindingID && sameWorkspace(before.Bindings[i].WorkspaceID, scope.WorkspaceID) {
			candidate := cloneBinding(before.Bindings[i])
			row = &candidate
			break
		}
	}
	if row == nil || row.ReplacesDefaultID == nil || row.ReplacesDefaultVersion == nil || row.ReplacesDefaultFingerprint == nil || row.ReviewStatus != string(commandbindings.NeedsReview) {
		return nil, ErrInvalid
	}
	if *row.ReplacesDefaultID != request.Default.ID {
		return nil, ErrStale
	}
	condition, err := decodeCondition(row.Condition)
	if err != nil || !reflect.DeepEqual(condition, request.Condition) {
		return nil, ErrInvalid
	}
	defaults, err := completeDefaultReferences(ctx, options)
	if err != nil {
		return nil, err
	}
	current, exists := defaults[request.Default.ID]
	if !exists || current != request.Default {
		return nil, ErrStale
	}

	after := cloneConfigSnapshot(before)
	for i := range after.Bindings {
		if after.Bindings[i].ID != request.BindingID {
			continue
		}
		row := &after.Bindings[i]
		version, fingerprint, active := request.Default.Version, request.Default.Fingerprint, "active"
		row.ReplacesDefaultVersion = &version
		row.ReplacesDefaultFingerprint = &fingerprint
		row.ReviewStatus = active
		break
	}
	if _, err := ProjectComplete(ctx, after, options); err != nil {
		return nil, err
	}
	return newDefaultPreparedMutation(s, before, after, DefaultRebase)
}

func validDefaultReference(reference DefaultReference) bool {
	return reference.ID != "" && reference.Version != "" && reference.Fingerprint != ""
}

func completeDefaultReferences(ctx context.Context, options CompleteProjection) (map[string]DefaultReference, error) {
	_, _, defaults, err := completeBuiltinLayers(ctx, options)
	if err != nil {
		return nil, err
	}
	result := make(map[string]DefaultReference, len(defaults))
	for _, current := range defaults {
		if _, exists := result[current.Candidate.ID]; exists {
			return nil, ErrInvalid
		}
		result[current.Candidate.ID] = DefaultReference{ID: current.Candidate.ID, Version: current.Version, Fingerprint: current.Fingerprint}
	}
	return result, nil
}

func newDefaultPreparedMutation(s *Store, before, after Snapshot, operation Operation) (*PreparedMutation, error) {
	if s == nil || before.stamp == nil || before.stamp.store != s || operation == "" {
		return nil, ErrInvalid
	}
	if reflect.DeepEqual(before.Layers, after.Layers) && reflect.DeepEqual(before.Bindings, after.Bindings) {
		return nil, ErrInvalid
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	return &PreparedMutation{
		store:  s,
		before: cloneConfigSnapshot(before),
		after:  cloneConfigSnapshot(after),
		diff: MutationDiff{
			MutationID: id.String(),
			Operation:  operation,
			Scope:      cloneScope(before.Scope),
		},
	}, nil
}
