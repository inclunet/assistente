package toolcatalog

import (
	"reflect"
	"testing"

	"assistente/internal/tools"
)

// builtin/mcp helpers para montar candidatas de forma legível nos testes.
func builtin(name string, bytes int, pkg string) ToolCandidate {
	return ToolCandidate{Name: name, SchemaBytes: bytes, Origin: tools.ToolOriginBuiltin, Package: pkg}
}

func bridge(name string, bytes int, pkg string) ToolCandidate {
	return ToolCandidate{Name: name, SchemaBytes: bytes, Origin: tools.ToolOriginMCPBridge, Package: pkg}
}

func itemReasons(plan Plan) map[string]PlanReason {
	m := make(map[string]PlanReason, len(plan.Items))
	for _, it := range plan.Items {
		m[it.Name] = it.Reason
	}
	return m
}

// TestPlanner_DefaultBudgetNaoCorta prova a regra de segurança principal: com
// budget default (0 = ilimitado), o planner não corta nada e preserva a ORDEM
// DE ENTRADA — base da não-regressão exigida pela Fase 4.
func TestPlanner_DefaultBudgetNaoCorta(t *testing.T) {
	in := []ToolCandidate{
		{Name: "tool_catalog", SchemaBytes: 100, Essential: true, Origin: tools.ToolOriginBuiltin},
		bridge("mcp_srv__do", 500, "mcp:srv"),
		builtin("read_file", 300, "coding_readonly"),
	}
	plan := PlannerConfig{}.Plan(in)

	if plan.DroppedCount != 0 {
		t.Fatalf("budget ilimitado não deveria cortar nada; cortadas=%d", plan.DroppedCount)
	}
	want := []string{"tool_catalog", "mcp_srv__do", "read_file"}
	if !reflect.DeepEqual(plan.Selected, want) {
		t.Fatalf("ordem de entrada deveria ser preservada\n got: %#v\nwant: %#v", plan.Selected, want)
	}
	if plan.UsedBytes != 900 {
		t.Fatalf("usados esperado 900, obtido %d", plan.UsedBytes)
	}
}

// TestPlanner_RankingDeterministico verifica que a telemetria (Items) sai na
// ordem de ranking estável: essenciais < perfil < pacote preferencial <
// builtins < MCP, com desempate alfabético por nome.
func TestPlanner_RankingDeterministico(t *testing.T) {
	in := []ToolCandidate{
		bridge("mcp_srv__do", 10, "mcp:srv"),
		builtin("write_file", 10, "coding_edit"),
		{Name: "load_skill", SchemaBytes: 10, Essential: true, Origin: tools.ToolOriginBuiltin},
		builtin("grep_search", 10, "coding_readonly"),
		{Name: "web_search", SchemaBytes: 10, Origin: tools.ToolOriginBuiltin, Package: "web", ProfilePinned: true},
		{Name: "calculator", SchemaBytes: 10, Origin: tools.ToolOriginBuiltin, Package: "math"},
	}
	cfg := PlannerConfig{PreferredPackages: []string{"coding_readonly"}}
	plan := cfg.Plan(in)

	gotOrder := make([]string, len(plan.Items))
	for i, it := range plan.Items {
		gotOrder[i] = it.Name
	}
	// load_skill (essencial) > web_search (perfil) > grep_search (pacote
	// preferencial) > calculator/write_file (builtins, alfabético) > mcp_srv__do.
	want := []string{"load_skill", "web_search", "grep_search", "calculator", "write_file", "mcp_srv__do"}
	if !reflect.DeepEqual(gotOrder, want) {
		t.Fatalf("ordem de ranking inesperada\n got: %#v\nwant: %#v", gotOrder, want)
	}
}

// TestPlanner_CortePorBudget verifica o corte determinístico pela ordem de
// ranking quando o budget é excedido, com o motivo correto na telemetria, e a
// preservação dos essenciais e da ordem de entrada na saída.
func TestPlanner_CortePorBudget(t *testing.T) {
	in := []ToolCandidate{
		{Name: "tool_catalog", SchemaBytes: 10, Essential: true, Origin: tools.ToolOriginBuiltin},
		builtin("read_file", 20, "coding_readonly"),
		builtin("write_file", 20, "coding_edit"),
		builtin("grep_search", 20, "coding_readonly"),
		bridge("mcp_srv__do", 20, "mcp:srv"),
	}
	plan := PlannerConfig{SchemaBytesBudget: 50}.Plan(in)

	// Essencial sempre entra (used=10); depois, em ordem alfabética de builtins,
	// grep_search(30) e read_file(50) cabem; write_file(70) e a bridge estouram.
	wantSelected := []string{"tool_catalog", "read_file", "grep_search"}
	if !reflect.DeepEqual(plan.Selected, wantSelected) {
		t.Fatalf("seleção sob budget inesperada\n got: %#v\nwant: %#v", plan.Selected, wantSelected)
	}
	if plan.UsedBytes != 50 {
		t.Fatalf("usados esperado 50, obtido %d", plan.UsedBytes)
	}
	if plan.DroppedCount != 2 {
		t.Fatalf("cortadas esperado 2, obtido %d", plan.DroppedCount)
	}
	reasons := itemReasons(plan)
	if reasons["write_file"] != PlanReasonBudgetExceeded || reasons["mcp_srv__do"] != PlanReasonBudgetExceeded {
		t.Fatalf("motivos de corte inesperados: %#v", reasons)
	}
	if reasons["tool_catalog"] != PlanReasonSelected {
		t.Fatalf("essencial deveria ser selecionada, obtido %s", reasons["tool_catalog"])
	}
}

// TestPlanner_PacotePreferencialPriorizado prova que, sob budget apertado, uma
// tool de pacote preferencial é mantida em detrimento de outra builtin que, sem
// a preferência, seria mantida pela ordem alfabética.
func TestPlanner_PacotePreferencialPriorizado(t *testing.T) {
	in := []ToolCandidate{
		builtin("alpha_tool", 20, "misc"),
		builtin("zeta_tool", 20, "priority_pkg"),
	}
	// Budget cabe só 1. Sem preferência, alpha_tool venceria (alfabético).
	semPref := PlannerConfig{SchemaBytesBudget: 20}.Plan(in)
	if !reflect.DeepEqual(semPref.Selected, []string{"alpha_tool"}) {
		t.Fatalf("sem preferência, esperado [alpha_tool], obtido %#v", semPref.Selected)
	}
	// Com priority_pkg preferencial, zeta_tool sobe de tier e vence.
	comPref := PlannerConfig{SchemaBytesBudget: 20, PreferredPackages: []string{"priority_pkg"}}.Plan(in)
	if !reflect.DeepEqual(comPref.Selected, []string{"zeta_tool"}) {
		t.Fatalf("com preferência, esperado [zeta_tool], obtido %#v", comPref.Selected)
	}
}

// TestPlanner_ConflitoBridgeNative verifica a resolução determinística do
// conflito bridge×native (AEP-0021): a bridge marcada como NativeServed é
// cortada com motivo native_conflict, independentemente do budget.
func TestPlanner_ConflitoBridgeNative(t *testing.T) {
	in := []ToolCandidate{
		builtin("read_file", 20, "coding_readonly"),
		{Name: "mcp_srv__do", SchemaBytes: 20, Origin: tools.ToolOriginMCPBridge, Package: "mcp:srv", NativeServed: true},
	}
	plan := PlannerConfig{}.Plan(in) // budget ilimitado: corte vem só do conflito

	if !reflect.DeepEqual(plan.Selected, []string{"read_file"}) {
		t.Fatalf("bridge nativa deveria ser removida; selecionadas=%#v", plan.Selected)
	}
	if itemReasons(plan)["mcp_srv__do"] != PlanReasonNativeConflict {
		t.Fatalf("motivo esperado native_conflict, obtido %s", itemReasons(plan)["mcp_srv__do"])
	}
}

// TestPlanner_Vazio garante robustez com entrada vazia.
func TestPlanner_Vazio(t *testing.T) {
	plan := PlannerConfig{SchemaBytesBudget: 100}.Plan(nil)
	if len(plan.Selected) != 0 || len(plan.Items) != 0 || plan.IncludedCount != 0 {
		t.Fatalf("entrada vazia deveria produzir plano vazio: %#v", plan)
	}
}
