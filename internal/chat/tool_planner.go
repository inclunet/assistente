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
func (p *ToolSelectionPolicy) applyPlanner(defs []llm.ToolDefinition, cfg ProfileToolConfig, nativeServed map[string]struct{}) []llm.ToolDefinition {
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

	log.Printf("[chat] ToolPlanner: %s", plan.LogLine())
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
		cands = append(cands, c)
	}
	return cands
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
