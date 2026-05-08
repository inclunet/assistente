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
