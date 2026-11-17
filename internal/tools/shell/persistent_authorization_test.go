package shell

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/allowlist"
	"assistente/internal/authorization"
	"assistente/internal/shelltrust"
	"assistente/internal/testutil/authorizationfixture"
)

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
