package chat

import (
	"context"
	"log"
	"sort"
	"strings"

	"assistente/internal/llm"
	mcplib "assistente/internal/mcp"
	"assistente/internal/tools"
)

// ToolSelectionPolicy é o ponto ÚNICO de política de seleção de tools por
// perfil/superfície (AEP-0077 Fase 3, #119). Antes desta consolidação a decisão
// de "quais tools entram no contexto de um turno" estava dispersa entre as
// funções de tool_defs.go e o callback de expansão dinâmica do use case de
// envio, com risco de divergência. Toda a lógica vive agora aqui; as funções
// exportadas em tool_defs.go são wrappers finos que delegam a esta policy, e os
// call sites (chat/use case, prompt builder, fallback adapter) usam os métodos
// de alto nível abaixo.
//
// Contratos respeitados:
//   - AEP-0021 (MCP nativo): a política tri-state (resolveNativeMCPEnabled) e a
//     remoção de bridge tools em ApplyNativeMCP permanecem idênticas.
//   - AEP-0040 (eventos): a policy não emite nem altera eventos.
type ToolSelectionPolicy struct {
	registry *tools.Registry
}

// NewToolSelectionPolicy cria a policy ligada a um registry de tools. O registry
// pode ser nil; nesse caso os métodos degradam para "sem tools" como antes.
func NewToolSelectionPolicy(registry *tools.Registry) *ToolSelectionPolicy {
	return &ToolSelectionPolicy{registry: registry}
}

// ProfileToolConfig descreve a configuração de perfil/turno que governa a
// seleção. Semântica de EnabledTools:
//   - nil   → todas as tools (gateado pelo catálogo quando ele existe no registry);
//   - []    → seleção explícita de zero tools (tool calling desligado);
//   - lista → allowlist explícita do perfil.
type ProfileToolConfig struct {
	EnabledTools []string
	DisableTools bool
	// NativeMCP é o override tri-state de MCP nativo do perfil ativo (AEP-0021):
	// nil=auto otimista, true=forçar nativo (se capaz), false=forçar adapter.
	NativeMCP *bool
	// RuntimeTools são tools disponibilizadas em runtime (ex.: load_skill quando
	// há skills sob demanda) que devem ser anexadas à seleção inicial.
	RuntimeTools []string
	// SchemaBytesBudget é o teto de bytes de schema injetados por turno usado
	// pelo ToolPlanner (AEP-0077 F4, #121). <= 0 → ilimitado (default seguro:
	// não corta nenhum fluxo atual). > 0 → corta deterministicamente pela ordem
	// de ranking quando a seleção excede o teto.
	SchemaBytesBudget int
	// PreferredPackages lista pacotes (ToolCatalogEntry.Package) priorizados no
	// ranking do planner para este perfil/superfície.
	PreferredPackages []string
}

// ---------------------------------------------------------------------------
// Alto nível: orquestração consolidada usada pelos call sites
// ---------------------------------------------------------------------------

// InitialEnabledToolNames resolve os nomes de tools habilitadas para o início do
// turno (perfil + runtime tools), antes de qualquer expansão dinâmica.
func (p *ToolSelectionPolicy) InitialEnabledToolNames(cfg ProfileToolConfig) []string {
	return p.resolveInitialEnabledToolsWithRuntime(cfg.EnabledTools, cfg.DisableTools, cfg.RuntimeTools)
}

// rawInitialToolDefs monta as tool definitions iniciais do turno SEM aplicar o
// ToolPlanner. É a base usada pela resolução native/adapter (PlanTurnToolDefs):
// o budget precisa ser aplicado ao conjunto FINAL de cada caminho, e não antes
// da remoção das bridges servidas via MCP nativo.
func (p *ToolSelectionPolicy) rawInitialToolDefs(cfg ProfileToolConfig) []llm.ToolDefinition {
	initial := p.resolveInitialEnabledToolsWithRuntime(cfg.EnabledTools, cfg.DisableTools, cfg.RuntimeTools)
	return p.buildLLMToolDefs(initial, cfg.DisableTools)
}

// InitialToolDefs monta as tool definitions iniciais do turno para o LLM na
// visão ADAPTER (bridge tools presentes), já com o ToolPlanner aplicado
// (budget/ranking; AEP-0077 F4, #121).
//
// Para o pipeline de envio, prefira PlanTurnToolDefs, que resolve native×adapter
// e aplica o planner ao conjunto FINAL de cada caminho na ordem correta.
func (p *ToolSelectionPolicy) InitialToolDefs(cfg ProfileToolConfig) []llm.ToolDefinition {
	return p.applyPlanner(p.rawInitialToolDefs(cfg), cfg, "inicial")
}

// PlanTurnToolDefs resolve os conjuntos FINAIS de tools do turno aplicando a
// resolução bridge×native (AEP-0021) e o ToolPlanner (AEP-0077 F4, #121) na
// ordem correta, mantendo a policy como ponto único:
//
//   - caminho NATIVO: ApplyNativeMCP anexa os servidores MCP HTTP ao streamer e
//     remove as bridge tools servidas nativamente; só DEPOIS o planner orça o
//     conjunto restante — essas bridges vão por passthrough e NÃO consomem
//     schema bytes, então não podem deslocar builtins fixadas pelo perfil;
//   - caminho ADAPTER: as bridge tools permanecem (são enviadas como função) e
//     são contadas no budget.
//
// Retorna o streamer nativo (com servidores MCP anexados quando aplicável), o
// conjunto nativo final (lista primária do turno) e o conjunto adapter final
// (usado no fallback nativo→adapter do mesmo turno). O streamer adapter é o
// próprio streamer recebido, inalterado.
func (p *ToolSelectionPolicy) PlanTurnToolDefs(streamer llm.ChatProvider, mcpMgr NativeMCPManager, cfg ProfileToolConfig) (nativeStreamer llm.ChatProvider, nativeDefs, adapterDefs []llm.ToolDefinition) {
	raw := p.rawInitialToolDefs(cfg)
	// Adapter: bridges contam no budget (são enviadas como function schemas).
	adapterDefs = p.applyPlanner(raw, cfg, "adapter")
	// Nativo: remove as bridges nativas ANTES de orçar (passthrough não consome budget).
	nativeStreamer, reduced := applyNativeMCP(streamer, raw, mcpMgr, cfg.EnabledTools, cfg.DisableTools, cfg.NativeMCP)
	nativeDefs = p.applyPlanner(reduced, cfg, "nativo")
	return nativeStreamer, nativeDefs, adapterDefs
}

// ApplyNativeMCP resolve bridge×native (AEP-0021): configura os servidores MCP
// HTTP nativos elegíveis no provider e remove as bridge tools correspondentes do
// toolDefs. Sem efeito quando disableTools, sem manager, provider incapaz ou
// override=false.
func (p *ToolSelectionPolicy) ApplyNativeMCP(streamer llm.ChatProvider, toolDefs []llm.ToolDefinition, mcpMgr NativeMCPManager, cfg ProfileToolConfig) (llm.ChatProvider, []llm.ToolDefinition) {
	return applyNativeMCP(streamer, toolDefs, mcpMgr, cfg.EnabledTools, cfg.DisableTools, cfg.NativeMCP)
}

// ResolveExpandedToolDefs é o ponto único da EXPANSÃO DINÂMICA: dada a lista de
// tools já ATIVAS no turno (active) e a lista de tools selecionadas em runtime
// (names, ex.: retorno do tool_catalog), aplica o allowlist do perfil, remove
// opt-ins quando o perfil não fixa tools, resolve bridge×native e devolve o
// conjunto ACUMULADO final de tool definitions (ativas preservadas + novas que
// couberam no budget). Substitui o callback que antes era duplicado no use case
// de envio (caminho principal e fallback adapter), diferindo apenas no streamer
// e no override de MCP nativo.
//
// O budget de schema bytes (AEP-0077 F4) vale para o conjunto ACUMULADO — as
// ativas são travadas (preservadas e consumindo budget primeiro) e as novas só
// entram no orçamento remanescente. O loop agêntico passa aqui as activeToolDefs
// correntes e usa o retorno como novo conjunto acumulado.
func (p *ToolSelectionPolicy) ResolveExpandedToolDefs(streamer llm.ChatProvider, mcpMgr NativeMCPManager, active []llm.ToolDefinition, names []string, cfg ProfileToolConfig) []llm.ToolDefinition {
	names = p.filterExpandedToolNames(names, cfg.EnabledTools, cfg.DisableTools)
	// O filtro nativo já remove aqui as bridges servidas via passthrough, então o
	// planner orça apenas os schemas realmente enviados como função.
	names = filterToolNamesForNativeMCP(streamer, mcpMgr, names, cfg.DisableTools, cfg.NativeMCP)
	newDefs := p.buildLLMToolDefsByNames(names, cfg.DisableTools)
	return p.planAccumulatedToolDefs(active, newDefs, cfg)
}

// ---------------------------------------------------------------------------
// Baixo nível: lógica de seleção (fonte única; wrappers em tool_defs.go delegam)
// ---------------------------------------------------------------------------

// buildLLMToolDefs constrói a lista de tool definitions para o LLM.
// Se disableTools for true, retorna nil. Se enabledTools for nil, inclui todas.
func (p *ToolSelectionPolicy) buildLLMToolDefs(enabledTools []string, disableTools bool) []llm.ToolDefinition {
	if disableTools || p.registry == nil || p.registry.Count() == 0 {
		return nil
	}

	var toolDefs []tools.ToolDefinition
	if enabledTools != nil {
		toolDefs = p.registry.FilterByNames(enabledTools)
	} else {
		toolDefs = p.registry.ToDefinitions()
	}

	result := make([]llm.ToolDefinition, len(toolDefs))
	for i, td := range toolDefs {
		result[i] = llm.ToolDefinition{
			Type: td.Type,
			Function: llm.FunctionDefinition{
				Name:        td.Function.Name,
				Description: td.Function.Description,
				Parameters:  td.Function.Parameters,
			},
		}
	}
	return result
}

// resolveInitialEnabledTools resolve a seleção inicial a partir do perfil: quando
// o perfil não fixa tools (enabledTools nil) e o catálogo existe, começa só com o
// tool_catalog (progressive disclosure); caso contrário preserva a seleção.
func (p *ToolSelectionPolicy) resolveInitialEnabledTools(enabledTools []string, disableTools bool) []string {
	if disableTools || enabledTools != nil || p.registry == nil {
		return enabledTools
	}
	if p.registry.Has(tools.ToolCatalogName) {
		return []string{tools.ToolCatalogName}
	}
	return nil
}

// resolveInitialEnabledToolsWithRuntime adiciona as runtime tools (ex.: load_skill)
// à seleção inicial, preservando a semântica de [] (zero tools explícito).
func (p *ToolSelectionPolicy) resolveInitialEnabledToolsWithRuntime(enabledTools []string, disableTools bool, runtimeTools []string) []string {
	initial := p.resolveInitialEnabledTools(enabledTools, disableTools)
	if disableTools || p.registry == nil || len(runtimeTools) == 0 {
		return initial
	}
	// [] é uma seleção explícita de zero tools no perfil; não adiciona runtime tools.
	if enabledTools != nil && len(enabledTools) == 0 {
		return initial
	}
	if initial == nil {
		defs := p.registry.ToDefinitions()
		names := make([]string, 0, len(defs)+len(runtimeTools))
		for _, def := range defs {
			names = append(names, def.Function.Name)
		}
		initial = names
	} else {
		initial = append([]string{}, initial...)
	}
	seen := make(map[string]struct{}, len(initial)+len(runtimeTools))
	for _, name := range initial {
		if name = strings.TrimSpace(name); name != "" {
			seen[name] = struct{}{}
		}
	}
	for _, name := range runtimeTools {
		name = strings.TrimSpace(name)
		if name == "" || !p.registry.Has(name) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		initial = append(initial, name)
		seen[name] = struct{}{}
	}
	if len(initial) == 0 {
		return []string{}
	}
	return initial
}

// buildLLMToolDefsByNames monta tool definitions a partir de uma lista de nomes
// (usada na expansão dinâmica). A ordem é determinística (definida pelo registry).
func (p *ToolSelectionPolicy) buildLLMToolDefsByNames(names []string, disableTools bool) []llm.ToolDefinition {
	if disableTools || p.registry == nil || len(names) == 0 {
		return nil
	}
	toolDefs := p.registry.FilterByNames(names)
	result := make([]llm.ToolDefinition, len(toolDefs))
	for i, td := range toolDefs {
		result[i] = llm.ToolDefinition{
			Type: td.Type,
			Function: llm.FunctionDefinition{
				Name:        td.Function.Name,
				Description: td.Function.Description,
				Parameters:  td.Function.Parameters,
			},
		}
	}
	return result
}

// filterExpandedToolNames aplica, sobre a lista expandida dinamicamente, o
// allowlist do perfil e — quando o perfil não fixa tools (enabledTools nil) —
// remove as tools opt-in (que só entram via seleção explícita). Antes vivia como
// helper privado no use case de envio.
func (p *ToolSelectionPolicy) filterExpandedToolNames(names, enabledTools []string, disableTools bool) []string {
	names = filterToolNamesByEnabledTools(names, enabledTools, disableTools)
	if enabledTools == nil && p.registry != nil {
		names = p.registry.FilterOutOptInNames(names)
	}
	return names
}

// ---------------------------------------------------------------------------
// Baixo nível: helpers independentes do registry (MCP nativo e allowlist)
// ---------------------------------------------------------------------------

// filterToolNamesByEnabledTools restringe uma lista de nomes ao allowlist do
// perfil. enabledTools nil significa "catálogo dinâmico pode selecionar qualquer
// tool" (sem allowlist); [] e listas filtram normalmente.
func filterToolNamesByEnabledTools(names, enabledTools []string, disableTools bool) []string {
	if disableTools || len(names) == 0 {
		return nil
	}
	if enabledTools == nil {
		filtered := make([]string, 0, len(names))
		for _, name := range names {
			if name = strings.TrimSpace(name); name != "" {
				filtered = append(filtered, name)
			}
		}
		return filtered
	}
	enabledSet := make(map[string]struct{}, len(enabledTools))
	for _, name := range enabledTools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		enabledSet[name] = struct{}{}
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := enabledSet[name]; ok {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

// resolveNativeMCPEnabled resolve a POLÍTICA de MCP nativo a partir do override do
// PERFIL ativo (AEP-0021). Não há heurística por URL/endpoint — a única dimensão
// de provider é a capacidade física de transporte (NativeMCPCapable). O default
// (auto) é OTIMISTA: tenta nativo quando o provider é capaz, e o erro de
// não-suporte (detectado no streaming) auto-ajusta o perfil para adapter (nil→false),
// persistindo a decisão para os próximos turnos. Semântica tri-state:
//
//   - nil   → auto OTIMISTA: nativo SE o provider for FISICAMENTE capaz
//     (NativeMCPCapable). Se o modelo/endpoint rejeitar type:"mcp", o pipeline
//     degrada para adapter no mesmo turno e persiste Profile.Chat.NativeMCP=false.
//   - true  → força MCP nativo, mas só se o provider for FISICAMENTE capaz
//     (NativeMCPCapable). Caso contrário (ex.: Chat Completions, Google) cai
//     em adapter, evitando remover bridge tools sem ter como enviar type:"mcp".
//   - false → força modo adapter (MCP como function/bridge tools).
func resolveNativeMCPEnabled(streamer llm.ChatProvider, override *bool) bool {
	if ChatProviderIsNil(streamer) {
		return false
	}
	if override != nil && !*override {
		return false
	}
	// nil (auto otimista) ou true → nativo se o provider for fisicamente capaz.
	return streamer.NativeMCPCapable()
}

// filterToolNamesForNativeMCP remove, de uma lista expandida dinamicamente, os
// nomes das bridge tools que serão atendidas via MCP nativo (para não duplicar).
func filterToolNamesForNativeMCP(streamer llm.ChatProvider, mcpMgr NativeMCPManager, names []string, disableTools bool, nativeMCPOverride *bool) []string {
	if disableTools {
		return nil
	}
	if len(names) == 0 || NativeMCPManagerIsNil(mcpMgr) || ChatProviderIsNil(streamer) {
		return names
	}
	if !resolveNativeMCPEnabled(streamer, nativeMCPOverride) {
		return names
	}
	nativeServers := mcpMgr.GetEligibleNativeMCPServers()
	if len(nativeServers) == 0 {
		return names
	}
	nativeServers = cloneNativeMCPServers(nativeServers)
	sortNativeMCPServers(nativeServers)
	nativeToolNames := make(map[string]struct{})
	for _, srv := range nativeServers {
		for _, name := range srv.ToolNames {
			nativeToolNames[name] = struct{}{}
		}
	}
	if len(nativeToolNames) == 0 {
		return names
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if _, native := nativeToolNames[name]; native {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

// applyNativeMCP configura servidores MCP HTTP nativos no ChatProvider e remove
// as bridge tools correspondentes do toolDefs para evitar duplicatas.
func applyNativeMCP(
	streamer llm.ChatProvider,
	toolDefs []llm.ToolDefinition,
	mcpMgr NativeMCPManager,
	enabledTools []string,
	disableTools bool,
	nativeMCPOverride *bool,
) (llm.ChatProvider, []llm.ToolDefinition) {
	if disableTools || NativeMCPManagerIsNil(mcpMgr) || ChatProviderIsNil(streamer) {
		return streamer, toolDefs
	}
	if !resolveNativeMCPEnabled(streamer, nativeMCPOverride) {
		return streamer, toolDefs
	}

	nativeServers := mcpMgr.GetEligibleNativeMCPServers()
	if len(nativeServers) == 0 {
		return streamer, toolDefs
	}
	nativeServers = cloneNativeMCPServers(nativeServers)
	sortNativeMCPServers(nativeServers)

	var enabledSet map[string]bool
	if enabledTools != nil {
		enabledSet = make(map[string]bool, len(enabledTools))
		for _, n := range enabledTools {
			enabledSet[n] = true
		}
	}

	var mcpConfigs []llm.MCPServerConfig
	nativeToolNames := make(map[string]bool)

	for _, srv := range nativeServers {
		cfg := llm.MCPServerConfig{
			Slug:      srv.Slug,
			Name:      srv.Name,
			URL:       srv.URL,
			AuthToken: srv.AuthToken,
			ToolNames: sortedToolNames(srv.ToolNames),
			Recover: func(slug string) func(context.Context) error {
				return func(ctx context.Context) error {
					return mcpMgr.RecoverServerBestEffort(ctx, slug).Err
				}
			}(srv.Slug),
		}

		if enabledSet != nil {
			var allowed []string
			var allowedFull []string
			for _, fullName := range srv.ToolNames {
				if enabledSet[fullName] {
					if _, originalName, ok := mcplib.ParseToolName(fullName); ok {
						allowed = append(allowed, originalName)
					}
					allowedFull = append(allowedFull, fullName)
				}
			}
			if len(allowed) == 0 {
				log.Printf("[chat] MCP nativo: servidor %q excluído (nenhuma tool habilitada no perfil)", srv.Name)
				continue
			}
			cfg.AllowedTools = allowed
			cfg.ToolNames = allowedFull
		}

		mcpConfigs = append(mcpConfigs, cfg)
		for _, tn := range cfg.ToolNames {
			nativeToolNames[tn] = true
		}
	}

	if len(mcpConfigs) > 0 {
		streamer = streamer.WithMCPServers(mcpConfigs)
		log.Printf("[chat] MCP nativo: %d servidores HTTP configurados", len(mcpConfigs))
	}

	if len(nativeToolNames) > 0 {
		filtered := make([]llm.ToolDefinition, 0, len(toolDefs))
		for _, td := range toolDefs {
			if !nativeToolNames[td.Function.Name] {
				filtered = append(filtered, td)
			}
		}
		removed := len(toolDefs) - len(filtered)
		if removed > 0 {
			log.Printf("[chat] MCP nativo: %d bridge tools removidas (nativas agora)", removed)
		}
		toolDefs = filtered
	}

	return streamer, toolDefs
}

func sortNativeMCPServers(servers []mcplib.NativeMCPServer) {
	sort.SliceStable(servers, func(i, j int) bool {
		if servers[i].Slug != servers[j].Slug {
			return servers[i].Slug < servers[j].Slug
		}
		return servers[i].Name < servers[j].Name
	})
	for i := range servers {
		servers[i].ToolNames = sortedToolNames(servers[i].ToolNames)
	}
}

func cloneNativeMCPServers(servers []mcplib.NativeMCPServer) []mcplib.NativeMCPServer {
	out := append([]mcplib.NativeMCPServer(nil), servers...)
	for i := range out {
		out[i].ToolNames = append([]string(nil), out[i].ToolNames...)
	}
	return out
}

func sortedToolNames(names []string) []string {
	out := append([]string(nil), names...)
	sort.Strings(out)
	return out
}
