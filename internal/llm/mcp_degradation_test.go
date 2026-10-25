package llm

import (
	"strings"
	"testing"
)

func TestLooksLikeNativeMCPUnsupported(t *testing.T) {
	supported := []string{
		`400 unknown variant ` + "`mcp`" + `, expected ` + "`function`",
		`Unknown variant "mcp", expected one of "function"`,
		`tool type "mcp" is not supported by this model`,
		`invalid tool type=mcp`,
	}
	for _, msg := range supported {
		if !looksLikeNativeMCPUnsupported(msg) {
			t.Errorf("esperava detectar não-suporte a MCP nativo em: %q", msg)
		}
	}

	notMatched := []string{
		"",
		"connection refused",
		"401 unauthorized",
		"unknown variant `foo`, expected `bar`", // sem menção a mcp
		"rate limit exceeded",
		`server unhealthy: failed dependency`, // falha de server MCP, não não-suporte ao tipo
	}
	for _, msg := range notMatched {
		if looksLikeNativeMCPUnsupported(msg) {
			t.Errorf("NÃO deveria detectar não-suporte a MCP nativo em: %q", msg)
		}
	}
}

func TestInferMCPFailure_UsesRawServerLabel(t *testing.T) {
	servers := []MCPServerConfig{
		{Name: "Atlassian", Slug: "atlassian", URL: "https://mcp.atlassian.com/v1/sse"},
	}

	failure := inferMCPFailure(
		MCPFailureStageListTools,
		"",
		`{"type":"response.mcp_list_tools.failed","server_label":"Atlassian"}`,
		"",
		servers,
	)
	if failure == nil {
		t.Fatal("esperava falha MCP classificada")
	}
	if failure.ServerName != "Atlassian" || failure.ServerSlug != "atlassian" {
		t.Fatalf("servidor incorreto: %+v", failure)
	}
	if failure.Message != "mcp_error:v1:Atlassian:list_tools" {
		t.Fatalf("marcador sanitizado = %q", failure.Message)
	}
}

func TestFormatMCPFailureUserMessageEscapesDelimiterInServerName(t *testing.T) {
	if got, want := formatMCPFailureUserMessage("Equipe: Produção", MCPFailureStageCall), "mcp_error:v1:Equipe%3A%20Produ%C3%A7%C3%A3o:call"; got != want {
		t.Fatalf("marcador = %q, want %q", got, want)
	}
}

func TestInferMCPFailurePrefersStructuredLabelWhenMessageMentionsAnotherServer(t *testing.T) {
	servers := []MCPServerConfig{
		{Name: "Atlassian", Slug: "atlassian", URL: "https://mcp.atlassian.com/v1/sse"},
		{Name: "Slack", Slug: "slack", URL: "https://mcp.slack.com/mcp"},
	}
	failure := inferMCPFailure(
		MCPFailureStageHandshake,
		"Atlassian connection was mentioned in provider detail",
		`{"server_label":"Slack"}`,
		"",
		servers,
	)
	if failure == nil || failure.ServerName != "Slack" {
		t.Fatalf("rótulo estruturado deveria prevalecer sobre texto da mensagem: %+v", failure)
	}
}

func TestInferMCPFailure_DoesNotExposeProviderErrorText(t *testing.T) {
	const providerText = "secret provider detail: rejected credential pattern"
	servers := []MCPServerConfig{{Name: "Atlassian", Slug: "atlassian", URL: "https://mcp.atlassian.com/v1/sse"}}
	failure := inferMCPFailure(MCPFailureStageHandshake, providerText, `{"server_label":"Atlassian"}`, "", servers)
	if failure == nil {
		t.Fatal("esperava falha MCP classificada")
	}
	if strings.Contains(failure.Message, providerText) || strings.Contains(failure.Message, "credential pattern") {
		t.Fatalf("mensagem expôs texto livre do provedor: %q", failure.Message)
	}
}

func TestInferMCPFailure_MatchesServerFromMessage(t *testing.T) {
	servers := []MCPServerConfig{
		{Name: "Atlassian", Slug: "atlassian", URL: "https://mcp.atlassian.com/v1/sse"},
		{Name: "Slack", Slug: "slack", URL: "https://mcp.slack.com/mcp"},
	}

	failure := inferMCPFailure(
		MCPFailureStageHandshake,
		"authentication error while connecting to mcp.atlassian.com",
		"",
		"",
		servers,
	)
	if failure == nil {
		t.Fatal("esperava falha MCP classificada")
	}
	if failure.ServerName != "Atlassian" {
		t.Fatalf("serverName = %q, want %q", failure.ServerName, "Atlassian")
	}
}

func TestInferMCPFailure_DoesNotAssumeSingleServerForGenericError(t *testing.T) {
	servers := []MCPServerConfig{
		{Name: "Atlassian", Slug: "atlassian", URL: "https://mcp.atlassian.com/v1/sse"},
	}

	failure := inferMCPFailure(
		MCPFailureStageHandshake,
		"provider rate limit exceeded",
		"",
		"",
		servers,
	)
	if failure != nil {
		t.Fatalf("não deveria inferir falha MCP para erro genérico, got %+v", failure)
	}
}

func TestInferMCPFailure_DoesNotMatchTinySlugAsSubstring(t *testing.T) {
	servers := []MCPServerConfig{
		{Name: "Servidor A", Slug: "a", URL: "https://mcp.example.com/a"},
		{Name: "Atlassian", Slug: "atlassian", URL: "https://mcp.atlassian.com/v1/sse"},
	}

	failure := inferMCPFailure(
		MCPFailureStageHandshake,
		"authentication error while connecting to mcp.atlassian.com",
		"",
		"",
		servers,
	)
	if failure == nil {
		t.Fatal("esperava falha MCP classificada")
	}
	if failure.ServerName != "Atlassian" {
		t.Fatalf("serverName = %q, want %q", failure.ServerName, "Atlassian")
	}
}
