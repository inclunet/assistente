package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/allowlist"
	"assistente/internal/authorization"
	"assistente/internal/logging"
	"assistente/internal/shelltrust"
	"assistente/internal/testutil/authorizationfixture"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/trustscope"
	"gorm.io/gorm"
)

func TestSensitiveProgramInPolicyLogsAndRefusals(t *testing.T) {
	const program = "sintetico-programa-417"
	const masked = "[redacted]"
	for _, route := range []string{"policy", "user"} {
		t.Run(route, func(t *testing.T) {
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			asked := false
			rc := NewRunCommand(nil, func(_ context.Context, command, _ string) (bool, error) {
				asked = true
				if command != program {
					t.Fatal("autorização recebeu o comando errado")
				}
				return false, nil
			}, func(context.Context) (*allowlist.Allowlist, error) {
				policy := &allowlist.Allowlist{DefaultAction: "confirm"}
				if route == "policy" {
					policy.AlwaysDeny = []string{program}
				}
				return policy, nil
			}, t.TempDir())
			args, err := json.Marshal(map[string]string{"command": program})
			if err != nil {
				t.Fatal(err)
			}
			ctx := invocationctx.WithShellCommandMetadata(t.Context(), masked)
			result, err := rc.Execute(ctx, args)
			if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != FailureCommandRefused || asked != (route == "user") {
				t.Fatal("semântica da recusa foi alterada")
			}
			if strings.Contains(logs.String(), program) || strings.Contains(result.Content, program) || !strings.Contains(logs.String(), masked) || !strings.Contains(result.Content, masked) {
				t.Fatal("log ou motivo da recusa não preservou a redação")
			}
		})
	}
}

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
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	result, err := rc.Execute(authorizationfixture.Context(), json.RawMessage(`{"command":"git push"}`))
	if result.Failure == nil || result.Failure.Code != FailurePrerequisiteUnavailable || !strings.Contains(logs.String(), "reason=directory_changed") {
		t.Fatalf("troca de diretório: %+v log=%s", result, logs.String())
	}
	if err != nil || !result.IsError || m.runEphemeralCalls.Load() != 0 {
		t.Fatalf("executou após troca: %+v %v calls=%d", result, err, m.runEphemeralCalls.Load())
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
	if !approved || err != nil || result.Failure == nil || result.Failure.Code != FailurePrerequisiteUnavailable || terminal.runEphemeralCalls.Load() != 0 {
		t.Fatalf("approved=%t result=%+v err=%v calls=%d", approved, result, err, terminal.runEphemeralCalls.Load())
	}
}

func TestPrerequisiteWarningIdentifiesCallAndCause(t *testing.T) {
	for _, tc := range []struct{ path, reason string }{
		{"directory_changed", "directory_changed"}, {"store_unavailable", "store_unavailable"},
		{"policy_unavailable", "policy_unavailable"}, {"initial_policy", "policy_unavailable"}, {"initial_directory", "directory_unresolved"}, {"explicit_directory", "directory_unresolved"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			const secret = "sensitive-test-value"
			dir := filepath.Join(t.TempDir(), secret)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			auth := &shelltrust.Authorizer{}
			command := "git push"
			var requestedDir string
			var policy GetAllowlistFunc
			switch tc.path {
			case "initial_policy":
				policy = func(context.Context) (*allowlist.Allowlist, error) { return nil, errors.New(secret) }
			case "explicit_directory":
				requestedDir = "."
				fallthrough
			case "initial_directory":
				dir += "\x00"
			case "directory_changed":
				auth.Prompt = func(context.Context, shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
					return shelltrust.PromptDecision{Approve: true, Scope: authorization.ScopeOnce}, os.Rename(dir, dir+"-moved")
				}
			case "store_unavailable":
				repo, db := authorizationfixture.Open(t, t.TempDir())
				if err := db.Callback().Query().Before("gorm:query").Register("test:store_unavailable", func(tx *gorm.DB) { _ = tx.AddError(errors.New(secret)) }); err != nil {
					t.Fatal(err)
				}
				auth.Manager = shelltrust.NewManager(trustscope.Options{Repository: repo})
			case "policy_unavailable":
				command = "example --token=" + secret
				auth.Policy = func(context.Context) (*allowlist.Allowlist, error) { return nil, errors.New(secret) }
			}
			reg := tools.NewRegistry()
			reg.MustRegister(NewRunCommand(nil, auth.Authorize, policy, dir))
			args, err := json.Marshal(runCommandArgs{Command: command, WorkingDirectory: requestedDir})
			if err != nil {
				t.Fatal(err)
			}
			const callID = "actual-call-id"
			result := tools.NewExecutor(reg, tools.DefaultExecutorConfig()).ExecuteOne(authorizationfixture.Context(), tools.ToolCall{ID: callID, Function: tools.FunctionCall{Name: "run_command", Arguments: string(args)}})
			record := output.String()
			if result.ErrorCode != FailurePrerequisiteUnavailable || !strings.Contains(record, "level=WARN") || !strings.Contains(record, "tool_call_id="+callID) || !strings.Contains(record, "reason="+tc.reason) || strings.Contains(record, secret) || strings.Contains(result.Result.Content, secret) {
				t.Fatalf("resultado=%+v log=%q", result, record)
			}
		})
	}
}

func TestInvalidRequestedDirectoryReturnsArgumentError(t *testing.T) {
	for _, name := range []string{"nul", "broken_symlink", "permission_denied"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			requested := "invalid\x00"
			if name == "broken_symlink" {
				requested = "broken-link"
				if err := os.Symlink(filepath.Join(dir, "absent"), filepath.Join(dir, requested)); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("symlink indisponível: %v", err)
					}
					t.Fatal(err)
				}
			}

			if name == "permission_denied" {
				if runtime.GOOS == "windows" {
					t.Skip("permissão de travessia POSIX")
				}
				blocked := filepath.Join(dir, "blocked")
				if err := os.Mkdir(blocked, 0700); err != nil {
					t.Fatal(err)
				}
				requested = filepath.Join("blocked", "child")
				child := filepath.Join(dir, requested)
				if err := os.Mkdir(child, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(blocked, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := os.Chmod(blocked, 0700); err != nil {
						t.Error(err)
					}
				})
				if _, err := os.Stat(child); err == nil {
					t.Skip("o processo consegue atravessar o diretório sem permissão")
				} else if !errors.Is(err, os.ErrPermission) {
					t.Fatal(err)
				}
			}

			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			terminal := &MockSessionManager{}
			rc := NewRunCommand(terminal, nil, nil, dir)
			args, err := json.Marshal(map[string]string{"command": "example", "working_directory": requested})
			if err != nil {
				t.Fatal(err)
			}
			result, err := rc.Execute(authorizationfixture.Context(), args)
			cause, invalid := strings.CutPrefix(result.Content, "working_directory inválido: ")
			if err != nil || !result.IsError || !invalid || cause == "" || strings.Contains(result.Content, dir) || strings.Contains(result.Content, requested) || result.Failure != nil || terminal.runEphemeralCalls.Load() != 0 {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, terminal.runEphemeralCalls.Load())
			}
			if strings.Contains(output.String(), "directory_unresolved") || strings.Contains(output.String(), dir) {
				t.Fatalf("argumento inválido diagnosticado como pré-requisito: %s", &output)
			}
		})
	}
}

func TestPrerequisiteWarningAfterConfirmationCancellation(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx, cancel := context.WithCancel(authorizationfixture.Context())
	defer cancel()
	ctx = logging.WithAttrScope(ctx, []string{"tool_call_id"}, slog.String("tool_call_id", "cancelled-call"))
	auth := shelltrust.Authorizer{Policy: func(context.Context) (*allowlist.Allowlist, error) {
		cancel()
		return nil, errors.New("private-policy-detail")
	}}
	result := confirmShellCommand(ctx, auth.Authorize, "example check", t.TempDir(), "example")
	if result == nil || result.Failure == nil || result.Failure.Code != FailureConfirmationUnavailable {
		t.Fatalf("resultado após cancelamento: %+v", result)
	}
	logs := output.String()
	if strings.Count(logs, "reason=policy_unavailable") != 1 || !strings.Contains(logs, "tool_call_id=cancelled-call") || strings.Contains(logs, "private-policy-detail") {
		t.Fatalf("diagnóstico: %s", logs)
	}
}
