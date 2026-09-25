package commandbindings

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// WithoutDeltas devolve um snapshot novo sem os deltas identificados.
//
// A operação é pura: não persiste a configuração, não restaura uma camada ou
// todos os dados do usuário e não modifica o snapshot de origem. IDs referem-se
// aos deltas, e a validação ocorre antes de qualquer remoção.
func (c *Configuration) WithoutDeltas(ids []string) (*Configuration, error) {
	if c == nil {
		return nil, fmt.Errorf("configuração ausente")
	}

	removed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("ID de delta vazio")
		}
		if _, duplicate := removed[id]; duplicate {
			return nil, fmt.Errorf("ID de delta duplicado: %s", id)
		}
		removed[id] = struct{}{}
	}

	known := make(map[string]struct{})
	for _, deltas := range c.deltas {
		for _, delta := range deltas {
			known[delta.ID] = struct{}{}
		}
	}
	for id := range removed {
		if _, exists := known[id]; !exists {
			return nil, fmt.Errorf("delta desconhecido: %s", id)
		}
	}

	defaults := make([]Default, 0, len(c.defaults))
	defaultIDs := make([]string, 0, len(c.defaults))
	for id := range c.defaults {
		defaultIDs = append(defaultIDs, id)
	}
	slices.Sort(defaultIDs)
	for _, id := range defaultIDs {
		base := c.defaults[id]
		base.Candidate.Condition = maps.Clone(base.Candidate.Condition)
		defaults = append(defaults, base)
	}

	deltas := make([]Delta, 0)
	deltaIDs := make([]string, 0, len(c.deltas))
	for defaultID := range c.deltas {
		deltaIDs = append(deltaIDs, defaultID)
	}
	slices.Sort(deltaIDs)
	for _, defaultID := range deltaIDs {
		// A restauração é um novo snapshot. Ordenar também o grupo evita que
		// a ordem incidental de inserção altere a materialização de deltas
		// equivalentes após restore/rebase.
		group := slices.Clone(c.deltas[defaultID])
		slices.SortFunc(group, func(a, b Delta) int { return strings.Compare(a.ID, b.ID) })
		for _, delta := range group {
			if _, drop := removed[delta.ID]; drop {
				continue
			}
			delta.Condition = maps.Clone(delta.Condition)
			deltas = append(deltas, delta)
		}
	}

	custom := make([]Candidate, 0)
	for _, candidates := range c.custom.byTrigger {
		for _, candidate := range candidates {
			candidate.Condition = maps.Clone(candidate.Condition)
			custom = append(custom, candidate)
		}
	}
	slices.SortFunc(custom, func(a, b Candidate) int { return strings.Compare(a.ID, b.ID) })

	result, err := NewConfiguration(defaults, deltas, custom)
	if err != nil {
		return nil, err
	}
	result, err = result.WithLayerProvenance(c.layerProvenance)
	if err != nil {
		return nil, err
	}
	result.validUntil = c.validUntil
	result.presentation = c.presentation.clone()
	result.layerPresentationTargets = maps.Clone(c.layerPresentationTargets)
	if result.presentation != nil {
		for id := range removed {
			delete(result.presentation.byBindingID, id)
		}
	}
	if c.adjustments == nil {
		result.adjustments = nil
		return result, nil
	}
	result.adjustments = make([]Adjustment, 0, len(c.adjustments))
	for _, adjustment := range c.adjustments {
		if _, drop := removed[adjustment.DeltaID]; !drop {
			result.adjustments = append(result.adjustments, adjustment)
		}
	}
	return result, nil
}
