package commandconfig

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	"assistente/internal/commandactivation"
	"github.com/google/uuid"
)

// ConfigImport é a operação auditável usada pelo pipeline de importação de
// command_layers. O valor também precisa estar na allowlist do host que monta
// MutationServiceConfig.Authorize; esta API não concede essa autorização.
const ConfigImport Operation = "config_import"

// ErrNoChanges identifica uma importação validada que não altera nenhuma
// linha. Diferenciar esse resultado de ErrInvalid permite ao chamador tratar
// reimportação Keep como idempotência sem transformar payload inválido,
// ownership recusado ou referência revogada em sucesso.
var ErrNoChanges = errors.New("importação de command_layers sem mudanças")

// ImportedSnapshot é o resultado final de um planejador confiável. Seu escopo
// e seus registros já foram derivados pelo host; o DTO não pode preencher
// owner, geração, grants ou claims como autoridade. Os conjuntos Touched são
// parte da prova do merge: eles discriminam as linhas que o import realmente
// substitui/remove das linhas que o builder preservou do destino.
type ImportedSnapshot struct {
	Snapshot          Snapshot
	TouchedLayerIDs   []string
	TouchedBindingIDs []string
	TouchedRuleIDs    []string
}

// PrepareImportedSnapshot usa apenas Layers, Bindings e ActivationRules. Os
// grants/claims fornecidos são deliberadamente descartados, enquanto o
// histórico já existente no destino é mantido e reconciliado pelo hook.
func (s *Store) PrepareImportedSnapshot(ctx context.Context, scope Scope, imported Snapshot, validate MutationValidator) (*PreparedMutation, error) {
	if s == nil || ctx == nil {
		return nil, ErrInvalid
	}
	before, err := s.Load(ctx, cloneScope(scope))
	if err != nil {
		return nil, err
	}
	return s.prepareImportedSnapshot(ctx, scope, before, ImportedSnapshot{
		Snapshot:          imported,
		TouchedLayerIDs:   snapshotIDs(imported.Layers),
		TouchedBindingIDs: snapshotBindingIDs(imported.Bindings),
		TouchedRuleIDs:    snapshotRuleIDs(imported.ActivationRules),
	}, validate)
}

func (s *Store) prepareImportedSnapshot(ctx context.Context, scope Scope, before Snapshot, imported ImportedSnapshot, validate MutationValidator) (*PreparedMutation, error) {
	if s == nil || ctx == nil || validate == nil || !validScope(scope) || imported.Snapshot.Scope.UserID != scope.UserID || !sameWorkspace(imported.Snapshot.Scope.WorkspaceID, scope.WorkspaceID) {
		return nil, ErrInvalid
	}
	if err := validateTouchedIDs(imported); err != nil {
		return nil, err
	}
	// Load inclui globais herdados ao ler um workspace. Essa visibilidade não
	// concede escrita global: o lote só pode tocar o escopo autorizado exato.
	for _, snapshot := range []Snapshot{before, imported.Snapshot} {
		for _, row := range snapshot.Layers {
			if containsID(imported.TouchedLayerIDs, row.ID) && !sameWorkspace(row.WorkspaceID, scope.WorkspaceID) {
				return nil, ErrInvalid
			}
		}
		for _, row := range snapshot.Bindings {
			if containsID(imported.TouchedBindingIDs, row.ID) && !sameWorkspace(row.WorkspaceID, scope.WorkspaceID) {
				return nil, ErrInvalid
			}
		}
		for _, row := range snapshot.ActivationRules {
			if containsID(imported.TouchedRuleIDs, row.ID) && !sameWorkspace(row.WorkspaceID, scope.WorkspaceID) {
				return nil, ErrInvalid
			}
		}
	}
	for _, layer := range imported.Snapshot.Layers {
		if layer.UserID != scope.UserID || layer.Source != "user" || !inScope(layer.WorkspaceID, scope) {
			return nil, ErrInvalid
		}
	}
	for _, binding := range imported.Snapshot.Bindings {
		if binding.UserID != scope.UserID || binding.Source != "user" || !inScope(binding.WorkspaceID, scope) {
			return nil, ErrInvalid
		}
	}
	for _, rule := range imported.Snapshot.ActivationRules {
		if rule.UserID != scope.UserID || rule.Source != "user" || !inScope(rule.WorkspaceID, scope) {
			return nil, ErrInvalid
		}
	}

	if err := proveUntouchedRows(before, imported); err != nil {
		return nil, err
	}
	after := cloneConfigSnapshot(before)
	after.Layers = append([]Layer(nil), imported.Snapshot.Layers...)
	// Timestamps são metadados do destino. Fixá-los antes da confirmação
	// impede que o ORM acrescente valores não presentes no diff assinado.
	now := time.Now().UTC()
	for i := range after.Layers {
		row := &after.Layers[i]
		if !containsID(imported.TouchedLayerIDs, row.ID) {
			continue
		}
		row.CreatedAt, row.UpdatedAt = now, now
		for _, old := range before.Layers {
			if old.ID != row.ID {
				continue
			}
			row.CreatedAt, row.UpdatedAt = old.CreatedAt, old.UpdatedAt
			if !reflect.DeepEqual(old, *row) {
				row.UpdatedAt = now
			}
			break
		}
	}
	after.Bindings = append([]Binding(nil), imported.Snapshot.Bindings...)
	slices.SortFunc(after.Layers, func(a, b Layer) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(after.Bindings, func(a, b Binding) int { return strings.Compare(a.ID, b.ID) })
	after.ActivationRules = cloneActivationRules(imported.Snapshot.ActivationRules)
	for i := range after.ActivationRules {
		if !containsID(imported.TouchedRuleIDs, after.ActivationRules[i].ID) {
			continue
		}
		// A portable import can never revive an event rule or carry a grant
		// produced in another session/instance. Re-enablement is a separate
		// confirmed operation after a new grant is issued by the host. Reject
		// the grant at this boundary instead of silently stripping an incoming
		// one; stripping is only safe for the explicit touched assertion.
		if hasGrantMetadata(after.ActivationRules[i]) {
			return nil, ErrInvalid
		}
		clearRuleGrant(&after.ActivationRules[i])
	}
	// Nunca copie grants/claims do arquivo. Grants e claims atuais permanecem
	// no snapshot para que projectActivationEffects possa revogar somente o
	// que a nova configuração tornou inválido, na mesma transação.
	projectActivationEffects(before, &after, ConfigImport, now)
	// Loaders/merges podem representar um conjunto vazio como nil ou como
	// slice vazia. Isso não é mudança de configuração: Keep repetido deve ser
	// no-op e não pode abrir decisão nem avançar a geração.
	if equalRows(before.Layers, after.Layers) && equalRows(before.Bindings, after.Bindings) && sameAggregateSnapshot(before, after) {
		return nil, ErrNoChanges
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
	return &PreparedMutation{
		store:  s,
		before: cloneConfigSnapshot(before),
		after:  cloneConfigSnapshot(after),
		diff: MutationDiff{
			MutationID:   id.String(),
			Operation:    ConfigImport,
			Scope:        cloneScope(scope),
			RevocationAt: now,
		},
	}, nil
}

func snapshotIDs(rows []Layer) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func snapshotBindingIDs(rows []Binding) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func snapshotRuleIDs(rows []commandactivation.Rule) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func validateTouchedIDs(imported ImportedSnapshot) error {
	for _, ids := range [][]string{imported.TouchedLayerIDs, imported.TouchedBindingIDs, imported.TouchedRuleIDs} {
		seen := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			if !validID(id) {
				return ErrInvalid
			}
			if _, ok := seen[id]; ok {
				return ErrInvalid
			}
			seen[id] = struct{}{}
		}
	}
	return nil
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func proveUntouchedRows(before Snapshot, imported ImportedSnapshot) error {
	layers := make(map[string]Layer, len(imported.Snapshot.Layers))
	for _, row := range imported.Snapshot.Layers {
		layers[row.ID] = row
	}
	for _, id := range imported.TouchedLayerIDs {
		if _, inIncoming := layers[id]; !inIncoming && !snapshotHasLayer(before.Layers, id) {
			return ErrInvalid
		}
	}
	for _, row := range before.Layers {
		if containsID(imported.TouchedLayerIDs, row.ID) {
			continue
		}
		candidate, ok := layers[row.ID]
		if !ok || !reflect.DeepEqual(row, candidate) {
			return ErrStale
		}
	}
	for _, row := range imported.Snapshot.Layers {
		if containsID(imported.TouchedLayerIDs, row.ID) {
			continue
		}
		found := false
		for _, prior := range before.Layers {
			if prior.ID == row.ID {
				found = true
				break
			}
		}
		if !found {
			return ErrInvalid
		}
	}

	bindings := make(map[string]Binding, len(imported.Snapshot.Bindings))
	for _, row := range imported.Snapshot.Bindings {
		bindings[row.ID] = row
	}
	for _, id := range imported.TouchedBindingIDs {
		if _, inIncoming := bindings[id]; !inIncoming && !snapshotHasBinding(before.Bindings, id) {
			return ErrInvalid
		}
	}
	for _, row := range before.Bindings {
		if containsID(imported.TouchedBindingIDs, row.ID) {
			continue
		}
		candidate, ok := bindings[row.ID]
		if !ok || !reflect.DeepEqual(row, candidate) {
			return ErrStale
		}
	}
	for _, row := range imported.Snapshot.Bindings {
		if containsID(imported.TouchedBindingIDs, row.ID) {
			continue
		}
		found := false
		for _, prior := range before.Bindings {
			if prior.ID == row.ID {
				found = true
				break
			}
		}
		if !found {
			return ErrInvalid
		}
	}

	rules := make(map[string]commandactivation.Rule, len(imported.Snapshot.ActivationRules))
	for _, row := range imported.Snapshot.ActivationRules {
		rules[row.ID] = row
	}
	for _, id := range imported.TouchedRuleIDs {
		if _, inIncoming := rules[id]; !inIncoming && !snapshotHasRule(before.ActivationRules, id) {
			return ErrInvalid
		}
	}
	for _, row := range before.ActivationRules {
		if containsID(imported.TouchedRuleIDs, row.ID) {
			continue
		}
		candidate, ok := rules[row.ID]
		if !ok || !reflect.DeepEqual(row, candidate) {
			return ErrStale
		}
	}
	for _, row := range imported.Snapshot.ActivationRules {
		if containsID(imported.TouchedRuleIDs, row.ID) {
			continue
		}
		found := false
		for _, prior := range before.ActivationRules {
			if prior.ID == row.ID {
				found = true
				break
			}
		}
		if !found {
			return ErrInvalid
		}
	}
	return nil
}

func snapshotHasLayer(rows []Layer, id string) bool {
	for _, row := range rows {
		if row.ID == id {
			return true
		}
	}
	return false
}

func snapshotHasBinding(rows []Binding, id string) bool {
	for _, row := range rows {
		if row.ID == id {
			return true
		}
	}
	return false
}

func snapshotHasRule(rows []commandactivation.Rule, id string) bool {
	for _, row := range rows {
		if row.ID == id {
			return true
		}
	}
	return false
}

// ImportedSnapshotProvider é uma porta interna de composição. É chamada
// somente depois que CompleteMutationService derivou e autorizou o scope a
// partir da sessão autenticada; não deve ser ligada diretamente ao payload da
// UI. O terceiro argumento é a leitura feita pelo Store do próprio serviço.
type ImportedSnapshotProvider func(context.Context, Scope, Snapshot) (ImportedSnapshot, error)

// ImportRevalidator revalida as referências e decisões do plano sob o gate
// exclusivo, imediatamente antes do commit. Não recebe conteúdo de uma UI.
type ImportRevalidator func(context.Context, Scope) error

// Import executa o mesmo percurso autenticado de Apply: preparação sob o
// gate, decisão fora dele, revalidação de sessão/versão/projeção e commit
// confirmado com CAS. O provider deve ser código confiável do host.
func (s *CompleteMutationService) Import(ctx context.Context, token string, workspace *string, provider ImportedSnapshotProvider, revalidate ImportRevalidator) (MutationDiff, error) {
	if s == nil || provider == nil || revalidate == nil {
		return MutationDiff{}, ErrInvalid
	}
	var importScope Scope
	return s.service.applyPreparedWithRevalidation(ctx, token, workspace, ConfigImport, func(ctx context.Context, scope Scope) (*PreparedMutation, error) {
		importScope = cloneScope(scope)
		current, err := s.service.config.Store.Load(ctx, importScope)
		if err != nil {
			return nil, err
		}
		imported, err := provider(ctx, importScope, current)
		if err != nil {
			return nil, err
		}
		return s.service.config.Store.prepareImportedSnapshot(ctx, scope, current, imported, s.service.config.Validate)
	}, func(ctx context.Context) error {
		return revalidate(ctx, cloneScope(importScope))
	})
}
