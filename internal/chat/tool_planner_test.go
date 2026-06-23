package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/tools"
)

// toolWithSchema cria uma mock tool cujo JSON Schema tem EXATAMENTE schemaLen
// bytes, para exercitar o budget do ToolPlanner de forma previsível.
func toolWithSchema(name string, schemaLen int) *mockToolDef {
	const prefix = `{"x":"`
	const suffix = `"}`
	pad := schemaLen - len(prefix) - len(suffix)
	if pad < 0 {
		pad = 0
	}
	return &mockToolDef{
		name:   name,
		descr:  "descr:" + name,
		params: json.RawMessage(prefix + strings.Repeat("a", pad) + suffix),
	}
}

func plannerRegistry() *tools.Registry {
	r := tools.NewRegistry()
	r.MustRegister(toolWithSchema(tools.ToolCatalogName, 30))
	r.MustRegister(toolWithSchema("read_file", 30))
	r.MustRegister(toolWithSchema("write_file", 30))
	r.MustRegister(toolWithSchema("grep_search", 30))
	return r
}

// TestPolicyPlanner_DefaultBudgetSemRegressao prova que, com budget default
// (0 = ilimitado), o ToolPlanner não corta nada: InitialToolDefs devolve o MESMO
// conjunto/ordem que a montagem sem planner. É a garantia de não-regressão F4.
func TestPolicyPlanner_DefaultBudgetSemRegressao(t *testing.T) {
	r := plannerRegistry()
	policy := NewToolSelectionPolicy(r)
	enabled := []string{"read_file", "write_file", "grep_search"}

	// Sem budget (default) deve coincidir exatamente com a montagem direta.
	legacy := defNames(BuildLLMToolDefs(r, enabled, false))
	got := defNames(policy.InitialToolDefs(ProfileToolConfig{EnabledTools: enabled}))
	assertNames(t, "default budget == legacy", got, legacy)

	// E a ordem canônica (alfabética) é preservada.
	assertNames(t, "ordem canônica", got, []string{"grep_search", "read_file", "write_file"})
}

// TestPolicyPlanner_BudgetCortaPorRanking verifica o corte determinístico por
// budget na montagem inicial, mantendo a ordem de entrada na saída.
func TestPolicyPlanner_BudgetCortaPorRanking(t *testing.T) {
	r := plannerRegistry()
	policy := NewToolSelectionPolicy(r)
	enabled := []string{"read_file", "write_file", "grep_search"}

	// Cada schema tem 30 bytes; budget 60 cabe 2. Todas pinned (mesmo tier) →
	// desempate alfabético: grep_search e read_file entram; write_file corta.
	got := defNames(policy.InitialToolDefs(ProfileToolConfig{
		EnabledTools:      enabled,
		SchemaBytesBudget: 60,
	}))
	assertNames(t, "budget 60 corta write_file", got, []string{"grep_search", "read_file"})
}

// TestPolicyPlanner_EssencialNuncaCortada garante que tool_catalog (essencial)
// sobrevive mesmo com budget que não comportaria as demais.
func TestPolicyPlanner_EssencialNuncaCortada(t *testing.T) {
	r := plannerRegistry()
	policy := NewToolSelectionPolicy(r)
	// enabled nil + catálogo presente → seleção inicial é só o tool_catalog.
	got := defNames(policy.InitialToolDefs(ProfileToolConfig{SchemaBytesBudget: 1}))
	assertNames(t, "essencial preservada sob budget mínimo", got, []string{tools.ToolCatalogName})
}

// TestPolicyPlanner_ExpansaoComBudget verifica o corte por budget também na
// expansão dinâmica (ResolveExpandedToolDefs), o segundo ponto de integração.
func TestPolicyPlanner_ExpansaoComBudget(t *testing.T) {
	r := plannerRegistry()
	policy := NewToolSelectionPolicy(r)

	// enabled nil (catálogo dinâmico) → tools não-pinned; budget 60 cabe 2 (tier
	// builtin, alfabético): grep_search e read_file entram, write_file corta.
	names := []string{"read_file", "write_file", "grep_search"}
	got := defNames(policy.ResolveExpandedToolDefs(nil, nil, names, ProfileToolConfig{SchemaBytesBudget: 60}))
	assertNames(t, "expansão sob budget", got, []string{"grep_search", "read_file"})
}
