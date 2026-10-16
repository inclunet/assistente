package commandbindings

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// ExecutionDependency is an immutable witness for the binding resolution
// already admitted by an execution. It is intentionally limited to pure
// resolver state: callers add the authenticated principal/epoch and context
// identity to their own admission proof.
type ExecutionDependency struct {
	trigger    string
	facts      Facts
	dialog     *DialogScope
	result     Result
	selected   []Candidate
	provenance []LayerProvenance
}

// CaptureExecutionDependency pins the complete resolution result and the
// provenance of the layers that contributed to it. Inputs and slices are
// detached so the witness can safely be retained by an admitted execution.
func (c *Configuration) CaptureExecutionDependency(trigger string, facts Facts, dialog *DialogScope, result Result) (*ExecutionDependency, error) {
	if c == nil || trigger == "" || result.Status != Selected || result.CommandID == "" || result.ExecutionScopeKey == "" {
		return nil, fmt.Errorf("testemunho de resolução incompleto")
	}
	resolved, err := c.Resolve(trigger, facts, dialog)
	if err != nil || !reflect.DeepEqual(resolved, result) {
		return nil, fmt.Errorf("resolução não corresponde ao snapshot")
	}
	selected, err := c.selectedCandidates(trigger, facts, dialog, result.BindingIDs)
	if err != nil || len(selected) != len(result.BindingIDs) {
		return nil, fmt.Errorf("candidatos selecionados não correspondem ao snapshot")
	}
	return &ExecutionDependency{
		trigger: trigger, facts: cloneFacts(facts), dialog: cloneDialog(dialog), result: cloneResult(result), selected: cloneCandidates(selected),
		provenance: cloneLayerProvenance(c.LayerProvenance(result.LayerRefs)),
	}, nil
}

// UnaffectedBy reports whether the same captured intention, context and
// authority still resolve identically against next. Any incomplete witness,
// resolver error, precedence/suppression change, changed target/binding/layer,
// or changed layer provenance fails closed.
func (d *ExecutionDependency) UnaffectedBy(next *Configuration) bool {
	if d == nil || next == nil {
		return false
	}
	resolved, err := next.Resolve(d.trigger, cloneFacts(d.facts), cloneDialog(d.dialog))
	if err != nil || !reflect.DeepEqual(resolved, d.result) {
		return false
	}
	selected, err := next.selectedCandidates(d.trigger, d.facts, cloneDialog(d.dialog), d.result.BindingIDs)
	if err != nil || !reflect.DeepEqual(selected, d.selected) {
		return false
	}
	return reflect.DeepEqual(cloneLayerProvenance(next.LayerProvenance(d.result.LayerRefs)), d.provenance)
}

// Equivalent reports whether two independently captured witnesses identify
// the same immutable trigger, input context, selected candidates and source
// provenance. It is used to verify admission revalidation has not changed the
// meaning of the original witness.
func (d *ExecutionDependency) Equivalent(other *ExecutionDependency) bool {
	return d != nil && other != nil && reflect.DeepEqual(d, other)
}

func (c *Configuration) selectedCandidates(trigger string, facts Facts, dialog *DialogScope, ids []string) ([]Candidate, error) {
	if c == nil {
		return nil, fmt.Errorf("configuração ausente")
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	var all []Candidate
	if c.custom != nil {
		all = append(all, c.custom.byTrigger[trigger]...)
	}
	for _, id := range c.byTrigger[trigger] {
		base, exists := c.defaults[id]
		removed := false
		for _, delta := range c.deltas[id] {
			if !exists || trigger != base.Candidate.Trigger || delta.ReviewStatus == NeedsReview {
				continue
			}
			candidate, err := withDelta(base.Candidate, delta)
			if err != nil {
				return nil, err
			}
			if !eligible(candidate, facts, dialog) {
				continue
			}
			removed = true
			if delta.Effect == Execute {
				all = append(all, candidate)
			}
		}
		if exists && trigger == base.Candidate.Trigger && !removed {
			all = append(all, base.Candidate)
		}
	}
	byID := make(map[string]Candidate, len(all))
	for _, candidate := range all {
		if wanted[candidate.ID] {
			byID[candidate.ID] = candidate
		}
	}
	selected := make([]Candidate, 0, len(ids))
	for _, id := range ids {
		candidate, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("binding selecionado ausente: %s", id)
		}
		selected = append(selected, candidate)
	}
	slices.SortFunc(selected, func(a, b Candidate) int { return strings.Compare(a.ID, b.ID) })
	return cloneCandidates(selected), nil
}

func cloneCandidates(candidates []Candidate) []Candidate {
	if candidates == nil {
		return nil
	}
	cloned := make([]Candidate, len(candidates))
	for i, candidate := range candidates {
		cloned[i] = candidate
		cloned[i].Condition = cloneFacts(candidate.Condition)
		cloned[i].LayerConditions = cloneLayerConditions(candidate.LayerConditions)
	}
	return cloned
}

func cloneLayerProvenance(entries []LayerProvenance) []LayerProvenance {
	if entries == nil {
		return nil
	}
	cloned := make([]LayerProvenance, len(entries))
	for i, entry := range entries {
		cloned[i] = LayerProvenance{SourceID: entry.SourceID}
		if entry.Provenance != nil {
			cloned[i].Provenance = append([]byte(nil), entry.Provenance...)
		}
	}
	return cloned
}

func cloneFacts(facts Facts) Facts {
	if facts == nil {
		return nil
	}
	clone := make(Facts, len(facts))
	for field, value := range facts {
		clone[field] = value
	}
	return clone
}

func cloneDialog(dialog *DialogScope) *DialogScope {
	if dialog == nil {
		return nil
	}
	clone := *dialog
	clone.AllowedCommandIDs = slices.Clone(dialog.AllowedCommandIDs)
	clone.AllowedTriggers = slices.Clone(dialog.AllowedTriggers)
	return &clone
}

func cloneResult(result Result) Result {
	result.BindingIDs = slices.Clone(result.BindingIDs)
	result.LayerRefs = slices.Clone(result.LayerRefs)
	return result
}
