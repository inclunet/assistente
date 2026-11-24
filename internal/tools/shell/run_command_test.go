package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/commandpolicy"
	"assistente/internal/shelltrust"
	"assistente/internal/terminal"
	"assistente/internal/testutil/authorizationfixture"
	"assistente/internal/tools"
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

func TestPolicyLoadFailureDoesNotLogEditableValues(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	rc := NewRunCommand(nil, nil, func(context.Context) (*allowlist.Allowlist, error) {
		return nil, fmt.Errorf("argumento inválido: secret-token")
	}, ".")
	if result, err := rc.evaluateCommand(context.Background(), "git status"); err == nil || result.Decision != allowlist.DecisionDeny {
		t.Fatalf("falha não bloqueou: %+v", result)
	}
	got, err := rc.Execute(t.Context(), json.RawMessage(`{"command":"git status"}`))
	if err != nil || got.Failure == nil || got.Failure.Code != FailurePrerequisiteUnavailable {
		t.Fatalf("pré-requisito não classificado: %+v err=%v", got, err)
	}
	if output.Len() == 0 || strings.Contains(output.String(), "secret-token") {
		t.Fatalf("log ausente ou com segredo: %s", output.String())
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

func TestFalhaDeLeituraNaoViraConfirmacaoOuExecucao(t *testing.T) {
	rc := NewRunCommand(nil, func(context.Context, string, string) (bool, error) {
		t.Fatal("consulta de política falhou, não deve pedir aprovação")
		return true, nil
	}, func(context.Context) (*allowlist.Allowlist, error) { return nil, fmt.Errorf("banco indisponível") }, ".")
	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"git status"}`))
	if err != nil || !result.IsError || result.Failure == nil || result.Failure.Code != FailurePrerequisiteUnavailable || result.Failure.Kind != tools.ErrorKindUnavailable {
		t.Fatalf("não bloqueou: %+v, %v", result, err)
	}
}

func TestRunCommand_DeniedByAllowlist(t *testing.T) {
	al := &allowlist.Allowlist{
		AlwaysDeny:    []string{"rm *"},
		AutoApprove:   []string{},
		DefaultAction: "confirm",
	}

	rc := NewRunCommand(nil, nil, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"rm -rf /"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError || result.Failure == nil || result.Failure.Code != FailurePolicyVetoed || result.Failure.Kind != tools.ErrorKindAuthorization || result.Failure.Retryable {
		t.Fatalf("classificação da negativa: %+v", result.Failure)
	}
}

func TestRunCommand_DeniesCompoundCommandWithDeniedAtom(t *testing.T) {
	al := &allowlist.Allowlist{
		AutoApprove:   []string{"git status"},
		AlwaysDeny:    []string{"rm -rf *"},
		DefaultAction: "confirm",
	}

	rc := NewRunCommand(nil, nil, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, ".")

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

	rc := NewRunCommand(nil, nil, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, ".")

	result, err := rc.evaluateCommand(context.Background(), "git status && git diff")
	if err != nil {
		t.Fatal(err)
	}
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

	rc := NewRunCommand(nil, confirmFn, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, ".")

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

func TestLogPolicyDecisionPreservesMessageAndDoesNotLeakCommand(t *testing.T) {
	const secret = "hunter2-supersecret"
	rawCommand := "TOKEN=" + secret + " psql -W " + secret
	baseResult := commandpolicy.Evaluate(rawCommand, &allowlist.Allowlist{
		AlwaysDeny:    []string{"psql"},
		DefaultAction: "confirm",
	})
	if baseResult.Decision != allowlist.DecisionDeny {
		t.Fatalf("pré-condição: decision = %s, want deny", baseResult.Decision.String())
	}
	commandSummary := redactCommandForLog(rawCommand, baseResult)

	for _, decision := range []allowlist.Decision{allowlist.DecisionApprove, allowlist.DecisionDeny} {
		t.Run(decision.String(), func(t *testing.T) {
			result := baseResult
			result.Decision = decision
			public := publicPolicyProjection(result, false)
			reasons := summarizePolicyReasons(public)
			wantMessage := fmt.Sprintf("Comando: %s, decisão: %s", commandSummary, decision.String())
			if decision != allowlist.DecisionApprove {
				wantMessage += fmt.Sprintf(", motivos: %s", reasons)
			}

			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, nil)).
				With(slog.String("component", "tools.shell.run-command"))
			var logCtx context.Context
			if decision != allowlist.DecisionApprove {
				logCtx = context.Background()
			}
			logPolicyDecision(logCtx, logger, commandSummary, public)

			if strings.Contains(output.String(), secret) {
				t.Fatalf("log estruturado vazou segredo: %s", output.String())
			}
			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatalf("decodificar log estruturado: %v", err)
			}
			for key, want := range map[string]string{
				"level":           "INFO",
				"component":       "tools.shell.run-command",
				"msg":             wantMessage,
				"command_summary": commandSummary,
				"decision":        decision.String(),
			} {
				if got := record[key]; got != want {
					t.Errorf("%s = %v, want %q", key, got, want)
				}
			}
			gotReasons, hasReasons := record["reasons"]
			if decision == allowlist.DecisionApprove && hasReasons {
				t.Errorf("approve não deve registrar reasons: %v", gotReasons)
			}
			if decision != allowlist.DecisionApprove && gotReasons != reasons {
				t.Errorf("reasons = %v, want %q", gotReasons, reasons)
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

	rc := NewRunCommand(nil, nil, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, ".")

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

	rc := NewRunCommand(nil, nil, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, ".")

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

	rc := NewRunCommand(nil, confirmFn, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, ".")

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

	rc := NewRunCommand(nil, nil, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, ".")

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

	rc := NewRunCommand(nil, confirmFn, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, ".")

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

	rc := NewRunCommand(nil, confirmFn, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, ".")

	result, err := rc.Execute(context.Background(), json.RawMessage(`{"command":"echo test"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError || result.Failure == nil || result.Failure.Code != FailureCommandRefused || result.Failure.Kind != tools.ErrorKindAuthorization || result.Failure.Retryable {
		t.Fatalf("classificação da negativa: %+v", result.Failure)
	}
}

func TestExecutorTimeoutDuringShellConfirmationDoesNotRetry(t *testing.T) {
	dir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		var prompts atomic.Int32
		release := make(chan struct{})
		defer close(release)
		rc := NewRunCommand(nil, func(ctx context.Context, _, _ string) (bool, error) {
			prompts.Add(1)
			<-release
			return false, ctx.Err()
		}, nil, dir)
		reg := tools.NewRegistry()
		reg.MustRegister(rc)
		cfg := tools.DefaultExecutorConfig()
		cfg.ToolTimeout = time.Second
		result := tools.NewExecutor(reg, cfg).ExecuteOne(t.Context(), tools.ToolCall{ID: "ask", Function: tools.FunctionCall{Name: "run_command", Arguments: `{"command":"example check"}`}})
		if prompts.Load() != 1 || result.ErrorCode != FailureConfirmationUnavailable || result.ErrorKind != tools.ErrorKindAuthorization || result.Retryable || !result.RetryabilityKnown {
			t.Fatalf("diálogos=%d resultado=%+v", prompts.Load(), result)
		}
	})
}

func TestExecutorClassifiesShellStartBeforeDispatch(t *testing.T) {
	dir := t.TempDir()
	cfg := terminal.DefaultManagerConfig()
	cfg.DefaultShell = filepath.Join(dir, "shell-inexistente")
	for _, persistent := range []bool{false, true} {
		for _, byPolicy := range []bool{false, true} {
			t.Run(fmt.Sprintf("persistente=%t/politica=%t", persistent, byPolicy), func(t *testing.T) {
				var prompts atomic.Int32
				al := &allowlist.Allowlist{DefaultAction: "confirm"}
				if byPolicy {
					al.AutoApprove = []string{"echo"}
				}
				manager := terminal.NewManager(cfg, nil)
				t.Cleanup(manager.CloseAll)
				reg := tools.NewRegistry()
				reg.MustRegister(NewRunCommand(manager, func(context.Context, string, string) (bool, error) {
					prompts.Add(1)
					return true, nil
				}, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, dir))
				call := tools.ToolCall{ID: "startup", Function: tools.FunctionCall{Name: "run_command", Arguments: fmt.Sprintf(`{"command":"echo ok","persistent":%t}`, persistent)}}
				got := tools.NewExecutor(reg, tools.DefaultExecutorConfig()).ExecuteOne(t.Context(), call)
				wantPrompts := int32(1)
				if byPolicy {
					wantPrompts = 0
				}
				if !got.Result.IsError || got.ErrorCode != FailureSessionStart || got.ErrorKind != tools.ErrorKindUnavailable || !got.RetryabilityKnown || got.Retryable || prompts.Load() != wantPrompts || strings.Contains(got.Result.Content, cfg.DefaultShell) || got.Result.Content != "Executável do terminal não encontrado." {
					t.Fatalf("resultado=%+v diálogos=%d", got, prompts.Load())
				}
			})
		}
	}
}

func TestRefusalReceivedWhenConfirmationContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	rc := NewRunCommand(nil, func(context.Context, string, string) (bool, error) { cancel(); return false, nil }, nil, t.TempDir())
	got, err := rc.Execute(ctx, json.RawMessage(`{"command":"example check"}`))
	if err != nil || got.Failure == nil || got.Failure.Code != FailureCommandRefused {
		t.Fatalf("recusa=%+v err=%v", got, err)
	}
}

func TestPolicyRefusalBeforeExecutorDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A escrita só termina quando o teste lê o log da política. O relógio
		// virtual avança enquanto a entrega do resultado está bloqueada.
		reader, writer := io.Pipe()
		defer func() { _ = reader.Close() }()
		defer func() { _ = writer.Close() }()
		previous := slog.Default()
		defer log.SetOutput(log.Writer())
		defer log.SetFlags(log.Flags())
		slog.SetDefault(slog.New(slog.NewTextHandler(writer, nil)))
		defer slog.SetDefault(previous)
		reg := tools.NewRegistry()
		reg.MustRegister(NewRunCommand(nil, nil, func(context.Context) (*allowlist.Allowlist, error) {
			return &allowlist.Allowlist{AlwaysDeny: []string{"git push"}}, nil
		}, "."))
		cfg := tools.DefaultExecutorConfig()
		cfg.ToolTimeout = time.Second
		got := tools.NewExecutor(reg, cfg).ExecuteOne(t.Context(), tools.ToolCall{ID: "policy", Function: tools.FunctionCall{Name: "run_command", Arguments: `{"command":"git push"}`}})
		if got.Error != nil || got.ErrorCode != FailurePolicyVetoed || got.ErrorKind != tools.ErrorKindAuthorization || got.Retryable || !got.RetryabilityKnown || got.DurationMs != cfg.ToolTimeout.Milliseconds() {
			t.Fatalf("negativa após prazo: %+v", got)
		}
		var log [4096]byte
		n, err := reader.Read(log[:])
		if err != nil || !strings.Contains(string(log[:n]), "decision=deny") {
			t.Fatalf("log da decisão: %q err=%v", log[:n], err)
		}
	})
}

func TestPrerequisiteFailureBeforeInterruption(t *testing.T) {
	for _, parentCancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "prazo da ferramenta", true: "cancelamento do turno"}[parentCancel], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				reader, writer := io.Pipe()
				defer func() { _ = reader.Close() }()
				defer func() { _ = writer.Close() }()
				previous := slog.Default()
				defer log.SetOutput(log.Writer())
				defer log.SetFlags(log.Flags())
				slog.SetDefault(slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: slog.LevelWarn})))
				defer slog.SetDefault(previous)

				reg := tools.NewRegistry()
				reg.MustRegister(NewRunCommand(nil, func(context.Context, string, string) (bool, error) {
					return false, shelltrust.ErrPrerequisiteUnavailable
				}, nil, "."))
				cfg := tools.DefaultExecutorConfig()
				cfg.ToolTimeout = time.Second
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				delivered := make(chan tools.ToolExecutionResult, 1)
				go func() {
					delivered <- tools.NewExecutor(reg, cfg).ExecuteOne(ctx, tools.ToolCall{ID: "prerequisite", Function: tools.FunctionCall{Name: "run_command", Arguments: `{"command":"example check"}`}})
				}()
				synctest.Wait() // a releitura falhou, e o aviso bloqueia a entrega do callback
				if parentCancel {
					cancel()
				}
				got := <-delivered
				if parentCancel {
					if !errors.Is(got.Error, context.Canceled) || got.ErrorKind != tools.ErrorKindCancelled || got.ErrorCode != "" || got.Retryable {
						t.Fatalf("cancelamento=%+v", got)
					}
				} else if got.Error != nil || got.ErrorKind != tools.ErrorKindUnavailable || got.ErrorCode != FailurePrerequisiteUnavailable || got.Retryable || !got.RetryabilityKnown || got.DurationMs != cfg.ToolTimeout.Milliseconds() {
					t.Fatalf("pré-requisito=%+v", got)
				}
				var record [4096]byte
				n, err := reader.Read(record[:])
				if err != nil || !strings.Contains(string(record[:n]), "Pré-requisito") {
					t.Fatalf("log=%q err=%v", record[:n], err)
				}
			})
		})
	}
}

func TestShellAuthorizerWithoutPromptCannotProduceRefusal(t *testing.T) {
	rc := NewRunCommand(nil, (&shelltrust.Authorizer{}).Authorize, nil, t.TempDir())
	got, err := rc.Execute(authorizationfixture.Context(), json.RawMessage(`{"command":"example check"}`))
	if err != nil || got.Failure == nil || got.Failure.Code != FailureConfirmationUnavailable {
		t.Fatalf("confirmação sem interlocutor: %+v erro=%v", got, err)
	}
}

func TestPrerequisiteFailureAfterConfirmationDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		rc := NewRunCommand(nil, func(ctx context.Context, _, _ string) (bool, error) {
			<-ctx.Done()
			return false, errors.Join(shelltrust.ErrPrerequisiteUnavailable, ctx.Err())
		}, nil, ".")
		got, err := rc.Execute(ctx, json.RawMessage(`{"command":"example check"}`))
		if err != nil || got.Failure == nil || got.Failure.Code != FailureConfirmationUnavailable || got.Failure.Retryable {
			t.Fatalf("falha observada depois do prazo=%+v err=%v", got, err)
		}
	})
}

type blockedDecisionLog struct {
	component string
	entered   chan struct{}
	release   chan struct{}
}

func (w blockedDecisionLog) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("component="+w.component+" ")) || bytes.HasSuffix(bytes.TrimSpace(p), []byte("component="+w.component)) {
		select {
		case <-w.entered:
		default:
			close(w.entered)
		}
		<-w.release
	}
	return len(p), nil
}

func TestExpiredShellAttemptCannotPromptOrDispatchAfterPolicyReturns(t *testing.T) {
	for _, auto := range []bool{true, false} {
		t.Run(fmt.Sprintf("pré-aprovado=%t", auto), func(t *testing.T) {
			dir := t.TempDir()
			synctest.Test(t, func(t *testing.T) {
				release := make(chan struct{})
				mgr := new(MockSessionManager)
				var prompts atomic.Int32
				rc := NewRunCommand(mgr, func(context.Context, string, string) (bool, error) {
					prompts.Add(1)
					return true, nil
				}, func(context.Context) (*allowlist.Allowlist, error) {
					<-release
					al := &allowlist.Allowlist{DefaultAction: "confirm"}
					if auto {
						al.AutoApprove = []string{"echo"}
					}
					return al, nil
				}, dir)
				reg := tools.NewRegistry()
				reg.MustRegister(rc)
				cfg := tools.DefaultExecutorConfig()
				cfg.ToolTimeout = time.Second
				got := tools.NewExecutor(reg, cfg).ExecuteOne(t.Context(), tools.ToolCall{ID: "expiry", Function: tools.FunctionCall{Name: rc.Name(), Arguments: `{"command":"echo ok"}`}})
				close(release)
				synctest.Wait()
				if !errors.Is(got.Error, context.DeadlineExceeded) || got.ErrorKind != tools.ErrorKindTimeout || !got.Retryable || prompts.Load() != 0 || mgr.runEphemeralCalls.Load() != 0 {
					t.Fatalf("resultado=%+v diálogos=%d despachos=%d", got, prompts.Load(), mgr.runEphemeralCalls.Load())
				}
			})
		})
	}
}

func TestShellAuthorizerPublishesBeforeDecisionLog(t *testing.T) {
	for _, tc := range []struct {
		name, component, code string
	}{
		{"aprovação", "authorization", ""},
		{"recusa do usuário", "authorization", FailureCommandRefused},
		{"recusa da política na releitura", "authorization", FailurePolicyVetoed},
		{"falha da política na releitura", "authorization", FailurePrerequisiteUnavailable},
		{"falha da primeira leitura", "tools.shell.run-command", FailurePrerequisiteUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			synctest.Test(t, func(t *testing.T) {
				blocked := blockedDecisionLog{tc.component, make(chan struct{}), make(chan struct{})}
				defer close(blocked.release)
				previous := slog.Default()
				defer log.SetOutput(log.Writer())
				defer log.SetFlags(log.Flags())
				slog.SetDefault(slog.New(slog.NewTextHandler(blocked, nil)))
				defer slog.SetDefault(previous)

				asked := false
				policy := func(context.Context) (*allowlist.Allowlist, error) {
					switch {
					case tc.name == "falha da primeira leitura", asked && tc.code == FailurePrerequisiteUnavailable:
						return nil, errors.New("loja indisponível")
					case asked && tc.name == "recusa da política na releitura":
						return &allowlist.Allowlist{AlwaysDeny: []string{"git push"}}, nil
					default:
						return &allowlist.Allowlist{DefaultAction: "confirm"}, nil
					}
				}
				auth := shelltrust.Authorizer{Policy: policy, Prompt: func(context.Context, shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
					asked = true
					return shelltrust.PromptDecision{Approve: tc.name != "recusa do usuário"}, nil
				}}
				reg := tools.NewRegistry()
				reg.MustRegister(NewRunCommand(nil, auth.Authorize, policy, dir))
				cfg := tools.DefaultExecutorConfig()
				cfg.ToolTimeout = time.Second
				got := tools.NewExecutor(reg, cfg).ExecuteOne(authorizationfixture.Context(), tools.ToolCall{ID: "policy-before-log", Function: tools.FunctionCall{Name: "run_command", Arguments: `{"command":"git push"}`}})
				select {
				case <-blocked.entered:
				default:
					t.Fatal("o executor terminou antes de o log receber o resultado")
				}
				if tc.name == "aprovação" {
					if !asked || !errors.Is(got.Error, context.DeadlineExceeded) || got.ErrorKind != tools.ErrorKindTimeout || got.ErrorCode != "" || got.Retryable || !got.RetryabilityKnown || got.DurationMs != cfg.ToolTimeout.Milliseconds() {
						t.Fatalf("prazo após aprovação: %+v", got)
					}
					return
				}
				if got.ErrorCode != tc.code || got.Error != nil || got.Retryable || !got.RetryabilityKnown || got.DurationMs != cfg.ToolTimeout.Milliseconds() || asked != (tc.name != "falha da primeira leitura") {
					t.Fatalf("perguntou=%t, resultado=%+v", asked, got)
				}
			})
		})
	}
}

func TestConfirmacaoAprovadaAntesOuDepoisDoCancelamento(t *testing.T) {
	t.Run("aprovacao revalidada antes do cancelamento", func(t *testing.T) {
		blocked := blockedDecisionLog{"authorization", make(chan struct{}), make(chan struct{})}
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(blocked, nil)))
		defer slog.SetDefault(previous)
		ctx, cancel := context.WithCancel(authorizationfixture.Context())
		defer cancel()
		auth := shelltrust.Authorizer{Policy: func(context.Context) (*allowlist.Allowlist, error) {
			return &allowlist.Allowlist{DefaultAction: "confirm"}, nil
		}, Prompt: func(context.Context, shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
			return shelltrust.PromptDecision{Approve: true}, nil
		}}
		dir := t.TempDir()
		done := make(chan *tools.ToolResult, 1)
		go func() { done <- confirmShellCommand(ctx, auth.Authorize, "example check", dir, "example") }()
		<-blocked.entered
		cancel()
		close(blocked.release)
		got := <-done
		if got == nil || !got.IsError || got.Failure != nil {
			t.Fatalf("interrupção após aprovação: %+v", got)
		}
	})
	t.Run("aprovacao customizada depois do cancelamento", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		got := confirmShellCommand(ctx, func(context.Context, string, string) (bool, error) { cancel(); return true, nil }, "example check", ".", "example")
		if got == nil || got.Failure == nil || got.Failure.Code != FailureConfirmationUnavailable {
			t.Fatalf("aprovação tardia: %+v", got)
		}
	})
}

func TestDiretorioPadraoIndisponivelComPoliticaAprovada(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "diretorio-secreto-inexistente")
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	manager := terminal.NewManager(terminal.DefaultManagerConfig(), nil)
	rc := NewRunCommand(manager, nil, func(context.Context) (*allowlist.Allowlist, error) {
		return &allowlist.Allowlist{AutoApprove: []string{"echo marker"}, DefaultAction: "confirm"}, nil
	}, dir)
	got, err := rc.Execute(t.Context(), json.RawMessage(`{"command":"echo marker"}`))
	if err != nil || got.Failure == nil || got.Failure.Code != FailurePrerequisiteUnavailable || got.Failure.Kind != tools.ErrorKindUnavailable || got.Failure.Retryable {
		t.Fatalf("resultado=%+v erro=%v", got, err)
	}
	if strings.Contains(got.Content, dir) || strings.Contains(logs.String(), dir) || strings.Contains(logs.String(), "diretorio-secreto-inexistente") {
		t.Fatalf("diretório exposto: resultado=%q logs=%q", got.Content, logs.String())
	}
	if !strings.Contains(logs.String(), "reason=directory_unresolved") {
		t.Fatalf("diagnóstico ausente: %q", logs.String())
	}
	if len(manager.List()) != 0 {
		t.Fatal("sessão criada apesar de diretório indisponível")
	}
}

func TestClassificacaoDeRetryNoInicioDoTerminal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		retry bool
	}{
		{"arquivo ausente", fmt.Errorf("início: %w", os.ErrNotExist), false},
		{"comando ausente", fmt.Errorf("início: %w", exec.ErrNotFound), false},
		{"sem permissão", fmt.Errorf("início: %w", os.ErrPermission), false},
		{"lotação temporária", terminal.ErrSessionCapacity, true},
		{"recurso ocupado", fmt.Errorf("PTY: %w", syscall.EBUSY), true},
		{"recurso temporário", fmt.Errorf("PTY: %w", syscall.EAGAIN), true},
		{"causa desconhecida", errors.New("capacidade indisponível"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sessionStartFailure(allowlist.DecisionApprove, tc.err)
			if got.Failure == nil || got.Failure.Code != FailureSessionStart || got.Failure.Retryable != tc.retry {
				t.Fatalf("resultado=%+v", got)
			}
			confirmed := sessionStartFailure(allowlist.DecisionConfirm, tc.err)
			if confirmed.Failure == nil || confirmed.Failure.Retryable {
				t.Fatalf("confirmação=%+v", confirmed)
			}
		})
	}
}

func TestCausaDeInicioPrevaleceSobreContextoEncerrado(t *testing.T) {
	for _, tc := range []struct {
		err  error
		kind tools.ErrorKind
	}{
		{os.ErrNotExist, tools.ErrorKindUnavailable}, {context.Canceled, tools.ErrorKindCancelled}, {context.DeadlineExceeded, tools.ErrorKindTimeout},
	} {
		got := sessionStartFailure(allowlist.DecisionApprove, tc.err)
		if got.Failure == nil || got.Failure.Kind != tc.kind || got.Failure.Retryable {
			t.Fatalf("causa=%v resultado=%+v", tc.err, got)
		}
	}
}

func TestFalhasAposDespachoNaoSaoRepetidas(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		output string
	}{
		{"timeout sem saída", context.DeadlineExceeded, ""},
		{"timeout com saída", context.DeadlineExceeded, "parcial"},
		{"falha de leitura", terminal.ErrSessionRead, "parcial"},
		{"sessão fechada", terminal.ErrSessionClosed, "parcial"},
		{"falha desconhecida", errors.New("falha após despacho"), "parcial"},
		{"sucesso", nil, "ok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := &MockSessionManager{fakeRunErr: tc.err, fakeEntry: &terminal.HistoryEntry{ID: "sent", Output: tc.output}}
			al := &allowlist.Allowlist{AutoApprove: []string{"echo"}, DefaultAction: "confirm"}
			reg := tools.NewRegistry()
			reg.MustRegister(NewRunCommand(mgr, nil, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, t.TempDir()))
			got := tools.NewExecutor(reg, tools.DefaultExecutorConfig()).ExecuteOne(t.Context(), tools.ToolCall{ID: "sent", Function: tools.FunctionCall{Name: "run_command", Arguments: `{"command":"echo ok"}`}})
			if mgr.runEphemeralCalls.Load() != 1 || got.Retryable || got.Result.Failure != nil && got.Result.Failure.NotDispatched {
				t.Fatalf("chamadas=%d resultado=%+v", mgr.runEphemeralCalls.Load(), got)
			}
		})
	}
}

type managerFalhaInicioCancelada struct {
	SessionManager
	cancel context.CancelFunc
}

func (m managerFalhaInicioCancelada) Acquire(context.Context, terminal.WorkingDirectory) (*terminal.Session, error) {
	m.cancel()
	return nil, os.ErrNotExist
}

func TestFalhaDeInicioPreservaCausaQuandoContextoCancela(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	mgr := managerFalhaInicioCancelada{cancel: cancel}
	rc := NewRunCommand(mgr, nil, func(context.Context) (*allowlist.Allowlist, error) {
		return &allowlist.Allowlist{AutoApprove: []string{"echo"}, DefaultAction: "confirm"}, nil
	}, t.TempDir())
	got, err := rc.Execute(ctx, json.RawMessage(`{"command":"echo ok","persistent":true}`))
	if err != nil || got.Failure == nil || got.Failure.Kind != tools.ErrorKindUnavailable || got.Failure.Retryable || got.Content != "Executável do terminal não encontrado." {
		t.Fatalf("resultado=%+v erro=%v", got, err)
	}
}

func TestTerminalExistenteOcupadoNaoAutorizaRetry(t *testing.T) {
	dir := t.TempDir()
	mgr := &MockSessionManager{liveSessions: map[string]bool{"terminal": true}, sessionCWD: map[string]string{"terminal": dir}, fakeRunErr: errors.New("sessão ocupada")}
	al := &allowlist.Allowlist{AutoApprove: []string{"echo"}, DefaultAction: "confirm"}
	reg := tools.NewRegistry()
	reg.MustRegister(NewRunCommand(mgr, nil, func(context.Context) (*allowlist.Allowlist, error) { return al, nil }, dir))
	got := tools.NewExecutor(reg, tools.DefaultExecutorConfig()).ExecuteOne(t.Context(), tools.ToolCall{ID: "busy", Function: tools.FunctionCall{Name: "run_command", Arguments: `{"command":"echo ok","terminal_id":"terminal"}`}})
	if mgr.runCommandCalls != 1 || !got.Result.IsError || got.Retryable {
		t.Fatalf("chamadas=%d resultado=%+v", mgr.runCommandCalls, got)
	}
}

func TestFalhaDePermissaoRealNaoExpoeCaminhoDoShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissão de execução POSIX")
	}
	dir := t.TempDir()
	executable := filepath.Join(dir, "shell-secreto")
	// Um diretório não pode ser executado, mesmo em filesystems que ignoram
	// bits de permissão. Confirme a causa antes de exercitar o gerenciador PTY.
	if err := os.Mkdir(executable, 0700); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command(executable).Run(); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("pré-condição: executar diretório deveria retornar EACCES, got %v", err)
	}
	cfg := terminal.DefaultManagerConfig()
	cfg.DefaultShell = executable
	mgr := terminal.NewManager(cfg, nil)
	rc := NewRunCommand(mgr, nil, func(context.Context) (*allowlist.Allowlist, error) {
		return &allowlist.Allowlist{AutoApprove: []string{"echo"}, DefaultAction: "confirm"}, nil
	}, dir)
	got, err := rc.Execute(t.Context(), json.RawMessage(`{"command":"echo ok"}`))
	if err != nil || got.Failure == nil || got.Failure.Code != FailureSessionStart || got.Failure.Retryable || got.Content != "Permissão negada ao iniciar a sessão de terminal." || strings.Contains(got.Content, executable) {
		t.Fatalf("resultado=%+v err=%v", got, err)
	}
}

func TestProjecaoPublicaNaoAlteraMotivosDaPolitica(t *testing.T) {
	original := commandpolicy.EvaluationResult{Decision: allowlist.DecisionDeny, Reasons: []string{"motivo privado", "outro motivo privado"}}
	public := publicPolicyProjection(original, true)
	if original.Reasons[0] != "motivo privado" || len(public.Reasons) != 2 || public.Reasons[0] != protectedPolicyReason || public.Reasons[1] != protectedPolicyReason || public.Decision != original.Decision || !strings.Contains(summarizePolicyReasons(public), "motivos=2") || strings.Contains(summarizePolicyReasons(public), "privado") {
		t.Fatalf("original=%+v pública=%+v", original, public)
	}
}

func TestPluginRevokedDuringCommandConfirmation(t *testing.T) {
	active := true
	rc := NewRunCommand(nil, func(context.Context, string, string) (bool, error) {
		active = false
		return true, nil
	}, nil, t.TempDir())
	ctx := tools.WithExecutionContext(context.Background(), tools.ExecutionContext{
		CheckCommandActivation: func(context.Context) error {
			if active {
				t.Fatal("expected the command confirmation before admission")
			}
			return fmt.Errorf("revoked")
		},
	})
	result, err := rc.Execute(ctx, json.RawMessage(`{"command":"echo test"}`))
	if err != nil || !result.IsError || !strings.Contains(result.Content, "ativação do plugin") {
		t.Fatal(result, err)
	}
}
