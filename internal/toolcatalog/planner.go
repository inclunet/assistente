package toolcatalog

import (
	"fmt"
	"sort"
	"strings"

	"assistente/internal/tools"
)

// ToolPlanner é o planejador determinístico de seleção de tools de um turno
// (AEP-0077, Fase 4 / #121). Ele evolui a antiga listagem do catálogo para uma
// DECISÃO com orçamento, ranking e telemetria, respondendo "quais tools entram
// no contexto deste turno e POR QUÊ".
//
// Ele NÃO substitui nem duplica a resolução tri-state de MCP nativo (AEP-0021),
// que continua sendo decidida pelos helpers existentes da política de seleção
// (resolveNativeMCPEnabled / ApplyNativeMCP); o planner apenas CONSOME o
// resultado (a marca NativeServed em cada candidata) para deduplicar de forma
// determinística e registrar o motivo na telemetria.
//
// Garantias:
//   - Determinismo: mesmas entradas → mesma saída e mesma telemetria. A ordem de
//     ranking é total (tier + nome), sem iteração de mapas sem ordenação.
//   - Default seguro: budget <= 0 significa ILIMITADO; nesse caso nada é cortado
//     e a ordem de entrada é preservada na íntegra (sem regressão).

// PlanReason classifica o destino de uma candidata no relatório de telemetria.
type PlanReason string

const (
	// PlanReasonSelected: candidata incluída no contexto do turno.
	PlanReasonSelected PlanReason = "selected"
	// PlanReasonBudgetExceeded: candidata cortada porque incluí-la estouraria o
	// budget de schema bytes do turno.
	PlanReasonBudgetExceeded PlanReason = "budget_exceeded"
	// PlanReasonNativeConflict: bridge tool cortada porque a mesma capability é
	// atendida via MCP nativo (AEP-0021); decisão tri-state feita upstream.
	PlanReasonNativeConflict PlanReason = "native_conflict"
)

// Tiers de ranking (quanto MENOR, mais relevante e mais protegido do corte).
const (
	rankLocked        = -1 // já ativas no turno (reservam budget antes das novas)
	rankEssential     = 0  // tool_catalog, load_skill: nunca cortadas
	rankProfilePinned = 1  // listadas explicitamente no enabled_tools do perfil
	rankPreferredPkg  = 2  // pacotes marcados como preferenciais para o perfil
	rankBuiltin       = 3  // demais builtins essenciais da aplicação
	rankMCPBridge     = 4  // tools de pontes MCP
	rankOther         = 5  // origem desconhecida / fallback
)

// ToolCandidate descreve, de forma neutra (sem depender de llm/chat), uma tool
// candidata à seleção do turno. A política de seleção (internal/chat) constrói
// estas candidatas a partir do registry/catálogo e chama o planner.
type ToolCandidate struct {
	Name        string // nome único da tool (chave de seleção)
	SchemaBytes int    // bytes do JSON Schema injetados no contexto
	Origin      string // tools.ToolOrigin* (builtin, mcp_bridge, mcp_native)
	Package     string // pacote de catálogo (ToolCatalogEntry.Package)
	// Essential marca tools que jamais são cortadas (ex.: tool_catalog é a porta
	// de entrada da progressive disclosure; load_skill carrega skills sob demanda).
	Essential bool
	// ProfilePinned indica que a tool foi fixada explicitamente no enabled_tools
	// do perfil — sinal forte de relevância para o turno.
	ProfilePinned bool
	// NativeServed indica que esta bridge tool já é atendida por um servidor MCP
	// nativo elegível (AEP-0021). A DECISÃO de habilitar nativo é tomada upstream;
	// aqui apenas deduplicamos de forma determinística.
	NativeServed bool
	// Locked indica que a tool JÁ ESTÁ ativa no turno (já foi enviada ao LLM em
	// iterações anteriores). Tools travadas são sempre mantidas e consumem budget
	// ANTES das novas candidatas, de modo que o teto valha para o conjunto
	// ACUMULADO do turno (e não só para o delta recém-expandido). Nunca são
	// cortadas nem reordenadas para fora da seleção.
	Locked bool
}

// PlannerConfig parametriza a decisão do planner por perfil/superfície.
type PlannerConfig struct {
	// SchemaBytesBudget é o teto de bytes de schema injetados por turno.
	//   - <= 0 → ILIMITADO (default seguro: nunca corta, preserva ordem de entrada).
	//   - >  0 → corta deterministicamente pela ordem de ranking quando excedido.
	SchemaBytesBudget int
	// PreferredPackages lista pacotes priorizados no ranking para este perfil.
	// Tools cujo Package consta aqui sobem para o tier de pacote preferencial.
	PreferredPackages []string
}

// PlanItem registra o destino de uma candidata no relatório de telemetria.
type PlanItem struct {
	Name        string     `json:"name"`
	SchemaBytes int        `json:"schema_bytes"`
	Rank        int        `json:"rank"`
	Origin      string     `json:"origin,omitempty"`
	Package     string     `json:"package,omitempty"`
	Included    bool       `json:"included"`
	Reason      PlanReason `json:"reason"`
}

// Plan é a saída do planner: a lista final de nomes selecionados (na ORDEM DE
// ENTRADA, para não regredir comportamento) + telemetria observável do que
// entrou, do que foi cortado e por quê.
type Plan struct {
	// Selected são os nomes incluídos, preservando a ordem de entrada das
	// candidatas (determinística e estável).
	Selected []string `json:"selected"`
	// Items lista TODAS as candidatas (incluídas e cortadas) ordenadas por
	// ranking, para logging/observabilidade.
	Items []PlanItem `json:"items"`

	BudgetBytes   int `json:"budget_bytes"`
	UsedBytes     int `json:"used_bytes"`
	IncludedCount int `json:"included_count"`
	DroppedCount  int `json:"dropped_count"`
}

// SelectedSet devolve um conjunto dos nomes selecionados para filtragem rápida.
func (p Plan) SelectedSet() map[string]struct{} {
	set := make(map[string]struct{}, len(p.Selected))
	for _, name := range p.Selected {
		set[name] = struct{}{}
	}
	return set
}

// LogLine resume o plano numa linha estável (determinística) para logging.
func (p Plan) LogLine() string {
	budget := "ilimitado"
	if p.BudgetBytes > 0 {
		budget = fmt.Sprintf("%dB", p.BudgetBytes)
	}
	var dropped []string
	for _, it := range p.Items {
		if !it.Included {
			dropped = append(dropped, fmt.Sprintf("%s(%dB,%s)", it.Name, it.SchemaBytes, it.Reason))
		}
	}
	line := fmt.Sprintf("budget=%s usados=%dB incluídas=%d cortadas=%d",
		budget, p.UsedBytes, p.IncludedCount, p.DroppedCount)
	if len(dropped) > 0 {
		line += " | cortadas: " + strings.Join(dropped, ", ")
	}
	return line
}

// Plan executa o planejamento determinístico sobre as candidatas:
//
//  1. Resolução de conflito bridge×native (consome NativeServed; AEP-0021).
//  2. Ranking por relevância de perfil/superfície (essenciais > perfil >
//     pacote preferencial > builtins > MCP), estável por nome.
//  3. Budget de schema bytes: essenciais sempre entram; as demais entram na
//     ordem de ranking até o teto; o excedente é cortado com motivo.
//
// A lista final preserva a ORDEM DE ENTRADA das candidatas incluídas, de modo
// que com budget ilimitado (default) a saída seja idêntica à entrada.
func (cfg PlannerConfig) Plan(candidates []ToolCandidate) Plan {
	plan := Plan{
		BudgetBytes: cfg.SchemaBytesBudget,
		Selected:    make([]string, 0, len(candidates)),
		Items:       make([]PlanItem, 0, len(candidates)),
	}
	if len(candidates) == 0 {
		return plan
	}

	preferred := make(map[string]struct{}, len(cfg.PreferredPackages))
	for _, pkg := range cfg.PreferredPackages {
		if pkg = strings.TrimSpace(pkg); pkg != "" {
			preferred[pkg] = struct{}{}
		}
	}

	// Preserva a posição de entrada para reconstruir a saída na ordem original.
	type ranked struct {
		cand     ToolCandidate
		rank     int
		inputPos int
	}
	rankedCands := make([]ranked, len(candidates))
	for i, c := range candidates {
		rankedCands[i] = ranked{cand: c, rank: rankFor(c, preferred), inputPos: i}
	}

	// Ordena por ranking (tier asc, depois nome asc) — total e determinístico.
	order := make([]int, len(rankedCands))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ra, rb := rankedCands[order[a]], rankedCands[order[b]]
		if ra.rank != rb.rank {
			return ra.rank < rb.rank
		}
		return ra.cand.Name < rb.cand.Name
	})

	decided := make(map[int]PlanItem, len(rankedCands))
	used := 0
	unlimited := cfg.SchemaBytesBudget <= 0

	for _, idx := range order {
		rc := rankedCands[idx]
		item := PlanItem{
			Name:        rc.cand.Name,
			SchemaBytes: rc.cand.SchemaBytes,
			Rank:        rc.rank,
			Origin:      rc.cand.Origin,
			Package:     rc.cand.Package,
		}

		switch {
		case rc.cand.NativeServed && !rc.cand.Essential && !rc.cand.Locked:
			// Conflito bridge×native: nativo prevalece (AEP-0021).
			item.Included = false
			item.Reason = PlanReasonNativeConflict
		case rc.cand.Locked || rc.cand.Essential || unlimited:
			// Travadas (já ativas) e essenciais nunca são cortadas; ordenadas
			// antes das novas (rankLocked/rankEssential), reservam o budget que
			// consomem. Budget ilimitado inclui tudo.
			item.Included = true
			item.Reason = PlanReasonSelected
			used += rc.cand.SchemaBytes
		case used+rc.cand.SchemaBytes <= cfg.SchemaBytesBudget:
			item.Included = true
			item.Reason = PlanReasonSelected
			used += rc.cand.SchemaBytes
		default:
			item.Included = false
			item.Reason = PlanReasonBudgetExceeded
		}

		decided[rc.inputPos] = item
		if item.Included {
			plan.IncludedCount++
		} else {
			plan.DroppedCount++
		}
	}
	plan.UsedBytes = used

	// Saída na ORDEM DE ENTRADA (não-regressão); telemetria na ordem de ranking.
	for pos := 0; pos < len(candidates); pos++ {
		if decided[pos].Included {
			plan.Selected = append(plan.Selected, candidates[pos].Name)
		}
	}
	for _, idx := range order {
		plan.Items = append(plan.Items, decided[rankedCands[idx].inputPos])
	}
	return plan
}

// rankFor calcula o tier de relevância de uma candidata (determinístico).
func rankFor(c ToolCandidate, preferred map[string]struct{}) int {
	if c.Locked {
		return rankLocked
	}
	if c.Essential {
		return rankEssential
	}
	if c.ProfilePinned {
		return rankProfilePinned
	}
	if _, ok := preferred[strings.TrimSpace(c.Package)]; ok && c.Package != "" {
		return rankPreferredPkg
	}
	switch c.Origin {
	case tools.ToolOriginBuiltin:
		return rankBuiltin
	case tools.ToolOriginMCPBridge, tools.ToolOriginMCPNative:
		return rankMCPBridge
	default:
		return rankOther
	}
}
