package chat

import (
	"strings"

	mcplib "assistente/internal/mcp"
)

// ToolPolicyTarget reúne os atributos usados pela gramática de seletores.
// Package deve vir de tools.CatalogMetadata para builtins e ficar vazio para
// tools MCP, cuja seleção é feita pelo namespace canônico.
type ToolPolicyTarget struct {
	Name    string
	Package string
	OptIn   bool
}

type ToolPolicySelectorKind int

const (
	ToolPolicySelectorLiteral ToolPolicySelectorKind = iota
	ToolPolicySelectorAllNative
	ToolPolicySelectorAllMCP
	ToolPolicySelectorAllPackages
	ToolPolicySelectorMCPServer
	ToolPolicySelectorPackage
)

// ToolPolicySelector é a representação compilada e reutilizável da gramática
// de tool_policy. A issue #630 pode usá-la para expandir load sem reimplementar
// aliases, namespaces ou precedência.
type ToolPolicySelector struct {
	Canonical string
	Kind      ToolPolicySelectorKind
	Value     string
}

// ParseToolPolicySelector aceita literais e os wildcards documentados:
// *, mcp/*, mcp/<slug>/*, mcp:<slug>/*, package/*,
// package/<pacote>/* e <pacote>/*. Também aceita a forma canônica interna
// mcp_<slug>__* (e mcp_*__*).
func ParseToolPolicySelector(raw string) (ToolPolicySelector, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ToolPolicySelector{}, false
	}
	if value == "*" {
		return ToolPolicySelector{Canonical: "*", Kind: ToolPolicySelectorAllNative}, true
	}
	if value == "mcp/*" || value == "mcp_*__*" {
		return ToolPolicySelector{Canonical: "mcp/*", Kind: ToolPolicySelectorAllMCP}, true
	}
	if value == "package/*" {
		return ToolPolicySelector{Canonical: "package/*", Kind: ToolPolicySelectorAllPackages}, true
	}

	if slug, ok := parseDelimitedSelector(value, "mcp/", "/*"); ok {
		return mcpServerSelector(slug)
	}
	if slug, ok := parseDelimitedSelector(value, "mcp:", "/*"); ok {
		return mcpServerSelector(slug)
	}
	// Compatibilidade com a grafia mcp__<slug>__* usada em documentação
	// histórica, embora o nome executável atual seja mcp_<slug>__<tool>.
	if slug, ok := parseDelimitedSelector(value, "mcp__", "__*"); ok {
		return mcpServerSelector(slug)
	}
	if slug, ok := parseDelimitedSelector(value, "mcp_", "__*"); ok {
		return mcpServerSelector(slug)
	}
	if pkg, ok := parseDelimitedSelector(value, "package/", "/*"); ok {
		return packageSelector(pkg)
	}
	if pkg, ok := parseDelimitedSelector(value, "", "/*"); ok && !strings.Contains(pkg, "/") {
		return packageSelector(pkg)
	}

	return ToolPolicySelector{
		Canonical: value,
		Kind:      ToolPolicySelectorLiteral,
		Value:     value,
	}, true
}

func parseDelimitedSelector(value, prefix, suffix string) (string, bool) {
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, suffix) {
		return "", false
	}
	middle := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, prefix), suffix))
	return middle, middle != "" && middle != "*"
}

func mcpServerSelector(slug string) (ToolPolicySelector, bool) {
	slug = strings.TrimSpace(slug)
	if slug == "" || strings.ContainsAny(slug, "/*:") {
		return ToolPolicySelector{}, false
	}
	return ToolPolicySelector{
		Canonical: "mcp/" + slug + "/*",
		Kind:      ToolPolicySelectorMCPServer,
		Value:     slug,
	}, true
}

func packageSelector(pkg string) (ToolPolicySelector, bool) {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" || strings.ContainsAny(pkg, "/*:") {
		return ToolPolicySelector{}, false
	}
	return ToolPolicySelector{
		Canonical: "package/" + pkg + "/*",
		Kind:      ToolPolicySelectorPackage,
		Value:     pkg,
	}, true
}

func (s ToolPolicySelector) Matches(target ToolPolicyTarget) bool {
	switch s.Kind {
	case ToolPolicySelectorLiteral:
		return target.Name == s.Value
	case ToolPolicySelectorAllNative:
		_, _, isMCP := mcplib.ParseToolName(target.Name)
		return !isMCP
	case ToolPolicySelectorAllMCP:
		_, _, isMCP := mcplib.ParseToolName(target.Name)
		return isMCP
	case ToolPolicySelectorAllPackages:
		return strings.TrimSpace(target.Package) != ""
	case ToolPolicySelectorMCPServer:
		slug, _, isMCP := mcplib.ParseToolName(target.Name)
		return isMCP && slug == s.Value
	case ToolPolicySelectorPackage:
		return target.Package == s.Value
	default:
		return false
	}
}

// Specificity ordena default < wildcard geral < wildcard específico < literal.
func (s ToolPolicySelector) Specificity() int {
	switch s.Kind {
	case ToolPolicySelectorLiteral:
		return 3
	case ToolPolicySelectorMCPServer, ToolPolicySelectorPackage:
		return 2
	default:
		return 1
	}
}

type toolPolicyRule struct {
	selector ToolPolicySelector
	state    ToolPolicyState
}

// ToolPolicyMatch descreve tanto o estado resolvido quanto sua origem.
type ToolPolicyMatch struct {
	State       ToolPolicyState
	Selector    string
	Specificity int
	Explicit    bool
	Literal     bool
	DeniedOptIn bool
}

// ToolPolicyMatcher compila uma política uma vez e a aplica de modo lazy a
// tools que podem entrar no registry depois da criação da política.
type ToolPolicyMatcher struct {
	rules        []toolPolicyRule
	defaultState ToolPolicyState
}

func NewToolPolicyMatcher(configured map[string]string, defaultState string) ToolPolicyMatcher {
	matcher := ToolPolicyMatcher{defaultState: normalizeToolPolicyDefault(defaultState)}
	normalized := make(map[string]toolPolicyRule, len(configured))
	for raw, rawState := range configured {
		selector, ok := ParseToolPolicySelector(raw)
		if !ok {
			continue
		}
		rule := toolPolicyRule{selector: selector, state: normalizeToolPolicyState(rawState)}
		if existing, found := normalized[selector.Canonical]; found &&
			toolPolicyStateRank(existing.state) <= toolPolicyStateRank(rule.state) {
			continue
		}
		normalized[selector.Canonical] = rule
	}
	for _, rule := range normalized {
		matcher.rules = append(matcher.rules, rule)
	}
	return matcher
}

func (m ToolPolicyMatcher) Resolve(target ToolPolicyTarget) ToolPolicyMatch {
	best := ToolPolicyMatch{State: m.defaultState}
	for _, rule := range m.rules {
		if !rule.selector.Matches(target) {
			continue
		}
		specificity := rule.selector.Specificity()
		if best.Explicit && specificity < best.Specificity {
			continue
		}
		if best.Explicit && specificity == best.Specificity &&
			toolPolicyStateRank(best.State) <= toolPolicyStateRank(rule.state) {
			continue
		}
		best = ToolPolicyMatch{
			State:       rule.state,
			Selector:    rule.selector.Canonical,
			Specificity: specificity,
			Explicit:    true,
			Literal:     rule.selector.Kind == ToolPolicySelectorLiteral,
		}
	}
	// Opt-in exige autorização literal; default e wildcard permissivos jamais
	// elevam a capability.
	if target.OptIn && best.State != ToolPolicyDisabled && !best.Literal {
		best.State = ToolPolicyDisabled
		best.DeniedOptIn = true
	}
	return best
}

func (m ToolPolicyMatcher) HasOnDemandRule() bool {
	for _, rule := range m.rules {
		if rule.state == ToolPolicyOnDemand {
			return true
		}
	}
	return false
}
