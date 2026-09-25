package commandbindings

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ConflictWitness é uma prova reproduzível de que um acionador, sob os fatos
// fornecidos, terminou em conflito. Facts contém somente valores normalizados;
// nenhum handler, provider ou comando é consultado durante o diagnóstico.
type ConflictWitness struct {
	Trigger    string
	Facts      Facts
	BindingIDs []string
}

// ErrConflictSearchLimit indica que a partição exata de fatos excedeu o limite
// do diagnóstico. O diagnóstico não retorna uma lista parcial: o chamador deve
// tratá-lo como falha fechada e pedir uma análise mais estreita.
var ErrConflictSearchLimit = errors.New("diagnóstico de conflitos excedeu o limite de combinações")

// DefaultConflictSearchLimit limita o produto cartesiano dos valores de
// condição. Cada campo recebe uma classe para ausência e uma para cada valor
// observado; portanto, a enumeração não escolhe um "pior caso" heurístico.
const DefaultConflictSearchLimit uint64 = 4096

// CheckConflicts enumera a partição finita exata dos fatos de igualdade que
// aparecem na configuração e retorna todos os conflitos observáveis fora de
// diálogo. A resolução usada é a mesma de Resolve, incluindo escopo,
// precedência, supressão, deltas e needs_review. A ordem de testemunhas e IDs é
// estável. Contexto é somente cancelamento; nada é executado.
func (c *Configuration) CheckConflicts(ctx context.Context) ([]ConflictWitness, error) {
	if c == nil || ctx == nil {
		return nil, errors.New("configuração ou contexto ausente")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	triggers := conflictTriggers(c)
	witnesses := make([]ConflictWitness, 0)
	seen := make(map[string]struct{})
	for _, trigger := range triggers {
		fields := conflictFields(c, trigger)
		values, count, err := conflictFactValues(c, trigger, fields)
		if err != nil {
			return nil, err
		}
		if count > DefaultConflictSearchLimit {
			return nil, ErrConflictSearchLimit
		}
		facts := make(Facts, len(values))
		var visit func(int) error
		visit = func(index int) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if index == len(fields) {
				result, err := c.Resolve(trigger, facts, nil)
				if err != nil {
					return err
				}
				if result.Status != Conflict {
					return nil
				}
				ids := slices.Clone(result.BindingIDs)
				slices.Sort(ids)
				key := trigger + "\x00" + conflictFactsKey(facts) + "\x00" + strings.Join(ids, "\x00")
				if _, exists := seen[key]; exists {
					return nil
				}
				seen[key] = struct{}{}
				witnesses = append(witnesses, ConflictWitness{Trigger: trigger, Facts: maps.Clone(facts), BindingIDs: ids})
				return nil
			}
			for _, value := range values[index] {
				if value == nil {
					delete(facts, fields[index])
				} else {
					facts[fields[index]] = value
				}
				if err := visit(index + 1); err != nil {
					return err
				}
			}
			return nil
		}
		if err := visit(0); err != nil {
			return nil, err
		}
	}

	slices.SortFunc(witnesses, func(a, b ConflictWitness) int {
		if a.Trigger != b.Trigger {
			return strings.Compare(a.Trigger, b.Trigger)
		}
		if keyA, keyB := conflictFactsKey(a.Facts), conflictFactsKey(b.Facts); keyA != keyB {
			return strings.Compare(keyA, keyB)
		}
		return strings.Compare(strings.Join(a.BindingIDs, "\x00"), strings.Join(b.BindingIDs, "\x00"))
	})
	return witnesses, nil
}

func conflictTriggers(c *Configuration) []string {
	set := make(map[string]struct{}, len(c.byTrigger))
	for trigger := range c.byTrigger {
		set[trigger] = struct{}{}
	}
	if c.custom != nil {
		for trigger := range c.custom.byTrigger {
			set[trigger] = struct{}{}
		}
	}
	triggers := make([]string, 0, len(set))
	for trigger := range set {
		triggers = append(triggers, trigger)
	}
	slices.Sort(triggers)
	return triggers
}

func conflictFields(c *Configuration, trigger string) []Field {
	set := make(map[Field]struct{})
	for _, id := range c.byTrigger[trigger] {
		if base, ok := c.defaults[id]; ok {
			for field := range base.Candidate.Condition {
				set[field] = struct{}{}
			}
			for _, condition := range base.Candidate.LayerConditions {
				for field := range condition {
					set[field] = struct{}{}
				}
			}
		}
		for _, delta := range c.deltas[id] {
			for field := range delta.Condition {
				set[field] = struct{}{}
			}
			for _, condition := range delta.LayerConditions {
				for field := range condition {
					set[field] = struct{}{}
				}
			}
		}
	}
	if c.custom != nil {
		for _, candidate := range c.custom.byTrigger[trigger] {
			for field := range candidate.Condition {
				set[field] = struct{}{}
			}
			for _, condition := range candidate.LayerConditions {
				for field := range condition {
					set[field] = struct{}{}
				}
			}
		}
	}
	// AppFocused também altera a elegibilidade de Foreground, mesmo quando não
	// aparece em uma cláusula de condição.
	for _, candidate := range conflictCandidates(c, trigger) {
		if candidate.Scope == Foreground {
			set[AppFocused] = struct{}{}
			break
		}
	}
	fields := make([]Field, 0, len(set))
	for field := range set {
		fields = append(fields, field)
	}
	slices.SortFunc(fields, func(a, b Field) int { return strings.Compare(string(a), string(b)) })
	return fields
}

func conflictCandidates(c *Configuration, trigger string) []Candidate {
	result := []Candidate{}
	if c.custom != nil {
		result = slices.Clone(c.custom.byTrigger[trigger])
	}
	for _, id := range c.byTrigger[trigger] {
		base, exists := c.defaults[id]
		if exists && base.Candidate.Trigger == trigger {
			result = append(result, base.Candidate)
		}
		for _, delta := range c.deltas[id] {
			if !exists || delta.Trigger != trigger || delta.Effect != Execute {
				continue
			}
			candidate, err := withDelta(base.Candidate, delta)
			if err == nil {
				result = append(result, candidate)
			}
		}
	}
	return result
}

func conflictFactValues(c *Configuration, trigger string, fields []Field) ([][]any, uint64, error) {
	result := make([][]any, len(fields))
	count := uint64(1)
	for i, field := range fields {
		values := make(map[string]any)
		add := func(facts Facts) {
			if value, ok := facts[field]; ok {
				values[fmt.Sprintf("%T:%v", value, value)] = value
			}
		}
		for _, id := range c.byTrigger[trigger] {
			if base, ok := c.defaults[id]; ok {
				add(base.Candidate.Condition)
				for _, condition := range base.Candidate.LayerConditions {
					add(condition)
				}
			}
			for _, delta := range c.deltas[id] {
				add(delta.Condition)
				for _, condition := range delta.LayerConditions {
					add(condition)
				}
			}
		}
		if c.custom != nil {
			for _, candidate := range c.custom.byTrigger[trigger] {
				add(candidate.Condition)
				for _, condition := range candidate.LayerConditions {
					add(condition)
				}
			}
		}
		ordered := make([]string, 0, len(values))
		for key := range values {
			ordered = append(ordered, key)
		}
		slices.Sort(ordered)
		result[i] = []any{nil}
		for _, key := range ordered {
			result[i] = append(result[i], values[key])
		}
		if field == AppFocused {
			// Foreground eligibility distinguishes false from true even without
			// an explicit condition. Keeping all three classes also covers a
			// foreground candidate with an AppFocused condition.
			result[i] = []any{nil, false, true}
		}
		if count > ^uint64(0)/uint64(len(result[i])) {
			return nil, ^uint64(0), ErrConflictSearchLimit
		}
		count *= uint64(len(result[i]))
	}
	return result, count, nil
}

func conflictFactsKey(facts Facts) string {
	fields := make([]Field, 0, len(facts))
	for field := range facts {
		fields = append(fields, field)
	}
	slices.SortFunc(fields, func(a, b Field) int { return strings.Compare(string(a), string(b)) })
	var builder strings.Builder
	for _, field := range fields {
		fmt.Fprintf(&builder, "%s=%T:%v\x00", field, facts[field], facts[field])
	}
	return builder.String()
}
