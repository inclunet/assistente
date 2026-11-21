package jobs

import (
	"context"
	"testing"

	"assistente/internal/shelltrust"
	"assistente/internal/terminal"
	"assistente/internal/tools/shell"
)

func TestShellConsentUsesOnlyLiteralTemplatePrefix(t *testing.T) {
	const marker = "valor-sintetico-consentimento"
	const template = `example {{ secret "token" }}`
	for _, tc := range []struct {
		name, prefix string
		inputs       map[string]any
	}{
		{"literal", "example", map[string]any{"Command": template}},
		{"programa do cofre", "", map[string]any{"command": `{{ secret "token" }}`}},
		{"token unido pelo template", "", map[string]any{"command": `example {{- secret "token" }}`}},
		{"colisão", "", map[string]any{"COMMAND": "public", "command": template}},
	} {
		for _, route := range []string{"job", "dry-run"} {
			t.Run(tc.name+"/"+route, func(t *testing.T) {
				var prompt shelltrust.PromptRequest
				auth := shelltrust.Authorizer{Prompt: func(_ context.Context, req shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
					prompt = req
					return shelltrust.PromptDecision{Approve: true, Scope: "once"}, nil
				}}
				probe := &shellDispatchProbe{entry: &terminal.HistoryEntry{Output: "ok"}}
				tool := shell.NewRunCommand(probe, auth.Authorize, nil, t.TempDir())
				executor, _, ctx := invocationBackedExecutor(t, tool)
				executor.secretStore = secretStoreFunc(func(context.Context, string) (string, error) { return marker, nil })
				if route == "job" {
					run := executor.Execute(ctx, &Job{ID: "teste", Tool: tool.Name(), Inputs: tc.inputs}, &TriggerContext{Type: TriggerManual})
					if run.Status != RunStatusCompleted {
						t.Fatal("job não concluiu")
					}
				} else {
					manager := mustNewManager(t, ManagerConfig{ToolRegistry: executor.toolRegistry, ToolInvocations: executor.toolInvocations, SecretStore: executor.secretStore})
					result, err := manager.TestToolDryRunContext(ctx, TestToolRequest{ToolName: tool.Name(), Inputs: tc.inputs, AllowUnsafe: true})
					if err != nil || result == nil || !result.Success {
						t.Fatal("teste de ferramenta não concluiu")
					}
				}
				if probe.calls != 1 || !prompt.ProtectedCommand || prompt.Command != "" || prompt.Directory != "" || prompt.LiteralPrefix != tc.prefix ||
					len(prompt.AvailableScopes) != 1 || prompt.AvailableScopes[0] != "once" {
					t.Fatal("a apresentação, os escopos ou a execução do comando foram incorretos")
				}
			})
		}
	}
}
