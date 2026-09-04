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
	registry           *tools.Registry
	matcher            ToolPolicyMatcher
	structured         bool
	legacyAllPreloaded bool
	disabled           bool
	unavailable        bool
}

func (p *ToolSelectionPolicy) ResolveEffectiveToolPolicy(cfg ProfileToolConfig) EffectiveToolPolicy {
	policy := EffectiveToolPolicy{
		states:   map[string]ToolPolicyState{},
		registry: p.registry,
		disabled: cfg.DisableTools,
	}
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
		policy.structured = true
		policy.matcher = NewToolPolicyMatcher(configured, cfg.ToolPolicyDefault)
		for _, name := range names {
			policy.states[name] = policy.matcher.Resolve(p.toolPolicyTarget(name)).State
		}
		policy.ensureCatalogForOnDemandTools(configured)
		policy.applyRuntimeTools(cfg.RuntimeTools, true)
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
			policy.applyRuntimeTools(cfg.RuntimeTools, true)
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
		policy.applyRuntimeTools(cfg.RuntimeTools, true)
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
	policy.applyRuntimeTools(cfg.RuntimeTools, allowRuntime)
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
	if p.structured {
		return p.matcher.Resolve(p.target(name)).State
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
	registered := p.registry.Names()
	names := make([]string, 0, len(registered))
	for _, name := range registered {
		if p.State(name) == ToolPolicyPreloaded {
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
	registered := p.registry.Names()
	names := make([]string, 0, len(registered))
	for _, name := range registered {
		state := p.State(name)
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

func (p *EffectiveToolPolicy) applyRuntimeTools(runtimeTools []string, allow bool) {
	if !allow {
		return
	}
	for _, name := range runtimeTools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if p.structured {
			match := p.matcher.Resolve(p.target(name))
			// RuntimeTools é uma autorização explícita do control-plane (D8 da
			// AEP-0081), não uma elevação causada pelo default/wildcard do
			// perfil. Bloqueios configurados continuam soberanos; DeniedOptIn
			// apenas registra que o matcher, isoladamente, não autorizou a tool.
			if match.Explicit && match.State == ToolPolicyDisabled && !match.DeniedOptIn {
				continue
			}
		}
		if _, ok := p.states[name]; ok || p.legacyAllPreloaded {
			p.states[name] = ToolPolicyPreloaded
		}
	}
}

func (p *ToolSelectionPolicy) toolPolicyTarget(name string) ToolPolicyTarget {
	if p.registry == nil {
		return ToolPolicyTarget{Name: name}
	}
	target := ToolPolicyTarget{Name: name, OptIn: p.registry.IsOptIn(name)}
	tool, ok := p.registry.Get(name)
	if !ok {
		return target
	}
	// Pontes MCP usam o namespace como fonte de seleção. Não aplique nelas o
	// package "basic" de fallback reservado a builtins sem metadados.
	if !strings.HasPrefix(name, "mcp_") {
		target.Package = tools.CatalogMetadataForTool(tool).Package
	}
	return target
}

func (p EffectiveToolPolicy) target(name string) ToolPolicyTarget {
	if p.registry == nil {
		return ToolPolicyTarget{Name: name}
	}
	target := ToolPolicyTarget{Name: name, OptIn: p.registry.IsOptIn(name)}
	tool, ok := p.registry.Get(name)
	if ok && !strings.HasPrefix(name, "mcp_") {
		target.Package = tools.CatalogMetadataForTool(tool).Package
	}
	return target
}

// normalizeToolPolicyMap apara os nomes e descarta os vazios, para que o mapa
// consultado durante a resolução tenha as mesmas chaves que a aplicação usa.
func normalizeToolPolicyMap(configured map[string]string) map[string]string {
	if len(configured) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(configured))
	for name, state := range configured {
		selector, ok := ParseToolPolicySelector(name)
		if !ok {
			continue
		}
		// Duas chaves cruas podem virar o mesmo nome ("read_file" e
		// " read_file "). A ordem de iteração do map em Go não é estável, então
		// sem desempate o estado aplicado sairia no sorteio. Vence o mais
		// restritivo, que é a escolha segura e reproduzível.
		name = selector.Canonical
		if existing, ok := normalized[name]; ok {
			if toolPolicyStateRank(normalizeToolPolicyState(state)) >= toolPolicyStateRank(normalizeToolPolicyState(existing)) {
				continue
			}
		}
		normalized[name] = state
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
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
	// Uma entrada configurada pode estar temporariamente fora do registry (por
	// exemplo, uma tool MCP indisponível). Manter o catálogo carregado não
	// concede essa capability ausente e permite que ela volte a ser descoberta
	// quando reaparecer. Também mantém a resolução alinhada com o editor, que
	// preserva políticas de tools fora do grid.
	for name, state := range configured {
		if name != tools.ToolCatalogName && normalizeToolPolicyState(state) == ToolPolicyOnDemand {
			if _, ok := p.states[tools.ToolCatalogName]; ok {
				p.states[tools.ToolCatalogName] = ToolPolicyPreloaded
				return
			}
		}
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

// toolPolicyStateRank ordena do mais restritivo ao mais permissivo.
func toolPolicyStateRank(state ToolPolicyState) int {
	switch state {
	case ToolPolicyDisabled:
		return 0
	case ToolPolicyOnDemand:
		return 1
	default:
		return 2
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
