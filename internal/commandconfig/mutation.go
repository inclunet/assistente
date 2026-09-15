package commandconfig

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Operation string

const (
	LayerCreate    Operation = "layer_create"
	LayerUpdate    Operation = "layer_update"
	LayerDelete    Operation = "layer_delete"
	LayerEnable    Operation = "layer_enable"
	LayerDisable   Operation = "layer_disable"
	LayerRestore   Operation = "layer_restore"
	BindingCreate  Operation = "binding_create"
	BindingUpdate  Operation = "binding_update"
	BindingDelete  Operation = "binding_delete"
	BindingEnable  Operation = "binding_enable"
	BindingDisable Operation = "binding_disable"
	BindingRestore Operation = "binding_restore"
	ConfigRestore  Operation = "config_restore"
)

// Intent só descreve a mudança. Scope, validação, decisão e autoridade vêm do
// host. Create gera UUID no backend; IDs de registros existentes são assertions.
type MutationIntent struct {
	Operation    Operation
	ID           string
	Layer        *Layer
	Binding      *Binding
	LayerRefKind string // layer_restore: builtin ou user
}

type MutationDiff struct {
	MutationID                    string
	Operation                     Operation
	Scope                         Scope
	BeforeLayers, AfterLayers     []Layer
	BeforeBindings, AfterBindings []Binding
}

type MutationValidator func(context.Context, Snapshot) error

// PreparedMutation é privada, imutável e ligada ao Store. A configuração
// validada no preview nunca é reconstruída a partir do texto do diálogo.
type PreparedMutation struct {
	store         *Store
	before, after Snapshot
	diff          MutationDiff
}

func cloneConfigSnapshot(s Snapshot) Snapshot {
	s.Scope = cloneScope(s.Scope)
	s.Generations = cloneGenerations(s.Generations)
	s.Layers = append([]Layer{}, s.Layers...)
	for i := range s.Layers {
		s.Layers[i].WorkspaceID = cloneScope(Scope{WorkspaceID: s.Layers[i].WorkspaceID}).WorkspaceID
	}
	s.Bindings = append([]Binding{}, s.Bindings...)
	for i := range s.Bindings {
		s.Bindings[i] = cloneBinding(s.Bindings[i])
	}
	return s
}

func (p *PreparedMutation) Diff() MutationDiff {
	if p == nil {
		return MutationDiff{}
	}
	d := p.diff
	d.Scope = cloneScope(d.Scope)
	b, a := cloneConfigSnapshot(p.before), cloneConfigSnapshot(p.after)
	d.BeforeLayers, d.AfterLayers = b.Layers, a.Layers
	d.BeforeBindings, d.AfterBindings = b.Bindings, a.Bindings
	return d
}

// EnsureScope inicializa apenas contadores, sem binding/default/claim. O host
// deve verificar ownership do workspace antes; nenhuma leitura chama isto.
func (s *Store) EnsureScope(ctx context.Context, scope Scope) error {
	if s == nil || ctx == nil || !validScope(scope) {
		return ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		workspaces := []*string{nil}
		if scope.WorkspaceID != nil {
			workspaces = append(workspaces, scope.WorkspaceID)
		}
		for _, ws := range workspaces {
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			row := Generation{ID: id.String(), UserID: scope.UserID, WorkspaceID: ws, Generation: 1, UpdatedAt: time.Now().UTC()}
			if err := tx.Exec("INSERT INTO command_config_generations (id,user_id,workspace_id,generation,updated_at) VALUES (?,?,?,?,?) ON CONFLICT DO NOTHING", row.ID, row.UserID, row.WorkspaceID, row.Generation, row.UpdatedAt).Error; err != nil {
				return err
			}
		}
		_, err := readGenerations(tx, scope)
		return err
	})
}

func (s *Store) PrepareMutation(ctx context.Context, scope Scope, intent MutationIntent, validate MutationValidator) (*PreparedMutation, error) {
	if validate == nil {
		return nil, ErrInvalid
	}
	before, err := s.Load(ctx, scope)
	if err != nil {
		return nil, err
	}
	after := cloneConfigSnapshot(before)
	li, bi := -1, -1
	for i, l := range after.Layers {
		if l.ID == intent.ID && sameWorkspace(l.WorkspaceID, scope.WorkspaceID) {
			li = i
		}
	}
	for i, b := range after.Bindings {
		if b.ID == intent.ID && sameWorkspace(b.WorkspaceID, scope.WorkspaceID) {
			bi = i
		}
	}
	now := time.Now().UTC()
	switch intent.Operation {
	case LayerCreate:
		if intent.Layer == nil || intent.ID != "" || intent.Binding != nil || intent.Layer.ID != "" {
			return nil, ErrInvalid
		}
		l := *intent.Layer
		if (l.UserID != "" && l.UserID != scope.UserID) || (l.WorkspaceID != nil && !sameWorkspace(l.WorkspaceID, scope.WorkspaceID)) {
			return nil, ErrInvalid
		}
		id, e := uuid.NewV7()
		if e != nil {
			return nil, e
		}
		l.ID = id.String()
		l.UserID = scope.UserID
		l.WorkspaceID = cloneScope(scope).WorkspaceID
		l.Source = "user"
		l.CreatedAt = now
		l.UpdatedAt = now
		after.Layers = append(after.Layers, l)
	case LayerUpdate, LayerEnable, LayerDisable, LayerDelete:
		if li < 0 {
			return nil, ErrInvalid
		}
		l := after.Layers[li]
		switch intent.Operation {
		case LayerUpdate:
			if intent.Layer == nil || intent.Layer.ID != l.ID || intent.Layer.UserID != l.UserID || !sameWorkspace(intent.Layer.WorkspaceID, l.WorkspaceID) || intent.Layer.Enabled != l.Enabled {
				return nil, ErrInvalid
			}
			l.Name = intent.Layer.Name
			l.Description = intent.Layer.Description
			l.ResolutionPriority = intent.Layer.ResolutionPriority
		case LayerEnable:
			l.Enabled = true
		case LayerDisable:
			l.Enabled = false
		case LayerDelete:
			after.Layers = slices.Delete(after.Layers, li, li+1)
			after.Bindings = slices.DeleteFunc(after.Bindings, func(b Binding) bool {
				return b.LayerRefKind == "user" && b.LayerRef == l.ID && sameWorkspace(b.WorkspaceID, scope.WorkspaceID)
			})
		}
		if intent.Operation != LayerDelete {
			if reflect.DeepEqual(l, after.Layers[li]) {
				return nil, ErrInvalid
			}
			l.UpdatedAt = now
			after.Layers[li] = l
		}
	case BindingCreate:
		if intent.Binding == nil || intent.ID != "" || intent.Binding.ID != "" || intent.Layer != nil {
			return nil, ErrInvalid
		}
		b := cloneBinding(*intent.Binding)
		if (b.UserID != "" && b.UserID != scope.UserID) || (b.WorkspaceID != nil && !sameWorkspace(b.WorkspaceID, scope.WorkspaceID)) {
			return nil, ErrInvalid
		}
		id, e := uuid.NewV7()
		if e != nil {
			return nil, e
		}
		b.ID = id.String()
		b.UserID = scope.UserID
		b.WorkspaceID = cloneScope(scope).WorkspaceID
		b.Source = "user"
		after.Bindings = append(after.Bindings, b)
	case BindingUpdate, BindingEnable, BindingDisable, BindingDelete, BindingRestore:
		if bi < 0 {
			return nil, ErrInvalid
		}
		b := after.Bindings[bi]
		switch intent.Operation {
		case BindingUpdate:
			if intent.Binding == nil || intent.Binding.ID != b.ID || intent.Binding.UserID != b.UserID || !sameWorkspace(intent.Binding.WorkspaceID, b.WorkspaceID) || intent.Binding.LayerRefKind != b.LayerRefKind || intent.Binding.LayerRef != b.LayerRef {
				return nil, ErrInvalid
			}
			b = cloneBinding(*intent.Binding)
			b.Source = after.Bindings[bi].Source
		case BindingEnable:
			b.Enabled = true
		case BindingDisable:
			b.Enabled = false
		case BindingRestore:
			if b.ReplacesDefaultID == nil {
				return nil, ErrInvalid
			}
		}
		if intent.Operation == BindingDelete || intent.Operation == BindingRestore {
			after.Bindings = slices.Delete(after.Bindings, bi, bi+1)
		} else {
			after.Bindings[bi] = b
		}
	case LayerRestore:
		if intent.LayerRefKind != "builtin" && intent.LayerRefKind != "user" {
			return nil, ErrInvalid
		}
		if intent.LayerRefKind == "user" && li < 0 {
			return nil, ErrInvalid
		}
		after.Bindings = slices.DeleteFunc(after.Bindings, func(b Binding) bool {
			return sameWorkspace(b.WorkspaceID, scope.WorkspaceID) && b.LayerRefKind == intent.LayerRefKind && b.LayerRef == intent.ID
		})
	case ConfigRestore:
		if intent.ID != "" {
			return nil, ErrInvalid
		}
		after.Bindings = slices.DeleteFunc(after.Bindings, func(b Binding) bool { return sameWorkspace(b.WorkspaceID, scope.WorkspaceID) })
		after.Layers = slices.DeleteFunc(after.Layers, func(l Layer) bool { return sameWorkspace(l.WorkspaceID, scope.WorkspaceID) })
	default:
		return nil, ErrInvalid
	}
	slices.SortFunc(after.Layers, func(a, b Layer) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	slices.SortFunc(after.Bindings, func(a, b Binding) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	if reflect.DeepEqual(before.Layers, after.Layers) && reflect.DeepEqual(before.Bindings, after.Bindings) {
		return nil, ErrInvalid
	}
	if err := validateSnapshot(after); err != nil {
		return nil, err
	}
	if err := validate(ctx, cloneConfigSnapshot(after)); err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	p := &PreparedMutation{store: s, before: cloneConfigSnapshot(before), after: cloneConfigSnapshot(after), diff: MutationDiff{MutationID: id.String(), Operation: intent.Operation, Scope: cloneScope(scope)}}
	return p, nil
}

// applyMutationTx não faz commit: receipt/auditoria/grants/claims e configuração
// devem pertencer à mesma transação do chamador.
func (s *Store) applyMutationTx(ctx context.Context, tx *gorm.DB, p *PreparedMutation) (Generation, error) {
	if s == nil || p == nil || p.store != s || p.before.stamp == nil || p.before.stamp.store != s {
		return Generation{}, ErrInvalid
	}
	current, err := readGenerations(tx, p.before.Scope)
	if err != nil {
		return Generation{}, err
	}
	if !reflect.DeepEqual(current, p.before.Generations) {
		return Generation{}, ErrStale
	}
	var layers []Layer
	var bindings []Binding
	if err := scoped(tx, p.before.Scope).Order("id").Find(&layers).Error; err != nil {
		return Generation{}, err
	}
	if err := scoped(tx, p.before.Scope).Order("id").Find(&bindings).Error; err != nil {
		return Generation{}, err
	}
	if !equalRows(layers, p.before.Layers) || !equalRows(bindings, p.before.Bindings) {
		return Generation{}, ErrStale
	}
	var g Generation
	for _, candidate := range current {
		if sameWorkspace(candidate.WorkspaceID, p.before.Scope.WorkspaceID) {
			g = candidate
		}
	}
	if g.ID == "" || g.Generation == math.MaxInt64 {
		return Generation{}, ErrInvalid
	}
	result := tx.Model(&Generation{}).Where("id = ? AND user_id = ? AND generation = ?", g.ID, g.UserID, g.Generation).Updates(map[string]any{"generation": g.Generation + 1, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return Generation{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Generation{}, ErrStale
	}
	// Diff row-by-row preserves unrelated rows, names and UUID ownership. Never
	// upsert by PK alone: a create collision remains an error, not a takeover.
	for _, b := range p.before.Bindings {
		if !hasBinding(p.after.Bindings, b.ID) {
			if err := tx.Where("id = ? AND user_id = ?", b.ID, b.UserID).Delete(&Binding{}).Error; err != nil {
				return Generation{}, err
			}
		}
	}
	for _, l := range p.before.Layers {
		if !hasLayer(p.after.Layers, l.ID) {
			if err := tx.Where("id = ? AND user_id = ?", l.ID, l.UserID).Delete(&Layer{}).Error; err != nil {
				return Generation{}, err
			}
		}
	}
	for _, l := range p.after.Layers {
		var e error
		if !hasLayer(p.before.Layers, l.ID) {
			e = tx.Create(&l).Error
		} else {
			for _, old := range p.before.Layers {
				if old.ID == l.ID && !reflect.DeepEqual(old, l) {
					e = tx.Model(&Layer{}).Where("id = ? AND user_id = ?", l.ID, l.UserID).Select("name", "description", "enabled", "resolution_priority", "updated_at").Updates(l).Error
				}
			}
		}
		if e != nil {
			return Generation{}, e
		}
	}
	for _, b := range p.after.Bindings {
		var e error
		if !hasBinding(p.before.Bindings, b.ID) {
			e = tx.Create(&b).Error
		} else {
			for _, old := range p.before.Bindings {
				if old.ID == b.ID && !reflect.DeepEqual(old, b) {
					e = tx.Model(&Binding{}).Where("id = ? AND user_id = ?", b.ID, b.UserID).Select("trigger_type", "trigger_spec", "command_id", "arguments", "condition", "effect", "enabled", "resolution_priority", "replaces_default_id", "replaces_default_version", "replaces_default_fingerprint", "review_status", "presentation").Updates(b).Error
				}
			}
		}
		if e != nil {
			return Generation{}, e
		}
	}
	return g, ctx.Err()
}
func equalRows[T any](a, b []T) bool {
	return reflect.DeepEqual(append([]T{}, a...), append([]T{}, b...))
}
func hasBinding(rows []Binding, id string) bool {
	for _, r := range rows {
		if r.ID == id {
			return true
		}
	}
	return false
}
func hasLayer(rows []Layer, id string) bool {
	for _, r := range rows {
		if r.ID == id {
			return true
		}
	}
	return false
}

func mutationDocuments(p *PreparedMutation) (string, string, error) {
	b, e := json.Marshal(struct {
		Layers   []Layer
		Bindings []Binding
	}{p.before.Layers, p.before.Bindings})
	if e != nil {
		return "", "", e
	}
	a, e := json.Marshal(struct {
		Layers   []Layer
		Bindings []Binding
	}{p.after.Layers, p.after.Bindings})
	return string(b), string(a), e
}
