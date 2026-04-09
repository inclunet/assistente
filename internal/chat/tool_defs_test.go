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
	name   string
	descr  string
	params json.RawMessage
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
	supportsNative bool
	calledWith     []llm.MCPServerConfig
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
func (m *mockChatProvider) SupportsNativeMCP() bool { return m.supportsNative }
func (m *mockChatProvider) WithMCPServers(servers []llm.MCPServerConfig) llm.ChatProvider {
	clone := &mockChatProvider{supportsNative: m.supportsNative, calledWith: servers}
	return clone
}

// mockNativeMCPMgr implements NativeMCPManager.
type mockNativeMCPMgr struct {
	servers []mcplib.NativeMCPServer
}

func (m *mockNativeMCPMgr) GetEligibleNativeMCPServers() []mcplib.NativeMCPServer {
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
	got := BuildLLMToolDefs(r, nil, true)
	if got != nil {
		t.Fatalf("esperava nil, obteve %v", got)
	}
}

func TestBuildLLMToolDefs_NilRegistry(t *testing.T) {
	got := BuildLLMToolDefs(nil, nil, false)
	if got != nil {
		t.Fatalf("esperava nil para registry nil, obteve %v", got)
	}
}

func TestBuildLLMToolDefs_EmptyRegistry(t *testing.T) {
	got := BuildLLMToolDefs(tools.NewRegistry(), nil, false)
	if got != nil {
		t.Fatalf("esperava nil para registry vazio, obteve %v", got)
	}
}

func TestBuildLLMToolDefs_AllTools(t *testing.T) {
	r := registryWith("alpha", "beta", "gamma")
	got := BuildLLMToolDefs(r, nil, false)
	if len(got) != 3 {
		t.Fatalf("esperava 3 defs, obteve %d", len(got))
	}
	names := map[string]bool{}
	for _, d := range got {
		if d.Type != "function" {
			t.Errorf("Type incorreto: %q", d.Type)
		}
		if d.Function.Description == "" {
			t.Errorf("Description vazia para %q", d.Function.Name)
		}
		names[d.Function.Name] = true
	}
	for _, want := range []string{"alpha", "beta", "gamma"} {
		if !names[want] {
			t.Errorf("tool %q ausente no resultado", want)
		}
	}
}

func TestBuildLLMToolDefs_FilteredTools(t *testing.T) {
	r := registryWith("alpha", "beta", "gamma")
	got := BuildLLMToolDefs(r, []string{"beta"}, false)
	if len(got) != 1 {
		t.Fatalf("esperava 1 def filtrada, obteve %d", len(got))
	}
	if got[0].Function.Name != "beta" {
		t.Errorf("esperava beta, obteve %q", got[0].Function.Name)
	}
}

func TestBuildLLMToolDefs_EnabledToolsEmptySlice(t *testing.T) {
	r := registryWith("alpha")
	got := BuildLLMToolDefs(r, []string{}, false)
	if len(got) != 0 {
		t.Fatalf("esperava slice vazio, obteve %v", got)
	}
}

func TestBuildLLMToolDefs_ParametersPreserved(t *testing.T) {
	r := tools.NewRegistry()
	params := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	r.MustRegister(&mockToolDef{name: "read_file", descr: "reads", params: params})
	got := BuildLLMToolDefs(r, nil, false)
	if len(got) != 1 {
		t.Fatalf("esperava 1 def, obteve %d", len(got))
	}
	if string(got[0].Function.Parameters) != string(params) {
		t.Errorf("Parameters nao preservado: %s", got[0].Function.Parameters)
	}
}

// ---------------------------------------------------------------------------
// ApplyNativeMCP
// ---------------------------------------------------------------------------

func TestApplyNativeMCP_DisableTools(t *testing.T) {
	p := &mockChatProvider{supportsNative: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{{Name: "s", URL: "https://x.io"}}}
	defs := makeToolDefs("mcp_s1__tool1")
	outP, outDefs := ApplyNativeMCP(p, defs, mgr, nil, true)
	if outP != p {
		t.Error("deveria retornar o mesmo provider quando disableTools=true")
	}
	if len(outDefs) != 1 {
		t.Errorf("deveria preservar toolDefs, obteve %v", outDefs)
	}
}

func TestApplyNativeMCP_NilMgr(t *testing.T) {
	p := &mockChatProvider{supportsNative: true}
	defs := makeToolDefs("toolA")
	outP, outDefs := ApplyNativeMCP(p, defs, nil, nil, false)
	if outP != p {
		t.Error("deveria retornar o mesmo provider quando mcpMgr=nil")
	}
	if len(outDefs) != len(defs) {
		t.Error("deveria preservar toolDefs quando mcpMgr=nil")
	}
}

// Regression: typed-nil ChatProvider must not reach SupportsNativeMCP() (method on nil receiver panics).
func TestApplyNativeMCP_TypedNilProviderDoesNotPanic(t *testing.T) {
	var p *mockChatProvider
	var streamer llm.ChatProvider = p
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{{Name: "s", URL: "https://x.io"}}}
	defs := makeToolDefs("toolA")
	outP, outDefs := ApplyNativeMCP(streamer, defs, mgr, nil, false)
	if outP != streamer {
		t.Error("deveria retornar o mesmo typed-nil streamer sem chamar métodos")
	}
	if len(outDefs) != 1 {
		t.Errorf("deveria preservar toolDefs, obteve %d", len(outDefs))
	}
}

func TestApplyNativeMCP_ProviderNotNative(t *testing.T) {
	p := &mockChatProvider{supportsNative: false}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{{Name: "s", URL: "https://x.io"}}}
	defs := makeToolDefs("toolA")
	outP, outDefs := ApplyNativeMCP(p, defs, mgr, nil, false)
	if outP != p {
		t.Error("deveria retornar o mesmo provider quando SupportsNativeMCP=false")
	}
	if len(outDefs) != 1 {
		t.Error("deveria preservar toolDefs quando provider nao suporta native MCP")
	}
}

func TestApplyNativeMCP_NoServers(t *testing.T) {
	p := &mockChatProvider{supportsNative: true}
	mgr := &mockNativeMCPMgr{servers: nil}
	defs := makeToolDefs("toolA")
	outP, outDefs := ApplyNativeMCP(p, defs, mgr, nil, false)
	if outP != p {
		t.Error("deveria retornar o mesmo provider quando nao ha servidores")
	}
	if len(outDefs) != 1 {
		t.Error("deveria preservar toolDefs sem servidores")
	}
}

func TestApplyNativeMCP_RemovesBridgeTools(t *testing.T) {
	p := &mockChatProvider{supportsNative: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Name: "MyServer", URL: "https://mcp.example.io", ToolNames: []string{"mcp_myserver__list", "mcp_myserver__create"}},
	}}
	defs := makeToolDefs("mcp_myserver__list", "mcp_myserver__create", "local_tool")
	_, outDefs := ApplyNativeMCP(p, defs, mgr, nil, false)
	if len(outDefs) != 1 {
		t.Fatalf("esperava 1 def restante (local_tool), obteve %d: %v", len(outDefs), outDefs)
	}
	if outDefs[0].Function.Name != "local_tool" {
		t.Errorf("def remanescente inesperada: %q", outDefs[0].Function.Name)
	}
}

func TestApplyNativeMCP_CallsWithMCPServers(t *testing.T) {
	p := &mockChatProvider{supportsNative: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Name: "Srv", URL: "https://srv.io", AuthToken: "tok", ToolNames: []string{"mcp_srv__do"}},
	}}
	defs := makeToolDefs("mcp_srv__do")
	outP, _ := ApplyNativeMCP(p, defs, mgr, nil, false)
	result, ok := outP.(*mockChatProvider)
	if !ok {
		t.Fatal("esperava *mockChatProvider de retorno")
	}
	if len(result.calledWith) != 1 {
		t.Fatalf("esperava 1 MCPServerConfig, obteve %d", len(result.calledWith))
	}
	cfg := result.calledWith[0]
	if cfg.Name != "Srv" || cfg.URL != "https://srv.io" || cfg.AuthToken != "tok" {
		t.Errorf("MCPServerConfig incorreto: %+v", cfg)
	}
}

func TestApplyNativeMCP_EnabledSetFiltersServer(t *testing.T) {
	p := &mockChatProvider{supportsNative: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Name: "S", URL: "https://s.io", ToolNames: []string{"mcp_s__alpha", "mcp_s__beta"}},
	}}
	defs := makeToolDefs("mcp_s__alpha", "mcp_s__beta", "local")
	_, outDefs := ApplyNativeMCP(p, defs, mgr, []string{"mcp_s__alpha"}, false)
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

func TestApplyNativeMCP_ServerExcludedWhenNoEnabledTools(t *testing.T) {
	p := &mockChatProvider{supportsNative: true}
	mgr := &mockNativeMCPMgr{servers: []mcplib.NativeMCPServer{
		{Name: "S", URL: "https://s.io", ToolNames: []string{"mcp_s__tool1"}},
	}}
	defs := makeToolDefs("mcp_s__tool1", "local")
	outP, outDefs := ApplyNativeMCP(p, defs, mgr, []string{"other_tool"}, false)
	if outP != p {
		t.Error("esperava provider original (WithMCPServers nao deveria ter sido chamado)")
	}
	if len(outDefs) != 2 {
		t.Errorf("esperava 2 defs intactas, obteve %d", len(outDefs))
	}
}
