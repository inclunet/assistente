package shell

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/allowlist"
	"assistente/internal/commandpolicy"
)

func TestRunCommand_RejectsMissingCommand(t *testing.T) {
	rc := NewRunCommand(nil, nil, nil, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for missing command")
	}
}

func TestRunCommand_InvalidJSON(t *testing.T) {
	rc := NewRunCommand(nil, nil, nil, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for invalid JSON")
	}
}

func TestRunCommand_DeniedByAllowlist(t *testing.T) {
	al := &allowlist.Allowlist{
		AlwaysDeny:    []string{"rm *"},
		AutoApprove:   []string{},
		DefaultAction: "confirm",
	}

	rc := NewRunCommand(nil, nil, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result when denied by allowlist")
	}
}

func TestRunCommand_DeniesCompoundCommandWithDeniedAtom(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"git status"},
		AlwaysDeny:    []string{"rm -rf *"},
		DefaultAction: "confirm",
	}

	rc := NewRunCommand(nil, nil, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"git status && rm -rf dist"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result when a compound command contains denied atom")
	}
}

func TestRunCommand_CompoundAllowedAtomsApprove(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"git status", "git diff"},
		DefaultAction: "confirm",
	}

	rc := NewRunCommand(nil, nil, func() *allowlist.Allowlist { return al }, ".")

	result := rc.evaluateCommand("git status && git diff")
	if result.Decision != allowlist.DecisionApprove {
		t.Fatalf("decision = %s, want approve; reasons=%v", result.Decision, result.Reasons)
	}
}

func TestRunCommand_RedirectForcesConfirmation(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"echo"},
		DefaultAction: "confirm",
	}

	confirmFn := func(ctx context.Context, command, workDir string) (bool, error) {
		return false, nil
	}

	rc := NewRunCommand(nil, confirmFn, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"echo ok > out.txt"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result when confirmation is rejected")
	}
}

func TestRedactCommandForLog_DoesNotIncludeArgs(t *testing.T) {
	// O log nao pode conter args (que podem ter tokens). Verificamos que o
	// resumo so menciona o programa e a contagem.
	cases := []struct {
		name        string
		command     string
		mustContain []string
		mustNotHave []string
	}{
		{
			name:        "single command com flag sensivel",
			command:     "psql -h db -U admin -W hunter2",
			mustContain: []string{"psql", "args"},
			mustNotHave: []string{"hunter2", "admin", "-W"},
		},
		{
			name:        "compound command",
			command:     "git status && curl -H 'Authorization: Bearer secret-token' https://api",
			mustContain: []string{"git", "curl", "|"},
			mustNotHave: []string{"secret-token", "Authorization", "Bearer", "api"},
		},
		{
			name:        "input vazio cai em fallback unparsed",
			command:     "",
			mustContain: []string{"unparsed"},
			mustNotHave: []string{},
		},
		{
			// Copilot review: env inline (KEY=VALUE cmd) ja era o caso mais
			// perigoso do redactCommandForLog. O parser agora consome essas
			// atribuicoes em Command.EnvAssignments, entao Program vira o
			// programa real ("git"), o resumo cita apenas "[env=N]" sem o
			// nome ou o valor da variavel.
			name:        "env inline assignment",
			command:     "TOKEN=supersecret git status",
			mustContain: []string{"git", "args", "env=1"},
			mustNotHave: []string{"supersecret", "TOKEN", "TOKEN=supersecret"},
		},
		{
			name:        "multiple env inline assignments",
			command:     "FOO=bar BAZ=qux curl https://api/admin",
			mustContain: []string{"curl", "env=2"},
			mustNotHave: []string{"bar", "qux", "FOO", "BAZ", "admin", "api"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parse := commandpolicy.Parse(tc.command)
			summary := redactCommandForLog(tc.command, commandpolicy.EvaluationResult{Parse: parse})
			for _, want := range tc.mustContain {
				if !strings.Contains(summary, want) {
					t.Errorf("summary %q nao contem %q", summary, want)
				}
			}
			for _, leak := range tc.mustNotHave {
				if strings.Contains(summary, leak) {
					t.Errorf("summary %q vazou %q (nunca deveria conter args)", summary, leak)
				}
			}
		})
	}
}

func TestRunCommand_DenyContentDoesNotLeakRawCommand(t *testing.T) {
	// O Content retornado em DecisionDeny vai para o LLM. Nunca deve conter
	// tokens/senhas em flags inline — usa a forma redigida em vez de %q da
	// string crua.
	al := &allowlist.Allowlist{
		AlwaysDeny:    []string{"psql"},
		DefaultAction: "confirm",
	}

	rc := NewRunCommand(nil, nil, func() *allowlist.Allowlist { return al }, ".")

	command := `psql -h db -U admin -W hunter2-supersecret`
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"`+command+`"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for denied command")
	}
	if strings.Contains(result.Content, "hunter2-supersecret") {
		t.Errorf("Content vazou senha (%q): %s", "hunter2-supersecret", result.Content)
	}
	if strings.Contains(result.Content, "-W") {
		t.Errorf("Content vazou flag sensivel (-W): %s", result.Content)
	}
	if !strings.Contains(result.Content, "psql") {
		t.Errorf("Content deveria mencionar o programa para diagnostico minimo: %s", result.Content)
	}
}

func TestRunCommand_DenyContentDoesNotLeakAllowlistPattern(t *testing.T) {
	// Copilot review thread (PR #117): o pattern legado de always_deny
	// pode conter conteudo sensivel que o usuario configurou (URLs internas,
	// hostnames, identificadores). O Content devolvido em DecisionDeny vai
	// pro LLM e nao pode replicar esse pattern bruto — apenas o slice
	// EvaluationResult.Reasons (safe) deve ser interpolado.
	al := &allowlist.Allowlist{
		AlwaysDeny:    []string{"curl https://internal.prod.corp/admin-secret-token"},
		DefaultAction: "confirm",
	}

	rc := NewRunCommand(nil, nil, func() *allowlist.Allowlist { return al }, ".")

	command := `curl https://internal.prod.corp/admin-secret-token`
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"`+command+`"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for denied command")
	}
	for _, fragment := range []string{
		"internal.prod.corp",
		"admin-secret-token",
	} {
		if strings.Contains(result.Content, fragment) {
			t.Errorf("Content (LLM-bound) vazou pattern legado %q: %s", fragment, result.Content)
		}
	}
	if !strings.Contains(result.Content, `"curl" bloqueado por always_deny[0]`) {
		t.Errorf("Content deveria citar o motivo safe (program + idx): %s", result.Content)
	}
}

func TestRunCommand_EnvInlineDoesNotLeakInContent(t *testing.T) {
	// Copilot review thread (PR #117): "TOKEN=secret cmd ..." nunca pode
	// vazar o valor no Content devolvido ao LLM. O parser consome o env
	// assignment em Command.EnvAssignments e a feature env_assignment forca
	// confirm; aqui validamos o caminho de confirmacao rejeitada (que tambem
	// retorna Content para o LLM).
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"git status"},
		DefaultAction: "confirm",
	}

	confirmFn := func(ctx context.Context, command, workDir string) (bool, error) {
		return false, nil
	}

	rc := NewRunCommand(nil, confirmFn, func() *allowlist.Allowlist { return al }, ".")

	command := `TOKEN=supersecret git status`
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"`+command+`"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result when env inline forces confirmation rejected")
	}
	for _, fragment := range []string{"supersecret", "TOKEN=supersecret"} {
		if strings.Contains(result.Content, fragment) {
			t.Errorf("Content (LLM-bound) vazou env inline %q: %s", fragment, result.Content)
		}
	}
	if !strings.Contains(result.Content, "git") {
		t.Errorf("Content deveria mencionar o programa real: %s", result.Content)
	}
}

func TestRunCommand_DenyContentDoesNotLeakStructuredRuleDescription(t *testing.T) {
	// Mesmo problema do teste acima, mas para regras estruturadas: Subcommands,
	// Args e Description sao editaveis pelo usuario e podem conter dados
	// sensiveis. O Content nao pode replicar describeRule(rule) — apenas a
	// reason safe (program + rule[N]).
	al := &allowlist.Allowlist{
		DefaultAction: "confirm",
		CommandRules: []allowlist.CommandRule{
			{
				Program:     "psql",
				Subcommands: []string{"-h", "db-prod-internal.corp.example"},
				Args:        []string{"*"},
				Decision:    "deny",
				Description: "block prod database with token shhh-don-t-tell",
			},
		},
	}

	rc := NewRunCommand(nil, nil, func() *allowlist.Allowlist { return al }, ".")

	command := `psql -h db-prod-internal.corp.example -U app`
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"`+command+`"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for denied command")
	}
	for _, fragment := range []string{
		"db-prod-internal.corp.example",
		"shhh-don-t-tell",
		"block prod database",
	} {
		if strings.Contains(result.Content, fragment) {
			t.Errorf("Content (LLM-bound) vazou detalhe da regra %q: %s", fragment, result.Content)
		}
	}
	if !strings.Contains(result.Content, `"psql" bloqueado por regra estruturada (rule[0])`) {
		t.Errorf("Content deveria citar o motivo safe (program + rule[N]): %s", result.Content)
	}
}

func TestRunCommand_ConfirmRejectedContentDoesNotLeakRawCommand(t *testing.T) {
	al := &allowlist.Allowlist{DefaultAction: "confirm"}

	confirmFn := func(ctx context.Context, command, workDir string) (bool, error) {
		return false, nil
	}

	rc := NewRunCommand(nil, confirmFn, func() *allowlist.Allowlist { return al }, ".")

	command := `curl -H 'Authorization: Bearer secret-bearer-token' https://api`
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"`+command+`"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error when confirmation rejected")
	}
	if strings.Contains(result.Content, "secret-bearer-token") {
		t.Errorf("Content vazou token: %s", result.Content)
	}
	if !strings.Contains(result.Content, "curl") {
		t.Errorf("Content deveria mencionar o programa: %s", result.Content)
	}
}

func TestRunCommand_ConfirmRejected(t *testing.T) {
	al := &allowlist.Allowlist{DefaultAction: "confirm"}

	confirmFn := func(ctx context.Context, command, workDir string) (bool, error) {
		return false, nil
	}

	rc := NewRunCommand(nil, confirmFn, func() *allowlist.Allowlist { return al }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"echo test"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result when user rejects confirmation")
	}
}
