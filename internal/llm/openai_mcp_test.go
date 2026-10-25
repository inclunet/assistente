package llm

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"assistente/internal/credentials"
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

func TestTruncateMCPError(t *testing.T) {
	if got := truncateMCPError("  erro simples  "); got != "erro simples" {
		t.Fatalf("truncateMCPError não normalizou espaços: %q", got)
	}
	long := strings.Repeat("x", mcpErrorLogMaxLen+50)
	got := truncateMCPError(long)
	if !strings.HasSuffix(got, "… (truncado)") {
		t.Fatalf("erro longo deveria terminar com marcador de truncamento: %q", got)
	}
	// O prefixo preservado nunca deve exceder o limite configurado.
	prefix := strings.TrimSuffix(got, "… (truncado)")
	if len(prefix) > mcpErrorLogMaxLen {
		t.Fatalf("prefixo truncado excedeu o limite: len=%d", len(prefix))
	}
}

func TestMcpFailureLogFields(t *testing.T) {
	got := mcpFailureLogFields("Atlassian", "getJiraIssue", "cloudId ausente ou inválido")
	for _, want := range []string{`server="Atlassian"`, `tool="getJiraIssue"`, "cloudId ausente ou inválido"} {
		if !strings.Contains(got, want) {
			t.Fatalf("campo ausente %q em %q", want, got)
		}
	}
}

// logCaptureHandler captura registros slog emitidos durante o stream para
// asserções de nível e conteúdo.
type logCaptureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (*logCaptureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *logCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r.Clone())
	h.mu.Unlock()
	return nil
}
func (h *logCaptureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *logCaptureHandler) WithGroup(string) slog.Handler      { return h }
func (h *logCaptureHandler) find(level slog.Level, substr string) *slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.records {
		if h.records[i].Level == level && strings.Contains(h.records[i].Message, substr) {
			rec := h.records[i]
			return &rec
		}
	}
	return nil
}

func (h *logCaptureHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.records...)
}

// TestOpenAIResponsesLogsNativeMCPCallError garante que, ao processar um
// output item mcp_call com o campo error preenchido, o provider (1) preserva o
// erro no MCPToolEvent e (2) o loga em ERRO com server_label, nome da tool e o
// texto do erro — tornando a causa diagnosticável sem correlação manual.
func TestOpenAIResponsesLogsNativeMCPCallError(t *testing.T) {
	const stream = "event: response.output_item.added\n" +
		`data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"mcp_1","type":"mcp_call","name":"getJiraIssue","server_label":"Atlassian"}}` + "\n\n" +
		"event: response.output_item.done\n" +
		`data: {"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":{"id":"mcp_1","type":"mcp_call","name":"getJiraIssue","server_label":"Atlassian","error":"cloudId ausente ou inválido"}}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","sequence_number":3,"response":{"id":"resp_1","object":"response","created_at":1,"status":"completed","model":"gpt-test","output":[]}}` + "\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	logHandler := &logCaptureHandler{}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(logHandler))
	defer slog.SetDefault(oldLogger)

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID:           "responses-mcp-error",
		Name:         "Responses MCP Error",
		BaseURL:      server.URL + "/v1",
		APIFormat:    APIFormatOpenAIResponses,
		AuthMode:     AuthModeNone,
		DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))

	handler := &mcpTrackingHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}},
		ChatParams{Model: "gpt-test"}, handler)

	var completed *MCPToolEvent
	for i := range handler.events {
		if handler.events[i].IsCompleted {
			completed = &handler.events[i]
		}
	}
	if completed == nil {
		t.Fatal("nenhum evento MCP concluído foi capturado")
	}
	if completed.Error != "cloudId ausente ou inválido" {
		t.Fatalf("erro do output item não parseado: %q", completed.Error)
	}
	if completed.ServerLabel != "Atlassian" || completed.Name != "getJiraIssue" {
		t.Fatalf("metadados do mcp_call incorretos: %+v", completed)
	}

	rec := logHandler.find(slog.LevelError, "MCP native call FAILED")
	if rec == nil {
		t.Fatal("log de ERRO com o motivo da falha MCP nativa não encontrado")
	}
	for _, want := range []string{`server="Atlassian"`, `tool="getJiraIssue"`, "cloudId ausente ou inválido"} {
		if !strings.Contains(rec.Message, want) {
			t.Fatalf("log de falha não contém %q: %q", want, rec.Message)
		}
	}
}
