package commandconfig

import (
	"context"
	"math"
	"reflect"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BindingEnabledChange é uma proposta privada e imutável, não uma autorização.
// Somente PrepareBindingEnabled a cria. O host deve apresentar Diff, aguardar
// decisão fora do gate, reautenticar e invalidar o mapa antes do commit sob gate.
type BindingEnabledChange struct {
	store      *Store
	baseline   *stamp
	before     Binding
	after      Binding
	mutationID string
}

// Diff devolve cópias independentes dos documentos validados. Nunca entrega
// os pointers usados no commit, nem aceita de volta uma versão editada da UI.
func (change *BindingEnabledChange) Diff() (Binding, Binding) {
	if change == nil {
		return Binding{}, Binding{}
	}
	return cloneBinding(change.before), cloneBinding(change.after)
}

// PrepareBindingEnabled suporta apenas a troca explícita de enabled em binding
// global existente e válido no subconjunto ProjectLocalRead. Não cria registros,
// não resolve needs_review, não ativa camadas e não escreve durante o preview.
func (s *Store) PrepareBindingEnabled(ctx context.Context, scope Scope, bindingID string, enabled bool, options LocalReadProjection) (*BindingEnabledChange, error) {
	if !validID(bindingID) || scope.WorkspaceID != nil {
		return nil, ErrInvalid
	}
	snapshot, err := s.Load(ctx, scope)
	if err != nil {
		return nil, err
	}
	if _, err := ProjectLocalRead(ctx, snapshot, options); err != nil {
		return nil, err
	}
	index := -1
	for i, row := range snapshot.Bindings {
		if row.ID == bindingID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, ErrInvalid
	}
	before := cloneBinding(snapshot.Bindings[index])
	if before.Enabled == enabled || before.ReviewStatus != "active" {
		return nil, ErrInvalid
	}
	after := cloneBinding(before)
	after.Enabled = enabled
	snapshot.Bindings[index] = after
	configuration, err := ProjectLocalRead(ctx, snapshot, options)
	if err != nil {
		return nil, err
	}
	// Pendências/avanços automáticos de defaults precisam de outro fluxo de
	// revisão. Não aproveitar a confirmação de enabled para fazer rebase implícito.
	if len(configuration.Adjustments()) != 0 {
		return nil, ErrInvalid
	}
	mutationID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	return &BindingEnabledChange{store: s, baseline: snapshot.stamp, before: before, after: after, mutationID: mutationID.String()}, nil
}

// CommitBindingEnabled é a primitiva SQLite, NÃO uma API autenticada. Exige
// caller confiável mantendo DispatchGate exclusivo e mapa já invalidado.
// CAS de geração e dados são uma transação; falha não é repetida automaticamente.
// Reaplicar uma proposta já consumida falha com ErrStale, sem novo incremento.
func (s *Store) CommitBindingEnabled(ctx context.Context, change *BindingEnabledChange) error {
	return s.commitBindingEnabled(ctx, change, func(apply func(*gorm.DB) error) error {
		return s.db.WithContext(ctx).Transaction(apply)
	})
}

// transact deve executar apply na única transação da operação. A composição
// confirmada usa a transação do receipt service, sem savepoint/commit interno.
func (s *Store) commitBindingEnabled(ctx context.Context, change *BindingEnabledChange, transact func(func(*gorm.DB) error) error) error {
	if s == nil || s.db == nil || ctx == nil || change == nil || change.store != s || change.baseline == nil || change.baseline.store != s {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stamp := change.baseline
	if stamp.scope.WorkspaceID != nil || len(stamp.generations) != 1 {
		return ErrInvalid
	}
	previous := stamp.generations[0]
	if previous.Generation == math.MaxInt64 {
		return ErrInvalid
	}
	return transact(func(tx *gorm.DB) error {
		// UPDATE condicional primeiro: a geração é a autoridade de concorrência,
		// incluindo substituição da linha com o mesmo valor numérico (ABA).
		result := tx.Model(&Generation{}).Where("id = ? AND user_id = ? AND workspace_id IS NULL AND generation = ?", previous.ID, stamp.scope.UserID, previous.Generation).
			Updates(map[string]any{"generation": previous.Generation + 1, "updated_at": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrStale
		}
		var current Binding
		if err := tx.Where("id = ? AND user_id = ? AND workspace_id IS NULL", change.before.ID, stamp.scope.UserID).Take(&current).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrStale
			}
			return err
		}
		// Também detecta alteração direta do binding sem incremento da geração;
		// escritores fora do protocolo continuam não suportados para outros dados.
		if !reflect.DeepEqual(current, change.before) {
			return ErrStale
		}
		result = tx.Model(&Binding{}).Where("id = ? AND user_id = ? AND workspace_id IS NULL AND enabled = ?", change.before.ID, stamp.scope.UserID, change.before.Enabled).
			Update("enabled", change.after.Enabled)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrStale
		}
		return ctx.Err()
	})
}

func cloneBinding(row Binding) Binding {
	clone := func(value *string) *string {
		if value == nil {
			return nil
		}
		copy := *value
		return &copy
	}
	row.WorkspaceID = clone(row.WorkspaceID)
	row.CommandID = clone(row.CommandID)
	row.ReplacesDefaultID = clone(row.ReplacesDefaultID)
	row.ReplacesDefaultVersion = clone(row.ReplacesDefaultVersion)
	row.ReplacesDefaultFingerprint = clone(row.ReplacesDefaultFingerprint)
	return row
}
