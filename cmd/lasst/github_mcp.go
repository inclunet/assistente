package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"assistente/internal/credentials"
	bridge "assistente/internal/mcp"
	"assistente/internal/tools"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const githubMCPEndpoint = "https://api.githubcopilot.com/mcp/"

type reviewScope struct {
	owner, repo string
	number      int
}
type githubConfig struct {
	GitHub struct {
		Enabled bool `json:"enabled"`
	} `json:"github"`
}

// A configuração versionada habilita apenas o conector fixo. O escopo e o
// segredo vêm do ambiente confiável, nunca de um arquivo do PR.
func openGitHubMCP(ctx context.Context, root string) ([]tools.Tool, func(), error) {
	f, err := os.OpenRoot(root)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := f.ReadFile(filepath.Join(".assistente", "lasst-mcp.json"))
	if os.IsNotExist(err) {
		return nil, func() {}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var cfg githubConfig
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&cfg); err != nil {
		return nil, nil, fmt.Errorf("configuração MCP inválida: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, nil, errors.New("configuração MCP contém dados adicionais")
	}
	if !cfg.GitHub.Enabled {
		return nil, func() {}, nil
	}
	repoParts := strings.Split(os.Getenv("LASST_GITHUB_REPOSITORY"), "/")
	number, err := strconv.Atoi(os.Getenv("LASST_GITHUB_PR"))
	token := os.Getenv("LASST_GITHUB_TOKEN")
	if len(repoParts) != 2 || repoParts[0] == "" || repoParts[1] == "" || err != nil || number <= 0 || token == "" {
		return nil, nil, errors.New("MCP exige LASST_GITHUB_REPOSITORY, LASST_GITHUB_PR e LASST_GITHUB_TOKEN válidos")
	}
	scope := &reviewScope{owner: repoParts[0], repo: repoParts[1], number: number}
	client := sdk.NewClient(&sdk.Implementation{Name: "lasst", Version: "poc"}, nil)
	transport := &sdk.StreamableClientTransport{Endpoint: githubMCPEndpoint, HTTPClient: &http.Client{Transport: bearerTransport{token: token}, CheckRedirect: credentials.SameOriginRedirect}}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("conectar MCP GitHub: %w", err)
	}
	closeSession := func() { _ = session.Close() }
	result, err := discoverGitHubTools(ctx, session, scope)
	if err != nil {
		closeSession()
		return nil, nil, err
	}
	return result, closeSession, nil
}

func discoverGitHubTools(ctx context.Context, session *sdk.ClientSession, scope *reviewScope) ([]tools.Tool, error) {
	found := map[string]bool{}
	var result []tools.Tool
	seen := map[string]bool{}
	cursor := ""
	for {
		if seen[cursor] {
			return nil, errors.New("paginação MCP repetida")
		}
		seen[cursor] = true
		page, err := session.ListTools(ctx, &sdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("listar MCP: %w", err)
		}
		for _, tool := range page.Tools {
			if tool == nil || !allowedGitHubTool(tool.Name) {
				continue
			}
			if found[tool.Name] {
				return nil, errors.New("ferramenta MCP duplicada")
			}
			found[tool.Name] = true
			result = append(result, &reviewTool{inner: bridge.NewMCPToolBridge("github", tool, session), name: tool.Name, scope: scope})
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	for _, name := range []string{"pull_request_read"} {
		if !found[name] {
			return nil, fmt.Errorf("servidor MCP não oferece %s", name)
		}
	}
	return result, nil
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || req.URL.Host != "api.githubcopilot.com" {
		return nil, errors.New("destino MCP não autorizado")
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(clone)
}
func allowedGitHubTool(name string) bool {
	return name == "pull_request_read"
}

type reviewTool struct {
	inner tools.Tool
	name  string
	scope *reviewScope
}

func (t *reviewTool) Name() string { return t.inner.Name() }
func (t *reviewTool) Description() string {
	return fmt.Sprintf("%s\nPR autorizado: owner=%s, repo=%s, pullNumber=%d. A CLI impõe estes valores.", t.inner.Description(), t.scope.owner, t.scope.repo, t.scope.number)
}
func (t *reviewTool) MCPIdentity() tools.MCPToolIdentity {
	return tools.MCPToolIdentity{ServerSlug: "github", ToolName: t.name}
}
func (t *reviewTool) Parameters() json.RawMessage { return t.inner.Parameters() }
func (t *reviewTool) Execute(ctx context.Context, raw json.RawMessage) (tools.ToolResult, error) {
	var args map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&args); err != nil || args == nil {
		return tools.ToolResult{Content: "argumentos inválidos", IsError: true}, nil
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return tools.ToolResult{Content: "argumentos inválidos", IsError: true}, nil
	}
	method, _ := args["method"].(string)
	if t.name != "pull_request_read" || (method != "get" && method != "get_diff" && method != "get_files" && method != "get_review_comments" && method != "get_reviews") {
		return tools.ToolResult{Content: "operação não autorizada", IsError: true}, nil
	}
	// A identidade é preenchida pela CLI a partir do ambiente confiável.
	args["owner"], args["repo"], args["pullNumber"] = t.scope.owner, t.scope.repo, t.scope.number
	sanitized, err := json.Marshal(args)
	if err != nil {
		return tools.ToolResult{}, err
	}
	return t.inner.Execute(ctx, sanitized)
}
