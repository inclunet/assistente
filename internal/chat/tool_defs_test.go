package chat

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/llm"
	mcplib "assistente/internal/mcp"
	"assistente/internal/tools"
)

// ---------------------------------------------------------------------------
// Helpers / mocks
// ---------------------------------------------------------------------------

type mockToolDef struct {
	identity tools.MCPToolIdentity
	name     string
	descr    string
	params   json.RawMessage
}

func (m *mockToolDef) MCPIdentity() tools.MCPToolIdentity { return m.identity }

func newMCPToolDef(slug, name string) *mockToolDef {
	tool := newToolDef("mcp_" + slug + "__" + name)
	tool.identity = tools.MCPToolIdentity{ServerSlug: slug, ToolName: name}
	return tool
}

func (m *mockToolDef) Name() string                { return m.name }
func (m *mockToolDef) Description() string         { return m.descr }
func (m *mockToolDef) Parameters() json.RawMessage { return m.params }
func (m *mockToolDef) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "ok"}, nil
}

func newToolDef(name string) *mockToolDef {
	return &mockToolDef{
		name:   name,
		descr:  "descr:" + name,
		params: json.RawMessage(`{}`),
	}
}

func registryWith(names ...string) *tools.Registry {
	r := tools.NewRegistry()
	for _, n := range names {
		r.MustRegister(newToolDef(n))
	}
	return r
}

// mockChatProvider implements llm.ChatProvider for tests.
type mockChatProvider struct {
	// nativeCapable controla NativeMCPCapable() — a única dimensão de provider que
	// influencia MCP nativo. Não há mais default por URL/endpoint.
	nativeCapable bool
	calledWith    []llm.MCPServerConfig
}

func (m *mockChatProvider) StreamChat(_ context.Context, _ []llm.Message, _ llm.ChatParams, _ llm.StreamHandler, _ ...llm.ToolDefinition) {
}
func (m *mockChatProvider) SendChat(_ context.Context, _ []llm.Message, _ llm.ChatParams) (string, error) {
	return "", nil
}
func (m *mockChatProvider) GetModels(_ context.Context) ([]string, error) { return nil, nil }
func (m *mockChatProvider) SimpleChat(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (m *mockChatProvider) NativeMCPCapable() bool { return m.nativeCapable }
func (m *mockChatProvider) WithMCPServers(servers []llm.MCPServerConfig) llm.ChatProvider {
	clone := &mockChatProvider{nativeCapable: m.nativeCapable, calledWith: servers}
	return clone
}

// mockNativeMCPMgr implements NativeMCPManager.
type mockNativeMCPMgr struct {
	servers []mcplib.NativeMCPServer
}

func (m *mockNativeMCPMgr) GetEligibleNativeMCPServers(context.Context, []string) []mcplib.NativeMCPServer {
	return m.servers
}

// makeToolDefs builds []llm.ToolDefinition with given names.
func makeToolDefs(names ...string) []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, len(names))
	for i, n := range names {
		defs[i] = llm.ToolDefinition{
			Type:     "function",
			Function: llm.FunctionDefinition{Name: n, Description: "d"},
		}
	}
	return defs
}

// ---------------------------------------------------------------------------
// BuildLLMToolDefs
// ---------------------------------------------------------------------------

func TestBuildLLMToolDefs_DisableTools(t *testing.T) {
	r := registryWith("toolA", "toolB")
	got := NewToolSelectionPolicy(r).buildLLMToolDefs(nil, true)
	if got != nil {
		t.Fatalf("esperava nil, obteve %v", got)
	}
}

func TestBuildLLMToolDefs_NilRegistry(t *testing.T) {
	got := NewToolSelectionPolicy(nil).buildLLMToolDefs(nil, false)
	if got != nil {
		t.Fatalf("esperava nil para registry nil, obteve %v", got)
	}
}

func TestBuildLLMToolDefs_EmptyRegistry(t *testing.T) {
	got := NewToolSelectionPolicy(tools.NewRegistry()).buildLLMToolDefs(nil, false)
	if got != nil {
		t.Fatalf("esperava nil para registry vazio, obteve %v", got)
	}
}

func TestBuildLLMToolDefs_NilSelectionFailsClosed(t *testing.T) {
	r := registryWith("alpha", "beta", "gamma")
	got := NewToolSelectionPolicy(r).buildLLMToolDefs(nil, false)
	if len(got) != 0 {
		t.Fatalf("seleção nil não deve expor o registry, obteve %v", got)
	}
}

func TestBuildLLMToolDefs_FilteredTools(t *testing.T) {
	r := registryWith("alpha", "beta", "gamma")
	got := NewToolSelectionPolicy(r).buildLLMToolDefs([]string{"beta"}, false)
	if len(got) != 1 {
		t.Fatalf("esperava 1 def filtrada, obteve %d", len(got))
	}
	if got[0].Function.Name != "beta" {
		t.Errorf("esperava beta, obteve %q", got[0].Function.Name)
	}
}

func TestBuildLLMToolDefs_EnabledToolsEmptySlice(t *testing.T) {
	r := registryWith("alpha")
	got := NewToolSelectionPolicy(r).buildLLMToolDefs([]string{}, false)
	if len(got) != 0 {
		t.Fatalf("esperava slice vazio, obteve %v", got)
	}
}

func TestBuildLLMToolDefs_ParametersPreserved(t *testing.T) {
	r := tools.NewRegistry()
	params := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	r.MustRegister(&mockToolDef{name: "read_file", descr: "reads", params: params})
	got := NewToolSelectionPolicy(r).buildLLMToolDefs([]string{"read_file"}, false)
	if len(got) != 1 {
		t.Fatalf("esperava 1 def, obteve %d", len(got))
	}
	if string(got[0].Function.Parameters) != string(params) {
		t.Errorf("Parameters nao preservado: %s", got[0].Function.Parameters)
	}
}

func TestResolveInitialEnabledTools_UsesCatalogWhenProfileDoesNotPinTools(t *testing.T) {
	r := registryWith(tools.ToolCatalogName, "read_file", "grep_search")
	got := NewToolSelectionPolicy(r).InitialEnabledToolNames(ProfileToolConfig{EnabledTools: nil, DisableTools: false})
	if len(got) != 1 || got[0] != tools.ToolCatalogName {
		t.Fatalf("expected only tool_catalog initially, got %#v", got)
	}
	defs := NewToolSelectionPolicy(r).buildLLMToolDefs(got, false)
	if len(defs) != 1 || defs[0].Function.Name != tools.ToolCatalogName {
		t.Fatalf("expected only tool_catalog definition, got %#v", defs)
	}
}

func TestResolveInitialEnabledTools_PreservesExplicitProfileSelection(t *testing.T) {
	r := registryWith(tools.ToolCatalogName, "read_file", "grep_search")
	got := NewToolSelectionPolicy(r).InitialEnabledToolNames(ProfileToolConfig{EnabledTools: []string{"read_file"}, DisableTools: false})
	if len(got) != 1 || got[0] != "read_file" {
		t.Fatalf("expected explicit enabled tools unchanged, got %#v", got)
	}
}

func TestResolveInitialEnabledTools_EmptyExplicitSelectionStaysEmpty(t *testing.T) {
	r := registryWith(tools.ToolCatalogName, "read_file")
	got := NewToolSelectionPolicy(r).InitialEnabledToolNames(ProfileToolConfig{EnabledTools: []string{}, DisableTools: false})
	if got == nil || len(got) != 0 {
		t.Fatalf("expected explicit empty selection, got %#v", got)
	}
	defs := NewToolSelectionPolicy(r).buildLLMToolDefs(got, false)
	if len(defs) != 0 {
		t.Fatalf("expected no tool definitions, got %#v", defs)
	}
}

func TestResolveInitialEnabledToolsWithRuntime_AppendsLoadSkillToCatalogFirst(t *testing.T) {
	r := registryWith(tools.ToolCatalogName, tools.LoadSkillName, "read_file")
	got := NewToolSelectionPolicy(r).InitialEnabledToolNames(ProfileToolConfig{EnabledTools: nil, DisableTools: false, RuntimeTools: []string{tools.LoadSkillName}})
	if len(got) != 2 || got[0] != tools.ToolCatalogName || got[1] != tools.LoadSkillName {
		t.Fatalf("expected catalog plus load_skill, got %#v", got)
	}
}

func TestResolveInitialEnabledToolsWithRuntime_ExplicitEmptyKeepsToolCallingOff(t *testing.T) {
	r := registryWith(tools.ToolCatalogName, tools.LoadSkillName, "read_file")
	got := NewToolSelectionPolicy(r).InitialEnabledToolNames(ProfileToolConfig{EnabledTools: []string{}, DisableTools: false, RuntimeTools: []string{tools.LoadSkillName}})
	if got == nil || len(got) != 0 {
		t.Fatalf("expected explicit empty selection, got %#v", got)
	}
}

func TestResolveInitialEnabledToolsWithRuntime_AppendsToExplicitTools(t *testing.T) {
	r := registryWith(tools.ToolCatalogName, tools.LoadSkillName, "read_file")
	got := NewToolSelectionPolicy(r).InitialEnabledToolNames(ProfileToolConfig{EnabledTools: []string{"read_file"}, DisableTools: false, RuntimeTools: []string{tools.LoadSkillName}})
	if len(got) != 2 || got[0] != tools.LoadSkillName || got[1] != "read_file" {
		t.Fatalf("expected explicit tools plus load_skill, got %#v", got)
	}
}

func TestResolveInitialEnabledTools_FailsClosedWithoutCatalog(t *testing.T) {
	r := registryWith("read_file", "grep_search")
	got := NewToolSelectionPolicy(r).InitialEnabledToolNames(ProfileToolConfig{EnabledTools: nil, DisableTools: false})
	if got == nil || len(got) != 0 {
		t.Fatalf("expected explicit fail-closed selection without catalog, got %#v", got)
	}
	defs := NewToolSelectionPolicy(r).buildLLMToolDefs(got, false)
	if len(defs) != 0 {
		t.Fatalf("expected no tools without catalog, got %#v", defs)
	}
}

func TestResolveInitialEnabledToolsWithRuntime_FailsClosedWithoutCatalog(t *testing.T) {
	r := registryWith(tools.LoadSkillName, "read_file")
	got := NewToolSelectionPolicy(r).InitialEnabledToolNames(ProfileToolConfig{EnabledTools: nil, DisableTools: false, RuntimeTools: []string{tools.LoadSkillName}})
	if got == nil || len(got) != 0 {
		t.Fatalf("runtime tools não devem abrir perfil implícito sem catálogo, got %#v", got)
	}
}

func TestBuildLLMToolDefsByNames(t *testing.T) {
	r := registryWith("alpha", "beta", "gamma")
	got := NewToolSelectionPolicy(r).buildLLMToolDefs([]string{"gamma", "missing", "alpha"}, false)
	if len(got) != 2 {
		t.Fatalf("expected 2 defs, got %#v", got)
	}
	if got[0].Function.Name != "alpha" || got[1].Function.Name != "gamma" {
		t.Fatalf("expected registry defs in deterministic order, got %#v", got)
	}
}

func TestBuildLLMToolDefsByNames_DisabledOrEmpty(t *testing.T) {
	r := registryWith("alpha")
	if got := NewToolSelectionPolicy(r).buildLLMToolDefs([]string{"alpha"}, true); got != nil {
		t.Fatalf("expected nil when disabled, got %#v", got)
	}
	if got := NewToolSelectionPolicy(r).buildLLMToolDefs(nil, false); got != nil {
		t.Fatalf("expected nil for nil names, got %#v", got)
	}
	if got := NewToolSelectionPolicy(nil).buildLLMToolDefs([]string{"alpha"}, false); got != nil {
		t.Fatalf("expected nil registry to return nil, got %#v", got)
	}
}

// ---------------------------------------------------------------------------
// ApplyNativeMCP
// ---------------------------------------------------------------------------

func TestApplyNativeMCP_DisableTools(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{{Name: "s", URL: "https://x.io"}}}
	defs := makeToolDefs("mcp_s1__tool1")
	outP, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, nil, true, boolPtr(true))
	if outP != p {
		t.Error("deveria retornar o mesmo provider quando disableTools=true")
	}
	if len(outDefs) != 1 {
		t.Errorf("deveria preservar toolDefs, obteve %v", outDefs)
	}
}

func TestApplyNativeMCP_NilMgr(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	defs := makeToolDefs("toolA")
	outP, outDefs := applyNativeMCP(context.Background(), p, defs, nil, nil, false, boolPtr(true))
	if outP != p {
		t.Error("deveria retornar o mesmo provider quando mcpMgr=nil")
	}
	if len(outDefs) != len(defs) {
		t.Error("deveria preservar toolDefs quando mcpMgr=nil")
	}
}

func TestApplyNativeMCP_TypedNilManagerDoesNotPanic(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	defs := makeToolDefs("toolA")
	var mgr *mockNativeMCPMgr
	var nativeMgr NativeMCPManager = mgr

	outP, outDefs := applyNativeMCP(context.Background(), p, defs, nativeMgr, nil, false, boolPtr(true))
	if outP != p {
		t.Error("deveria retornar o mesmo provider quando mcpMgr é typed-nil")
	}
	if len(outDefs) != len(defs) {
		t.Error("deveria preservar toolDefs quando mcpMgr é typed-nil")
	}
}

// Regression: typed-nil ChatProvider must not reach NativeMCPCapable() (method on nil receiver panics).
func TestApplyNativeMCP_TypedNilProviderDoesNotPanic(t *testing.T) {
	var p *mockChatProvider
	var streamer llm.ChatProvider = p
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{{Name: "s", URL: "https://x.io"}}}
	defs := makeToolDefs("toolA")
	outP, outDefs := applyNativeMCP(context.Background(), streamer, defs, mgr, nil, false, boolPtr(true))
	if outP != streamer {
		t.Error("deveria retornar o mesmo typed-nil streamer sem chamar métodos")
	}
	if len(outDefs) != 1 {
		t.Errorf("deveria preservar toolDefs, obteve %d", len(outDefs))
	}
}

func TestApplyNativeMCP_ProviderNotNative(t *testing.T) {
	p := &mockChatProvider{nativeCapable: false}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{{Name: "s", URL: "https://x.io"}}}
	defs := makeToolDefs("toolA")
	outP, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, nil, false, boolPtr(true))
	if outP != p {
		t.Error("deveria retornar o mesmo provider quando NativeMCPCapable=false")
	}
	if len(outDefs) != 1 {
		t.Error("deveria preservar toolDefs quando provider nao e capaz de native MCP")
	}
}

func TestApplyNativeMCP_NoServers(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: nil}
	defs := makeToolDefs("toolA")
	outP, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, nil, false, boolPtr(true))
	if outP != p {
		t.Error("deveria retornar o mesmo provider quando nao ha servidores")
	}
	if len(outDefs) != 1 {
		t.Error("deveria preservar toolDefs sem servidores")
	}
}

func TestApplyNativeMCP_RemovesBridgeTools(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Name: "MyServer", URL: "https://mcp.example.io", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_myserver__list", Name: "list"}, {RegisteredName: "mcp_myserver__create", Name: "create"}}},
	}}
	defs := makeToolDefs("mcp_myserver__list", "mcp_myserver__create", "local_tool")
	_, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, nil, false, boolPtr(true))
	if len(outDefs) != 1 {
		t.Fatalf("esperava 1 def restante (local_tool), obteve %d: %v", len(outDefs), outDefs)
	}
	if outDefs[0].Function.Name != "local_tool" {
		t.Errorf("def remanescente inesperada: %q", outDefs[0].Function.Name)
	}
}

func TestApplyNativeMCP_CallsWithMCPServers(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "srv", Name: "Srv", URL: "https://srv.io", AuthToken: "tok", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_srv__do", Name: "do"}}},
	}}
	defs := makeToolDefs("mcp_srv__do")
	outP, _ := applyNativeMCP(context.Background(), p, defs, mgr, nil, false, boolPtr(true))
	result, ok := outP.(*mockChatProvider)
	if !ok {
		t.Fatal("esperava *mockChatProvider de retorno")
	}
	if len(result.calledWith) != 1 {
		t.Fatalf("esperava 1 MCPServerConfig, obteve %d", len(result.calledWith))
	}
	cfg := result.calledWith[0]
	if cfg.Slug != "srv" || cfg.Name != "Srv" || cfg.URL != "https://srv.io" || cfg.AuthToken != "tok" {
		t.Errorf("MCPServerConfig incorreto: %+v", cfg)
	}

}

func TestApplyNativeMCP_SortsServersAndToolNames(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "zeta", Name: "Zeta", URL: "https://zeta.io", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_zeta__z", Name: "z"}, {RegisteredName: "mcp_zeta__a", Name: "a"}}},
		{Slug: "alpha", Name: "Alpha", URL: "https://alpha.io", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_alpha__z", Name: "z"}, {RegisteredName: "mcp_alpha__a", Name: "a"}}},
	}}
	defs := makeToolDefs("mcp_zeta__z", "mcp_zeta__a", "mcp_alpha__z", "mcp_alpha__a")

	outP, _ := applyNativeMCP(context.Background(), p, defs, mgr, nil, false, boolPtr(true))
	result, ok := outP.(*mockChatProvider)
	if !ok {
		t.Fatal("provider retornado deveria ser mockChatProvider")
	}
	if len(result.calledWith) != 2 {
		t.Fatalf("esperava 2 servidores MCP, got %#v", result.calledWith)
	}
	if result.calledWith[0].Slug != "alpha" || result.calledWith[1].Slug != "zeta" {
		t.Fatalf("servidores MCP fora de ordem: %#v", result.calledWith)
	}
	if got := result.calledWith[0].ToolNames; len(got) != 2 || got[0] != "mcp_alpha__a" || got[1] != "mcp_alpha__z" {
		t.Fatalf("tools do servidor alpha fora de ordem: %#v", got)
	}
	if got := result.calledWith[1].ToolNames; len(got) != 2 || got[0] != "mcp_zeta__a" || got[1] != "mcp_zeta__z" {
		t.Fatalf("tools do servidor zeta fora de ordem: %#v", got)
	}
}

func TestApplyNativeMCPDoesNotMutateNativeServers(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "zeta", Name: "Zeta", URL: "https://zeta.io", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_zeta__z", Name: "z"}, {RegisteredName: "mcp_zeta__a", Name: "a"}}},
		{Slug: "alpha", Name: "Alpha", URL: "https://alpha.io", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_alpha__z", Name: "z"}, {RegisteredName: "mcp_alpha__a", Name: "a"}}},
	}}
	defs := makeToolDefs("mcp_zeta__z", "mcp_zeta__a", "mcp_alpha__z", "mcp_alpha__a")

	_, _ = applyNativeMCP(context.Background(), p, defs, mgr, nil, false, boolPtr(true))

	if mgr.servers[0].Slug != "zeta" || mgr.servers[1].Slug != "alpha" {
		t.Fatalf("native servers were reordered: %#v", mgr.servers)
	}
	if got := mgr.servers[0].Tools; got[0].RegisteredName != "mcp_zeta__z" || got[1].RegisteredName != "mcp_zeta__a" {
		t.Fatalf("native server tool names were reordered: %#v", got)
	}
}

func TestApplyNativeMCP_NilEnabledToolsKeepsDynamicCatalogSeparateFromWhitelist(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "srv", Name: "Srv", URL: "https://srv.io", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_srv__do", Name: "do"}}},
	}}
	defs := makeToolDefs(tools.ToolCatalogName)

	outP, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, nil, false, boolPtr(true))
	result, ok := outP.(*mockChatProvider)
	if !ok {
		t.Fatal("esperava *mockChatProvider de retorno")
	}
	if len(result.calledWith) != 1 || len(result.calledWith[0].AllowedTools) != 0 {
		t.Fatalf("MCP nativo deveria ser configurado sem whitelist explícita, got %+v", result.calledWith)
	}
	if len(outDefs) != 1 || outDefs[0].Function.Name != tools.ToolCatalogName {
		t.Fatalf("tool_catalog deveria permanecer como tool inicial, got %#v", outDefs)
	}
}

func TestApplyNativeMCP_EnabledSetFiltersServer(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Name: "S", URL: "https://s.io", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_s__alpha", Name: "alpha"}, {RegisteredName: "mcp_s__beta", Name: "beta"}}},
	}}
	defs := makeToolDefs("mcp_s__alpha", "mcp_s__beta", "local")
	_, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, []string{"mcp_s__alpha"}, false, boolPtr(true))
	names := map[string]bool{}
	for _, d := range outDefs {
		names[d.Function.Name] = true
	}
	if names["mcp_s__alpha"] {
		t.Error("mcp_s__alpha deveria ser removida (foi para native)")
	}
	if !names["mcp_s__beta"] {
		t.Error("mcp_s__beta deveria permanecer (nao foi configurada como native)")
	}
	if !names["local"] {
		t.Error("local deveria permanecer")
	}
}

// ---------------------------------------------------------------------------
// ResolveNativeMCPEnabled — tri-state; auto = nativo otimista (se capaz)
// ---------------------------------------------------------------------------

func TestResolveNativeMCPEnabled_AutoIsOptimisticNativeWhenCapable(t *testing.T) {
	// nil override (auto) → nativo SE o provider for fisicamente capaz (otimista),
	// adapter caso contrário. Não há heurística por URL/endpoint.
	capable := &mockChatProvider{nativeCapable: true}
	incapable := &mockChatProvider{nativeCapable: false}

	if !resolveNativeMCPEnabled(capable, nil) {
		t.Error("auto: provider capaz deveria tentar nativo por default (otimista)")
	}
	if resolveNativeMCPEnabled(incapable, nil) {
		t.Error("auto: provider incapaz deveria cair em adapter")
	}
}

func TestResolveNativeMCPEnabled_ForceTrueOnCapableProvider(t *testing.T) {
	// Perfil força true num provider fisicamente capaz (Responses/Anthropic).
	capable := &mockChatProvider{nativeCapable: true}
	if !resolveNativeMCPEnabled(capable, boolPtr(true)) {
		t.Error("override true deveria habilitar nativo em provider fisicamente capaz")
	}
}

func TestResolveNativeMCPEnabled_ForceTrueOnIncapableProviderStaysAdapter(t *testing.T) {
	// Perfil força true, mas o provider não é capaz (ex.: Chat Completions, Google).
	// Não deve habilitar nativo (evita remover bridge tools sem enviar type:mcp).
	incapable := &mockChatProvider{nativeCapable: false}
	if resolveNativeMCPEnabled(incapable, boolPtr(true)) {
		t.Error("override true NÃO deveria habilitar nativo em provider incapaz")
	}
}

func TestResolveNativeMCPEnabled_ForceFalseStaysAdapter(t *testing.T) {
	// Perfil força false: sempre adapter, mesmo num provider capaz.
	capable := &mockChatProvider{nativeCapable: true}
	if resolveNativeMCPEnabled(capable, boolPtr(false)) {
		t.Error("override false deveria forçar adapter mesmo em provider capaz")
	}
}

// TestApplyNativeMCP_ProfileOverride cobre os caminhos end-to-end na montagem de
// tools: força true num provider capaz → configura servers nativos e remove
// bridges; força false (ou auto) → mantém bridges (adapter).
func TestApplyNativeMCP_ProfileOverrideForceTrueCapableConfiguresNative(t *testing.T) {
	// Provider fisicamente capaz e perfil força true.
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "github", Name: "GitHub", URL: "https://api.githubcopilot.com/mcp/", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_github__list", Name: "list"}}},
	}}
	defs := makeToolDefs("mcp_github__list", "local")
	outP, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, nil, false, boolPtr(true))
	result, ok := outP.(*mockChatProvider)
	if !ok || len(result.calledWith) != 1 {
		t.Fatalf("override true deveria configurar 1 server nativo, got %#v", outP)
	}
	if len(outDefs) != 1 || outDefs[0].Function.Name != "local" {
		t.Fatalf("bridge nativa deveria ter sido removida, got %#v", outDefs)
	}
}

func TestApplyNativeMCP_AutoConfiguresNativeWhenCapable(t *testing.T) {
	// Provider capaz + auto (nil override) → nativo otimista: configura servers
	// nativos e remove as bridges (só voltam se o perfil for ajustado p/ adapter).
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "github", Name: "GitHub", URL: "https://api.githubcopilot.com/mcp/", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_github__list", Name: "list"}}},
	}}
	defs := makeToolDefs("mcp_github__list", "local")
	outP, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, nil, false, nil)
	result, ok := outP.(*mockChatProvider)
	if !ok || len(result.calledWith) != 1 {
		t.Fatalf("auto+capaz deveria configurar 1 server nativo, got %#v", outP)
	}
	if len(outDefs) != 1 || outDefs[0].Function.Name != "local" {
		t.Fatalf("auto+capaz deveria remover a bridge nativa, got %#v", outDefs)
	}
}

func TestApplyNativeMCP_AutoIncapableKeepsBridges(t *testing.T) {
	// Provider incapaz + auto (nil) → adapter: bridges permanecem.
	p := &mockChatProvider{nativeCapable: false}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "github", Name: "GitHub", URL: "https://api.githubcopilot.com/mcp/", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_github__list", Name: "list"}}},
	}}
	defs := makeToolDefs("mcp_github__list", "local")
	outP, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, nil, false, nil)
	if outP != p {
		t.Error("auto+incapaz: WithMCPServers não deveria ser chamado (adapter)")
	}
	if len(outDefs) != 2 {
		t.Errorf("auto+incapaz: bridge tools deveriam permanecer, got %#v", outDefs)
	}
}

func TestApplyNativeMCP_ProfileOverrideForceFalseCapableKeepsBridges(t *testing.T) {
	// Provider capaz, mas perfil força adapter (false).
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Slug: "github", Name: "GitHub", URL: "https://api.githubcopilot.com/mcp/", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_github__list", Name: "list"}}},
	}}
	defs := makeToolDefs("mcp_github__list", "local")
	outP, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, nil, false, boolPtr(false))
	if outP != p {
		t.Error("override false: WithMCPServers não deveria ser chamado (adapter)")
	}
	if len(outDefs) != 2 {
		t.Errorf("override false: bridge tools deveriam permanecer, got %#v", outDefs)
	}
}

func TestApplyNativeMCP_ServerExcludedWhenNoEnabledTools(t *testing.T) {
	p := &mockChatProvider{nativeCapable: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Name: "S", URL: "https://s.io", Tools: []mcplib.NativeMCPTool{{RegisteredName: "mcp_s__tool1", Name: "tool1"}}},
	}}
	defs := makeToolDefs("mcp_s__tool1", "local")
	outP, outDefs := applyNativeMCP(context.Background(), p, defs, mgr, []string{"other_tool"}, false, boolPtr(true))
	if outP != p {
		t.Error("esperava provider original (WithMCPServers nao deveria ter sido chamado)")
	}
	if len(outDefs) != 2 {
		t.Errorf("esperava 2 defs intactas, obteve %d", len(outDefs))
	}
}
