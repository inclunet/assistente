package chat

import (
	"sort"
	"strings"

	"assistente/internal/tools"
)

type ToolPolicyState string

const (
	ToolPolicyDisabled  ToolPolicyState = "disabled"
	ToolPolicyOnDemand  ToolPolicyState = "on_demand"
	ToolPolicyPreloaded ToolPolicyState = "preloaded"
)

type EffectiveToolPolicy struct {
	states             map[string]ToolPolicyState
	legacyAllPreloaded bool
	disabled           bool
	unavailable        bool
}

func (p *ToolSelectionPolicy) ResolveEffectiveToolPolicy(cfg ProfileToolConfig) EffectiveToolPolicy {
	policy := EffectiveToolPolicy{states: map[string]ToolPolicyState{}, disabled: cfg.DisableTools}
	if p.registry == nil {
		policy.unavailable = true
		return policy
	}
	if cfg.DisableTools {
		return policy
	}

	names := p.registry.Names()
	// Uma chave com espaços é aceita ao aplicar o mapa, mas some em qualquer
	// consulta direta. Normalizar antes de decidir mantém uma negação explícita
	// valendo em toda a resolução.
	configured := normalizeToolPolicyMap(cfg.ToolPolicy)
	if len(configured) > 0 || strings.TrimSpace(cfg.ToolPolicyDefault) != "" {
		// Sozinho, o default não vale para perfil que ainda descreve as tools
		// pela allowlist legada: ele diria "on_demand" para nomes que a
		// allowlist bloqueia, abrindo capability que ninguém escolheu. Enquanto
		// a allowlist não virar tool_policy explícita, ela continua soberana.
		if len(configured) == 0 && cfg.EnabledTools != nil {
			p.applyLegacyAllowlist(&policy, names, cfg)
			return policy
		}
		defaultState := normalizeToolPolicyDefault(cfg.ToolPolicyDefault)
		for _, name := range names {
			state := defaultState
			if p.registry.IsOptIn(name) {
				state = ToolPolicyDisabled
			}
			policy.states[name] = state
		}
		for name, state := range configured {
			if !p.registry.Has(name) {
				continue
			}
			policy.states[name] = normalizeToolPolicyState(state)
		}
		policy.ensureCatalogForOnDemandTools(configured)
		policy.applyRuntimeTools(cfg.RuntimeTools, true, explicitDisabledToolPolicyNames(configured))
		return policy
	}

	if cfg.EnabledTools == nil {
		if !p.registry.Has(tools.ToolCatalogName) {
			policy.legacyAllPreloaded = true
			for _, name := range names {
				if p.registry.IsOptIn(name) {
					policy.states[name] = ToolPolicyDisabled
					continue
				}
				policy.states[name] = ToolPolicyPreloaded
			}
			policy.applyRuntimeTools(cfg.RuntimeTools, true, nil)
			return policy
		}
		for _, name := range names {
			if p.registry.IsOptIn(name) {
				policy.states[name] = ToolPolicyDisabled
				continue
			}
			policy.states[name] = ToolPolicyOnDemand
		}
		policy.states[tools.ToolCatalogName] = ToolPolicyPreloaded
		policy.applyRuntimeTools(cfg.RuntimeTools, true, nil)
		return policy
	}

	p.applyLegacyAllowlist(&policy, names, cfg)
	return policy
}

func (p *ToolSelectionPolicy) applyLegacyAllowlist(policy *EffectiveToolPolicy, names []string, cfg ProfileToolConfig) {
	for _, name := range names {
		policy.states[name] = ToolPolicyDisabled
	}
	allowRuntime := len(cfg.EnabledTools) > 0
	for _, name := range cfg.EnabledTools {
		name = strings.TrimSpace(name)
		if name == "" || !p.registry.Has(name) {
			continue
		}
		policy.states[name] = ToolPolicyPreloaded
	}
	policy.applyRuntimeTools(cfg.RuntimeTools, allowRuntime, nil)
}

func (p EffectiveToolPolicy) State(name string) ToolPolicyState {
	if p.disabled {
		return ToolPolicyDisabled
	}
	if p.legacyAllPreloaded {
		if state, ok := p.states[name]; ok {
			return state
		}
		return ToolPolicyPreloaded
	}
	if state, ok := p.states[name]; ok {
		return state
	}
	return ToolPolicyDisabled
}

func (p EffectiveToolPolicy) AllowsRuntimeLoad(name string) bool {
	state := p.State(name)
	return state == ToolPolicyOnDemand || state == ToolPolicyPreloaded
}

func (p EffectiveToolPolicy) IsVisibleInCatalog(name string) bool {
	return p.AllowsRuntimeLoad(name)
}

func (p EffectiveToolPolicy) PreloadedNames() []string {
	if p.unavailable {
		return nil
	}
	if p.disabled {
		return []string{}
	}
	if p.legacyAllPreloaded {
		return nil
	}
	names := make([]string, 0, len(p.states))
	for name, state := range p.states {
		if state == ToolPolicyPreloaded {
			names = append(names, name)
		}
	}
	sortToolPolicyNames(names)
	return names
}

func (p EffectiveToolPolicy) CatalogVisibleNames() []string {
	if p.unavailable {
		return nil
	}
	if p.disabled {
		return []string{}
	}
	if p.legacyAllPreloaded {
		return nil
	}
	names := make([]string, 0, len(p.states))
	for name, state := range p.states {
		if state == ToolPolicyOnDemand || state == ToolPolicyPreloaded {
			names = append(names, name)
		}
	}
	sortToolPolicyNames(names)
	return names
}

func (p EffectiveToolPolicy) NativePreloadedAllowlist() []string {
	return p.PreloadedNames()
}

func (p *EffectiveToolPolicy) applyRuntimeTools(runtimeTools []string, allow bool, explicitlyDisabled map[string]struct{}) {
	if !allow {
		return
	}
	for _, name := range runtimeTools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, blocked := explicitlyDisabled[name]; blocked {
			continue
		}
		if _, ok := p.states[name]; ok || p.legacyAllPreloaded {
			p.states[name] = ToolPolicyPreloaded
		}
	}
}

// normalizeToolPolicyMap apara os nomes e descarta os vazios, para que o mapa
// consultado durante a resolução tenha as mesmas chaves que a aplicação usa.
func normalizeToolPolicyMap(configured map[string]string) map[string]string {
	if len(configured) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(configured))
	for name, state := range configured {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		normalized[name] = state
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func explicitDisabledToolPolicyNames(configured map[string]string) map[string]struct{} {
	if len(configured) == 0 {
		return nil
	}
	disabled := make(map[string]struct{})
	for name, state := range configured {
		if normalizeToolPolicyState(state) == ToolPolicyDisabled {
			disabled[name] = struct{}{}
		}
	}
	if len(disabled) == 0 {
		return nil
	}
	return disabled
}

func (p *EffectiveToolPolicy) ensureCatalogForOnDemandTools(configured map[string]string) {
	if p.State(tools.ToolCatalogName) == ToolPolicyPreloaded {
		return
	}
	if normalizeToolPolicyState(configured[tools.ToolCatalogName]) == ToolPolicyDisabled {
		if _, ok := configured[tools.ToolCatalogName]; ok {
			return
		}
	}
	if p.State(tools.ToolCatalogName) == ToolPolicyOnDemand {
		p.states[tools.ToolCatalogName] = ToolPolicyPreloaded
		return
	}
	if _, ok := p.states[tools.ToolCatalogName]; !ok {
		for name, state := range p.states {
			if state == ToolPolicyOnDemand {
				p.states[name] = ToolPolicyPreloaded
			}
		}
		return
	}
	for name, state := range p.states {
		if name == tools.ToolCatalogName {
			continue
		}
		if state == ToolPolicyOnDemand {
			p.states[tools.ToolCatalogName] = ToolPolicyPreloaded
			return
		}
	}
}

func normalizeToolPolicyState(state string) ToolPolicyState {
	switch ToolPolicyState(strings.TrimSpace(state)) {
	case ToolPolicyOnDemand:
		return ToolPolicyOnDemand
	case ToolPolicyPreloaded:
		return ToolPolicyPreloaded
	default:
		return ToolPolicyDisabled
	}
}

func normalizeToolPolicyDefault(state string) ToolPolicyState {
	if ToolPolicyState(strings.TrimSpace(state)) == ToolPolicyOnDemand {
		return ToolPolicyOnDemand
	}
	return ToolPolicyDisabled
}

func sortToolPolicyNames(names []string) {
	sort.SliceStable(names, func(i, j int) bool {
		left, right := names[i], names[j]
		if toolPolicyNameRank(left) != toolPolicyNameRank(right) {
			return toolPolicyNameRank(left) < toolPolicyNameRank(right)
		}
		return left < right
	})
}

func toolPolicyNameRank(name string) int {
	switch name {
	case tools.ToolCatalogName:
		return 0
	case tools.LoadSkillName:
		return 1
	default:
		return 10
	}
}
