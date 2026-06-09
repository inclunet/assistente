package skills

// SkillSelectionPolicy (AEP-0072 D1) — fonte única de verdade, determinística e
// sem LLM, para "esta skill é aplicável/visível agora?". Espelha a
// ToolSelectionPolicy (#119): recebe o contexto (capabilities do perfil/runtime)
// e classifica cada skill em autoload, sob demanda ou oculta, com um motivo
// estável para telemetria.
//
// Esta fase entrega a política pura e testável; o consumo no system prompt
// (BuildSkillsSection) é feito na Fase 4.

// SkillVisibility é a classificação de uma skill no contexto atual.
type SkillVisibility string

const (
	// VisibilityAutoload: corpo injetado no system prompt (Nível 1+2 juntos).
	VisibilityAutoload SkillVisibility = "autoload"
	// VisibilityOnDemand: declarada no catálogo compacto; corpo lido sob demanda.
	VisibilityOnDemand SkillVisibility = "on_demand"
	// VisibilityHidden: não exposta ao modelo no contexto atual.
	VisibilityHidden SkillVisibility = "hidden"
)

// Motivos estáveis (para telemetria/teste).
const (
	ReasonAutoload              = "autoload"
	ReasonOnDemand              = "on_demand"
	ReasonSkillsDisabled        = "skills_disabled"
	ReasonRequiresTools         = "requires_tools"
	ReasonRequiresFilesystem    = "requires_filesystem"
	ReasonRequiresNetwork       = "requires_network"
	ReasonRequiresMCP           = "requires_mcp"
	ReasonModelInvocationOff    = "model_invocation_disabled"
	ReasonOnDemandDisabled      = "on_demand_disabled"
	ReasonNotInAutoloadAllowlst = "not_in_autoload_allowlist"
)

// SkillSelectionContext descreve as capacidades disponíveis no contexto/perfil.
type SkillSelectionContext struct {
	// Capabilities habilitadas no runtime/perfil.
	ToolsEnabled      bool
	FilesystemEnabled bool
	NetworkEnabled    bool
	MCPEnabled        bool

	// SkillsDisabled desliga completamente o subsistema de skills (perfil).
	SkillsDisabled bool

	// DisableOnDemand oculta skills sob demanda (mantém só autoload).
	DisableOnDemand bool

	// AutoloadAllowlist, quando não-nil, define explicitamente quais slugs
	// fazem autoload (seleção do perfil), sobrepondo o auto_load do metadado.
	// nil = usa o auto_load declarado em cada skill.
	AutoloadAllowlist []string
}

// SkillDecision é o resultado da política para uma skill.
type SkillDecision struct {
	Slug       string          `json:"slug"`
	Visibility SkillVisibility `json:"visibility"`
	Reason     string          `json:"reason"`
}

// SkillSelectionPolicy decide visibilidade de skills. É stateless; reúne a lógica
// num tipo para espelhar a ToolSelectionPolicy e facilitar extensão futura.
type SkillSelectionPolicy struct{}

// NewSkillSelectionPolicy cria a política padrão.
func NewSkillSelectionPolicy() SkillSelectionPolicy { return SkillSelectionPolicy{} }

// Decide classifica uma skill no contexto. Ordem: (1) skills desligadas,
// (2) gating de capability, (3) autoload, (4) invocação pelo modelo, (5) sob demanda.
func (p SkillSelectionPolicy) Decide(m *SkillMetadata, slug string, ctx SkillSelectionContext) SkillDecision {
	hidden := func(reason string) SkillDecision {
		return SkillDecision{Slug: slug, Visibility: VisibilityHidden, Reason: reason}
	}

	if ctx.SkillsDisabled {
		return hidden(ReasonSkillsDisabled)
	}

	// Gating de capability: skill que exige uma capacidade ausente é oculta.
	if m.EffectiveRequiresTools() && !ctx.ToolsEnabled {
		return hidden(ReasonRequiresTools)
	}
	if m.EffectiveRequiresFilesystem() && !ctx.FilesystemEnabled {
		return hidden(ReasonRequiresFilesystem)
	}
	if m.EffectiveRequiresNetwork() && !ctx.NetworkEnabled {
		return hidden(ReasonRequiresNetwork)
	}
	if m.EffectiveRequiresMCP() && !ctx.MCPEnabled {
		return hidden(ReasonRequiresMCP)
	}

	// Autoload: allowlist do perfil sobrepõe o auto_load do metadado.
	if p.isAutoload(m, slug, ctx) {
		return SkillDecision{Slug: slug, Visibility: VisibilityAutoload, Reason: ReasonAutoload}
	}

	// Sem autoload: precisa ser invocável pelo modelo para entrar no catálogo.
	if !m.IsModelInvocable() {
		return hidden(ReasonModelInvocationOff)
	}
	if ctx.DisableOnDemand {
		return hidden(ReasonOnDemandDisabled)
	}
	return SkillDecision{Slug: slug, Visibility: VisibilityOnDemand, Reason: ReasonOnDemand}
}

// isAutoload resolve se a skill deve autoloadar no contexto.
func (p SkillSelectionPolicy) isAutoload(m *SkillMetadata, slug string, ctx SkillSelectionContext) bool {
	if ctx.AutoloadAllowlist != nil {
		// Perfil define explicitamente o conjunto de autoload; ainda assim a
		// skill precisa ser invocável pelo modelo.
		return m.IsModelInvocable() && containsString(ctx.AutoloadAllowlist, slug)
	}
	return m.IsAutoLoad()
}

// SkillSelection agrupa o resultado de DecideAll por visibilidade, preservando
// a ordem de entrada dentro de cada grupo.
type SkillSelection struct {
	Autoload []SkillDecision
	OnDemand []SkillDecision
	Hidden   []SkillDecision
}

// DecideAll aplica a política a uma lista de skills e agrupa por visibilidade.
func (p SkillSelectionPolicy) DecideAll(list []Skill, ctx SkillSelectionContext) SkillSelection {
	var sel SkillSelection
	for i := range list {
		d := p.Decide(&list[i].SkillMetadata, list[i].Slug, ctx)
		switch d.Visibility {
		case VisibilityAutoload:
			sel.Autoload = append(sel.Autoload, d)
		case VisibilityOnDemand:
			sel.OnDemand = append(sel.OnDemand, d)
		default:
			sel.Hidden = append(sel.Hidden, d)
		}
	}
	return sel
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
