package shell

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/allowlist"
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
