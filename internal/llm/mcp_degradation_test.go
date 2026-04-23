package llm

import (
	"context"
	"errors"
	"testing"
)

func TestMaxMCPDegradationRetries(t *testing.T) {
	tests := []struct {
		count int
		want  int
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{3, 3},
		{5, 3},
	}

	for _, tc := range tests {
		if got := maxMCPDegradationRetries(tc.count); got != tc.want {
			t.Fatalf("maxMCPDegradationRetries(%d) = %d, want %d", tc.count, got, tc.want)
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
	if !failure.Degradable {
		t.Fatal("falha deveria ser degradável")
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
	if !failure.Recoverable {
		t.Fatal("falha de autenticação deveria ser recoverable")
	}
}

func TestPlanMCPDegradationRetry_RemovesServerAndCallsRecover(t *testing.T) {
	called := false
	servers := []MCPServerConfig{
		{
			Name: "Atlassian",
			Slug: "atlassian",
			URL:  "https://mcp.atlassian.com/v1/sse",
			Recover: func(context.Context) error {
				called = true
				return nil
			},
		},
		{Name: "Slack", Slug: "slack", URL: "https://mcp.slack.com/mcp"},
	}

	remaining, ok := planMCPDegradationRetry(context.Background(), "openai", 1, servers, &MCPAttemptFailure{
		ServerName: "Atlassian",
		ServerSlug: "atlassian",
		Stage:      MCPFailureStageListTools,
		Message:    "failed dependency",
		Degradable: true,
	})
	if !ok {
		t.Fatal("esperava retry planejado")
	}
	if !called {
		t.Fatal("callback de recovery não foi chamado")
	}
	if len(remaining) != 1 || remaining[0].Name != "Slack" {
		t.Fatalf("remaining = %+v, want only Slack", remaining)
	}
}

func TestPlanMCPDegradationRetry_IgnoresUnknownServer(t *testing.T) {
	servers := []MCPServerConfig{{Name: "Slack", Slug: "slack", URL: "https://mcp.slack.com/mcp"}}
	remaining, ok := planMCPDegradationRetry(context.Background(), "anthropic", 1, servers, &MCPAttemptFailure{
		ServerName: "Atlassian",
		ServerSlug: "atlassian",
		Stage:      MCPFailureStageHandshake,
		Message:    "authentication error",
		Degradable: true,
	})
	if ok {
		t.Fatal("não deveria planejar retry para servidor desconhecido")
	}
	if remaining != nil {
		t.Fatalf("remaining deveria ser nil, got %+v", remaining)
	}
}

func TestPlanMCPDegradationRetry_RecoveryErrorStillRemovesServer(t *testing.T) {
	servers := []MCPServerConfig{
		{
			Name: "Atlassian",
			Slug: "atlassian",
			URL:  "https://mcp.atlassian.com/v1/sse",
			Recover: func(context.Context) error {
				return errors.New("refresh failed")
			},
		},
		{Name: "Slack", Slug: "slack", URL: "https://mcp.slack.com/mcp"},
	}

	remaining, ok := planMCPDegradationRetry(context.Background(), "openai", 2, servers, &MCPAttemptFailure{
		ServerName: "Atlassian",
		ServerSlug: "atlassian",
		Stage:      MCPFailureStageCall,
		Message:    "connector timeout",
		Degradable: true,
	})
	if !ok {
		t.Fatal("esperava retry mesmo com erro na recuperação")
	}
	if len(remaining) != 1 || remaining[0].Name != "Slack" {
		t.Fatalf("remaining = %+v, want only Slack", remaining)
	}
}
