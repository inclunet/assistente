package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/tools"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type recordingTool struct {
	arguments map[string]any
	calls     int
	fail      bool
}

func (*recordingTool) Name() string                { return "mcp_github__test" }
func (*recordingTool) Description() string         { return "test" }
func (*recordingTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f *recordingTool) Execute(_ context.Context, raw json.RawMessage) (tools.ToolResult, error) {
	f.calls++
	if err := json.Unmarshal(raw, &f.arguments); err != nil {
		return tools.ToolResult{}, err
	}
	if f.fail {
		return tools.ToolResult{IsError: true, Content: "falha"}, nil
	}
	return tools.ToolResult{Content: "ok"}, nil
}
func invoke(t *testing.T, tool *reviewTool, value string) tools.ToolResult {
	t.Helper()
	r, err := tool.Execute(context.Background(), json.RawMessage(value))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestReadToolForcesPRAndDeniesOtherOperations(t *testing.T) {
	scope := &reviewScope{owner: "org", repo: "repo", number: 9}
	rec := &recordingTool{}
	read := &reviewTool{inner: rec, name: "pull_request_read", scope: scope}
	if r := invoke(t, read, `{"owner":"outra","repo":"outro","pullNumber":12,"method":"get_diff"}`); r.IsError {
		t.Fatal(r.Content)
	}
	if rec.arguments["owner"] != "org" || rec.arguments["repo"] != "repo" || rec.arguments["pullNumber"] != float64(9) {
		t.Fatalf("argumentos: %#v", rec.arguments)
	}
	if r := invoke(t, read, `{"method":"delete"}`); !r.IsError || rec.calls != 1 {
		t.Fatal("operação não permitida alcançou MCP")
	}
	if allowedGitHubTool("pull_request_review_write") || allowedGitHubTool("add_comment_to_pending_review") {
		t.Fatal("escrita ainda não é suportada")
	}
}
func TestMCPConfigMissingOrUntrustedFields(t *testing.T) {
	root := t.TempDir()
	_, closeFn, err := openGitHubMCP(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	closeFn()
	dir := filepath.Join(root, ".assistente")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lasst-mcp.json"), []byte(`{"github":{"enabled":true,"url":"https://evil.invalid"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err = openGitHubMCP(context.Background(), root)
	if err == nil {
		t.Fatal("configuração deve recusar URL arbitrária")
	}
}
func TestBearerTransportRejectsOtherHosts(t *testing.T) {
	r, _ := http.NewRequest("GET", "https://example.com", nil)
	_, err := (bearerTransport{token: "secret"}).RoundTrip(r)
	if err == nil || !strings.Contains(err.Error(), "não autorizado") {
		t.Fatalf("err=%v", err)
	}
	if r.Header.Get("Authorization") != "" {
		t.Fatal(errors.New("token exposto"))
	}
}
func TestMCPIntegrationInMemory(t *testing.T) {
	ctx := context.Background()
	server := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	var got map[string]any
	server.AddTool(&sdk.Tool{Name: "pull_request_read", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"method": map[string]any{"type": "string"}}}}, func(_ context.Context, r *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		if err := json.Unmarshal(r.Params.Arguments, &got); err != nil {
			return nil, err
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "resultado do PR"}}}, nil
	})
	server.AddTool(&sdk.Tool{Name: "create_or_update_file", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		t.Error("escrita executada")
		return &sdk.CallToolResult{}, nil
	})
	st, ct := sdk.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ss.Close() }()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.Close() }()
	all, err := discoverGitHubTools(ctx, cs, &reviewScope{owner: "org", repo: "repo", number: 9})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("tools=%d", len(all))
	}
	if _, ok := tools.MCPIdentityOf(all[0]); !ok {
		t.Fatal("identidade MCP ausente")
	}
	if !strings.Contains(all[0].Description(), "pullNumber=9") {
		t.Fatal("contexto do PR ausente")
	}
	result, err := all[0].Execute(ctx, json.RawMessage(`{"method":"get_diff","owner":"outra","repo":"outro","pullNumber":99}`))
	if err != nil || result.IsError || !strings.Contains(result.Content, "resultado do PR") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got["owner"] != "org" || got["repo"] != "repo" || got["pullNumber"] != float64(9) {
		t.Fatalf("argumentos=%v", got)
	}
}

type largeTool struct{}

func (largeTool) Name() string                { return "mcp_github__large" }
func (largeTool) Description() string         { return "large" }
func (largeTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (largeTool) MCPIdentity() tools.MCPToolIdentity {
	return tools.MCPToolIdentity{ServerSlug: "github", ToolName: "large"}
}
func (largeTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: strings.Repeat("x", 110*1024)}, nil
}
func TestMCPExecutorRejectsOversizedResult(t *testing.T) {
	registry := tools.NewRegistry()
	if err := registry.Register(largeTool{}); err != nil {
		t.Fatal(err)
	}
	cfg := tools.DefaultExecutorConfig()
	cfg.RequireCompleteResult = true
	result := tools.NewExecutor(registry, cfg).ExecuteOne(context.Background(), tools.ToolCall{ID: "1", Type: "function", Function: tools.FunctionCall{Name: "mcp_github__large", Arguments: `{}`}})
	if !result.Result.IsError || result.Error == nil {
		t.Fatalf("resultado grande não recusado: %+v", result)
	}
}

type largeResultStreamer struct {
	t     *testing.T
	calls int
}

func (s *largeResultStreamer) StreamChat(_ context.Context, messages []llm.Message, _ llm.ChatParams, h llm.StreamHandler, _ ...llm.ToolDefinition) {
	s.t.Helper()
	s.calls++
	if s.calls == 1 {
		h.OnToolCalls([]llm.ToolCall{{ID: "large", Type: "function", Function: llm.FunctionCall{Name: "mcp_github__large", Arguments: `{}`}}}, "", llm.Usage{}, "")
		return
	}
	content := messages[len(messages)-1].Content.(string)
	if strings.Contains(content, strings.Repeat("x", 100)) || !strings.Contains(content, "limite") {
		s.t.Fatalf("resultado inesperado: %q", content)
	}
	h.OnDone("ok", llm.Usage{}, "")
}
func TestRunRejectsLargeMCPResult(t *testing.T) {
	s := &largeResultStreamer{t: t}
	out, err := runWithTools(context.Background(), s, "modelo", "skill", "prompt", t.TempDir(), []tools.Tool{largeTool{}})
	if err != nil || out != "ok" || s.calls != 2 {
		t.Fatalf("out=%q err=%v calls=%d", out, err, s.calls)
	}
}
