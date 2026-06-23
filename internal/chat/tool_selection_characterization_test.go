package chat

import (
	"testing"

	"assistente/internal/llm"
	mcplib "assistente/internal/mcp"
	"assistente/internal/tools"
)

// ---------------------------------------------------------------------------
// Testes de caracterização (AEP-0077 Fase 3, #119)
//
// Estes testes travam (snapshot) o comportamento ATUAL do pipeline de seleção
// de tools — montado hoje de forma dispersa entre tool_defs.go e o callback de
// expansão dinâmica no use case de envio. Eles devem permanecer verdes ANTES e
// DEPOIS da consolidação em ToolSelectionPolicy: a refatoração não pode mudar
// quais tools são oferecidas em nenhum cenário (regra de ouro de paridade).
//
// "legacy*" reproduz a composição exata dos call sites atuais (send_message.go).
// Após a consolidação, ToolSelectionPolicy é exercitada com os MESMOS cenários
// em tool_selection_policy_test.go, garantindo equivalência.
// ---------------------------------------------------------------------------

// charRegistry monta um registry representativo: builtins gateadas pelo catálogo,
// uma tool de runtime (load_skill), uma bridge MCP e uma tool opt-in (text_edit).
func charRegistry(t *testing.T) *tools.Registry {
	t.Helper()
	r := tools.NewRegistry()
	for _, n := range []string{tools.ToolCatalogName, tools.LoadSkillName, "read_file", "grep_search", "write_file", "mcp_srv__do"} {
		r.MustRegister(newToolDef(n))
	}
	r.MustRegisterOptIn(newToolDef("text_edit"))
	return r
}

func charRegistryNoCatalog(t *testing.T) *tools.Registry {
	t.Helper()
	r := tools.NewRegistry()
	for _, n := range []string{"read_file", "grep_search"} {
		r.MustRegister(newToolDef(n))
	}
	return r
}

func defNames(defs []llm.ToolDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Function.Name
	}
	return names
}

func assertNames(t *testing.T, scenario string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %#v, want %#v", scenario, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: got %#v, want %#v", scenario, got, want)
		}
	}
}

// legacyInitialDefs reproduz a montagem inicial atual:
//
//	initial := ResolveInitialEnabledToolsWithRuntime(...)
//	defs    := BuildLLMToolDefs(registry, initial, disable)
func legacyInitialDefs(r *tools.Registry, enabled []string, disable bool, runtime []string) []llm.ToolDefinition {
	initial := ResolveInitialEnabledToolsWithRuntime(r, enabled, disable, runtime)
	return BuildLLMToolDefs(r, initial, disable)
}

// legacyExpandedDefs reproduz o callback de expansão dinâmica atual
// (filterExpandedToolNames + FilterToolNamesForNativeMCP + BuildLLMToolDefsByNames).
func legacyExpandedDefs(r *tools.Registry, streamer llm.ChatProvider, mgr NativeMCPManager, names, enabled []string, disable bool, override *bool) []llm.ToolDefinition {
	names = FilterToolNamesByEnabledTools(names, enabled, disable)
	if enabled == nil && r != nil {
		names = r.FilterOutOptInNames(names)
	}
	names = FilterToolNamesForNativeMCP(streamer, mgr, names, disable, override)
	return BuildLLMToolDefsByNames(r, names, disable)
}

// initialSelectionCases são os cenários canônicos de seleção inicial por perfil.
// Reusados pelo teste de paridade da policy.
type initialSelectionCase struct {
	name     string
	registry *tools.Registry
	enabled  []string
	disable  bool
	runtime  []string
	want     []string
}

func initialSelectionCases(t *testing.T) []initialSelectionCase {
	r := charRegistry(t)
	noCatalog := charRegistryNoCatalog(t)
	return []initialSelectionCase{
		{name: "enabledNil_semRuntime_gateadoPeloCatalogo", registry: r, enabled: nil, want: []string{tools.ToolCatalogName}},
		{name: "enabledNil_comLoadSkillRuntime", registry: r, enabled: nil, runtime: []string{tools.LoadSkillName}, want: []string{tools.ToolCatalogName, tools.LoadSkillName}},
		{name: "enabledVazioExplicito_ficaVazio", registry: r, enabled: []string{}, runtime: []string{tools.LoadSkillName}, want: []string{}},
		{name: "enabledExplicito_appendLoadSkill", registry: r, enabled: []string{"read_file", "write_file"}, runtime: []string{tools.LoadSkillName}, want: []string{tools.LoadSkillName, "read_file", "write_file"}},
		{name: "semCatalogo_enabledNil_caiEmTodasAsTools", registry: noCatalog, enabled: nil, want: []string{"grep_search", "read_file"}},
		{name: "disableTools_retornaNada", registry: r, enabled: []string{"read_file"}, disable: true, want: []string{}},
	}
}

func TestCharacterization_InitialSelectionByProfile(t *testing.T) {
	for _, tc := range initialSelectionCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			got := defNames(legacyInitialDefs(tc.registry, tc.enabled, tc.disable, tc.runtime))
			assertNames(t, tc.name, got, tc.want)
		})
	}
}

// dynamicExpansionCases são os cenários canônicos da expansão dinâmica.
type dynamicExpansionCase struct {
	name     string
	names    []string
	enabled  []string
	disable  bool
	native   bool // se true, usa streamer capaz + manager com bridge mcp_srv__do
	override *bool
	want     []string
}

func dynamicExpansionCases() []dynamicExpansionCase {
	return []dynamicExpansionCase{
		{name: "enabledNil_removeOptIn", names: []string{"read_file", "grep_search", "text_edit"}, want: []string{"grep_search", "read_file"}},
		{name: "enabledAllowlist_filtraPorPerfil", names: []string{"read_file", "grep_search", "text_edit"}, enabled: []string{"read_file", "text_edit"}, want: []string{"read_file", "text_edit"}},
		{name: "mcpNativoForcado_removeBridge", names: []string{"read_file", "mcp_srv__do"}, native: true, override: boolPtr(true), want: []string{"read_file"}},
		{name: "mcpAdapterForcado_mantemBridge", names: []string{"read_file", "mcp_srv__do"}, native: true, override: boolPtr(false), want: []string{"mcp_srv__do", "read_file"}},
		{name: "disableTools_retornaNada", names: []string{"read_file"}, disable: true, want: []string{}},
	}
}

func expansionStreamerAndMgr(native bool) (llm.ChatProvider, NativeMCPManager) {
	if !native {
		return nil, nil
	}
	return &mockChatProvider{nativeCapable: true}, &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "srv", Name: "Srv", URL: "https://srv.io", ToolNames: []string{"mcp_srv__do"}},
	}}
}

func TestCharacterization_DynamicExpansion(t *testing.T) {
	r := charRegistry(t)
	for _, tc := range dynamicExpansionCases() {
		t.Run(tc.name, func(t *testing.T) {
			streamer, mgr := expansionStreamerAndMgr(tc.native)
			got := defNames(legacyExpandedDefs(r, streamer, mgr, tc.names, tc.enabled, tc.disable, tc.override))
			assertNames(t, tc.name, got, tc.want)
		})
	}
}

// TestCharacterization_InitialThenApplyNativeMCP fixa o pipeline completo:
// montagem inicial → ApplyNativeMCP remove a bridge que virou nativa.
func TestCharacterization_InitialThenApplyNativeMCP(t *testing.T) {
	r := charRegistry(t)
	capable := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "srv", Name: "Srv", URL: "https://srv.io", ToolNames: []string{"mcp_srv__do"}},
	}}
	enabled := []string{"read_file", "mcp_srv__do"}

	legacyDefs := BuildLLMToolDefs(r, enabled, false)
	_, legacyOut := ApplyNativeMCP(capable, legacyDefs, mgr, enabled, false, boolPtr(true))
	assertNames(t, "legacy", defNames(legacyOut), []string{"read_file"})
}
