package llm

import (
	"context"
	"testing"

	"assistente/internal/credentials"
	mcplib "assistente/internal/mcp"
	"github.com/openai/openai-go/responses"
)

func TestOpenAIResponsesProvider_WithMCPServers_StoresServers(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewOpenAIResponsesProvider(p, credMgr)
	servers := []MCPServerConfig{
		{Name: "weather-mcp", URL: "https://mcp.weather.com/sse", AuthToken: "tok123"},
	}
	result := provider.WithMCPServers(servers)
	if result == nil {
		t.Fatal("WithMCPServers returned nil")
	}

	openaiP, ok := result.(*OpenAIProvider)
	if !ok {
		t.Fatal("WithMCPServers should return *OpenAIProvider")
	}
	if len(openaiP.mcpServers) != 1 {
		t.Fatalf("Expected 1 MCP server, got %d", len(openaiP.mcpServers))
	}
	if openaiP.mcpServers[0].Name != "weather-mcp" {
		t.Errorf("MCP server name = %q, want %q", openaiP.mcpServers[0].Name, "weather-mcp")
	}
	if !openaiP.useResponses {
		t.Error("WithMCPServers should preserve useResponses=true")
	}
}

func hasMCPTool(tools []responses.ToolUnionParam) bool {
	for i := range tools {
		if tools[i].OfMcp != nil {
			return true
		}
	}
	return false
}

// TestBuildResponsesParams_EmitsMCPToolWhenServersPresent garante, no nível da
// request/wire, que type:"mcp" é emitido sempre que há MCP servers anexados ao
// provider e ausente quando não há. A POLÍTICA de anexar (default por URL +
// override de perfil) é decidida em internal/chat antes de WithMCPServers.
func TestBuildResponsesParams_EmitsMCPToolWhenServersPresent(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	ctx := context.Background()
	msgs := []Message{{Role: "user", Content: "oi"}}
	servers := []MCPServerConfig{
		{Name: "GitHub", Slug: "github", URL: "https://api.githubcopilot.com/mcp/", AuthToken: "tok"},
	}

	// Proxy via Responses (capaz): quando o perfil força nativo, o chat layer
	// chama WithMCPServers e a request passa a conter type:"mcp".
	proxy := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "l", Name: "LiteLLM", Type: ProviderCustom,
		APIFormat: APIFormatOpenAIResponses, BaseURL: "http://llm.inclunet.com.br/v1",
	}, credMgr)
	withServers, ok := proxy.WithMCPServers(servers).(*OpenAIProvider)
	if !ok {
		t.Fatal("WithMCPServers deveria retornar *OpenAIProvider (proxy é capaz)")
	}
	withParams := withServers.buildResponsesParams(ctx, "deepseek-v4-flash", msgs, ChatParams{}, withServers.mcpServers)
	if !hasMCPTool(withParams.Tools) {
		t.Error("com servers anexados, a request deveria conter tool type:mcp")
	}

	// Sem servers anexados (caso adapter): nenhuma tool type:"mcp".
	noParams := proxy.buildResponsesParams(ctx, "deepseek-v4-flash", msgs, ChatParams{}, proxy.mcpServers)
	if hasMCPTool(noParams.Tools) {
		t.Error("sem servers anexados, a request NÃO deve conter tool type:mcp")
	}
}

func TestWithMCPServers_ImmutableOriginal(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	provider := NewOpenAIResponsesProvider(&ProviderConfig{ID: "o", Name: "O", BaseURL: "https://api.openai.com/v1"}, credMgr)

	servers := []MCPServerConfig{{Name: "test", URL: "http://test"}}
	modified := provider.WithMCPServers(servers)

	if len(provider.mcpServers) != 0 {
		t.Error("Original provider should not be modified")
	}
	modP := modified.(*OpenAIProvider)
	if len(modP.mcpServers) != 1 {
		t.Error("Modified provider should have 1 MCP server")
	}
	if !modP.useResponses {
		t.Error("Modified provider should preserve useResponses=true")
	}
}

func TestNativeMCP_ToolDeduplication(t *testing.T) {
	// Simula cenário: tools do bridge incluem MCP tools + tools internas.
	// Quando MCP nativo está ativo, bridge tools dos servidores nativos devem ser removidas.
	allTools := []ToolDefinition{
		{Function: FunctionDefinition{Name: "internal_search"}},
		{Function: FunctionDefinition{Name: "mcp__github__list_repos"}},
		{Function: FunctionDefinition{Name: "mcp__github__create_issue"}},
		{Function: FunctionDefinition{Name: "mcp__slack__send_message"}},
		{Function: FunctionDefinition{Name: "internal_calc"}},
	}

	nativeToolNames := map[string]bool{
		"mcp__github__list_repos":   true,
		"mcp__github__create_issue": true,
	}

	var filtered []ToolDefinition
	for _, td := range allTools {
		if !nativeToolNames[td.Function.Name] {
			filtered = append(filtered, td)
		}
	}

	if len(filtered) != 3 {
		t.Fatalf("Expected 3 tools after dedup, got %d", len(filtered))
	}
	expectedNames := []string{"internal_search", "mcp__slack__send_message", "internal_calc"}
	for i, name := range expectedNames {
		if filtered[i].Function.Name != name {
			t.Errorf("filtered[%d] = %q, want %q", i, filtered[i].Function.Name, name)
		}
	}
}

func TestMCPServerConfig_AllowedToolsPreserved(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{ID: "o", Name: "O", BaseURL: "https://api.openai.com/v1"}
	provider := NewOpenAIResponsesProvider(p, credMgr)

	servers := []MCPServerConfig{
		{
			Name:         "github-mcp",
			URL:          "https://mcp.github.com/sse",
			ToolNames:    []string{"mcp_github__create_issue"},
			AllowedTools: []string{"create_issue"},
		},
	}
	result := provider.WithMCPServers(servers)
	openaiP := result.(*OpenAIProvider)
	if len(openaiP.mcpServers[0].AllowedTools) != 1 {
		t.Fatalf("Expected 1 AllowedTools, got %d", len(openaiP.mcpServers[0].AllowedTools))
	}
	if openaiP.mcpServers[0].AllowedTools[0] != "create_issue" {
		t.Errorf("AllowedTools[0] = %q, want %q", openaiP.mcpServers[0].AllowedTools[0], "create_issue")
	}
}

func TestMCPServerConfig_EmptyAllowedToolsMeansAll(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "test-mcp",
		URL:       "https://test.com/mcp",
		ToolNames: []string{"mcp_test__tool_a", "mcp_test__tool_b"},
	}
	if len(cfg.AllowedTools) != 0 {
		t.Error("Empty AllowedTools should mean all tools are allowed")
	}
}

// filterNativeMCPByProfile replica a lógica de filtragem do chat loop (llm.go)
// usando mcplib.ParseToolName como fonte canônica de naming.
func filterNativeMCPByProfile(enabledTools []string, servers []struct {
	slug      string
	toolNames []string
}) (configs []MCPServerConfig, nativeToolNames map[string]bool) {
	var enabledSet map[string]bool
	if enabledTools != nil {
		enabledSet = make(map[string]bool, len(enabledTools))
		for _, n := range enabledTools {
			enabledSet[n] = true
		}
	}

	nativeToolNames = make(map[string]bool)
	for _, srv := range servers {
		cfg := MCPServerConfig{
			Name:      srv.slug + "-mcp",
			URL:       "https://" + srv.slug + ".com/mcp",
			ToolNames: srv.toolNames,
		}
		if enabledSet != nil {
			var allowed []string
			var allowedFull []string
			for _, fullName := range srv.toolNames {
				if enabledSet[fullName] {
					if _, originalName, ok := mcplib.ParseToolName(fullName); ok {
						allowed = append(allowed, originalName)
					}
					allowedFull = append(allowedFull, fullName)
				}
			}
			if len(allowed) == 0 {
				continue
			}
			cfg.AllowedTools = allowed
			cfg.ToolNames = allowedFull
		}
		configs = append(configs, cfg)
		for _, tn := range cfg.ToolNames {
			nativeToolNames[tn] = true
		}
	}
	return
}

func TestProfileEnabledTools_FilterNativeMCPServers(t *testing.T) {
	type serverInput struct {
		slug      string
		toolNames []string
	}
	type testCase struct {
		name         string
		enabledTools []string
		servers      []serverInput
		wantConfigs  int
		wantAllowed  map[string][]string
	}

	tests := []testCase{
		{
			name:         "nil EnabledTools passes all servers",
			enabledTools: nil,
			servers: []serverInput{
				{slug: "github", toolNames: []string{"mcp_github__create_issue", "mcp_github__list_repos"}},
			},
			wantConfigs: 1,
			wantAllowed: map[string][]string{"github": nil},
		},
		{
			name:         "filter keeps only enabled tools",
			enabledTools: []string{"mcp_github__create_issue", "internal_search"},
			servers: []serverInput{
				{slug: "github", toolNames: []string{"mcp_github__create_issue", "mcp_github__list_repos"}},
			},
			wantConfigs: 1,
			wantAllowed: map[string][]string{"github": {"create_issue"}},
		},
		{
			name:         "server excluded when no tools match",
			enabledTools: []string{"internal_search"},
			servers: []serverInput{
				{slug: "github", toolNames: []string{"mcp_github__create_issue"}},
			},
			wantConfigs: 0,
		},
		{
			name:         "multiple servers with partial match",
			enabledTools: []string{"mcp_github__create_issue", "mcp_slack__send_message"},
			servers: []serverInput{
				{slug: "github", toolNames: []string{"mcp_github__create_issue", "mcp_github__list_repos"}},
				{slug: "slack", toolNames: []string{"mcp_slack__send_message"}},
				{slug: "jira", toolNames: []string{"mcp_jira__create_ticket"}},
			},
			wantConfigs: 2,
			wantAllowed: map[string][]string{
				"github": {"create_issue"},
				"slack":  {"send_message"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srvInputs := make([]struct {
				slug      string
				toolNames []string
			}, len(tc.servers))
			for i, s := range tc.servers {
				srvInputs[i].slug = s.slug
				srvInputs[i].toolNames = s.toolNames
			}
			configs, _ := filterNativeMCPByProfile(tc.enabledTools, srvInputs)

			if len(configs) != tc.wantConfigs {
				t.Fatalf("got %d configs, want %d", len(configs), tc.wantConfigs)
			}
			for slug, wantAllowed := range tc.wantAllowed {
				found := false
				for _, cfg := range configs {
					if cfg.Name == slug+"-mcp" {
						found = true
						if wantAllowed == nil {
							if len(cfg.AllowedTools) != 0 {
								t.Errorf("server %q: want nil AllowedTools, got %v", slug, cfg.AllowedTools)
							}
						} else {
							if len(cfg.AllowedTools) != len(wantAllowed) {
								t.Errorf("server %q: got %d AllowedTools, want %d", slug, len(cfg.AllowedTools), len(wantAllowed))
							}
							for i, want := range wantAllowed {
								if i < len(cfg.AllowedTools) && cfg.AllowedTools[i] != want {
									t.Errorf("server %q AllowedTools[%d] = %q, want %q", slug, i, cfg.AllowedTools[i], want)
								}
							}
						}
					}
				}
				if !found {
					t.Errorf("server %q not found in configs", slug)
				}
			}
		})
	}
}

func TestNativeMCP_NoDuplicateWithBridge(t *testing.T) {
	bridgeToolDefs := []ToolDefinition{
		{Function: FunctionDefinition{Name: "internal_search"}},
		{Function: FunctionDefinition{Name: "mcp_github__create_issue"}},
		{Function: FunctionDefinition{Name: "mcp_github__list_repos"}},
		{Function: FunctionDefinition{Name: "mcp_slack__send_message"}},
	}

	servers := []struct {
		slug      string
		toolNames []string
	}{
		{slug: "github", toolNames: []string{"mcp_github__create_issue", "mcp_github__list_repos"}},
	}

	_, nativeToolNames := filterNativeMCPByProfile(nil, servers)

	var afterDedup []ToolDefinition
	for _, td := range bridgeToolDefs {
		if !nativeToolNames[td.Function.Name] {
			afterDedup = append(afterDedup, td)
		}
	}

	nativeNames := make(map[string]bool)
	for name := range nativeToolNames {
		nativeNames[name] = true
	}
	bridgeNames := make(map[string]bool)
	for _, td := range afterDedup {
		bridgeNames[td.Function.Name] = true
	}

	for name := range nativeNames {
		if bridgeNames[name] {
			t.Errorf("tool %q presente tanto em native quanto em bridge (duplicata!)", name)
		}
	}

	if len(afterDedup) != 2 {
		t.Fatalf("após dedup, esperado 2 tools restantes, obtido %d", len(afterDedup))
	}
	if afterDedup[0].Function.Name != "internal_search" {
		t.Errorf("afterDedup[0] = %q, esperado internal_search", afterDedup[0].Function.Name)
	}
	if afterDedup[1].Function.Name != "mcp_slack__send_message" {
		t.Errorf("afterDedup[1] = %q, esperado mcp_slack__send_message", afterDedup[1].Function.Name)
	}
}

func TestNativeMCP_NoDuplicateWithProfile(t *testing.T) {
	bridgeToolDefs := []ToolDefinition{
		{Function: FunctionDefinition{Name: "internal_search"}},
		{Function: FunctionDefinition{Name: "mcp_github__create_issue"}},
	}

	servers := []struct {
		slug      string
		toolNames []string
	}{
		{slug: "github", toolNames: []string{"mcp_github__create_issue", "mcp_github__list_repos"}},
	}

	enabledTools := []string{"mcp_github__create_issue", "internal_search"}
	configs, nativeToolNames := filterNativeMCPByProfile(enabledTools, servers)

	if len(configs) != 1 {
		t.Fatalf("esperado 1 config, obtido %d", len(configs))
	}
	if len(configs[0].AllowedTools) != 1 || configs[0].AllowedTools[0] != "create_issue" {
		t.Errorf("AllowedTools = %v, esperado [create_issue]", configs[0].AllowedTools)
	}

	var afterDedup []ToolDefinition
	for _, td := range bridgeToolDefs {
		if !nativeToolNames[td.Function.Name] {
			afterDedup = append(afterDedup, td)
		}
	}

	for _, td := range afterDedup {
		if nativeToolNames[td.Function.Name] {
			t.Errorf("tool %q duplicada: aparece em bridge E native", td.Function.Name)
		}
	}

	if len(afterDedup) != 1 || afterDedup[0].Function.Name != "internal_search" {
		t.Errorf("após dedup com perfil, esperado apenas [internal_search], obtido %v",
			func() []string {
				var names []string
				for _, td := range afterDedup {
					names = append(names, td.Function.Name)
				}
				return names
			}())
	}
}

// mcpTrackingHandler captura MCPToolEvents para validação em testes.
type mcpTrackingHandler struct {
	captureHandler
	events []MCPToolEvent
}

func (h *mcpTrackingHandler) OnMCPToolEvent(event MCPToolEvent) {
	h.events = append(h.events, event)
}

func TestMCPToolEvent_StreamHandlerInterface(t *testing.T) {
	var handler StreamHandler = &mcpTrackingHandler{}

	handler.OnMCPToolEvent(MCPToolEvent{
		ID:          "call_001",
		Name:        "jira_search",
		ServerLabel: "Atlassian",
		IsCompleted: false,
	})

	handler.OnMCPToolEvent(MCPToolEvent{
		ID:          "call_001",
		Name:        "jira_search",
		ServerLabel: "Atlassian",
		Arguments:   `{"query":"project=FSD"}`,
		Output:      `{"issues":[{"key":"FSD-123"}]}`,
		IsCompleted: true,
	})

	h := handler.(*mcpTrackingHandler)
	if len(h.events) != 2 {
		t.Fatalf("esperado 2 eventos, obtido %d", len(h.events))
	}

	start := h.events[0]
	if start.Name != "jira_search" || start.ServerLabel != "Atlassian" || start.IsCompleted {
		t.Errorf("evento start incorreto: %+v", start)
	}

	end := h.events[1]
	if end.Name != "jira_search" || !end.IsCompleted || end.Output == "" {
		t.Errorf("evento end incorreto: %+v", end)
	}
	if end.Arguments != `{"query":"project=FSD"}` {
		t.Errorf("Arguments = %q, esperado query JQL", end.Arguments)
	}
}

func TestMCPToolEvent_ErrorTracking(t *testing.T) {
	handler := &mcpTrackingHandler{}

	handler.OnMCPToolEvent(MCPToolEvent{
		ID:          "call_err",
		Name:        "search",
		ServerLabel: "Slack",
		Error:       "unauthorized",
		IsCompleted: true,
	})

	if len(handler.events) != 1 {
		t.Fatalf("esperado 1 evento, obtido %d", len(handler.events))
	}
	ev := handler.events[0]
	if ev.Error != "unauthorized" || !ev.IsCompleted {
		t.Errorf("evento de erro incorreto: %+v", ev)
	}
}

func TestMCPToolEvent_MultipleServers(t *testing.T) {
	handler := &mcpTrackingHandler{}

	handler.OnMCPToolEvent(MCPToolEvent{
		ID: "mc1", Name: "search", ServerLabel: "Atlassian", IsCompleted: false,
	})
	handler.OnMCPToolEvent(MCPToolEvent{
		ID: "mc2", Name: "read_channel", ServerLabel: "Slack", IsCompleted: false,
	})
	handler.OnMCPToolEvent(MCPToolEvent{
		ID: "mc1", Name: "search", ServerLabel: "Atlassian",
		Output: `[{"key":"FSD-1"}]`, IsCompleted: true,
	})
	handler.OnMCPToolEvent(MCPToolEvent{
		ID: "mc2", Name: "read_channel", ServerLabel: "Slack",
		Output: "messages...", IsCompleted: true,
	})

	if len(handler.events) != 4 {
		t.Fatalf("esperado 4 eventos, obtido %d", len(handler.events))
	}

	starts := 0
	ends := 0
	servers := map[string]bool{}
	for _, ev := range handler.events {
		if ev.IsCompleted {
			ends++
		} else {
			starts++
		}
		servers[ev.ServerLabel] = true
	}
	if starts != 2 || ends != 2 {
		t.Errorf("esperado 2 starts + 2 ends, obtido %d starts + %d ends", starts, ends)
	}
	if !servers["Atlassian"] || !servers["Slack"] {
		t.Errorf("servidores rastreados: %v", servers)
	}
}
