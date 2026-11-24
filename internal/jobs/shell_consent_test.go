package jobs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/allowlist"
	"assistente/internal/shelltrust"
	"assistente/internal/terminal"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/tools/shell"
)

func TestShellConsentUsesOnlyLiteralTemplatePrefix(t *testing.T) {
	const marker = "valor-sintetico-consentimento"
	const template = `example {{ secret "token" }}`
	for _, tc := range []struct {
		name, prefix, source string
		inputs               map[string]any
	}{
		{"literal", "example", template, map[string]any{"Command": template}},
		{"programa do cofre", "", "", map[string]any{"command": `{{ secret "token" }}`}},
		{"token unido pelo template", "", "", map[string]any{"command": `example {{- secret "token" }}`}},
		{"argumento literal privado", "example", "", map[string]any{"command": `example valor-particular {{ secret "token" }}`}},
		{"expressão adicional", "example", "", map[string]any{"command": `example {{ print "privado" }} {{ secret "token" }}`}},
		{"colisão inversa", "", "", map[string]any{"Command": template, "command": "public"}},
		{"colisão", "", "", map[string]any{"COMMAND": "public", "command": template}},
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
				if probe.calls != 1 || !prompt.ProtectedCommand || prompt.Command != "" || prompt.Directory != "" || prompt.LiteralPrefix != tc.prefix || prompt.PublicTemplate != tc.source ||
					len(prompt.AvailableScopes) != 1 || prompt.AvailableScopes[0] != "once" {
					t.Fatal("a apresentação, os escopos ou a execução do comando foram incorretos")
				}
			})
		}
	}
}

func TestProtectedWorkingDirectoryInJobAndCatalog(t *testing.T) {
	const marker = "diretorio-confidencial-sintetico"
	for _, route := range []string{"job", "catalog"} {
		t.Run(route, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, marker), 0700); err != nil {
				t.Fatal(err)
			}
			var prompt shelltrust.PromptRequest
			auth := shelltrust.Authorizer{Prompt: func(_ context.Context, req shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
				prompt = req
				return shelltrust.PromptDecision{Approve: true, Scope: "once"}, nil
			}}
			probe := &shellDispatchProbe{entry: &terminal.HistoryEntry{Output: "ok"}}
			tool := shell.NewRunCommand(probe, auth.Authorize, nil, root)
			executor, repo, ctx := invocationBackedExecutor(t, tool)
			executor.secretStore = secretStoreFunc(func(context.Context, string) (string, error) { return marker, nil })
			inputs := map[string]any{"command": "example", "Working_Directory": `{{ secret "pasta" }}`}
			var result any
			if route == "job" {
				run := executor.Execute(ctx, &Job{ID: "dir", Tool: tool.Name(), Inputs: inputs}, &TriggerContext{Type: TriggerManual})
				if run.Status != RunStatusCompleted {
					t.Fatal("job falhou", run.Error)
				}
				result = run
			} else {
				manager := mustNewManager(t, ManagerConfig{ToolRegistry: executor.toolRegistry, ToolInvocations: executor.toolInvocations, SecretStore: executor.secretStore})
				test, err := manager.TestToolDryRunContext(ctx, TestToolRequest{ToolName: tool.Name(), Inputs: inputs, AllowUnsafe: true})
				if err != nil || test == nil || !test.Success {
					t.Fatal("teste de catálogo falhou", err)
				}
				result = test
			}
			if filepath.Base(probe.directory) != marker || probe.calls != 1 || prompt.Command != "example" || prompt.Directory != redactedValue || len(prompt.AvailableScopes) != 1 || prompt.AvailableScopes[0] != "once" {
				t.Fatal("destino operacional, pergunta ou escopos incorretos")
			}
			rows, err := repo.List(ctx, toolinvocations.Filter{})
			if err != nil || len(rows) != 1 {
				t.Fatal("invocação ausente", err)
			}
			encoded, err := json.Marshal([]any{result, rows[0]})
			if err != nil || strings.Contains(string(encoded), marker) || invocationOutput(string(rows[0].Output))["_meta_workDir"] != redactedValue {
				t.Fatal("o resultado ou histórico expôs o diretório")
			}
		})
	}
}

func TestShellJobRecordsWhoRefusedTheCommand(t *testing.T) {
	for _, tc := range []struct {
		name, diagnostic, code string
		approve                bool
	}{
		{"veto após aprovação", "bloqueado pela política de comandos", shell.FailurePolicyVetoed, true},
		{"recusa explícita", "negado pelo usuário", shell.FailureCommandRefused, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const marker = "valor-protegido-79"
			policy := &allowlist.Allowlist{DefaultAction: "confirm"}
			readPolicy := func(context.Context) (*allowlist.Allowlist, error) { return policy, nil }
			prompts := 0
			auth := shelltrust.Authorizer{Policy: readPolicy, Prompt: func(context.Context, shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
				prompts++
				policy.AlwaysDeny = []string{marker}
				return shelltrust.PromptDecision{Approve: tc.approve, Scope: "once"}, nil
			}}
			probe := new(shellDispatchProbe)
			tool := shell.NewRunCommand(probe, auth.Authorize, readPolicy, t.TempDir())
			executor, repo, ctx := invocationBackedExecutor(t, tool)
			executor.secretStore = secretStoreFunc(func(context.Context, string) (string, error) { return marker, nil })
			job := &Job{ID: "recusa", Tool: tool.Name(), Inputs: map[string]any{"command": `{{ secret "programa" }}`},
				ErrorPolicy: ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 1, RetryDelay: "1ms"}}
			run := executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual})
			if run.Status != RunStatusFailed || !strings.Contains(run.Error, tc.diagnostic) || strings.Contains(run.Error, marker) ||
				run.RetryCount != 0 || prompts != 1 || probe.calls != 0 {
				t.Fatal("o job não preservou a origem da recusa ou despachou a operação")
			}
			records, err := repo.List(ctx, toolinvocations.Filter{OriginID: run.RunID})
			if err != nil || len(records) != 1 {
				t.Fatal("invocação ausente")
			}
			record := records[0]
			content := toolinvocations.ExtractToolInvocationResult(string(record.Output)).Content
			if record.ErrorCode != tc.code || record.Retryable || !record.RetryabilityKnown ||
				!strings.Contains(content, tc.diagnostic) || strings.Contains(content, marker) {
				t.Fatal("a invocação registrada perdeu a origem da recusa ou expôs o valor protegido")
			}
		})
	}
}

func TestProtecaoDoShellDependeDaOrigemDaMascara(t *testing.T) {
	for _, original := range []string{redactedValue, `{{ secret "valor" }}`} {
		input := map[string]any{"working_directory": original, "command": original}
		resolved := map[string]any{"working_directory": redactedValue, "command": redactedValue}
		ctx := withRedactedShellMetadata(t.Context(), input, resolved)
		_, protected := invocationctx.LookupShellCommandMetadata(ctx)
		want := original != redactedValue
		if protected != want || (invocationctx.ShellDirectoryMetadata(ctx) != "") != want {
			t.Fatalf("origem %q: comando protegido=%t diretório=%q", original, protected, invocationctx.ShellDirectoryMetadata(ctx))
		}
	}
}
