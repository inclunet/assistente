package chat

import (
	"testing"

	"assistente/internal/llm"
	mcplib "assistente/internal/mcp"
	"assistente/internal/tools"
)

// Fixtures e cenários compartilhados pelos testes da política de seleção.

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

// initialSelectionCases são os cenários canônicos de seleção inicial por perfil.
// Reusados pelos testes da política de seleção.
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
		{name: "semCatalogo_enabledNil_falhaFechado", registry: noCatalog, enabled: nil, want: []string{}},
		{name: "disableTools_retornaNada", registry: r, enabled: []string{"read_file"}, disable: true, want: []string{}},
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
		{name: "mcpNativoForcado_mantemBridgeOnDemand", names: []string{"read_file", "mcp_srv__do"}, native: true, override: boolPtr(true), want: []string{"mcp_srv__do", "read_file"}},
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
