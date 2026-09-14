package commandconfig

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Store struct{ db *gorm.DB }
type stamp struct {
	store       *Store
	scope       Scope
	generations []Generation
}

func New(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, ErrInvalid
	}
	return &Store{db: db}, nil
}

// Load usa uma única transação de leitura para dados e gerações. Não migra,
// cria defaults, repara documentos ou escreve no banco durante o carregamento.
func (s *Store) Load(ctx context.Context, scope Scope) (Snapshot, error) {
	if s == nil || s.db == nil || ctx == nil || !validScope(scope) {
		return Snapshot{}, ErrInvalid
	}
	scope = cloneScope(scope)
	var snapshot Snapshot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		snapshot.Scope = cloneScope(scope)
		if err := scoped(tx, scope).Order("id").Find(&snapshot.Layers).Error; err != nil {
			return err
		}
		if err := scoped(tx, scope).Order("id").Find(&snapshot.Bindings).Error; err != nil {
			return err
		}
		generations, err := readGenerations(tx, scope)
		if err != nil {
			return err
		}
		snapshot.Generations = generations
		return validateSnapshot(snapshot)
	})
	if err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	snapshot.stamp = &stamp{store: s, scope: cloneScope(scope), generations: cloneGenerations(snapshot.Generations)}
	return snapshot, nil
}

// CheckCurrent é curto e pode ser usado na revalidação sob DispatchGate.
// O stamp é privado e não compartilha slices/pointers mutáveis com o projetor.
// Escritores DEVEM alterar dados+geração na mesma transação sob esse gate;
// este carregador não autoriza writes externos fora do protocolo.
func (s *Store) CheckCurrent(ctx context.Context, snapshot Snapshot) error {
	if s == nil || s.db == nil || ctx == nil || snapshot.stamp == nil || snapshot.stamp.store != s {
		return ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := readGenerations(tx, snapshot.stamp.scope)
		if err != nil {
			return err
		}
		if len(current) != len(snapshot.stamp.generations) {
			return ErrStale
		}
		for i, previous := range snapshot.stamp.generations {
			if current[i].ID != previous.ID || current[i].Generation != previous.Generation || !sameWorkspace(current[i].WorkspaceID, previous.WorkspaceID) {
				return ErrStale
			}
		}
		return ctx.Err()
	})
}

func scoped(tx *gorm.DB, scope Scope) *gorm.DB {
	query := tx.Where("user_id = ?", scope.UserID)
	if scope.WorkspaceID == nil {
		return query.Where("workspace_id IS NULL")
	}
	return query.Where("(workspace_id IS NULL OR workspace_id = ?)", *scope.WorkspaceID)
}

func readGenerations(tx *gorm.DB, scope Scope) ([]Generation, error) {
	var rows []Generation
	if err := scoped(tx, scope).Order("workspace_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	want := 1
	if scope.WorkspaceID != nil {
		want = 2
	}
	if len(rows) != want {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if !validID(row.ID) || row.UserID != scope.UserID || row.Generation < 1 || !inScope(row.WorkspaceID, scope) {
			return nil, ErrInvalid
		}
		key := "global"
		if row.WorkspaceID != nil {
			key = *row.WorkspaceID
		}
		if seen[key] {
			return nil, ErrInvalid
		}
		seen[key] = true
	}
	if !seen["global"] {
		return nil, ErrInvalid
	}
	return rows, nil
}

func validateSnapshot(snapshot Snapshot) error {
	layers := map[string]Layer{}
	for _, row := range snapshot.Layers {
		if !validID(row.ID) || row.UserID != snapshot.Scope.UserID || !inScope(row.WorkspaceID, snapshot.Scope) || strings.TrimSpace(row.Name) == "" || strings.TrimSpace(row.Source) == "" {
			return ErrInvalid
		}
		if _, exists := layers[row.ID]; exists {
			return ErrInvalid
		}
		layers[row.ID] = row
	}
	bindings := map[string]bool{}
	for _, row := range snapshot.Bindings {
		if !validID(row.ID) || row.UserID != snapshot.Scope.UserID || !inScope(row.WorkspaceID, snapshot.Scope) {
			return ErrInvalid
		}
		if bindings[row.ID] {
			return ErrInvalid
		}
		bindings[row.ID] = true
		if row.ReviewStatus != "active" && row.ReviewStatus != "needs_review" {
			return ErrInvalid
		}
		if strings.TrimSpace(row.TriggerType) == "" || strings.TrimSpace(row.Source) == "" {
			return ErrInvalid
		}
		for _, doc := range []string{row.TriggerSpec, row.Arguments, row.Condition, row.Presentation} {
			if !objectJSON(doc) {
				return ErrInvalid
			}
		}
		replacement := row.ReplacesDefaultID != nil
		if replacement != (row.ReplacesDefaultVersion != nil) || replacement != (row.ReplacesDefaultFingerprint != nil) {
			return ErrInvalid
		}
		if replacement && (!nonempty(row.ReplacesDefaultID) || !nonempty(row.ReplacesDefaultVersion) || !nonempty(row.ReplacesDefaultFingerprint)) {
			return ErrInvalid
		}
		switch row.LayerRefKind {
		case "user":
			layer, ok := layers[row.LayerRef]
			if !ok || !sameWorkspace(layer.WorkspaceID, row.WorkspaceID) {
				return ErrInvalid
			}
		case "builtin":
			if !replacement || !strings.Contains(row.LayerRef, ".") || strings.TrimSpace(row.LayerRef) != row.LayerRef {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		switch row.Effect {
		case "execute":
			if !nonempty(row.CommandID) {
				return ErrInvalid
			}
		case "suppress":
			if row.CommandID != nil || strings.TrimSpace(row.Arguments) != "{}" || !replacement {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	}
	return nil
}

func objectJSON(value string) bool {
	trimmed := strings.TrimSpace(value)
	return len(trimmed) >= 2 && trimmed[0] == '{' && json.Valid([]byte(trimmed))
}
func nonempty(value *string) bool { return value != nil && strings.TrimSpace(*value) != "" }
func validID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}
func validScope(scope Scope) bool {
	return validID(scope.UserID) && (scope.WorkspaceID == nil || validID(*scope.WorkspaceID))
}
func inScope(workspace *string, scope Scope) bool {
	return workspace == nil || (scope.WorkspaceID != nil && validID(*workspace) && *workspace == *scope.WorkspaceID)
}
func sameWorkspace(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
func cloneScope(scope Scope) Scope {
	if scope.WorkspaceID != nil {
		value := *scope.WorkspaceID
		scope.WorkspaceID = &value
	}
	return scope
}
func cloneGenerations(rows []Generation) []Generation {
	result := append([]Generation(nil), rows...)
	for i := range result {
		if result[i].WorkspaceID != nil {
			value := *result[i].WorkspaceID
			result[i].WorkspaceID = &value
		}
	}
	return result
}
