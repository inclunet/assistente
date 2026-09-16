package chat

import (
	"testing"

	"assistente/internal/tools"
)

// Estes testes cobrem o escopo de tools do skill invocado (AEP-0072 D5): tools
// bloqueadas pela allowlist/denylist do skill NÃO podem ser anunciadas ao modelo.
// É a contraparte, na seleção, do gate de execução em
// internal/tools/executor.go (validateExecutionContextToolAccess), reusando as
// MESMAS listas para não divergir. A raiz do bug de produção era justamente uma
// tool (ex.: `memory`) fora da allowlist do skill continuar sendo oferecida,
// tentada pelo modelo e só então barrada — falhando repetidamente.

func containsName(names []string, target string) bool {
	for _, n := range names {
		if n == target {
			return true
		}
	}
	return false
}

// Allowlist não-vazia: só as tools listadas pelo skill sobrevivem nas defs.
func TestToolSelectionPolicy_SkillAllowlist_OmiteToolForaDaLista(t *testing.T) {
	policy := NewToolSelectionPolicy(charRegistry(t))
	got := defNames(policy.InitialToolDefs(ProfileToolConfig{
		EnabledTools:      []string{"read_file", "write_file", "grep_search"},
		SkillAllowedTools: []string{"read_file", "write_file"},
	}))
	assertNames(t, "allowlist do skill", got, []string{"read_file", "write_file"})
	if containsName(got, "grep_search") {
		t.Fatalf("grep_search deveria sumir das defs sob allowlist do skill: %#v", got)
	}
}

// Sem escopo do skill (listas vazias), a seleção permanece idêntica à do perfil.
func TestToolSelectionPolicy_SkillScopeVazio_NaoAltera(t *testing.T) {
	policy := NewToolSelectionPolicy(charRegistry(t))
	cfg := ProfileToolConfig{EnabledTools: []string{"read_file", "write_file", "grep_search"}}
	base := defNames(policy.InitialToolDefs(cfg))
	assertNames(t, "sem escopo do skill", base, []string{"grep_search", "read_file", "write_file"})
}

// Denylist do skill remove a tool mesmo quando o perfil a habilita.
func TestToolSelectionPolicy_SkillDenylist_RemoveToolHabilitada(t *testing.T) {
	policy := NewToolSelectionPolicy(charRegistry(t))
	got := defNames(policy.InitialToolDefs(ProfileToolConfig{
		EnabledTools:     []string{"read_file", "write_file", "grep_search"},
		SkillDeniedTools: []string{"grep_search"},
	}))
	assertNames(t, "denylist do skill", got, []string{"read_file", "write_file"})
	if containsName(got, "grep_search") {
		t.Fatalf("grep_search deveria ser removida pela denylist do skill: %#v", got)
	}
}

// A denylist vence a allowlist para a mesma tool (defesa consistente com o gate).
func TestToolSelectionPolicy_SkillDenylist_VenceAllowlist(t *testing.T) {
	policy := NewToolSelectionPolicy(charRegistry(t))
	got := defNames(policy.InitialToolDefs(ProfileToolConfig{
		EnabledTools:      []string{"read_file", "write_file"},
		SkillAllowedTools: []string{"read_file", "write_file"},
		SkillDeniedTools:  []string{"write_file"},
	}))
	assertNames(t, "denylist vence allowlist", got, []string{"read_file"})
}

// Expansão dinâmica: uma tool fora da allowlist do skill não pode ser carregada
// via tool_catalog, mesmo que o perfil (aberto) a exponha como on_demand.
func TestToolSelectionPolicy_SkillAllowlist_BloqueiaExpansaoDinamica(t *testing.T) {
	policy := NewToolSelectionPolicy(charRegistry(t))
	cfg := ProfileToolConfig{
		// Perfil legado aberto: read_file/grep_search são on_demand e o catálogo
		// as descobre. O skill precisa manter o catálogo para permitir a carga.
		SkillAllowedTools: []string{tools.ToolCatalogName, "read_file"},
	}
	got := defNames(policy.ResolveExpandedToolDefs(nil, nil, nil, []string{"read_file", "grep_search"}, cfg))
	assertNames(t, "expansão sob allowlist do skill", got, []string{"read_file"})
	if containsName(got, "grep_search") {
		t.Fatalf("grep_search fora da allowlist do skill não deveria ser expandida: %#v", got)
	}
}

// PlanTurnToolDefs é o método que o pipeline de envio usa para montar os
// conjuntos native/adapter enviados ao provider: ele também precisa respeitar o
// escopo do skill nos dois caminhos.
func TestToolSelectionPolicy_PlanTurnToolDefs_RespeitaEscopoDoSkill(t *testing.T) {
	policy := NewToolSelectionPolicy(charRegistry(t))
	_, nativeDefs, adapterDefs := policy.PlanTurnToolDefs(nil, nil, ProfileToolConfig{
		EnabledTools:      []string{"read_file", "write_file", "grep_search"},
		SkillAllowedTools: []string{"read_file"},
	})
	assertNames(t, "plan nativo", defNames(nativeDefs), []string{"read_file"})
	assertNames(t, "plan adapter", defNames(adapterDefs), []string{"read_file"})
}

// O catálogo visível ao modelo também respeita a denylist do skill: a tool
// bloqueada não é descoberta.
func TestToolSelectionPolicy_SkillDenylist_OcultaDoCatalogo(t *testing.T) {
	policy := NewToolSelectionPolicy(charRegistry(t))
	effective := policy.ResolveEffectiveToolPolicy(ProfileToolConfig{
		SkillDeniedTools: []string{"grep_search"},
	})
	visible := effective.CatalogVisibleNames()
	if containsName(visible, "grep_search") {
		t.Fatalf("grep_search bloqueada pelo skill não deveria aparecer no catálogo: %#v", visible)
	}
	if !containsName(visible, "read_file") {
		t.Fatalf("read_file (não bloqueada) deveria continuar visível no catálogo: %#v", visible)
	}
	if effective.AllowsRuntimeLoad("grep_search") {
		t.Fatal("grep_search bloqueada pelo skill não deveria permitir carga em runtime")
	}
}

// baseScopeRegistry inclui, além de tools de domínio, uma tool base de runtime
// (memory) e o control-plane (tool_catalog/load_skill), para exercitar o
// conjunto base protegido na seleção.
func baseScopeRegistry(t *testing.T) *tools.Registry {
	t.Helper()
	r := tools.NewRegistry()
	for _, n := range []string{tools.ToolCatalogName, tools.LoadSkillName, "memory", "read_file", "grep_search"} {
		r.MustRegister(newToolDef(n))
	}
	return r
}

// A allowlist do skill não pode rebaixar o conjunto base protegido: mesmo omitida
// da allowlist, memory (base de runtime) segue anunciada, enquanto uma tool de
// domínio fora da allowlist some. Contraparte, na seleção, da isenção do gate
// (tools.IsProtectedBaseTool em validateExecutionContextToolAccess).
func TestToolSelectionPolicy_SkillAllowlist_NaoRebaixaConjuntoBase(t *testing.T) {
	policy := NewToolSelectionPolicy(baseScopeRegistry(t))
	got := defNames(policy.InitialToolDefs(ProfileToolConfig{
		EnabledTools:      []string{"memory", "read_file", "grep_search"},
		SkillAllowedTools: []string{"read_file"}, // allowlist de domínio, omite a base
	}))
	assertNames(t, "allowlist não rebaixa base", got, []string{"memory", "read_file"})
	if containsName(got, "grep_search") {
		t.Fatalf("grep_search de domínio deveria sumir sob allowlist do skill: %#v", got)
	}
}

// Caminho catalog-first: a allowlist do skill não remove o control-plane. O
// tool_catalog permanece preloaded (coerente com o prompt/EnabledTools) e as
// tools base seguem visíveis no catálogo, enquanto a tool de domínio fora da
// allowlist deixa de ser descoberta.
func TestToolSelectionPolicy_SkillAllowlist_CatalogFirstMantemBase(t *testing.T) {
	policy := NewToolSelectionPolicy(baseScopeRegistry(t))
	effective := policy.ResolveEffectiveToolPolicy(ProfileToolConfig{
		// EnabledTools nil → catalog-first: tools viram on_demand e o catálogo as descobre.
		SkillAllowedTools: []string{"read_file"},
	})

	if effective.State(tools.ToolCatalogName) != ToolPolicyPreloaded {
		t.Fatalf("tool_catalog protegido deveria permanecer preloaded; got %q", effective.State(tools.ToolCatalogName))
	}
	// Coerência prompt↔defs: o control-plane preloaded é justamente o que o prompt anuncia.
	if !containsName(effective.PreloadedNames(), tools.ToolCatalogName) {
		t.Fatalf("tool_catalog deveria constar em PreloadedNames (coerência com o prompt): %#v", effective.PreloadedNames())
	}

	visible := effective.CatalogVisibleNames()
	for _, base := range []string{"memory", tools.LoadSkillName} {
		if !containsName(visible, base) {
			t.Fatalf("tool base %q deveria seguir visível no catálogo sob allowlist do skill: %#v", base, visible)
		}
	}
	if !containsName(visible, "read_file") {
		t.Fatalf("read_file na allowlist deveria seguir visível: %#v", visible)
	}
	if containsName(visible, "grep_search") {
		t.Fatalf("grep_search fora da allowlist não deveria aparecer no catálogo: %#v", visible)
	}
}

// O estado disabled do perfil (AEP-0081 D2) NÃO é afetado pelo conjunto base
// protegido: se o perfil desliga memory, ela permanece indisponível mesmo sendo
// base — a isenção vale só contra o narrowing IMPLÍCITO da allowlist, nunca eleva.
func TestToolSelectionPolicy_PerfilDisabled_RemoveBaseMesmoProtegida(t *testing.T) {
	policy := NewToolSelectionPolicy(baseScopeRegistry(t))
	effective := policy.ResolveEffectiveToolPolicy(ProfileToolConfig{
		ToolPolicy: map[string]string{
			"memory":    string(ToolPolicyDisabled),
			"read_file": string(ToolPolicyPreloaded),
		},
		ToolPolicyDefault: string(ToolPolicyDisabled),
		SkillAllowedTools: []string{"read_file"},
	})
	if effective.State("memory") != ToolPolicyDisabled {
		t.Fatalf("memory desligada pelo perfil deveria permanecer disabled mesmo sendo base; got %q", effective.State("memory"))
	}
	if containsName(effective.PreloadedNames(), "memory") {
		t.Fatalf("memory desligada pelo perfil não deveria constar como preloaded: %#v", effective.PreloadedNames())
	}
}
