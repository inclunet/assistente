package chat

import (
	"log"
	"strings"

	"assistente/internal/llm"
	mcplib "assistente/internal/mcp"
	"assistente/internal/toolcatalog"
	"assistente/internal/tools"
)

// Este arquivo conecta o ToolSelectionPolicy (cérebro da seleção por
// perfil/superfície) ao ToolPlanner determinístico (AEP-0077 Fase 4, #121). O
// planner vive em internal/toolcatalog e decide, com budget de schema bytes e
// ranking, quais tools entram no contexto do turno; aqui apenas montamos as
// candidatas a partir do registry/catálogo, invocamos o planner e remapeamos a
// saída para []llm.ToolDefinition preservando a ordem de entrada.
//
// A resolução de conflito bridge×native (AEP-0021) permanece nos helpers
// existentes (ApplyNativeMCP / filterToolNamesForNativeMCP); o planner apenas a
// CONSOME via ToolCandidate.NativeServed e nunca a duplica.

// applyPlanner aplica o ToolPlanner sobre uma lista de tool definitions já
// resolvida pela política. Com budget ilimitado (default seguro) e sem conflito
// nativo, o planner não corta nada e a saída é idêntica à entrada — garantindo
// não-regressão para os perfis cujos schemas já cabem.
//
// IMPORTANTE (AEP-0077 F4 / bugfix): o budget deve refletir os schemas de função
// REALMENTE enviados ao LLM. Tools servidas via MCP nativo NÃO são enviadas como
// function schemas (vão por passthrough), então o planner precisa ser aplicado
// ao conjunto FINAL de cada caminho — depois de remover as bridges nativas no
// caminho nativo (ver PlanTurnToolDefs) e antes do planner na expansão dinâmica
// (onde o filtro nativo já ocorreu).
//
// surface rotula o caminho/superfície para a telemetria (ex.: "inicial",
// "nativo", "adapter", "expansão").
func (p *ToolSelectionPolicy) applyPlanner(defs []llm.ToolDefinition, cfg ProfileToolConfig, nativeServed map[string]struct{}, surface string) []llm.ToolDefinition {
	if len(defs) == 0 {
		return defs
	}
	plan := toolcatalog.PlannerConfig{
		SchemaBytesBudget: cfg.SchemaBytesBudget,
		PreferredPackages: cfg.PreferredPackages,
	}.Plan(p.buildPlannerCandidates(defs, cfg, nativeServed))

	// Nada cortado: preserva identidade/ordem e evita log ruidoso.
	if plan.DroppedCount == 0 {
		return defs
	}

	log.Printf("[chat] ToolPlanner (%s): %s", surface, plan.LogLine())
	selected := plan.SelectedSet()
	out := make([]llm.ToolDefinition, 0, len(plan.Selected))
	for _, d := range defs {
		if _, ok := selected[d.Function.Name]; ok {
			out = append(out, d)
		}
	}
	return out
}

// buildPlannerCandidates monta as candidatas neutras do planner a partir das
// tool definitions e dos metadados de catálogo (origem/pacote) do registry.
func (p *ToolSelectionPolicy) buildPlannerCandidates(defs []llm.ToolDefinition, cfg ProfileToolConfig, nativeServed map[string]struct{}) []toolcatalog.ToolCandidate {
	pinned := pinnedToolSet(cfg.EnabledTools)
	cands := make([]toolcatalog.ToolCandidate, 0, len(defs))
	for _, d := range defs {
		cands = append(cands, p.plannerCandidate(d, pinned, nativeServed))
	}
	return cands
}

// plannerCandidate monta a candidata neutra de uma única tool definition.
func (p *ToolSelectionPolicy) plannerCandidate(d llm.ToolDefinition, pinned, nativeServed map[string]struct{}) toolcatalog.ToolCandidate {
	name := d.Function.Name
	origin, pkg := p.toolOriginPackage(name)
	c := toolcatalog.ToolCandidate{
		Name:        name,
		SchemaBytes: len(d.Function.Parameters),
		Origin:      origin,
		Package:     pkg,
		Essential:   name == tools.ToolCatalogName || name == tools.LoadSkillName,
	}
	if _, ok := pinned[name]; ok {
		c.ProfilePinned = true
	}
	if _, ok := nativeServed[name]; ok {
		c.NativeServed = true
	}
	return c
}

// planAccumulatedToolDefs aplica o ToolPlanner ao conjunto ACUMULADO do turno:
// as tools já ATIVAS (enviadas em iterações anteriores) são travadas — sempre
// preservadas e consumindo budget primeiro — e as NOVAS candidatas (delta da
// expansão via tool_catalog) só entram no orçamento REMANESCENTE, de forma
// determinística. Assim o teto de schema bytes vale para o total realmente
// enviado ao LLM, e não apenas para o delta recém-expandido.
//
// Retorna o conjunto acumulado final (ativas preservadas em ordem + novas que
// couberam, na ordem de entrada). Com budget ilimitado (default), o resultado é
// idêntico ao append determinístico anterior (ativas + novas deduplicadas).
func (p *ToolSelectionPolicy) planAccumulatedToolDefs(active, newDefs []llm.ToolDefinition, cfg ProfileToolConfig) []llm.ToolDefinition {
	pinned := pinnedToolSet(cfg.EnabledTools)

	activeNames := make(map[string]struct{}, len(active))
	for _, d := range active {
		activeNames[d.Function.Name] = struct{}{}
	}

	combined := make([]llm.ToolDefinition, 0, len(active)+len(newDefs))
	combined = append(combined, active...)
	candidates := make([]toolcatalog.ToolCandidate, 0, len(active)+len(newDefs))
	for _, d := range active {
		c := p.plannerCandidate(d, pinned, nil)
		c.Locked = true
		candidates = append(candidates, c)
	}
	// activeNames serve como conjunto "já visto": além das ativas, vai
	// acumulando os nomes das novas defs anexadas, para que duplicatas DENTRO de
	// newDefs (ex.: FilterByNames preserva nomes repetidos) não entrem duas vezes
	// no conjunto acumulado — o que quebraria o tool calling e distorceria o budget.
	for _, d := range newDefs {
		name := d.Function.Name
		if name == "" {
			continue // ignora nomes vazios
		}
		if _, dup := activeNames[name]; dup {
			continue // já ativa ou já incluída nesta passada; evita duplicar
		}
		activeNames[name] = struct{}{}
		combined = append(combined, d)
		candidates = append(candidates, p.plannerCandidate(d, pinned, nil))
	}

	plan := toolcatalog.PlannerConfig{
		SchemaBytesBudget: cfg.SchemaBytesBudget,
		PreferredPackages: cfg.PreferredPackages,
	}.Plan(candidates)

	// Nada cortado (inclui budget ilimitado): preserva o acumulado deduplicado.
	if plan.DroppedCount == 0 {
		return combined
	}

	log.Printf("[chat] ToolPlanner (acumulado): %s", plan.LogLine())
	selected := plan.SelectedSet()
	out := make([]llm.ToolDefinition, 0, len(plan.Selected))
	for _, d := range combined {
		if _, ok := selected[d.Function.Name]; ok {
			out = append(out, d)
		}
	}
	return out
}

// toolOriginPackage resolve origem e pacote de catálogo de uma tool pelo nome.
// Tools MCP (nome namespaced) são bridge com pacote "mcp:<slug>"; as demais são
// builtins cujo pacote vem dos metadados declarados na própria tool (Fase 1).
func (p *ToolSelectionPolicy) toolOriginPackage(name string) (origin, pkg string) {
	if slug, _, ok := mcplib.ParseToolName(name); ok {
		return tools.ToolOriginMCPBridge, "mcp:" + slug
	}
	if p.registry != nil {
		if tool, ok := p.registry.Get(name); ok {
			return tools.ToolOriginBuiltin, tools.CatalogEntryFromTool(tool).Package
		}
	}
	return tools.ToolOriginBuiltin, ""
}

// pinnedToolSet devolve o conjunto de tools fixadas explicitamente pelo perfil.
// enabledTools nil (seleção dinâmica/catálogo) ou vazio não fixa nada.
func pinnedToolSet(enabledTools []string) map[string]struct{} {
	if len(enabledTools) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(enabledTools))
	for _, name := range enabledTools {
		if name = strings.TrimSpace(name); name != "" {
			set[name] = struct{}{}
		}
	}
	return set
}
