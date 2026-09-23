package commandbindings

// LayerPresentationState is immutable metadata for presenting the effective
// state of a configured layer action. It belongs to the same snapshot as
// binding resolution.
type LayerPresentationState struct {
	LayerID       string
	Scope         string
	LayerEnabled  bool
	RuleEnabled   bool
	RuleMode      string
	RuleCondition string
	RuleSource    string
	ReviewStatus  string
	AlwaysActive  bool
	Contextual    bool
}

// WithLayerPresentationTargets attaches a detached map to this immutable
// configuration snapshot.
func (c *Configuration) WithLayerPresentationTargets(targets map[string]LayerPresentationState) *Configuration {
	if c == nil {
		return nil
	}
	clone := *c
	clone.layerPresentationTargets = make(map[string]LayerPresentationState, len(targets))
	for id, target := range targets {
		clone.layerPresentationTargets[id] = target
	}
	return &clone
}

// LayerPresentationTarget returns state metadata for a configured rule.
func (c *Configuration) LayerPresentationTarget(ruleID string) (LayerPresentationState, bool) {
	if c == nil {
		return LayerPresentationState{}, false
	}
	target, ok := c.layerPresentationTargets[ruleID]
	return target, ok
}
