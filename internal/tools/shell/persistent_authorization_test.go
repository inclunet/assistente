package shell

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"assistente/internal/allowlist"
	"assistente/internal/authorization"
	"assistente/internal/shelltrust"
	"assistente/internal/testutil/authorizationfixture"
)

func TestDirectorySymlinkSwapDuringConsentDoesNotExecute(t *testing.T) {
	root, first, second := t.TempDir(), t.TempDir(), t.TempDir()
	link := filepath.Join(root, "workspace")
	if err := os.Symlink(first, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink indisponível: %v", err)
		}
		t.Fatal(err)
	}
	m := &MockSessionManager{}
	policy := func(context.Context) (*allowlist.Allowlist, error) {
		return &allowlist.Allowlist{DefaultAction: "confirm"}, nil
	}
	rc := NewRunCommand(m, func(_ context.Context, _, directory string) (bool, error) {
		expected, err := filepath.EvalSymlinks(first)
		if err != nil || directory != expected {
			t.Fatalf("diretório apresentado: %s, esperado %s: %v", directory, expected, err)
		}
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(second, link); err != nil {
			t.Fatal(err)
		}
		return true, nil
	}, policy, link)
	result, err := rc.Execute(authorizationfixture.Context(), json.RawMessage(`{"command":"git push"}`))
	if err != nil || !result.IsError || m.runEphemeralCalls != 0 {
		t.Fatalf("executou após troca: %+v %v calls=%d", result, err, m.runEphemeralCalls)
	}
}

func TestPersistentTerminalNeverReusesInitialDirectoryGrant(t *testing.T) {
	dir := t.TempDir()
	manager := shelltrust.NewManager(authorizationfixture.Options(t, dir, dir))
	ctx := authorizationfixture.Context()
	selector, _ := shelltrust.Represent("git push", dir)
	if err := manager.Add(ctx, shelltrust.Entry{Selector: selector, Scope: authorization.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	policy := func(context.Context) (*allowlist.Allowlist, error) {
		return &allowlist.Allowlist{DefaultAction: "confirm"}, nil
	}
	prompts := 0
	authorizer := shelltrust.Authorizer{Manager: manager, Policy: policy, PersistentAllowed: func(context.Context) bool { return true },
		Prompt: func(_ context.Context, req shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
			prompts++
			if !req.PersistentTerminal || len(req.AvailableScopes) != 1 || req.AvailableScopes[0] != authorization.ScopeOnce {
				t.Errorf("terminal ofereceu concessão reutilizável: %+v", req)
			}
			return shelltrust.PromptDecision{Approve: true, Scope: authorization.ScopeOnce}, nil
		},
	}
	// CWD permanece o diretório inicial, como no Manager real após um cd.
	terminalManager := &MockSessionManager{liveSessions: map[string]bool{"terminal": true}, sessionCWD: map[string]string{"terminal": dir}}
	rc := NewRunCommand(terminalManager, authorizer.Authorize, policy, dir)
	for _, args := range []string{
		`{"command":"git push","terminal_id":"terminal"}`,
		`{"command":"cd ..","terminal_id":"terminal"}`,
		`{"command":"git push","terminal_id":"terminal"}`,
		`{"command":"git push","persistent":true}`,
	} {
		result, err := rc.Execute(ctx, json.RawMessage(args))
		if err != nil || result.IsError {
			t.Fatalf("execução: %+v %v", result, err)
		}
	}
	if prompts != 4 || terminalManager.runCommandCalls != 4 {
		t.Fatalf("perguntas=%d execuções=%d", prompts, terminalManager.runCommandCalls)
	}
	// A concessão continua válida para execução efêmera com diretório garantido.
	if allowed, err := authorizer.Authorize(ctx, "git push", dir); err != nil || !allowed || prompts != 4 {
		t.Fatalf("execução efêmera: %v %v perguntas=%d", allowed, err, prompts)
	}
}

func TestPolicyUnavailableAfterApproval(t *testing.T) {
	dir := t.TempDir()
	approved := false
	policy := func(context.Context) (*allowlist.Allowlist, error) {
		if approved {
			return nil, errors.New("store offline")
		}
		return &allowlist.Allowlist{DefaultAction: "confirm"}, nil
	}
	authorizer := shelltrust.Authorizer{Policy: policy, Prompt: func(context.Context, shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
		approved = true
		return shelltrust.PromptDecision{Approve: true, Scope: authorization.ScopeOnce}, nil
	}}
	terminal := &MockSessionManager{}
	rc := NewRunCommand(terminal, authorizer.Authorize, policy, dir)
	result, err := rc.Execute(authorizationfixture.Context(), json.RawMessage(`{"command":"example check"}`))
	if !approved || err != nil || result.Failure == nil || result.Failure.Code != FailurePrerequisiteUnavailable || terminal.runEphemeralCalls != 0 {
		t.Fatalf("approved=%t result=%+v err=%v calls=%d", approved, result, err, terminal.runEphemeralCalls)
	}
}
