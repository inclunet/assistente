package chat

import (
	"assistente/internal/logging"
	"context"
	"sort"
	"strings"

	"assistente/internal/llm"
	mcplib "assistente/internal/mcp"
	"assistente/internal/tools"
)

// ToolSelectionPolicy seleciona e orça as ferramentas autorizadas pelo perfil.
type ToolSelectionPolicy struct {
	registry *tools.Registry
}

// NewToolSelectionPolicy cria a policy ligada a um registry de tools. O registry
// pode ser nil; nesse caso os métodos degradam para "sem tools" como antes.
func NewToolSelectionPolicy(registry *tools.Registry) *ToolSelectionPolicy {
	return &ToolSelectionPolicy{registry: registry}
}

// ProfileToolConfig descreve a configuração de perfil/turno que governa a
// seleção. Semântica legada de EnabledTools:
//   - nil   → seleção dinâmica via tool_catalog; sem catálogo, falha fechado;
//   - []    → seleção explícita de zero tools (tool calling desligado);
//   - lista → allowlist explícita do perfil.
//
// ToolPolicy, quando presente, representa a política tri-state nova da AEP-0081
// por nome de tool: disabled, on_demand ou preloaded.
type ProfileToolConfig struct {
	EnabledTools []string
	ToolPolicy   map[string]string
	// ToolPolicyDefault define o estado das tools ausentes de ToolPolicy.
	// Vazio preserva a compatibilidade: políticas explícitas falham fechadas.
	ToolPolicyDefault string
	DisableTools      bool
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
	// SkillAllowedTools e SkillDeniedTools carregam a allowlist/denylist do skill
	// invocado no turno (AEP-0072 D5). São a MESMA fonte de verdade do gate do
	// executor (tools.ExecutionContext.AllowedTools/DeniedTools): quando presentes,
	// tools fora da allowlist (se não-vazia) ou dentro da denylist deixam de ser
	// anunciadas ao modelo, além de continuarem bloqueadas na execução (defesa em
	// profundidade). Escopo LOCAL/builtin; a allowlist de MCP nativo é outro
	// conceito, resolvido na montagem do plano do turno.
	SkillAllowedTools []string
	SkillDeniedTools  []string
}

// ---------------------------------------------------------------------------
// Alto nível: orquestração consolidada usada pelos call sites
// ---------------------------------------------------------------------------

// InitialEnabledToolNames resolve os nomes de tools habilitadas para o início do
// turno (perfil + runtime tools), antes de qualquer expansão dinâmica.
func (p *ToolSelectionPolicy) InitialEnabledToolNames(cfg ProfileToolConfig) []string {
	return p.ResolveEffectiveToolPolicy(cfg).PreloadedNames()
}

// TurnToolPlan guarda a seleção enviada e a identidade das tools delegadas.
// A expansão usa este snapshot, sem consultar novamente conexões ou credenciais.
type TurnToolPlan struct {
	Streamer           llm.ChatProvider
	Definitions        []llm.ToolDefinition
	AdapterDefinitions []llm.ToolDefinition
	NativeNames        map[string]struct{}
}

func (p *ToolSelectionPolicy) PlanTurnToolDefs(streamer llm.ChatProvider, mcpMgr NativeMCPManager, cfg ProfileToolConfig) TurnToolPlan {
	preloaded := p.ResolveEffectiveToolPolicy(cfg).PreloadedNames()
	raw := p.buildLLMToolDefs(preloaded, cfg.DisableTools)
	nativeStreamer, reduced := applyNativeMCP(streamer, raw, mcpMgr, preloaded, cfg.DisableTools, cfg.NativeMCP)
	plan := TurnToolPlan{Streamer: nativeStreamer, Definitions: p.applyPlanner(reduced, cfg, "inicial")}
	if nativeStreamer == streamer {
		return plan
	}
	plan.NativeNames = make(map[string]struct{})
	local := make(map[string]struct{}, len(reduced))
	for _, tool := range reduced {
		local[tool.Function.Name] = struct{}{}
	}
	for _, tool := range raw {
		if _, ok := local[tool.Function.Name]; !ok {
			plan.NativeNames[tool.Function.Name] = struct{}{}
		}
	}
	plan.AdapterDefinitions = p.applyPlanner(raw, cfg, "adapter")
	return plan
}

// ResolveExpandedToolDefs mantém as ativas e acrescenta tools autorizadas dentro
// do orçamento, excluindo somente as ferramentas delegadas no plano enviado.
func (p *ToolSelectionPolicy) ResolveExpandedToolDefs(nativeNames map[string]struct{}, active []llm.ToolDefinition, names []string, cfg ProfileToolConfig) []llm.ToolDefinition {
	effective := p.ResolveEffectiveToolPolicy(cfg)
	names = p.filterExpandedToolNames(names, cfg, effective)
	// O filtro nativo já remove aqui as bridges servidas via passthrough, então o
	// planner orça apenas os schemas realmente enviados como função.
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if _, native := nativeNames[name]; !native {
			filtered = append(filtered, name)
		}
	}
	names = filtered
	newDefs := p.buildLLMToolDefs(names, cfg.DisableTools)
	return p.planAccumulatedToolDefs(active, newDefs, cfg)
}

// ---------------------------------------------------------------------------
// Conversão do registry para definições do modelo
// ---------------------------------------------------------------------------

// buildLLMToolDefs constrói a lista de tool definitions para o LLM.
// Se disableTools for true ou a seleção for nil/vazia, retorna nil.
func (p *ToolSelectionPolicy) buildLLMToolDefs(enabledTools []string, disableTools bool) []llm.ToolDefinition {
	if disableTools || p.registry == nil || p.registry.Count() == 0 || len(enabledTools) == 0 {
		return nil
	}

	toolDefs := p.registry.FilterByNames(enabledTools)

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
func (p *ToolSelectionPolicy) filterExpandedToolNames(names []string, cfg ProfileToolConfig, effective EffectiveToolPolicy) []string {
	if cfg.DisableTools || len(names) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || !effective.AllowsRuntimeLoad(name) {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

// ---------------------------------------------------------------------------
// Baixo nível: helpers independentes do registry (MCP nativo e allowlist)
// ---------------------------------------------------------------------------

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
		}
		for _, tool := range srv.Tools {
			cfg.ToolNames = append(cfg.ToolNames, tool.RegisteredName)
		}

		if enabledSet != nil {
			var allowed []string
			var allowedFull []string
			for _, tool := range srv.Tools {
				fullName := tool.RegisteredName
				if enabledSet[fullName] {
					if tool.Name != "" {
						allowed = append(allowed, tool.Name)
					}
					allowedFull = append(allowedFull, fullName)
				}
			}
			if len(allowed) == 0 {
				logging.Infof(context.Background(), "chat.tool-selection-policy", "[chat] MCP nativo: servidor %q excluído (nenhuma tool habilitada no perfil)", srv.Name)
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
		logging.Infof(context.Background(), "chat.tool-selection-policy", "[chat] MCP nativo: %d servidores HTTP configurados", len(mcpConfigs))
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
			logging.Infof(context.Background(), "chat.tool-selection-policy", "[chat] MCP nativo: %d bridge tools removidas (nativas agora)", removed)
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
		sort.SliceStable(servers[i].Tools, func(a, b int) bool { return servers[i].Tools[a].RegisteredName < servers[i].Tools[b].RegisteredName })
	}
}

func cloneNativeMCPServers(servers []mcplib.NativeMCPServer) []mcplib.NativeMCPServer {
	out := append([]mcplib.NativeMCPServer(nil), servers...)
	for i := range out {
		out[i].Tools = append([]mcplib.NativeMCPTool(nil), out[i].Tools...)
	}
	return out
}
