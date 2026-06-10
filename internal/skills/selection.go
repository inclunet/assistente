package skills

import "strings"

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
	ReasonAutoloadNoReason      = "autoload_missing_reason"
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

	// RequireAutoloadReason, no modo metadata-driven (sem AutoloadAllowlist),
	// exige que a skill declare autoload_reason para permanecer em autoload
	// (AEP-0072 D5). Skills com auto_load=true mas sem motivo são rebaixadas
	// para sob demanda. Ignorado quando AutoloadAllowlist está definido
	// (o perfil é a fonte explícita da decisão).
	RequireAutoloadReason bool
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
	return SkillDecision{Slug: slug, Visibility: VisibilityOnDemand, Reason: p.onDemandReason(m.IsAutoLoad(), m.AutoloadReason, ctx)}
}

// onDemandReason distingue um on-demand normal de um autoload REBAIXADO por falta
// de autoload_reason no modo metadata-driven (AEP-0072 D5). A visibilidade é a
// mesma (sob demanda); só o motivo muda, para que o rebaixamento seja observável
// em telemetria/diagnóstico em vez de indistinguível de um on-demand comum.
func (p SkillSelectionPolicy) onDemandReason(isAutoLoad bool, autoloadReason string, ctx SkillSelectionContext) string {
	if ctx.AutoloadAllowlist == nil && isAutoLoad && ctx.RequireAutoloadReason && strings.TrimSpace(autoloadReason) == "" {
		return ReasonAutoloadNoReason
	}
	return ReasonOnDemand
}

// isAutoload resolve se a skill deve autoloadar no contexto.
func (p SkillSelectionPolicy) isAutoload(m *SkillMetadata, slug string, ctx SkillSelectionContext) bool {
	if ctx.AutoloadAllowlist != nil {
		// Perfil define explicitamente o conjunto de autoload; ainda assim a
		// skill precisa ser invocável pelo modelo. A allowlist é um conjunto de
		// membership por slug OU nome — ela NÃO resolve colisões slug/nome de forma
		// determinística como CatalogByNamesOrdered. Callers que precisam de
		// determinismo (ex.: o prompt builder) pré-resolvem a allowlist para slugs
		// canônicos antes de montar o contexto.
		return m.IsModelInvocable() && allowlistMatches(ctx.AutoloadAllowlist, slug, m.Name)
	}
	if !m.IsAutoLoad() {
		return false
	}
	// Modo metadata-driven: autoload sem motivo é rebaixado (AEP-0072 D5).
	if ctx.RequireAutoloadReason && strings.TrimSpace(m.AutoloadReason) == "" {
		return false
	}
	return true
}

// DecideCatalog classifica uma entrada de catálogo (sem corpo) no contexto. É o
// análogo de Decide para o Nível 1 servido direto do catálogo persistido: as
// pré-condições de capability já vêm efetivas na entry (explícitas OU inferidas),
// portanto não há reconstrução de SkillMetadata aqui. A ordem das regras espelha
// Decide para garantir decisões idênticas.
func (p SkillSelectionPolicy) DecideCatalog(entry SkillCatalogEntry, ctx SkillSelectionContext) SkillDecision {
	hidden := func(reason string) SkillDecision {
		return SkillDecision{Slug: entry.Slug, Visibility: VisibilityHidden, Reason: reason}
	}

	if ctx.SkillsDisabled {
		return hidden(ReasonSkillsDisabled)
	}

	if entry.RequiresTools && !ctx.ToolsEnabled {
		return hidden(ReasonRequiresTools)
	}
	if entry.RequiresFilesystem && !ctx.FilesystemEnabled {
		return hidden(ReasonRequiresFilesystem)
	}
	if entry.RequiresNetwork && !ctx.NetworkEnabled {
		return hidden(ReasonRequiresNetwork)
	}
	if entry.RequiresMCP && !ctx.MCPEnabled {
		return hidden(ReasonRequiresMCP)
	}

	if p.isAutoloadCatalog(entry, ctx) {
		return SkillDecision{Slug: entry.Slug, Visibility: VisibilityAutoload, Reason: ReasonAutoload}
	}

	if !entry.ModelInvocable {
		return hidden(ReasonModelInvocationOff)
	}
	if ctx.DisableOnDemand {
		return hidden(ReasonOnDemandDisabled)
	}
	return SkillDecision{Slug: entry.Slug, Visibility: VisibilityOnDemand, Reason: p.onDemandReason(entry.AutoLoad, entry.AutoloadReason, ctx)}
}

// isAutoloadCatalog espelha isAutoload usando os campos já efetivos da entry.
func (p SkillSelectionPolicy) isAutoloadCatalog(entry SkillCatalogEntry, ctx SkillSelectionContext) bool {
	if ctx.AutoloadAllowlist != nil {
		return entry.ModelInvocable && allowlistMatches(ctx.AutoloadAllowlist, entry.Slug, entry.Name)
	}
	if !entry.AutoLoad {
		return false
	}
	if ctx.RequireAutoloadReason && strings.TrimSpace(entry.AutoloadReason) == "" {
		return false
	}
	return true
}

// DecideAllCatalog aplica a política a uma lista de entradas de catálogo e agrupa
// por visibilidade, preservando a ordem de entrada dentro de cada grupo.
func (p SkillSelectionPolicy) DecideAllCatalog(entries []SkillCatalogEntry, ctx SkillSelectionContext) SkillSelection {
	var sel SkillSelection
	for i := range entries {
		d := p.DecideCatalog(entries[i], ctx)
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

// allowlistMatches verifica se a allowlist contém o slug ou o nome da skill (o
// nome só conta quando não-vazio, para não casar entradas vazias). É um teste de
// membership por entrada — diferente de CatalogByNamesOrdered, NÃO resolve
// colisões slug/nome de forma determinística (slug-first). Em caso de colisão
// (um identificador que é slug de uma skill e nome de outra) pode casar ambas;
// por isso o builder pré-resolve a allowlist para slugs canônicos antes de
// chamar a política.
func allowlistMatches(allowlist []string, slug, name string) bool {
	if containsString(allowlist, slug) {
		return true
	}
	return name != "" && containsString(allowlist, name)
}
