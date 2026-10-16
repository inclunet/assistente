package llm

import "testing"

// TestMCPFailureRecoverablyHandled cobre o gate que decide entre WARN (falha
// recuperável que será tratada pela degradação) e ERRO (falha real/definitiva)
// no log do provider Responses.
func TestMCPFailureRecoverablyHandled(t *testing.T) {
	cases := []struct {
		name        string
		failure     *MCPAttemptFailure
		nonRetry    bool
		wantHandled bool
	}{
		{"nil", nil, false, false},
		{"recuperável, sem efeito não-retentável", &MCPAttemptFailure{Recoverable: true}, false, true},
		{"recuperável, mas já houve efeito não-retentável", &MCPAttemptFailure{Recoverable: true}, true, false},
		{"não-recuperável", &MCPAttemptFailure{Recoverable: false}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mcpFailureRecoverablyHandled(tc.failure, tc.nonRetry); got != tc.wantHandled {
				t.Fatalf("mcpFailureRecoverablyHandled = %v; want %v", got, tc.wantHandled)
			}
		})
	}
}

// TestInferMCPFailure_Listing424EhRecuperavel garante que o 424 (Failed
// Dependency) de listagem de tools do Slack — exatamente o que aparecia como
// ERRO em série no assistente.log — é classificado como recuperável, de modo
// que o provider passe a logá-lo em WARN (tratado pela degradação).
func TestInferMCPFailure_Listing424EhRecuperavel(t *testing.T) {
	servers := []MCPServerConfig{{Name: "Slack", Slug: "slack"}}
	msg := "litellm.APIError: Error retrieving tool list from MCP server: 'Slack'. Http status code: 424 (Failed Dependency)"

	for _, stage := range []MCPFailureStage{MCPFailureStageListTools, MCPFailureStageHandshake} {
		failure := inferMCPFailure(stage, msg, "", "", servers)
		if failure == nil {
			t.Fatalf("stage %s: inferMCPFailure retornou nil para o 424 do Slack", stage)
		}
		if !failure.Recoverable {
			t.Fatalf("stage %s: 424 (Failed Dependency) deveria ser recuperável", stage)
		}
		if !mcpFailureRecoverablyHandled(failure, false) {
			t.Fatalf("stage %s: falha recuperável sem efeito não-retentável deveria ser logada em WARN", stage)
		}
	}
}
