package llm

import "testing"

func TestMCPListingFailurePreservesServerDiagnostic(t *testing.T) {
	servers := []MCPServerConfig{{Name: "Slack", Slug: "slack"}}
	for _, stage := range []MCPFailureStage{MCPFailureStageListTools, MCPFailureStageHandshake} {
		failure := inferMCPFailure(stage, "Error retrieving tool list from MCP server: Slack, 424 Failed Dependency", "", "", servers)
		if failure == nil || failure.ServerSlug != "slack" || failure.Message != formatMCPFailureUserMessage("Slack", stage) {
			t.Fatalf("diagnóstico da falha perdido: %#v", failure)
		}
		if len(servers) != 1 {
			t.Fatal("diagnóstico não pode retirar ferramentas")
		}
	}
}
